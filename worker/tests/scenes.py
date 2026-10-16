"""The full Morris water maze apparatus: a 100 cm tank seen at about 2 px/cm,
with a tank-centred calibration (x east, y north), ripples, moving reflections
and the experimenter's hand at the pool side."""

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
