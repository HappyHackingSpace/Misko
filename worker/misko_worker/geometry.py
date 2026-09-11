"""The calibration homography, row-major, from full-frame pixels to arena centimeters."""

from __future__ import annotations

from collections.abc import Sequence

import numpy as np


def to_arena(transform: Sequence[float], px: float, py: float) -> tuple[float, float]:
    h = transform
    w = h[6] * px + h[7] * py + h[8]
    return (h[0] * px + h[1] * py + h[2]) / w, (h[3] * px + h[4] * py + h[5]) / w


def to_pixels(transform: Sequence[float], x: float, y: float) -> tuple[float, float]:
    inverse = np.linalg.inv(np.asarray(transform, dtype=np.float64).reshape(3, 3)).reshape(9)
    return to_arena(inverse, x, y)
