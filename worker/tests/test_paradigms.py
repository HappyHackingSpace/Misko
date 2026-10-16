"""MWM, the one tracked paradigm, on a synthetic recording of the full apparatus.

Tolerances (see tests/scenes.py; about 2 px/cm, 40 to 50 grey levels of
contrast, ripples and moving reflections): mean error <= 0.8 cm, maximum <= 3 cm.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import av
import numpy as np
import pytest

from misko_worker.paradigms import SUPPORTED, TankLayout, layout
from misko_worker.pipeline import analyze, check_supported, trajectory_document
from tests import scenes, synthetic


def observed_errors(analysis, truth):
    by_time = {t.t_us: t for t in truth}
    return [math.hypot(s.x_cm - by_time[s.t_us].x, s.y_cm - by_time[s.t_us].y) for s in analysis.samples if s.tracked and not s.inferred]


def test_only_mwm_is_supported() -> None:
    assert SUPPORTED == {("MWM", 1)}
    job = synthetic.job()
    check_supported(job)
    assert layout(job).key == "MWM"
    job["run"]["paradigmKey"] = "NOVEL_OBJECT"
    with pytest.raises(ValueError):
        layout(job)


@pytest.mark.parametrize(("water", "subject"), [(150, 105), (70, 120)], ids=["dark-animal-milky-water", "light-animal-dark-water"])
def test_mwm_tracks_in_a_tank_centred_frame_through_ripples_and_reflections(tmp_path: Path, water: int, subject: int) -> None:
    scene, job = scenes.mwm()
    source = tmp_path / "mwm.mp4"
    # The clip ends as the animal reaches the platform (5.5 s). An animal that
    # then rests on it is absorbed by the median background over part of the
    # frames, depending on the CPU's floating point; that is a known
    # limitation tracked separately.
    truth = synthetic.render(source, synthetic.regular(5.5), scenes.swim, scene=scene, floor_level=water, subject_level=subject,
                             radius_cm=3.5, noise=3.0)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    assert analysis.polarity == ("dark" if subject < water else "light")
    assert analysis.lost_ratio == 0 and analysis.mean_confidence >= 0.7, (analysis.lost_ratio, analysis.mean_confidence)
    err = observed_errors(analysis, truth)
    assert np.mean(err) <= 0.8 and max(err) <= 3.0, (np.mean(err), max(err))
    # The loop at 30 cm from the centre comes out as a circle around (0, 0).
    radii = [math.hypot(s.x_cm, s.y_cm) for s, t in zip(analysis.samples, truth) if t.t_us < 4_000_000]
    assert abs(np.mean(radii) - 30) <= 0.6
    document = json.loads(trajectory_document(job, analysis))
    assert document["coordinateFrame"] == "TANK_CM" and document["worker"]["settings"]["noise_factor"] > 0
    assert document["worker"]["paradigm"] == {"key": "MWM", "version": 1}
    # Nothing is inferred on MWM: every sample is seen or lost.
    assert document["samples"]["inferred"] == [False] * len(truth) and document["worker"]["qc"]["inferredRatio"] == 0


def test_the_overlay_draws_the_tank_zones(tmp_path: Path) -> None:
    scene, job = scenes.mwm(hand=False)
    source = tmp_path / "mwm.mp4"
    synthetic.render(source, synthetic.regular(2.0), scenes.swim, scene=scene, floor_level=150, subject_level=105, radius_cm=3.5)
    overlay = tmp_path / "overlay.mp4"
    analyze(source, job, overlay)
    with av.open(str(overlay)) as container:
        image = next(container.decode(container.streams.video[0])).to_ndarray(format="rgb24").astype(int)

    def colored(x: float, y: float, check) -> bool:
        px, py = synthetic.project(x, y, scenes.MWM_CM_TO_PX)
        patch = image[int(py) - 1 : int(py) + 2, int(px) - 1 : int(px) + 2].reshape(-1, 3)
        return any(check(*rgb) for rgb in patch)

    # The tank wall (first zone color, gold) in the south-west, away from the
    # target quadrant, and the platform (fourth, orchid) on the grey water.
    wall = 50 / math.sqrt(2)
    assert colored(-wall, -wall, lambda r, g, b: r - b > 80 and g - b > 60)
    assert colored(25, 20, lambda r, g, b: b - g > 60 and r - g > 40)


def test_mwm_layout_limits_detection_to_the_tank() -> None:
    _, job = scenes.mwm()
    apparatus = layout(job)
    assert isinstance(apparatus, TankLayout) and apparatus.water and apparatus.radius == 50
    assert apparatus.inside(0, 49.9) and not apparatus.inside(36, 36)
    calibration = job["calibration"]
    crop = tuple(calibration["crop"][k] for k in ("x", "y", "width", "height"))
    mask = apparatus.detection_mask(calibration["transform"], crop)
    # The tank centre is searched; the crop corner, where the experimenter's hand moves, is not.
    cx, cy = synthetic.project(0, 0, scenes.MWM_CM_TO_PX)
    assert mask[int(cy) - crop[1], int(cx) - crop[0]] and not mask[30 - crop[1], 70 - crop[0]]
    keys = [d.key for d in apparatus.drawings(calibration["transform"])]
    assert keys == ["tank", "wall_annulus", "target_quadrant", "platform"]
