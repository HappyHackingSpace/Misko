"""The full apparatus of each paradigm, with its calibration and, for
NOVEL_OBJECT, POLE and TREADMILL, the calibrated zones as the claim sends them.

- MWM: a 100 cm tank seen at about 2 px/cm, with a tank-centred calibration
  (x east, y north), ripples, moving reflections and the experimenter's hand
  at the pool side.
- NOVEL_OBJECT: a box seen from above with two static objects.
- POLE: a pole and its base seen from the side, y the height above the base.
- TREADMILL: a lane seen from the side, a moving textured belt and a static
  shock grid, x from the grid end.
"""

from __future__ import annotations

import math

import numpy as np

from tests import synthetic
from tests.synthetic import Scene, _homography, disc_mask, pixel_to_cm


# MWM: a 100 cm tank, tank-centred calibration with x east and y north (up on screen).
MWM_RADIUS = 50.0
MWM_CM_TO_PX = _homography([(50, 0), (0, 50), (-50, 0), (0, -50)], [(262, 120), (160, 20), (58, 124), (162, 224)])
MWM_PARAMETERS = {"tank_diameter_cm": 100.0, "platform_diameter_cm": 10.0, "platform_x_cm": 20.0, "platform_y_cm": 20.0,
                  "wall_annulus_width_cm": 10.0, "max_sample_gap_s": 0.5}
# Reflections of the room lights shimmer around fixed spots on the water.
GLINTS_CM = [(-25.0, 15.0), (10.0, -30.0), (30.0, 30.0)]


def mwm(*, hand: bool = True) -> tuple[Scene, dict]:
    circle = disc_mask(MWM_CM_TO_PX, MWM_RADIUS)
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
    job = synthetic.job(clip_end_us=6_000_000)
    job["run"]["parameters"], job["calibration"] = dict(MWM_PARAMETERS), calibration
    return scene, job


def swim(t: float) -> tuple[float, float, bool]:
    """A loop around the tank at 30 cm from the centre, then straight to the platform."""
    if t < 4.0:
        a = math.pi * t / 2
        return 30 * math.cos(a), 30 * math.sin(a), True
    f = min(1.0, (t - 4.0) / 1.5)
    return 30 + (20 - 30) * f, 20 * f, True


# Helpers for the paradigms whose apparatus comes as calibrated zones.

Point = tuple[float, float]


def cm_grid(cm_to_px: np.ndarray, rows: np.ndarray, cols: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
    """Centimeter coordinates of pixel centers."""
    to_cm = pixel_to_cm(cm_to_px)
    px, py = cols + 0.5, rows + 0.5
    w = to_cm[2, 0] * px + to_cm[2, 1] * py + to_cm[2, 2]
    return (to_cm[0, 0] * px + to_cm[0, 1] * py + to_cm[0, 2]) / w, (to_cm[1, 0] * px + to_cm[1, 1] * py + to_cm[1, 2]) / w


def rect(x0: float, y0: float, x1: float, y1: float) -> list[Point]:
    return [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]


def polygon_zone(key: str, cm_to_px: np.ndarray, polygon: list[Point]) -> dict:
    """A calibrated zone as the claim sends it: the shape in pixels and in centimeters."""
    pixels = [synthetic.project(x, y, cm_to_px) for x, y in polygon]
    return {"key": key, "pixels": {"polygon": [{"x": x, "y": y} for x, y in pixels]},
            "centimeters": {"polygon": [{"x": x, "y": y} for x, y in polygon]}}


def circle_zone(key: str, cm_to_px: np.ndarray, x: float, y: float, radius: float) -> dict:
    """A circle drawn on the frame: the pixel circle has the projected centre and
    the mean distance to the projected boundary as its radius; the centimeter
    circle is the one the engine uses."""
    cx, cy = synthetic.project(x, y, cm_to_px)
    boundary = [synthetic.project(x + radius * math.cos(a), y + radius * math.sin(a), cm_to_px) for a in np.linspace(0, 2 * math.pi, 32, endpoint=False)]
    r_px = float(np.mean([math.hypot(u - cx, v - cy) for u, v in boundary]))
    return {"key": key, "pixels": {"circle": {"x": cx, "y": cy, "radius": r_px}},
            "centimeters": {"circle": {"x": x, "y": y, "radius": radius}}}


def job_for(key: str, parameters: dict, calibration: dict, clip_end_us: int) -> dict:
    job = synthetic.job(clip_end_us=clip_end_us)
    job["run"]["paradigmKey"], job["run"]["parameters"] = key, parameters
    job["calibration"] = calibration
    return job


Waypoint = tuple[float, float, float]


def piecewise(waypoints: list[Waypoint]):
    """Linear motion through (t, x, y) waypoints; still before the first and after the last."""

    def at(t: float) -> tuple[float, float, bool]:
        if t <= waypoints[0][0]:
            return waypoints[0][1], waypoints[0][2], True
        for (t0, x0, y0), (t1, x1, y1) in zip(waypoints, waypoints[1:]):
            if t <= t1:
                f = (t - t0) / (t1 - t0) if t1 > t0 else 1.0
                return x0 + f * (x1 - x0), y0 + f * (y1 - y0), True
        return waypoints[-1][1], waypoints[-1][2], True

    return at


# NOVEL_OBJECT: a 40 x 40 cm box seen from above, origin at the top-left corner,
# x to the right and y down the image (about 5.4 px/cm). Two objects stand on
# the floor: a dark familiar one and a light novel one, 2.5 cm in radius; their
# zones add 2 cm of exploration distance.
NOR_SIDE = 40.0
NOR_CM_TO_PX = _homography([(0, 0), (NOR_SIDE, 0), (NOR_SIDE, NOR_SIDE), (0, NOR_SIDE)], [(52, 28), (268, 36), (292, 222), (30, 214)])
NOR_OBJECTS = {"familiar_object": (12.0, 13.0, 95.0), "novel_object": (28.0, 27.0, 235.0)}
NOR_OBJECT_RADIUS, NOR_ZONE_RADIUS = 2.5, 4.5
NOR_PARAMETERS = {"arena_width_cm": NOR_SIDE, "arena_height_cm": NOR_SIDE, "min_bout_s": 0.3, "min_total_exploration_s": 1.0,
                  "max_sample_gap_s": 0.5}


def novel_object() -> tuple[Scene, dict]:
    def floor(rows: np.ndarray, cols: np.ndarray) -> np.ndarray:
        x, y = cm_grid(NOR_CM_TO_PX, rows, cols)
        return (x >= 0) & (x <= NOR_SIDE) & (y >= 0) & (y <= NOR_SIDE)

    def props(image: np.ndarray, rows: np.ndarray, cols: np.ndarray) -> None:
        x, y = cm_grid(NOR_CM_TO_PX, rows, cols)
        for ox, oy, level in NOR_OBJECTS.values():
            image[np.hypot(x - ox, y - oy) <= NOR_OBJECT_RADIUS] = level

    scene = Scene(NOR_CM_TO_PX, floor, surround_level=60.0, props=props)
    calibration = synthetic.calibration(transform=pixel_to_cm(NOR_CM_TO_PX), crop=(20, 16, 290, 216))
    calibration["zones"] = [circle_zone(key, NOR_CM_TO_PX, ox, oy, NOR_ZONE_RADIUS) for key, (ox, oy, _) in NOR_OBJECTS.items()]
    return scene, job_for("NOVEL_OBJECT", dict(NOR_PARAMETERS), calibration, 12_000_000)


# 12 s: a short visit to the familiar object, then the novel one, where the
# subject rests for 7.2 s (60% of the clip), and away.
nor_walk = piecewise([
    (0.0, 6.0, 34.0), (1.5, 15.5, 14.5), (2.5, 15.0, 15.0), (4.0, 24.8, 27.5), (11.2, 24.8, 27.5), (12.0, 34.0, 36.0),
])


# POLE: the camera at the side, x horizontal and y the height above the base
# (y up, about 3.7 px/cm). A 50 cm pole, 1.2 cm thick, stands on a base slab;
# the zones are the pole (where the subject climbs) and the base.
POLE_LENGTH = 50.0
POLE_CM_TO_PX = _homography([(-15, 0), (15, 0), (15, 55), (-15, 55)], [(105, 220), (215, 218), (213, 15), (107, 13)])
POLE_ZONES = {"pole": rect(-4.0, 4.5, 4.0, 52.0), "base": rect(-14.0, -1.0, 14.0, 4.5)}
POLE_PARAMETERS = {"pole_length_cm": POLE_LENGTH, "max_trial_duration_s": 60.0, "turn_descent_cm": 2.0, "fall_speed_cm_s": 80.0,
                   "max_sample_gap_s": 0.5}


def pole() -> tuple[Scene, dict]:
    def props(image: np.ndarray, rows: np.ndarray, cols: np.ndarray) -> None:
        x, y = cm_grid(POLE_CM_TO_PX, rows, cols)
        image[(np.abs(x) <= 0.6) & (y >= 0) & (y <= POLE_LENGTH)] = 120.0
        image[(np.abs(x) <= 15) & (y < 0) & (y >= -3)] = 90.0

    scene = Scene(POLE_CM_TO_PX, None, props=props)
    calibration = synthetic.calibration(transform=pixel_to_cm(POLE_CM_TO_PX), crop=(80, 4, 160, 232))
    calibration["zones"] = [polygon_zone(key, POLE_CM_TO_PX, polygon) for key, polygon in POLE_ZONES.items()]
    return scene, job_for("POLE", dict(POLE_PARAMETERS), calibration, 10_000_000)


POLE_TOP = 47.5
SEATED = 2.0  # centroid height of a subject sitting on the base


def pole_descent(t: float) -> tuple[float, float, bool]:
    """10 s: still head up at the top, a turn (sideways swing while slipping
    3 cm), a steady descent at about 12 cm/s and a stay on the base."""
    if t < 1.5:
        return 0.0, POLE_TOP, True
    if t < 2.5:
        f = t - 1.5
        return 1.5 * math.sin(math.pi * f), POLE_TOP - 3.0 * f, True
    if t < 6.0:
        f = (t - 2.5) / 3.5
        return 0.0, POLE_TOP - 3.0 - f * (POLE_TOP - 3.0 - SEATED), True
    return 2.0, SEATED, True


FALL_START = 2.0
GRAVITY = 981.0


def pole_fall(t: float) -> tuple[float, float, bool]:
    """6 s: still at the top, a partial turn, then a free fall from 46.5 cm to
    the base (0.3 s, about 300 cm/s at the end) and a stay there."""
    if t < 1.5:
        return 0.0, POLE_TOP, True
    if t < FALL_START:
        f = (t - 1.5) / 0.5
        return 1.0 * math.sin(math.pi * f), POLE_TOP - 1.0 * f, True
    y = POLE_TOP - 1.0 - 0.5 * GRAVITY * (t - FALL_START) ** 2
    return 0.0, max(y, SEATED), True


# TREADMILL: the camera at the side of the lane, x along the lane from the
# shock grid end (x = 0) to the front (45 cm), y up from the belt surface
# (about 5.5 px/cm). The grid (x 0 to 8 cm) is static rods; the belt (x 8 to
# 46 cm, seen 4 cm deep from slightly above) carries a texture moving toward
# the grid at 20 cm/s.
LANE = 45.0
GRID_END = 8.0
BELT_SPEED = 20.0
TREADMILL_CM_TO_PX = _homography([(-3, -5), (48, -5), (48, 20), (-3, 20)], [(22, 200), (300, 204), (298, 64), (24, 60)])
TREADMILL_ZONES = {"shock_grid": rect(0.0, -4.0, GRID_END, 7.0)}
TREADMILL_PARAMETERS = {"lane_length_cm": LANE, "max_trial_duration_s": 900.0, "exhaustion_dwell_s": 5.0, "min_shock_dwell_s": 0.2,
                        "max_sample_gap_s": 0.5}
RUNNING_HEIGHT = 2.0  # centroid height of the subject on the belt


def treadmill() -> tuple[Scene, dict]:
    rows, cols = np.mgrid[0 : synthetic.HEIGHT, 0 : synthetic.WIDTH]
    x, y = cm_grid(TREADMILL_CM_TO_PX, rows, cols)
    belt = (x >= GRID_END) & (x <= LANE + 1) & (y <= 0) & (y >= -4.0)
    texture_rng = np.random.default_rng(11)
    waves = [(texture_rng.uniform(0.6, 3.0), texture_rng.uniform(0, 2 * math.pi)) for _ in range(6)]

    def props(image: np.ndarray, rows: np.ndarray, cols: np.ndarray) -> None:
        # Housing under the belt, and the grid rods (alternating every 1 cm).
        image[(x >= -1) & (x <= LANE + 1) & (y < -4.0) & (y >= -6.5)] = 80.0
        grid = (x >= 0) & (x < GRID_END) & (y <= 0.3) & (y >= -4.0)
        image[grid] = np.where(np.floor(x[grid]) % 2 == 0, 70.0, 200.0)

    def backdrop(frame: np.ndarray, t: float, rng: np.random.Generator) -> None:
        u = x[belt] + BELT_SPEED * t
        frame[belt] = 120.0 + sum(20.0 * np.sin(k * u + phase) for k, phase in waves)

    scene = Scene(TREADMILL_CM_TO_PX, None, surround_level=175.0, props=props, backdrop=backdrop)
    calibration = synthetic.calibration(transform=pixel_to_cm(TREADMILL_CM_TO_PX), crop=(10, 40, 300, 180))
    calibration["zones"] = [polygon_zone(key, TREADMILL_CM_TO_PX, polygon) for key, polygon in TREADMILL_ZONES.items()]
    return scene, job_for("TREADMILL", dict(TREADMILL_PARAMETERS), calibration, 15_000_000)


_treadmill_back = piecewise([
    (2.5, 36.0, RUNNING_HEIGHT), (4.0, 11.0, RUNNING_HEIGHT),
    # First grid contact (0.3 s), back to the belt.
    (4.1, 5.0, RUNNING_HEIGHT), (4.4, 5.0, RUNNING_HEIGHT), (4.6, 13.0, RUNNING_HEIGHT), (5.4, 13.0, RUNNING_HEIGHT),
    # Second contact (0.2 s).
    (5.5, 5.0, RUNNING_HEIGHT), (5.7, 5.0, RUNNING_HEIGHT), (5.9, 12.0, RUNNING_HEIGHT), (6.4, 12.0, RUNNING_HEIGHT),
    # The final stay on the grid from 6.6 s to the end of the 15 s clip (56%).
    (6.6, 4.0, RUNNING_HEIGHT),
])


def treadmill_run(t: float) -> tuple[float, float, bool]:
    """Running near the front (36 +- 2 cm), drifting back to the grid with two
    short contacts and a final long stay."""
    if t < 2.5:
        return 36.0 + 2.0 * math.sin(2 * math.pi * t / 1.25), RUNNING_HEIGHT, True
    return _treadmill_back(t)
