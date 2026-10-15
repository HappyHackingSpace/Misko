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

    def contains(self, x: float, y: float) -> bool:
        if self.circle is not None:
            cx, cy, r = self.circle
            return math.hypot(x - cx, y - cy) <= r
        return point_in_polygon(self.polygon, x, y)

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


def point_in_polygon(polygon: Sequence[Point], x: float, y: float) -> bool:
    """Even-odd rule; points on an edge count as inside."""
    inside = False
    n = len(polygon)
    for i in range(n):
        (x1, y1), (x2, y2) = polygon[i], polygon[(i + 1) % n]
        if _distance_to_segment((x1, y1), (x2, y2), x, y) <= 1e-9:
            return True
        if (y1 > y) != (y2 > y) and x < x1 + (y - y1) * (x2 - x1) / (y2 - y1):
            inside = not inside
    return inside


def convex_hull(points: Sequence[Point]) -> list[Point]:
    """Monotone chain; counter-clockwise in a y-up frame."""
    pts = sorted(set(points))
    if len(pts) < 3:
        return pts

    def cross(o: Point, a: Point, b: Point) -> float:
        return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])

    lower: list[Point] = []
    for p in pts:
        while len(lower) >= 2 and cross(lower[-2], lower[-1], p) <= 0:
            lower.pop()
        lower.append(p)
    upper: list[Point] = []
    for p in reversed(pts):
        while len(upper) >= 2 and cross(upper[-2], upper[-1], p) <= 0:
            upper.pop()
        upper.append(p)
    return lower[:-1] + upper[:-1]


def distance_to_polygon(polygon: Sequence[Point], x: float, y: float) -> float:
    """0 inside the polygon, else the distance to its nearest edge."""
    if point_in_polygon(polygon, x, y):
        return 0.0
    n = len(polygon)
    return min(_distance_to_segment(polygon[i], polygon[(i + 1) % n], x, y) for i in range(n))


def _distance_to_segment(a: Point, b: Point, x: float, y: float) -> float:
    dx, dy = b[0] - a[0], b[1] - a[1]
    length = dx * dx + dy * dy
    t = 0.0 if length == 0 else max(0.0, min(1.0, ((x - a[0]) * dx + (y - a[1]) * dy) / length))
    return math.hypot(x - (a[0] + t * dx), y - (a[1] + t * dy))
