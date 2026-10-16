"""The calibration homography, row-major, from full-frame pixels to the
calibration's centimeter frame, and the planar shapes zones are made of."""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np

Point = tuple[float, float]
# Vertices used when a circle has to be drawn or projected as a polygon.
CIRCLE_VERTICES = 64


def to_arena(transform: Sequence[float], px: float, py: float) -> tuple[float, float]:
    h = transform
    w = h[6] * px + h[7] * py + h[8]
    return (h[0] * px + h[1] * py + h[2]) / w, (h[3] * px + h[4] * py + h[5]) / w


def to_pixels(transform: Sequence[float], x: float, y: float) -> tuple[float, float]:
    return to_arena(inverse(transform), x, y)


def inverse(transform: Sequence[float]) -> list[float]:
    return [float(v) for v in np.linalg.inv(np.asarray(transform, dtype=np.float64).reshape(3, 3)).reshape(9)]


def grid_to_arena(transform: Sequence[float], crop: tuple[int, int, int, int]) -> tuple[np.ndarray, np.ndarray]:
    """Centimeter coordinates of every pixel center of a crop (x, y, width, height)."""
    x0, y0, width, height = crop
    rows, cols = np.mgrid[y0 : y0 + height, x0 : x0 + width]
    px, py = cols + 0.5, rows + 0.5
    h = transform
    w = h[6] * px + h[7] * py + h[8]
    return (h[0] * px + h[1] * py + h[2]) / w, (h[3] * px + h[4] * py + h[5]) / w


@dataclass(frozen=True)
class Shape:
    """A circle (center and radius) or a polygon, in one coordinate frame."""

    circle: tuple[float, float, float] | None = None
    polygon: tuple[Point, ...] = ()

    def outline(self) -> list[Point]:
        """The boundary as a polygon; a circle is sampled at CIRCLE_VERTICES points."""
        if self.circle is None:
            return list(self.polygon)
        cx, cy, r = self.circle
        steps = np.linspace(0, 2 * math.pi, CIRCLE_VERTICES, endpoint=False)
        return [(cx + r * math.cos(a), cy + r * math.sin(a)) for a in steps]

    def mapped(self, transform: Sequence[float]) -> Shape:
        """The shape in another frame. A homography maps lines to lines, so a
        polygon stays exact; a circle becomes a sampled polygon."""
        return Shape(polygon=tuple(to_arena(transform, x, y) for x, y in self.outline()))
