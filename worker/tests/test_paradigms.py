"""Every tracked paradigm on synthetic recordings of its apparatus.

Tolerances per fixture (see tests/scenes.py):
- EPM and Y maze (about 3.2 px/cm): mean error <= 0.2 cm, maximum <= 0.5 cm;
- MWM (about 2 px/cm, 40 to 50 grey levels of contrast, ripples and moving
  reflections): mean error <= 0.8 cm, maximum <= 3 cm;
- LIGHT_DARK (about 6.5 px/cm): visible samples mean <= 0.3 cm; at the doorway
  only the lit part of the animal is seen, which moves its centroid up to 2.5 cm
  toward the light side.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import av
import numpy as np
import pytest

from misko_worker.geometry import Shape
from misko_worker.paradigms import SUPPORTED, LightDarkLayout, TankLayout, ZonedLayout, calibrated_zones, layout
from misko_worker.pipeline import INFERRED_CONFIDENCE, Sample, analyze, check_supported, infer_hidden, trajectory_document
from tests import scenes, synthetic


def observed_errors(analysis, truth):
    by_time = {t.t_us: t for t in truth}
    return [math.hypot(s.x_cm - by_time[s.t_us].x, s.y_cm - by_time[s.t_us].y) for s in analysis.samples if s.tracked and not s.inferred]


def test_every_tracked_paradigm_is_supported_and_nothing_else() -> None:
    assert SUPPORTED == {("OPEN_FIELD", 1), ("EPM", 1), ("Y_MAZE", 1), ("BARNES_MAZE", 1), ("THREE_CHAMBER", 1), ("MWM", 1), ("LIGHT_DARK", 1)}
    for key, version in SUPPORTED:
        job = synthetic.job()
        job["run"]["paradigmKey"], job["run"]["paradigmVersion"] = key, version
        check_supported(job)
        assert layout(job).key == key


def test_epm_samples_are_in_the_calibration_frame_with_its_zones(tmp_path: Path) -> None:
    scene, job = scenes.epm()
    source = tmp_path / "epm.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0), scenes.epm_walk, scene=scene)
    overlay = tmp_path / "overlay.mp4"
    analysis = analyze(source, job, overlay)
    assert analysis.lost_ratio == 0 and analysis.mean_confidence > 0.9
    err = observed_errors(analysis, truth)
    assert np.mean(err) <= 0.2 and max(err) <= 0.5, (np.mean(err), max(err))
    # Samples on the east open arm have x > 3 cm in the maze-centred frame.
    assert max(s.x_cm for s in analysis.samples) > 25 and min(s.y_cm for s in analysis.samples) < -25
    document = json.loads(trajectory_document(job, analysis))
    assert document["coordinateFrame"] == "CALIBRATION_CM" and document["worker"]["paradigm"] == {"key": "EPM", "version": 1}
    assert document["samples"]["inferred"] == [False] * len(truth)

    apparatus = layout(job)
    assert isinstance(apparatus, ZonedLayout)
    drawn = apparatus.drawings(job["calibration"]["transform"])
    assert sorted(d.key for d in drawn) == ["center", "closed_arms", "closed_arms", "open_arms", "open_arms"]
    # The closed arms came in pixels only: converted to cm they match the maze.
    closed = [z for z in calibrated_zones(job["calibration"]["zones"], job["calibration"]["transform"]) if z.key == "closed_arms"]
    assert closed[0].cm.contains(0, -20) and closed[1].cm.contains(0, 20) and not closed[0].cm.contains(10, -20)
    # Between two arms, beyond the margin around the maze, is outside.
    assert apparatus.inside(0, 0) and apparatus.inside(35, 0) and not apparatus.inside(25, 20)

    # The overlay shows the arms in their own colors on the grey maze.
    with av.open(str(overlay)) as container:
        image = next(container.decode(container.streams.video[0])).to_ndarray(format="rgb24").astype(int)
    for (x, y), check in (((18, -3), lambda r, g, b: r - g > 60), ((-3, -18), lambda r, g, b: b - r > 100)):
        px, py = synthetic.project(x, y, scenes.EPM_CM_TO_PX)
        assert check(*image[int(py), int(px)]), (x, y, image[int(py), int(px)])


def test_y_maze_junction_counts_as_inside(tmp_path: Path) -> None:
    scene, job = scenes.y_maze()
    source = tmp_path / "y.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0), scenes.y_walk, scene=scene)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    # The arm zones start 4 cm from the center: the junction is in no zone and
    # still tracked, so crossing it is not lost time.
    near_center = [s for s, t in zip(analysis.samples, truth) if math.hypot(t.x, t.y) < 4]
    assert near_center and all(s.tracked for s in near_center)
    assert analysis.lost_ratio == 0
    err = observed_errors(analysis, truth)
    assert np.mean(err) <= 0.2 and max(err) <= 0.5, (np.mean(err), max(err))
    zones = {z.key: z for z in calibrated_zones(job["calibration"]["zones"], job["calibration"]["transform"])}
    assert set(zones) == {"arm_a", "arm_b", "arm_c"}
    visits = [next((k for k, z in zones.items() if z.cm.contains(s.x_cm, s.y_cm)), None) for s in analysis.samples]
    order = [k for i, k in enumerate(visits) if k and (i == 0 or visits[i - 1] != k)]
    assert order == ["arm_a", "arm_b", "arm_c"]


@pytest.mark.parametrize(("water", "subject"), [(150, 105), (70, 120)], ids=["dark-animal-milky-water", "light-animal-dark-water"])
def test_mwm_tracks_in_a_tank_centred_frame_through_ripples_and_reflections(tmp_path: Path, water: int, subject: int) -> None:
    scene, job = scenes.mwm()
    source = tmp_path / "mwm.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0), scenes.swim, scene=scene, floor_level=water, subject_level=subject,
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


def test_light_dark_carries_a_hidden_animal_inside_the_dark_compartment(tmp_path: Path) -> None:
    scene, job = scenes.light_dark()
    source = tmp_path / "ld.mp4"
    truth = synthetic.render(source, synthetic.regular(9.0), scenes.ld_walk, scene=scene, radius_cm=2.5)
    overlay = tmp_path / "overlay.mp4"
    analysis = analyze(source, job, overlay)
    by_time = {t.t_us: t for t in truth}
    inferred = [s for s in analysis.samples if s.inferred]
    # Every sample of an animal fully under the lid is inferred, inside the dark compartment.
    fully_hidden = {t.t_us for t in truth if t.x - 2.5 >= 20}
    assert fully_hidden <= {s.t_us for s in inferred}
    assert all(s.tracked and s.x_cm >= 20 and 0 <= s.y_cm <= 20 and s.confidence == INFERRED_CONFIDENCE for s in inferred)
    assert all(by_time[s.t_us].x > 19 for s in inferred)
    # Hidden in the middle of the lit compartment: lost, not guessed.
    lost = {s.t_us for s in analysis.samples if not s.tracked}
    assert lost == {t.t_us for t in truth if not t.visible}
    seen = [s for s in analysis.samples if s.tracked and not s.inferred and by_time[s.t_us].x <= 17]
    assert np.mean([math.hypot(s.x_cm - by_time[s.t_us].x, s.y_cm - by_time[s.t_us].y) for s in seen]) <= 0.3
    assert max(observed_errors(analysis, truth)) <= 2.5
    assert analysis.mean_confidence > 0.9 and 0.3 < analysis.inferred_ratio < 0.45

    document = json.loads(trajectory_document(job, analysis))
    samples = document["samples"]
    assert document["coordinateFrame"] == "BOX_CM"
    assert samples["inferred"].count(True) == len(inferred)
    for tracked, inferred_flag, x in zip(samples["tracked"], samples["inferred"], samples["xCm"]):
        assert tracked == (x is not None) and (not inferred_flag or tracked)
    assert document["worker"]["qc"]["inferredRatio"] == round(analysis.inferred_ratio, 6)
    assert [d.key for d in layout(job).drawings(job["calibration"]["transform"])] == ["light", "dark"]


def test_hidden_stretches_are_inferred_only_between_dark_side_sightings() -> None:
    _, job = scenes.light_dark()
    apparatus = layout(job)
    assert isinstance(apparatus, LightDarkLayout) and apparatus.split == 20

    def seen(t: int, x: float, y: float = 10.0) -> Sample:
        return Sample(t, x, y, True, 1.0, 1.0, 1.0)

    def hidden(t: int) -> Sample:
        return Sample(t, None, None, False, 0.0)

    # Doorway, hidden, doorway again: carried just inside the dark compartment.
    samples = [seen(0, 17), hidden(1), hidden(2), seen(3, 18)]
    infer_hidden(samples, apparatus)
    assert [s.inferred for s in samples] == [False, True, True, False] and (samples[1].x_cm, samples[1].y_cm) == (22.0, 10.0)
    # Already in a visible part of the dark compartment: carried where it was.
    samples = [seen(0, 30, 5), hidden(1)]
    infer_hidden(samples, apparatus)
    assert samples[1].inferred and (samples[1].x_cm, samples[1].y_cm) == (30, 5)
    # Vanishing at the doorway but reappearing deep in the light side, from the
    # start of the clip, or vanishing far from the doorway: lost.
    for case in ([seen(0, 17), hidden(1), seen(2, 5)], [hidden(0), seen(1, 18)], [seen(0, 8), hidden(1), seen(2, 18)]):
        infer_hidden(case, apparatus)
        assert not any(s.inferred for s in case) and not case[1 if case[0].tracked else 0].tracked
    # A detection outside the box is not a hidden animal.
    outside = [seen(0, 18), Sample(1, 45.0, 10.0, False, 0.9, 5.0, 5.0), seen(2, 18)]
    infer_hidden(outside, apparatus)
    assert not outside[1].tracked and not outside[1].inferred


def test_barnes_and_three_chamber_use_the_hull_of_their_calibrated_zones() -> None:
    to_cm = [1, 0, 0, 0, 1, 0, 0, 0, 1]  # pixels equal centimeters in this layout test

    def job_for(key: str, zones) -> dict:
        job = synthetic.job()
        job["run"]["paradigmKey"] = key
        job["calibration"]["transform"] = to_cm
        job["calibration"]["zones"] = zones
        return job

    # Barnes: 12 holes on a 50 cm circle; the platform between them is inside.
    holes = [{"cm": {"circle": {"x": 50 * math.cos(a), "y": 50 * math.sin(a), "radius": 2.5}}}
             for a in np.linspace(0, 2 * math.pi, 12, endpoint=False)]
    barnes = layout(job_for("BARNES_MAZE", {"target_hole": holes[:1], "holes": holes[1:]}))
    assert barnes.inside(0, 0) and barnes.inside(30, -20) and barnes.inside(55, 0) and not barnes.inside(70, 0)

    # Three chamber, as a list of entries and as separate unit maps.
    chambers = {"social_chamber": (0, 20), "center_chamber": (20, 40), "object_chamber": (40, 60)}
    entries = [{"key": key, "polygon": [{"x": a, "y": 0}, {"x": b, "y": 0}, {"x": b, "y": 40}, {"x": a, "y": 40}]} for key, (a, b) in chambers.items()]
    by_unit = {"pixels": {e["key"]: [{"polygon": e["polygon"]}] for e in entries}, "cm": {e["key"]: [{"polygon": e["polygon"]}] for e in entries}}
    for zones in (entries, by_unit):
        three = layout(job_for("THREE_CHAMBER", zones))
        assert [d.key for d in three.drawings(to_cm)] == list(chambers)
        assert three.inside(30, 20) and three.inside(-2, 20) and not three.inside(30, 50)

    # Without zones only the crop limits the subject.
    assert layout(job_for("EPM", None)).inside(1e4, 1e4)


def test_circle_zones_drawn_in_pixels_are_converted_to_centimeters() -> None:
    scale = [0.5, 0, 0, 0, 0.5, 0, 0, 0, 1]  # 2 px per cm
    zones = calibrated_zones({"platform": [{"circle": {"x": 100, "y": 60, "radius": 10}}]}, scale)
    (zone,) = zones
    assert zone.pixels == Shape(circle=(100.0, 60.0, 10.0))
    assert zone.cm.contains(50, 30) and zone.cm.contains(54.9, 30) and not zone.cm.contains(55.2, 30)
