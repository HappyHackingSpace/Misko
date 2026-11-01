"""Synthetic stress scenes for arena auto-calibration.

Each case renders frames of a rectangular arena floor under one realistic lab
degradation (perspective, low contrast, vignetting, shadow, glare, bedding
texture, a stationary subject, clutter, polarity, lens distortion) together
with the ground-truth corner pixels, so tests can measure corner error and
confidence against known geometry. Run as a module for the measurement table:

    uv run --no-sync python -m tests.stress_arena
"""

from __future__ import annotations

import math
from collections.abc import Callable, Sequence

import numpy as np

from tests.synthetic import HEIGHT, WIDTH, _homography

Point = tuple[float, float]

SIDE_CM = 40.0
TILT_CORNERS = [(52.0, 28.0), (268.0, 36.0), (292.0, 222.0), (30.0, 214.0)]
STRONG_TILT_CORNERS = [(96.0, 26.0), (224.0, 26.0), (302.0, 218.0), (18.0, 218.0)]
SMALL_TILT_CORNERS = [(118.0, 68.0), (268.0, 72.0), (282.0, 206.0), (108.0, 200.0)]


def stress_frames(
    corners: Sequence[Point],
    *,
    floor_level: float = 185.0,
    surround_level: float = 60.0,
    count: int = 7,
    noise: float = 2.0,
    seed: int = 11,
    subject_level: float = 45.0,
    subject_radius_cm: float = 2.2,
    stationary_at: Point | None = None,
    no_subject: bool = False,
    floor_texture: float = 0.0,
    vignette: float = 0.0,
    shadow: tuple[Point, Point, float, float] | None = None,
    glare: tuple[float, float, float] | None = None,
    clutter: tuple[int, int, int, int, float] | None = None,
    barrel: float = 0.0,
) -> tuple[list[np.ndarray], list[Point]]:
    """Frames of a SIDE_CM x SIDE_CM floor whose corners project to the given
    pixels, with the requested degradations, and the ground-truth corner
    pixels (barrel distortion moves them).

    shadow is (a, b, half_width_px, factor): pixels within half_width_px of
    the line a-b are multiplied by factor. glare is (cx, cy, radius_px) set to
    255. clutter is (x0, y0, w, h, level) painted outside the arena. vignette
    multiplies by 1 - vignette * (r / r_max)^2 from the frame center. barrel
    is the radial coefficient k of ideal = center + (p - center) * (1 + k *
    (|p - center| / r_norm)^2) applied to every output pixel.
    """
    cm_to_px = _homography([(0, 0), (SIDE_CM, 0), (SIDE_CM, SIDE_CM), (0, SIDE_CM)], corners)
    to_cm = np.linalg.inv(cm_to_px)
    rng = np.random.default_rng(seed)
    rows, cols = np.mgrid[0:HEIGHT, 0:WIDTH]
    px, py = (cols + 0.5).astype(np.float64), (rows + 0.5).astype(np.float64)
    cx0, cy0 = WIDTH / 2, HEIGHT / 2
    r_norm = math.hypot(cx0, cy0)
    if barrel:
        scale = 1.0 + barrel * ((px - cx0) ** 2 + (py - cy0) ** 2) / r_norm**2
        px, py = cx0 + (px - cx0) * scale, cy0 + (py - cy0) * scale
    w = to_cm[2, 0] * px + to_cm[2, 1] * py + to_cm[2, 2]
    x = (to_cm[0, 0] * px + to_cm[0, 1] * py + to_cm[0, 2]) / w
    y = (to_cm[1, 0] * px + to_cm[1, 1] * py + to_cm[1, 2]) / w
    inside = (x >= 0) & (x <= SIDE_CM) & (y >= 0) & (y <= SIDE_CM)

    base = np.full((HEIGHT, WIDTH), surround_level)
    base[inside] = floor_level
    if floor_texture:
        texture = rng.normal(0, floor_texture, (HEIGHT, WIDTH))
        base[inside] += texture[inside]
    if clutter is not None:
        x0, y0, cw, ch, level = clutter
        base[y0 : y0 + ch, x0 : x0 + cw] = level

    multiplier = np.ones((HEIGHT, WIDTH))
    if vignette:
        pr, pc = (cols + 0.5) - cx0, (rows + 0.5) - cy0
        multiplier *= 1.0 - vignette * (pc**2 + pr**2) / r_norm**2
    if shadow is not None:
        (ax, ay), (bx, by), half_width, factor = shadow
        nx, ny = by - ay, ax - bx
        norm = math.hypot(nx, ny)
        distance = np.abs((cols + 0.5 - ax) * nx / norm + (rows + 0.5 - ay) * ny / norm)
        multiplier = np.where(distance <= half_width, multiplier * factor, multiplier)

    frames = []
    for i in range(count):
        frame = base.copy()
        if not no_subject:
            if stationary_at is not None:
                sx, sy = stationary_at
            else:
                sx = 8.0 + i * 24.0 / max(count - 1, 1)
                sy = 12.0 + i * 16.0 / max(count - 1, 1)
            frame[np.hypot(x - sx, y - sy) <= subject_radius_cm] = subject_level
        frame *= multiplier
        if glare is not None:
            gx, gy, radius = glare
            frame[np.hypot(cols + 0.5 - gx, rows + 0.5 - gy) <= radius] = 255.0
        frame += rng.normal(0, noise, (HEIGHT, WIDTH))
        frames.append(np.clip(frame, 0, 255).astype(np.uint8))
    return frames, [_barrel_truth(corner, barrel, (cx0, cy0), r_norm) for corner in corners]


def _barrel_truth(corner: Point, k: float, center: Point, r_norm: float) -> Point:
    """The output pixel whose ideal position is the corner: inverts the radial
    map r_ideal = r * (1 + k * (r / r_norm)^2) by Newton iteration."""
    if not k:
        return corner
    cx0, cy0 = center
    dx, dy = corner[0] - cx0, corner[1] - cy0
    r_ideal = math.hypot(dx, dy)
    if r_ideal == 0:
        return corner
    r = r_ideal
    for _ in range(25):
        f = r * (1 + k * (r / r_norm) ** 2) - r_ideal
        r -= f / (1 + 3 * k * (r / r_norm) ** 2)
    return cx0 + dx * r / r_ideal, cy0 + dy * r / r_ideal


Case = Callable[[], tuple[list[np.ndarray], list[Point]]]

CASES: dict[str, Case] = {
    "strong_tilt": lambda: stress_frames(STRONG_TILT_CORNERS),
    "low_contrast": lambda: stress_frames(TILT_CORNERS, floor_level=135.0, surround_level=120.0, subject_level=100.0),
    "vignette": lambda: stress_frames(TILT_CORNERS, vignette=0.45),
    "shadow_band": lambda: stress_frames(TILT_CORNERS, shadow=((0.0, 160.0), (120.0, 240.0), 16.0, 0.45)),
    "glare_on_edge": lambda: stress_frames(TILT_CORNERS, glare=(160.0, 32.0, 14.0)),
    "textured_floor": lambda: stress_frames(TILT_CORNERS, floor_texture=22.0),
    "stationary_subject": lambda: stress_frames(TILT_CORNERS, stationary_at=(1.4, 1.4), subject_level=40.0),
    "clutter_rectangle": lambda: stress_frames(
        SMALL_TILT_CORNERS, floor_level=150.0, surround_level=120.0, subject_level=95.0, clutter=(6, 4, 100, 50, 250.0)
    ),
    "dark_arena_tilt": lambda: stress_frames(
        STRONG_TILT_CORNERS, floor_level=60.0, surround_level=185.0, subject_level=230.0
    ),
    "barrel_distortion": lambda: stress_frames(TILT_CORNERS, barrel=0.06),
}


def max_corner_error(found: Sequence[Point], expected: Sequence[Point]) -> float:
    return max(math.hypot(fx - ex, fy - ey) for (fx, fy), (ex, ey) in zip(found, expected))


def measure() -> list[tuple[str, float | None, float | None]]:
    """(case, max corner error px, confidence) per case; None, None when the
    detector returns no arena."""
    from misko_worker import autocalibrate

    out: list[tuple[str, float | None, float | None]] = []
    for name, case in CASES.items():
        frames, truth = case()
        detection = autocalibrate.detect_arena(frames, expected_aspect=1.0)
        if detection is None:
            out.append((name, None, None))
        else:
            out.append((name, max_corner_error(detection.corners, truth), detection.confidence))
    return out


if __name__ == "__main__":
    print(f"{'case':<22} {'max corner err px':>18} {'confidence':>11}")
    for name, error, confidence in measure():
        if error is None:
            print(f"{name:<22} {'-':>18} {'rejected':>11}")
        else:
            print(f"{name:<22} {error:>18.2f} {confidence:>11.3f}")
