"""Tracking, timing and failure behavior on synthetic recordings.

Tolerances documented per fixture (320 x 240 frame, about 4.4 px/cm):
- regular 25 fps walk: mean position error <= 0.25 cm, maximum <= 0.6 cm;
- sample times equal the decoded presentation timestamps exactly.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import av
import numpy as np
import pytest

from misko_worker.pipeline import NonRetryable, analyze, trajectory_document
from misko_worker.tracking import TrackerSettings
from misko_worker.video import CorruptVideo
from tests import synthetic


def errors(analysis, truth, offset_us=0):
    by_time = {t.t_us - offset_us: t for t in truth}
    return [math.hypot(s.x_cm - by_time[s.t_us].x, s.y_cm - by_time[s.t_us].y) for s in analysis.samples if s.tracked]


def test_positions_follow_the_subject_in_arena_centimeters(tmp_path: Path) -> None:
    source = tmp_path / "walk.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0))
    analysis = analyze(source, synthetic.job(), tmp_path / "overlay.mp4")
    assert [s.t_us for s in analysis.samples] == [t.t_us for t in truth]
    assert analysis.lost_ratio == 0 and analysis.mean_confidence > 0.9
    err = errors(analysis, truth)
    assert np.mean(err) <= 0.25 and max(err) <= 0.6, (np.mean(err), max(err))
    assert analysis.duration_us == 6_000_000
    # Pixel coordinates put a pixel's center at +0.5: no systematic offset remains.
    dx = np.mean([s.px - synthetic.project(t.x, t.y)[0] for s, t in zip(analysis.samples, truth)])
    dy = np.mean([s.py - synthetic.project(t.x, t.y)[1] for s, t in zip(analysis.samples, truth)])
    assert abs(dx) <= 0.15 and abs(dy) <= 0.15, (dx, dy)

    document = json.loads(trajectory_document(synthetic.job(), analysis))
    samples = document["samples"]
    assert document["schema"] == "misko.trajectory.v1" and document["runId"] == synthetic.job()["run"]["id"] and document["attempt"] == 1
    assert document["durationUs"] == 6_000_000 and len(samples["tUs"]) == len(truth) == len(samples["xCm"]) == len(samples["confidence"])
    assert all(samples["tracked"]) and document["worker"]["algorithm"].startswith("misko-open-field-bgsub")


def test_occluded_frames_are_lost_not_guessed(tmp_path: Path) -> None:
    def hidden(t: float):
        x, y, _ = synthetic.walk_with_pauses(t)
        return x, y, not (2.0 <= t < 2.6)

    source = tmp_path / "occluded.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0), hidden)
    analysis = analyze(source, synthetic.job(), tmp_path / "overlay.mp4")
    lost = {s.t_us for s in analysis.samples if not s.tracked}
    hidden_times = {t.t_us for t in truth if not t.visible}
    assert lost == hidden_times
    document = json.loads(trajectory_document(synthetic.job(), analysis))
    for tracked, x in zip(document["samples"]["tracked"], document["samples"]["xCm"]):
        assert tracked == (x is not None)
    assert max(errors(analysis, truth)) <= 0.6


def test_positions_outside_the_arena_are_lost(tmp_path: Path) -> None:
    # The subject climbs over the east wall to x = 53 cm, still inside the crop.
    def over_the_wall(t: float):
        if t < 1.0:
            return 40 + 13 * t, 25.0, True
        if t < 2.0:
            return 53.0, 25.0, True
        return 53 - 13 * min(t - 2.0, 1.0), 25.0, True

    source = tmp_path / "wall.mp4"
    truth = synthetic.render(source, synthetic.regular(3.5), over_the_wall)
    analysis = analyze(source, synthetic.job(clip_end_us=3_500_000), tmp_path / "overlay.mp4")
    by_time = {t.t_us: t for t in truth}
    outside = [s for s in analysis.samples if by_time[s.t_us].x > 50.6]
    inside = [s for s in analysis.samples if by_time[s.t_us].x < 49.4]
    assert outside and not any(s.tracked for s in outside) and all(s.px is not None for s in outside)
    assert inside and all(s.tracked for s in inside)
    document = json.loads(trajectory_document(synthetic.job(clip_end_us=3_500_000), analysis))
    assert document["samples"]["xCm"].count(None) == document["samples"]["tracked"].count(False) >= len(outside)


def test_low_contrast_lowers_confidence_but_keeps_the_timeline(tmp_path: Path) -> None:
    source = tmp_path / "faint.mp4"
    truth = synthetic.render(source, synthetic.regular(4.0), subject_level=135)
    analysis = analyze(source, synthetic.job(clip_end_us=4_000_000), tmp_path / "overlay.mp4")
    assert len(analysis.samples) == len(truth)
    # A contrast of about 50 grey levels gives about 0.6: below the paradigm's 0.7 QC rule.
    assert 0.3 < analysis.mean_confidence < 0.7
    assert np.mean(errors(analysis, truth)) <= 0.4


def test_variable_frame_rate_and_missing_frames_keep_their_timestamps(tmp_path: Path) -> None:
    times, t = [], 0
    while t < 5_000_000:
        times.append(t)
        t += 30_000 if len(times) % 2 else 50_000
    # A dropped stretch from 2.0 s to 2.8 s.
    times = [t for t in times if not 2_000_000 <= t < 2_800_000]
    source = tmp_path / "vfr.mp4"
    truth = synthetic.render(source, times)
    analysis = analyze(source, synthetic.job(clip_end_us=5_000_000), tmp_path / "overlay.mp4")
    assert [s.t_us for s in analysis.samples] == times
    gaps = np.diff([s.t_us for s in analysis.samples])
    assert set(gaps[:10]) == {30_000, 50_000} and gaps.max() >= 800_000
    assert np.mean(errors(analysis, truth)) <= 0.3


def test_clip_offset_is_removed_from_sample_times(tmp_path: Path) -> None:
    source = tmp_path / "long.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0))
    job = synthetic.job(clip_start_us=1_500_000, clip_end_us=4_500_000)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    expected = [t.t_us - 1_500_000 for t in truth if 1_500_000 <= t.t_us < 4_500_000]
    assert [s.t_us for s in analysis.samples] == expected and analysis.duration_us == 3_000_000
    assert max(errors(analysis, truth, offset_us=1_500_000)) <= 0.6

    # The clip is half-open: a frame exactly at its end belongs to the next clip.
    on_frame = synthetic.job(clip_start_us=1_500_000, clip_end_us=4_520_000)
    analysis = analyze(source, on_frame, tmp_path / "overlay3.mp4")
    assert analysis.samples[-1].t_us == 4_480_000 - 1_500_000 and analysis.duration_us == 3_020_000

    open_ended = synthetic.job(clip_start_us=1_500_000, clip_end_us=None)
    analysis = analyze(source, open_ended, tmp_path / "overlay2.mp4")
    assert analysis.samples[-1].t_us == truth[-1].t_us - 1_500_000
    assert abs(analysis.duration_us - (6_000_000 - 1_500_000)) <= 40_000


def test_a_subject_resting_for_most_of_the_clip_stays_tracked(tmp_path: Path) -> None:
    # Still at one spot for 80% of the clip: a median background would absorb it
    # and detect its ghost elsewhere.
    def mostly_still(t: float):
        return (10 + 30 * t, 10.0, True) if t < 1.0 else (40.0, 10.0, True)

    source = tmp_path / "resting.mp4"
    truth = synthetic.render(source, synthetic.regular(5.0), mostly_still)
    analysis = analyze(source, synthetic.job(clip_end_us=5_000_000), tmp_path / "overlay.mp4")
    assert analysis.lost_ratio == 0 and max(errors(analysis, truth)) <= 0.6

    # A lighter subject on a dark floor is recognized from the frames.
    light = tmp_path / "light.mp4"
    truth = synthetic.render(light, synthetic.regular(4.0), floor_level=60, subject_level=210)
    analysis = analyze(light, synthetic.job(clip_end_us=4_000_000), tmp_path / "overlay2.mp4")
    assert analysis.lost_ratio == 0 and max(errors(analysis, truth)) <= 0.6
    # Forcing the wrong polarity tracks the spots the subject rested on instead:
    # the positions are wrong while every sample looks tracked, which is why
    # "auto" is the default.
    wrong = analyze(light, synthetic.job(clip_end_us=4_000_000), tmp_path / "overlay3.mp4", TrackerSettings(subject="dark"))
    assert wrong.lost_ratio < 0.5 and max(errors(wrong, truth)) > 5


def test_the_overlay_keeps_the_recording_timeline_for_seeking(tmp_path: Path) -> None:
    source = tmp_path / "walk.mp4"
    synthetic.render(source, synthetic.regular(6.0))
    job = synthetic.job(clip_start_us=1_000_000, clip_end_us=6_000_000)
    overlay = tmp_path / "overlay.mp4"
    analysis = analyze(source, job, overlay)
    by_time = {s.t_us: s for s in analysis.samples}

    with av.open(str(overlay)) as container:
        stream = container.streams.video[0]
        decoded = [int(round(f.pts * f.time_base * 1_000_000)) for f in container.decode(stream)]
    assert decoded == [s.t_us for s in analysis.samples]

    # Seeking to an event start, as the side panel does, lands on the frame of
    # that recording time with the marker on the tracked position.
    for start in (1_000_000, 2_520_000, 4_000_000):
        with av.open(str(overlay)) as container:
            stream = container.streams.video[0]
            container.seek(int(start / 1_000_000 / stream.time_base), stream=stream, backward=True)
            frame = next(f for f in container.decode(stream) if f.pts * f.time_base * 1_000_000 >= start - 1)
            t = int(round(frame.pts * frame.time_base * 1_000_000))
            image = frame.to_ndarray(format="rgb24").astype(int)
        sample = by_time[t]
        assert t == start and sample.tracked
        r, g, b = image[int(sample.py), int(sample.px)]
        assert g > r + 60 and g > b + 60, (t, r, g, b)


def test_unusable_inputs_fail_without_retry(tmp_path: Path) -> None:
    source = tmp_path / "walk.mp4"
    synthetic.render(source, synthetic.regular(1.0))
    truncated = tmp_path / "truncated.mp4"
    truncated.write_bytes(source.read_bytes()[: source.stat().st_size // 3])
    with pytest.raises(CorruptVideo):
        analyze(truncated, synthetic.job(), tmp_path / "o1.mp4")
    garbage = tmp_path / "garbage.mp4"
    garbage.write_bytes(b"not a video" * 200)
    with pytest.raises(CorruptVideo):
        analyze(garbage, synthetic.job(), tmp_path / "o2.mp4")

    wrong_size = synthetic.job()
    wrong_size["calibration"] = synthetic.calibration(frame_width=640, frame_height=480)
    with pytest.raises(NonRetryable, match="FRAME_SIZE_MISMATCH"):
        analyze(source, wrong_size, tmp_path / "o3.mp4")
    uncalibrated = synthetic.job()
    uncalibrated["calibration"] = None
    with pytest.raises(NonRetryable, match="MISSING_CALIBRATION"):
        analyze(source, uncalibrated, tmp_path / "o4.mp4")
    beyond = synthetic.job(clip_start_us=5_000_000, clip_end_us=6_000_000)
    with pytest.raises(NonRetryable, match="NO_FRAMES_IN_CLIP"):
        analyze(source, beyond, tmp_path / "o5.mp4")
