"""Arena detection and the calibration proposal on synthetic frames."""

from __future__ import annotations

import math
from pathlib import Path

import numpy as np
import pytest

from misko_worker import autocalibrate
from tests import scenes, synthetic
from tests.synthetic import HEIGHT, WIDTH, _homography

# The NOVEL_OBJECT box of tests/scenes.py: a 40 x 40 cm floor seen by a tilted
# camera, its corners projected to these pixels.
ARENA_CORNERS = [(52.0, 28.0), (268.0, 36.0), (292.0, 222.0), (30.0, 214.0)]
SIDE_CM = 40.0


def arena_frames(corners, *, floor_level=185.0, surround_level=60.0, count=7, noise=2.0, seed=3, subject_level=45.0):
    """Frames of a quadrilateral floor on a plain surround with sensor noise
    and a dark disc moving over the floor, one position per frame."""
    cm_to_px = _homography([(0, 0), (SIDE_CM, 0), (SIDE_CM, SIDE_CM), (0, SIDE_CM)], corners)
    to_cm = np.linalg.inv(cm_to_px)
    rng = np.random.default_rng(seed)
    rows, cols = np.mgrid[0:HEIGHT, 0:WIDTH]
    px, py = cols + 0.5, rows + 0.5
    w = to_cm[2, 0] * px + to_cm[2, 1] * py + to_cm[2, 2]
    x = (to_cm[0, 0] * px + to_cm[0, 1] * py + to_cm[0, 2]) / w
    y = (to_cm[1, 0] * px + to_cm[1, 1] * py + to_cm[1, 2]) / w
    inside = (x >= 0) & (x <= SIDE_CM) & (y >= 0) & (y <= SIDE_CM)
    base = np.full((HEIGHT, WIDTH), surround_level)
    base[inside] = floor_level
    out = []
    for i in range(count):
        frame = base + rng.normal(0, noise, (HEIGHT, WIDTH))
        sx, sy = 8.0 + i * 24.0 / max(count - 1, 1), 12.0 + i * 16.0 / max(count - 1, 1)
        frame[np.hypot(x - sx, y - sy) <= 2.2] = subject_level
        out.append(np.clip(frame, 0, 255).astype(np.uint8))
    return out


def assert_corners_close(found, expected, tolerance):
    for (fx, fy), (ex, ey) in zip(found, expected):
        assert math.hypot(fx - ex, fy - ey) <= tolerance, f"corner {found} vs {expected}"


def test_detects_tilted_bright_arena():
    detection = autocalibrate.detect_arena(arena_frames(ARENA_CORNERS))
    assert detection is not None
    assert detection.confidence >= 0.85
    assert_corners_close(detection.corners, ARENA_CORNERS, 3.0)


def test_detects_dark_arena_on_bright_surround():
    detection = autocalibrate.detect_arena(arena_frames(ARENA_CORNERS, floor_level=60.0, surround_level=185.0, subject_level=235.0))
    assert detection is not None
    assert_corners_close(detection.corners, ARENA_CORNERS, 3.0)


def test_median_removes_the_moving_subject():
    # One frame alone keeps the subject's dark disc as a hole or dent; several
    # frames with the subject in different places must not move a corner.
    detection = autocalibrate.detect_arena(arena_frames(ARENA_CORNERS, count=9, subject_level=0.0))
    assert detection is not None
    assert_corners_close(detection.corners, ARENA_CORNERS, 3.0)


def test_rejects_featureless_frames():
    rng = np.random.default_rng(5)
    frames = [np.clip(rng.normal(120, 8, (HEIGHT, WIDTH)), 0, 255).astype(np.uint8) for _ in range(5)]
    assert autocalibrate.detect_arena(frames) is None


def test_rejects_empty_input():
    assert autocalibrate.detect_arena([]) is None


def test_proposal_pairs_corners_with_the_arena_frame():
    detection = autocalibrate.ArenaDetection(
        corners=tuple(ARENA_CORNERS), confidence=0.95, rectangularity=0.99, solidity=0.97, border_clearance=1.0
    )
    proposal = autocalibrate.propose(detection, 50.0, 40.0)
    assert proposal["measurementPlane"] == "ARENA_FLOOR"
    worlds = [(p["worldX"], p["worldY"]) for p in proposal["fitPoints"]]
    assert worlds == [(0.0, 0.0), (50.0, 0.0), (50.0, 40.0), (0.0, 40.0)]
    pixels = [(p["pixelX"], p["pixelY"]) for p in proposal["fitPoints"]]
    assert pixels == ARENA_CORNERS

    # Derived CHECK points are the world edge midpoints through the homography
    # the corners define, so they carry zero error by construction.
    cm_to_px = _homography([(0, 0), (50, 0), (50, 40), (0, 40)], ARENA_CORNERS)
    mids = [(25.0, 0.0), (50.0, 20.0), (25.0, 40.0), (0.0, 20.0)]
    assert [(p["worldX"], p["worldY"]) for p in proposal["derivedCheckPoints"]] == mids
    for p, (mx, my) in zip(proposal["derivedCheckPoints"], mids):
        u, v, w = cm_to_px @ np.array([mx, my, 1.0])
        assert math.hypot(p["pixelX"] - u / w, p["pixelY"] - v / w) < 1e-9
    assert "cannot validate" in proposal["warning"]


def test_run_on_a_rendered_novel_object_video(tmp_path: Path) -> None:
    scene, _ = scenes.novel_object()
    source = tmp_path / "nor.mp4"
    synthetic.render(source, synthetic.regular(2.0), scenes.nor_walk, scene=scene)

    result = autocalibrate.run(source, SIDE_CM, SIDE_CM, samples=5)
    assert result is not None
    assert (result["frameWidth"], result["frameHeight"]) == (WIDTH, HEIGHT)
    expected = [synthetic.project(x, y, scenes.NOR_CM_TO_PX) for x, y in [(0, 0), (SIDE_CM, 0), (SIDE_CM, SIDE_CM), (0, SIDE_CM)]]
    found = [(c["x"], c["y"]) for c in result["corners"]]
    assert_corners_close(found, expected, 4.0)
    assert result["proposal"]["fitPoints"][2]["worldX"] == SIDE_CM


def test_cli_reports_no_arena(tmp_path: Path, capsys) -> None:
    source = tmp_path / "flat.mp4"
    synthetic.render(source, synthetic.regular(0.5), lambda t: (0.0, 0.0, False),
                     scene=synthetic.Scene(synthetic.CM_TO_PX, None, surround_level=120.0), noise=3.0)
    code = autocalibrate.main([str(source), "--width-cm", "40", "--height-cm", "40"])
    assert code == 1
    assert "NO_ARENA_DETECTED" in capsys.readouterr().out


# MWM: the rendered tank with ripples, glints and the experimenter's hand. The
# frame is tank-centred with north up on screen, so each proposed wall point
# must land near the true projection of its compass point.
def test_run_tank_on_a_rendered_mwm_video(tmp_path: Path) -> None:
    scene, _ = scenes.mwm()
    source = tmp_path / "mwm.mp4"
    synthetic.render(source, synthetic.regular(2.0), scenes.swim, scene=scene)

    result = autocalibrate.run_tank(source, 2 * scenes.MWM_RADIUS, samples=5)
    assert result is not None and result["confidence"] >= autocalibrate.MIN_CONFIDENCE
    proposal = result["proposal"]
    assert proposal["measurementPlane"] == "WATER_SURFACE"
    for point in proposal["fitPoints"]:
        ex, ey = synthetic.project(point["worldX"], point["worldY"], scenes.MWM_CM_TO_PX)
        assert math.hypot(point["pixelX"] - ex, point["pixelY"] - ey) <= 5.0, point
    assert [(p["worldX"], p["worldY"]) for p in proposal["fitPoints"]] == [(50, 0), (0, 50), (-50, 0), (0, -50)]


def test_a_rectangular_arena_is_not_a_tank() -> None:
    assert autocalibrate.detect_tank(arena_frames(ARENA_CORNERS)) is None


def test_cli_needs_one_apparatus_size(capsys) -> None:
    with pytest.raises(SystemExit):
        autocalibrate.main(["v.mp4", "--width-cm", "40", "--height-cm", "40", "--tank-diameter-cm", "100"])
    with pytest.raises(SystemExit):
        autocalibrate.main(["v.mp4"])
