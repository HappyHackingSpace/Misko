"""Synthetic apparatus per paradigm: the camera homography, the floor, covers,
water effects, the job and the calibration (with zones for the paradigms whose
zones are drawn on the frame). Coordinates follow each paradigm's frame:
LIGHT_DARK box-local, MWM tank-centred with y north, the zone paradigms the
calibration's own frame (here centred on the maze)."""

from __future__ import annotations

import math
from collections.abc import Sequence

import numpy as np

from tests import synthetic
from tests.synthetic import Mask, Scene, _homography, pixel_to_cm, polygon_mask

Point = tuple[float, float]


def union(*masks: Mask) -> Mask:
    def mask(rows: np.ndarray, cols: np.ndarray) -> np.ndarray:
        out = np.zeros(rows.shape, dtype=bool)
        for m in masks:
            out |= m(rows, cols)
        return out

    return mask


def projected(cm_to_px: np.ndarray, points: Sequence[Point]) -> list[Point]:
    return [synthetic.project(x, y, cm_to_px) for x, y in points]


def screen_clockwise(points: Sequence[Point]) -> list[Point]:
    """Orders a convex polygon clockwise on screen (y down), as polygon_mask expects."""
    cx = sum(p[0] for p in points) / len(points)
    cy = sum(p[1] for p in points) / len(points)
    return sorted(points, key=lambda p: math.atan2(p[1] - cy, p[0] - cx))


def rect(x0: float, y0: float, x1: float, y1: float) -> list[Point]:
    return [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]


def zone_payload(cm_to_px: np.ndarray, zones: dict[str, list[list[Point]]], pixels_only: Sequence[str] = ()) -> dict:
    """{key: [{"pixels": {"polygon": [...]}, "cm": {"polygon": [...]}}]}; keys in
    pixels_only carry pixels only, as drawn, for the worker to convert."""
    out: dict[str, list[dict]] = {}
    for key, polygons in zones.items():
        entries = []
        for polygon in polygons:
            entry = {"pixels": {"polygon": [{"x": x, "y": y} for x, y in projected(cm_to_px, polygon)]}}
            if key not in pixels_only:
                entry["cm"] = {"polygon": [{"x": x, "y": y} for x, y in polygon]}
            entries.append(entry)
        out[key] = entries
    return out


def job_for(key: str, parameters: dict, calibration: dict, clip_end_us: int) -> dict:
    job = synthetic.job(clip_end_us=clip_end_us)
    job["run"]["paradigmKey"], job["run"]["parameters"] = key, parameters
    job["calibration"] = calibration
    return job


# EPM: a plus maze centred on the origin, arms 30 cm long and 6 cm wide.
EPM_CM_TO_PX = _homography([(-35, -35), (35, -35), (35, 35), (-35, 35)], [(58, 8), (268, 14), (276, 232), (46, 228)])
EPM_ZONES = {
    "center": [rect(-3, -3, 3, 3)],
    "open_arms": [rect(3, -3, 33, 3), rect(-33, -3, -3, 3)],
    "closed_arms": [rect(-3, -33, 3, -3), rect(-3, 3, 3, 33)],
}


def epm() -> tuple[Scene, dict]:
    bars = [rect(-33, -3, 33, 3), rect(-3, -33, 3, 33)]
    floor = union(*(polygon_mask(screen_clockwise(projected(EPM_CM_TO_PX, bar))) for bar in bars))
    scene = Scene(EPM_CM_TO_PX, floor, surround_level=55.0)
    calibration = synthetic.calibration(transform=pixel_to_cm(EPM_CM_TO_PX), crop=(30, 0, 270, 240))
    calibration["zones"] = zone_payload(EPM_CM_TO_PX, EPM_ZONES, pixels_only=("closed_arms",))
    return scene, job_for("EPM", {"max_sample_gap_s": 0.5}, calibration, 6_000_000)


def epm_walk(t: float) -> tuple[float, float, bool]:
    """Center, out along the east open arm and back, then up the north closed arm."""
    if t < 1.5:
        return 26 * t / 1.5, 0.0, True
    if t < 3.0:
        return 26 * (3.0 - t) / 1.5, 0.0, True
    if t < 4.5:
        return 0.0, -26 * (t - 3.0) / 1.5, True
    return 0.0, -26 + 16 * (t - 4.5) / 1.5, True


# Y maze: three arms 120 degrees apart around the origin, 28 cm long, 7 cm wide.
# The arm zones start 4 cm from the center, so the junction belongs to no zone.
Y_CM_TO_PX = _homography([(-32, -32), (32, -32), (32, 32), (-32, 32)], [(52, 4), (270, 10), (278, 236), (44, 232)])
Y_ANGLES = {"arm_a": -90.0, "arm_b": 150.0, "arm_c": 30.0}


def arm(angle: float, start: float, end: float, width: float = 7.0) -> list[Point]:
    ux, uy = math.cos(math.radians(angle)), math.sin(math.radians(angle))
    nx, ny = -uy, ux
    h = width / 2
    return [(start * ux + h * nx, start * uy + h * ny), (end * ux + h * nx, end * uy + h * ny),
            (end * ux - h * nx, end * uy - h * ny), (start * ux - h * nx, start * uy - h * ny)]


def y_maze() -> tuple[Scene, dict]:
    floor = union(*(polygon_mask(screen_clockwise(projected(Y_CM_TO_PX, arm(a, 0, 28)))) for a in Y_ANGLES.values()))
    scene = Scene(Y_CM_TO_PX, floor, surround_level=55.0)
    calibration = synthetic.calibration(transform=pixel_to_cm(Y_CM_TO_PX), crop=(30, 0, 260, 240))
    calibration["zones"] = zone_payload(Y_CM_TO_PX, {key: [arm(a, 4, 28)] for key, a in Y_ANGLES.items()})
    return scene, job_for("Y_MAZE", {"max_sample_gap_s": 0.5, "novel_arm": 0}, calibration, 6_000_000)


def along(angle: float, distance: float) -> Point:
    return distance * math.cos(math.radians(angle)), distance * math.sin(math.radians(angle))


def y_walk(t: float) -> tuple[float, float, bool]:
    """End of arm A to the center, out to the end of arm B, back through the center into arm C."""
    a, b, c = Y_ANGLES["arm_a"], Y_ANGLES["arm_b"], Y_ANGLES["arm_c"]
    if t < 1.0:
        return (*along(a, 24 * (1.0 - t)), True)
    if t < 2.5:
        return (*along(b, 24 * (t - 1.0) / 1.5), True)
    if t < 4.0:
        return (*along(b, 24 * (4.0 - t) / 1.5), True)
    return (*along(c, min(24.0, 24 * (t - 4.0) / 1.5)), True)


# MWM: a 100 cm tank, tank-centred calibration with x east and y north (up on screen).
MWM_RADIUS = 50.0
MWM_CM_TO_PX = _homography([(50, 0), (0, 50), (-50, 0), (0, -50)], [(262, 120), (160, 20), (58, 124), (162, 224)])
MWM_PARAMETERS = {"tank_diameter_cm": 100.0, "platform_diameter_cm": 10.0, "platform_x_cm": 20.0, "platform_y_cm": 20.0,
                  "wall_annulus_width_cm": 10.0, "max_sample_gap_s": 0.5}
# Reflections of the room lights shimmer around fixed spots on the water.
GLINTS_CM = [(-25.0, 15.0), (10.0, -30.0), (30.0, 30.0)]


def mwm(*, hand: bool = True) -> tuple[Scene, dict]:
    circle = _tank_mask(MWM_RADIUS)
    rows, cols = np.mgrid[0 : synthetic.HEIGHT, 0 : synthetic.WIDTH]
    glints = [synthetic.project(x, y, MWM_CM_TO_PX) for x, y in GLINTS_CM]
    hand_path = [(66.0, 30.0), (86.0, 22.0)]

    def effects(frame: np.ndarray, t: float, rng: np.random.Generator) -> None:
        # A travelling ripple of 4 grey levels over the water.
        water = circle(rows, cols)
        frame[water] += 4.0 * np.sin(0.21 * cols[water] + 0.17 * rows[water] - 5.0 * t)
        for gx, gy in glints:
            jx, jy = gx + 3 * math.sin(7 * t + gx), gy + 3 * math.cos(5 * t + gy)
            spot = (cols + 0.5 - jx) ** 2 + (rows + 0.5 - jy) ** 2 <= 6.0
            frame[spot] = 250.0
        if hand:
            # The experimenter's hand moving outside the tank but inside the crop.
            f = 0.5 + 0.5 * math.sin(2 * t)
            hx = hand_path[0][0] + f * (hand_path[1][0] - hand_path[0][0])
            hy = hand_path[0][1] + f * (hand_path[1][1] - hand_path[0][1])
            frame[(cols + 0.5 - hx) ** 2 + (rows + 0.5 - hy) ** 2 <= 12.0**2] = 40.0

    scene = Scene(MWM_CM_TO_PX, circle, surround_level=95.0, effects=effects)
    calibration = synthetic.calibration(transform=pixel_to_cm(MWM_CM_TO_PX), crop=(48, 10, 226, 226))
    return scene, job_for("MWM", MWM_PARAMETERS, calibration, 6_000_000)


def _tank_mask(radius: float) -> Mask:
    to_cm = pixel_to_cm(MWM_CM_TO_PX)

    def mask(rows: np.ndarray, cols: np.ndarray) -> np.ndarray:
        px, py = cols + 0.5, rows + 0.5
        w = to_cm[2, 0] * px + to_cm[2, 1] * py + to_cm[2, 2]
        x = (to_cm[0, 0] * px + to_cm[0, 1] * py + to_cm[0, 2]) / w
        y = (to_cm[1, 0] * px + to_cm[1, 1] * py + to_cm[1, 2]) / w
        return np.hypot(x, y) <= radius

    return mask


def swim(t: float) -> tuple[float, float, bool]:
    """A loop around the tank at 30 cm from the centre, then straight to the platform."""
    if t < 4.0:
        a = math.pi * t / 2
        return 30 * math.cos(a), 30 * math.sin(a), True
    f = min(1.0, (t - 4.0) / 1.5)
    return 30 + (20 - 30) * f, 20 * f, True


# LIGHT_DARK: a 40 x 20 cm box, light compartment 0 <= x < 20, the dark one under a black lid.
LD_CM_TO_PX = _homography([(0, 0), (40, 0), (40, 20), (0, 20)], [(30, 50), (290, 46), (294, 186), (26, 190)])
LD_PARAMETERS = {"box_width_cm": 40.0, "box_height_cm": 20.0, "light_fraction": 0.5, "max_sample_gap_s": 0.5}


def light_dark() -> tuple[Scene, dict]:
    box = polygon_mask(projected(LD_CM_TO_PX, rect(0, 0, 40, 20)))
    lid = polygon_mask(projected(LD_CM_TO_PX, rect(20, 0, 40, 20)))
    scene = Scene(LD_CM_TO_PX, box, surround_level=90.0, cover=lid, cover_level=25.0)
    calibration = synthetic.calibration(transform=pixel_to_cm(LD_CM_TO_PX), crop=(16, 36, 288, 164))
    return scene, job_for("LIGHT_DARK", LD_PARAMETERS, calibration, 9_000_000)


def ld_walk(t: float) -> tuple[float, float, bool]:
    """Light side to the doorway, into the dark (hidden under the lid), out again,
    then a moment hidden in the middle of the light side (an occlusion there is lost)."""
    if t < 2.0:
        return 5 + 12.5 * t / 2.0, 10.0, True
    if t < 3.0:
        return 17.5 + 12.5 * (t - 2.0), 10.0, True
    if t < 5.0:
        return 30.0, 10.0, True
    if t < 6.0:
        return 30 - 12 * (t - 5.0), 10 - 2 * (t - 5.0), True
    if t < 7.5:
        return 18 - 10 * (t - 6.0) / 1.5, 8.0, True
    return 8.0, 8.0, not (7.8 <= t < 8.2)
