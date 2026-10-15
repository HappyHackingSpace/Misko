"""Where the subject may be, per paradigm, in the calibration's centimeter frame.

The worker measures one paradigm, MWM (Morris water maze). Every sample is
reported in the frame the calibration's world points define: tank-centred, x
east and y north, the tank is hypot(x, y) <= diameter / 2.

A layout answers whether a position counts as inside the apparatus (samples
outside are untracked, as the engine treats them), which pixels detection may
look at, and which shapes the overlay draws.
"""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Any

import numpy as np

from .geometry import Point, Shape, grid_to_arena, inverse

SUPPORTED: frozenset[tuple[str, int]] = frozenset({("MWM", 1)})

# Detection looks this far beyond the apparatus, so a subject on its edge is
# seen whole and a subject past it is measured (and reported untracked).
DETECTION_MARGIN_CM = 6.0


@dataclass(frozen=True)
class Drawing:
    """A zone outline for the overlay, in full-frame pixels."""

    key: str
    points: tuple[Point, ...]
    emphasis: bool = False


@dataclass
class Layout:
    key: str
    coordinate_frame: str
    # Shapes drawn on the overlay, in the calibration's centimeter frame.
    zones_cm: list[tuple[str, Shape, bool]] = field(default_factory=list)
    # Water: low contrast, ripples and reflections need a noise-aware threshold.
    water: bool = False

    def inside(self, x: float, y: float) -> bool:
        return True

    def distance_outside(self, xs: np.ndarray, ys: np.ndarray) -> np.ndarray | None:
        """Distance in cm from the apparatus for a grid of positions (0 inside),
        or None when detection may use the whole crop."""
        return None

    def detection_mask(self, transform: Sequence[float], crop: tuple[int, int, int, int]) -> np.ndarray | None:
        xs, ys = grid_to_arena(transform, crop)
        distance = self.distance_outside(xs, ys)
        if distance is None:
            return None
        return distance <= DETECTION_MARGIN_CM

    def drawings(self, transform: Sequence[float]) -> list[Drawing]:
        back = inverse(transform)
        return [Drawing(key, tuple(shape.mapped(back).outline()), emphasis) for key, shape, emphasis in self.zones_cm]


@dataclass
class TankLayout(Layout):
    radius: float = 60.0

    def inside(self, x: float, y: float) -> bool:
        return bool(math.hypot(x, y) <= self.radius)

    def distance_outside(self, xs: np.ndarray, ys: np.ndarray) -> np.ndarray:
        return np.clip(np.hypot(xs, ys) - self.radius, 0, None)


def layout(job: dict[str, Any]) -> Layout:
    """The layout of the job's paradigm, from its pinned parameters."""
    run = job["run"]
    key = run.get("paradigmKey")
    p = run.get("parameters") or {}

    def param(name: str, default: float) -> float:
        value = p.get(name)
        return default if value is None else float(value)

    if key == "MWM":
        radius = param("tank_diameter_cm", 120.0) / 2
        px, py = param("platform_x_cm", 30.0), param("platform_y_cm", 30.0)
        platform, annulus = param("platform_diameter_cm", 10.0) / 2, param("wall_annulus_width_cm", 12.0)
        sx, sy = (1 if px >= 0 else -1), (1 if py >= 0 else -1)
        arc = np.linspace(0, math.pi / 2, 17)
        quadrant = [(0.0, 0.0)] + [(sx * radius * math.cos(a), sy * radius * math.sin(a)) for a in arc]
        return TankLayout(
            key, "TANK_CM",
            zones_cm=[
                ("tank", Shape(circle=(0.0, 0.0, radius)), False),
                ("wall_annulus", Shape(circle=(0.0, 0.0, max(radius - annulus, 0.0))), False),
                ("target_quadrant", Shape(polygon=tuple(quadrant)), False),
                ("platform", Shape(circle=(px, py, platform)), True),
            ],
            water=True, radius=radius,
        )
    raise ValueError(f"no layout for {key}")
