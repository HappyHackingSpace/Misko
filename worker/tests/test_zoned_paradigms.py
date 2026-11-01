"""NOVEL_OBJECT, POLE and TREADMILL: calibrated zones, the inside rule and
tracking on synthetic recordings of each apparatus (see tests/scenes.py).

Each recording has a ground-truth trajectory. Besides the position error, the
tests compute what the Go engine derives from the samples (time inside a zone,
the first entry, stays on the grid) from both the samples and the truth with
the same rule, and require them to agree within FRAME_SLACK frames. The scenes
are clean (a disc for the subject, no motion blur, about 4 to 5 px/cm); they
check the geometry and the tuning, not accuracy on real subjects.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import av
import numpy as np
import pytest

from misko_worker.geometry import Shape
from misko_worker.paradigms import SUPPORTED, ZONE_MARGIN_CM, ZonedLayout, calibrated_zones, layout
from misko_worker.pipeline import analyze, check_supported, trajectory_document
from tests import scenes, synthetic

FPS = 25.0
FRAME_S = 1 / FPS
FRAME_SLACK = 2
MEAN_ERROR_CM, MAX_ERROR_CM = 0.3, 1.0


def zone_shapes(job: dict) -> dict[str, Shape]:
    return {z.key: z.cm for z in layout(job).zones}


def errors(analysis, truth) -> list[float]:
    return [math.hypot(s.x_cm - t.x, s.y_cm - t.y) for s, t in zip(analysis.samples, truth) if s.tracked]


def positions(analysis) -> list[tuple[float, float] | None]:
    return [(s.x_cm, s.y_cm) if s.tracked else None for s in analysis.samples]


def truth_positions(truth) -> list[tuple[float, float] | None]:
    return [(t.x, t.y) for t in truth]


def inside_flags(points, shape: Shape) -> list[bool]:
    return [p is not None and shape.contains(*p) for p in points]


def stays(flags: list[bool]) -> list[tuple[int, int]]:
    """(first frame, frame count) of each run of consecutive frames inside."""
    out, start = [], None
    for i, inside in enumerate([*flags, False]):
        if inside and start is None:
            start = i
        elif not inside and start is not None:
            out.append((start, i - start))
            start = None
    return out


def check_tracking(analysis, truth) -> None:
    assert analysis.lost_ratio == 0, analysis.lost_ratio
    err = errors(analysis, truth)
    assert np.mean(err) <= MEAN_ERROR_CM and max(err) <= MAX_ERROR_CM, (np.mean(err), max(err))


def test_the_four_lab_paradigms_are_supported() -> None:
    assert SUPPORTED == {("MWM", 1), ("NOVEL_OBJECT", 1), ("POLE", 1), ("TREADMILL", 1)}
    for make in (scenes.novel_object, scenes.pole, scenes.treadmill):
        _, job = make()
        check_supported(job)
        apparatus = layout(job)
        assert isinstance(apparatus, ZonedLayout) and apparatus.coordinate_frame == "CALIBRATION_CM"


def test_calibrated_zones_are_read_from_the_claim() -> None:
    _, job = scenes.pole()
    transform = job["calibration"]["transform"]
    zones = calibrated_zones(job["calibration"]["zones"], transform)
    assert [z.key for z in zones] == ["pole", "base"]
    assert zones[0].cm.polygon == tuple(scenes.POLE_ZONES["pole"])
    # A shape sent in one unit only is converted with the transform.
    pixels_only = [{"key": "base", "pixels": job["calibration"]["zones"][1]["pixels"]}]
    (base,) = calibrated_zones(pixels_only, transform)
    for (x, y), (ex, ey) in zip(base.cm.polygon, scenes.POLE_ZONES["base"]):
        assert abs(x - ex) < 1e-6 and abs(y - ey) < 1e-6
    # Malformed entries are skipped.
    bad = [{"key": "pole"}, {"pixels": {"circle": {"x": 1, "y": 1, "radius": 2}}}, {"key": "x", "pixels": {"circle": {"x": 1}}},
           {"key": "y", "centimeters": {"polygon": [{"x": 0, "y": 0}]}}, "zone"]
    assert calibrated_zones(bad, transform) == [] and calibrated_zones(None, transform) == []


def test_the_inside_rule_is_the_zone_hull_plus_a_margin() -> None:
    _, job = scenes.pole()
    apparatus = layout(job)
    # Between the pole and the base edge, inside the hull of both zones.
    assert apparatus.inside(8.0, 6.0) and apparatus.inside(0.0, 52.0 + ZONE_MARGIN_CM - 0.1)
    assert not apparatus.inside(0.0, 52.0 + ZONE_MARGIN_CM + 0.1) and not apparatus.inside(-20.0, 1.0)
    # NOVEL_OBJECT: the arena box from the parameters joins the object zones.
    _, job = scenes.novel_object()
    apparatus = layout(job)
    assert apparatus.inside(1.0, 39.0) and apparatus.inside(-2.9, 20.0) and not apparatus.inside(-3.1, 20.0)
    # TREADMILL: the whole lane (x from 0 to lane_length_cm) at any height.
    _, job = scenes.treadmill()
    apparatus = layout(job)
    assert apparatus.inside(40.0, 3.0) and apparatus.inside(47.9, 15.0) and not apparatus.inside(48.1, 3.0)
    # Without zones (or an extent from the parameters) the crop is the only limit.
    job["calibration"]["zones"], job["run"]["parameters"] = [], {}
    apparatus = layout(job)
    assert apparatus.inside(500.0, -500.0) and apparatus.detection_mask(job["calibration"]["transform"], (10, 40, 300, 180)) is None


def test_novel_object_tracks_visits_to_both_objects(tmp_path: Path) -> None:
    scene, job = scenes.novel_object()
    source = tmp_path / "nor.mp4"
    truth = synthetic.render(source, synthetic.regular(12.0), scenes.nor_walk, scene=scene, radius_cm=2.0)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    # The subject rests beside the novel object for 60% of the clip; the
    # objects never move and stay in the background, so neither is detected.
    assert analysis.polarity == "dark" and analysis.settings.background_percentile == 90.0
    check_tracking(analysis, truth)
    shapes = zone_shapes(job)
    for key in ("familiar_object", "novel_object"):
        measured = sum(inside_flags(positions(analysis), shapes[key]))
        expected = sum(inside_flags(truth_positions(truth), shapes[key]))
        assert expected > 0 and abs(measured - expected) <= FRAME_SLACK, (key, measured, expected)
        assert [s for s, _ in stays(inside_flags(positions(analysis), shapes[key]))] == pytest.approx(
            [s for s, _ in stays(inside_flags(truth_positions(truth), shapes[key]))], abs=FRAME_SLACK)
    document = json.loads(trajectory_document(job, analysis))
    assert document["coordinateFrame"] == "CALIBRATION_CM" and document["worker"]["paradigm"] == {"key": "NOVEL_OBJECT", "version": 1}


def turn_frame(points, pole: Shape, descent: float = 2.0) -> int | None:
    """The first frame the centroid is `descent` cm below its highest point so far, on the pole."""
    highest = -math.inf
    for i, p in enumerate(points):
        if p is None:
            continue
        highest = max(highest, p[1])
        if pole.contains(*p) and p[1] <= highest - descent:
            return i
    return None


def first_inside(points, shape: Shape) -> int | None:
    return next((i for i, inside in enumerate(inside_flags(points, shape)) if inside), None)


def test_pole_tracks_the_turn_and_the_descent(tmp_path: Path) -> None:
    scene, job = scenes.pole()
    source = tmp_path / "pole.mp4"
    truth = synthetic.render(source, synthetic.regular(10.0), scenes.pole_descent, scene=scene, radius_cm=2.0, subject_level=40)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    check_tracking(analysis, truth)
    shapes = zone_shapes(job)
    measured, expected = positions(analysis), truth_positions(truth)
    assert abs(turn_frame(measured, shapes["pole"]) - turn_frame(expected, shapes["pole"])) <= FRAME_SLACK
    assert abs(first_inside(measured, shapes["base"]) - first_inside(expected, shapes["base"])) <= FRAME_SLACK
    # A steady descent is no fall: no step faster than 80 cm/s.
    steps = [(a[1] - b[1]) / FRAME_S for a, b in zip(measured, measured[1:])]
    assert max(steps) < job["run"]["parameters"]["fall_speed_cm_s"]


def test_pole_keeps_tracking_a_fall(tmp_path: Path) -> None:
    scene, job = scenes.pole()
    job["run"]["clipEndUs"] = 6_000_000
    source = tmp_path / "fall.mp4"
    truth = synthetic.render(source, synthetic.regular(6.0), scenes.pole_fall, scene=scene, radius_cm=2.0, subject_level=40)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    check_tracking(analysis, truth)
    falling = [i for i, t in enumerate(truth) if scenes.FALL_START < t.t_us / 1e6 and t.y > scenes.SEATED]
    assert len(falling) >= 6 and all(analysis.samples[i].tracked for i in falling)
    measured = positions(analysis)
    speeds = [(a[1] - b[1]) / FRAME_S for a, b in zip(measured, measured[1:])]
    assert max(speeds) > job["run"]["parameters"]["fall_speed_cm_s"] * 2
    shapes = zone_shapes(job)
    assert abs(first_inside(measured, shapes["base"]) - first_inside(truth_positions(truth), shapes["base"])) <= FRAME_SLACK


def test_treadmill_tracks_grid_contacts_over_a_moving_belt(tmp_path: Path) -> None:
    scene, job = scenes.treadmill()
    source = tmp_path / "treadmill.mp4"
    truth = synthetic.render(source, synthetic.regular(15.0), scenes.treadmill_run, scene=scene, radius_cm=2.2, floor_level=175)
    analysis = analyze(source, job, tmp_path / "overlay.mp4")
    # The belt texture needs the noise-aware threshold; the final stay on the
    # grid (56% of the clip) stays foreground with the 75th percentile background.
    assert analysis.settings.noise_factor > 0 and analysis.settings.background_percentile == 75.0
    check_tracking(analysis, truth)
    grid = zone_shapes(job)["shock_grid"]
    measured, expected = stays(inside_flags(positions(analysis), grid)), stays(inside_flags(truth_positions(truth), grid))
    parameters = job["run"]["parameters"]
    shocks = [s for s in measured if s[1] * FRAME_S >= parameters["min_shock_dwell_s"]]
    true_shocks = [s for s in expected if s[1] * FRAME_S >= parameters["min_shock_dwell_s"]]
    assert len(shocks) == len(true_shocks) == 3
    for (start, length), (true_start, true_length) in zip(shocks, true_shocks):
        assert abs(start - true_start) <= FRAME_SLACK and abs(length - true_length) <= FRAME_SLACK
    # Exhaustion: the start of the first stay of at least exhaustion_dwell_s.
    exhausted = [s for s, n in measured if n * FRAME_S >= parameters["exhaustion_dwell_s"]]
    true_exhausted = [s for s, n in expected if n * FRAME_S >= parameters["exhaustion_dwell_s"]]
    assert len(exhausted) == 1 and abs(exhausted[0] - true_exhausted[0]) <= FRAME_SLACK


def test_the_overlay_draws_the_calibrated_zones(tmp_path: Path) -> None:
    scene, job = scenes.novel_object()
    job["run"]["clipEndUs"] = 1_000_000
    source = tmp_path / "nor.mp4"
    synthetic.render(source, synthetic.regular(1.0), scenes.nor_walk, scene=scene, radius_cm=2.0)
    overlay = tmp_path / "overlay.mp4"
    analyze(source, job, overlay)
    with av.open(str(overlay)) as container:
        image = next(container.decode(container.streams.video[0])).to_ndarray(format="rgb24").astype(int)

    def colored(x: float, y: float, check) -> bool:
        px, py = synthetic.project(x, y, scenes.NOR_CM_TO_PX)
        # A circle drawn in pixels has one radius, while the camera tilt makes
        # the projected centimeter circle slightly oval: look a few pixels around.
        patch = image[int(py) - 3 : int(py) + 4, int(px) - 3 : int(px) + 4].reshape(-1, 3)
        return any(check(*rgb) for rgb in patch)

    radius = scenes.NOR_ZONE_RADIUS
    fx, fy, _ = scenes.NOR_OBJECTS["familiar_object"]
    nx, ny, _ = scenes.NOR_OBJECTS["novel_object"]
    # The arena box (gold), the familiar zone (pink) and the novel zone (blue).
    assert colored(20.0, 0.0, lambda r, g, b: r - b > 80 and g - b > 60)
    assert colored(fx, fy - radius, lambda r, g, b: r - g > 80 and b - g > 30)
    assert colored(nx + radius, ny, lambda r, g, b: b - r > 100)
