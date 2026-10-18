"""Where the subject may be, per paradigm, in the calibration's centimeter frame.

Every sample is reported in the frame the calibration's world points define:

- MWM: tank-centred, x east and y north, the tank is hypot(x, y) <= diameter / 2;
- NOVEL_OBJECT: camera above the arena, origin at an arena corner, x to the
  right and y down the image;
- POLE: camera at the side, x horizontal and y the height above the base;
- TREADMILL: camera at the side, x along the lane from the shock grid end
  (x = 0) to the front (x = lane_length_cm), y vertical.

The last three carry their apparatus geometry as calibrated zones (drawn on the
frame by the calibrator and sent with the claim in both pixels and
centimeters): `novel_object` and `familiar_object`, `pole` and `base`,
`shock_grid`.

A layout answers whether a position counts as inside the apparatus (samples
outside are untracked, as the engine treats them), which pixels detection may
look at, which shapes the overlay draws and how the tracker is tuned.
"""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Any

import numpy as np

from .geometry import Point, Shape, convex_distance, convex_hull, distance_to_polygon, grid_to_arena, inverse

SUPPORTED: frozenset[tuple[str, int]] = frozenset({("MWM", 1), ("NOVEL_OBJECT", 1), ("POLE", 1), ("TREADMILL", 1)})
CALIBRATED_ZONE_PARADIGMS = frozenset({"NOVEL_OBJECT", "POLE", "TREADMILL"})

# A position this close to the calibrated zones (or the apparatus extent from
# the parameters) still counts as inside: the centroid of a subject leaning out
# of a drawn zone edge reaches past it.
ZONE_MARGIN_CM = 3.0
# Detection looks this far beyond the apparatus, so a subject on its edge is
# seen whole and a subject past it is measured (and reported untracked).
DETECTION_MARGIN_CM = 6.0

# How the tracker is tuned for an apparatus (see pipeline.settings_for).
FLOOR, WATER, POLE, BELT = "floor", "water", "pole", "belt"


@dataclass(frozen=True)
class Drawing:
    """A zone outline for the overlay, in full-frame pixels."""

    key: str
    points: tuple[Point, ...]
    emphasis: bool = False


@dataclass(frozen=True)
class CalibratedZone:
    """One shape of a calibrated zone: centimeters for the inside rule,
    pixels (as drawn) for the overlay."""

    key: str
    cm: Shape
    pixels: Shape


@dataclass
class Layout:
    key: str
    coordinate_frame: str
    # Shapes drawn on the overlay, in the calibration's centimeter frame or,
    # for calibrated zones, as drawn in pixels.
    zones_cm: list[tuple[str, Shape, bool]] = field(default_factory=list)
    zones_px: list[tuple[str, Shape]] = field(default_factory=list)
    tracking: str = FLOOR

    @property
    def water(self) -> bool:
        return self.tracking == WATER

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
        out = [Drawing(key, tuple(shape.mapped(back).outline()), emphasis) for key, shape, emphasis in self.zones_cm]
        out += [Drawing(key, tuple(shape.outline()), True) for key, shape in self.zones_px]
        return out


@dataclass
class TankLayout(Layout):
    radius: float = 60.0

    def inside(self, x: float, y: float) -> bool:
        return bool(math.hypot(x, y) <= self.radius)

    def distance_outside(self, xs: np.ndarray, ys: np.ndarray) -> np.ndarray:
        return np.clip(np.hypot(xs, ys) - self.radius, 0, None)


@dataclass
class ZonedLayout(Layout):
    """Paradigms with calibrated zones. Inside is the convex hull of every zone
    (and of the apparatus extent the parameters give, such as the arena box)
    plus ZONE_MARGIN_CM, so the floor between the zones counts as inside; the
    crop limits it further, since detection never looks past the crop. A lane
    (TREADMILL) is a strip of x values: the shock grid is its only zone, while
    the subject runs over the whole lane. Without zones or an extent the crop
    is the only limit."""

    hull: list[Point] = field(default_factory=list)
    # (x_min, x_max) of a lane in cm, any y.
    strip: tuple[float, float] | None = None
    zones: list[CalibratedZone] = field(default_factory=list)

    def inside(self, x: float, y: float) -> bool:
        if len(self.hull) < 3 and self.strip is None:
            return True
        return self._distance(x, y) <= ZONE_MARGIN_CM

    def _distance(self, x: float, y: float) -> float:
        best = math.inf
        if len(self.hull) >= 3:
            best = distance_to_polygon(self.hull, x, y)
        if self.strip is not None:
            best = min(best, max(0.0, self.strip[0] - x, x - self.strip[1]))
        return best

    def distance_outside(self, xs: np.ndarray, ys: np.ndarray) -> np.ndarray | None:
        if len(self.hull) < 3 and self.strip is None:
            return None
        best = np.full(xs.shape, np.inf)
        if len(self.hull) >= 3:
            best = convex_distance(self.hull, xs, ys)
        if self.strip is not None:
            best = np.minimum(best, np.clip(np.maximum(self.strip[0] - xs, xs - self.strip[1]), 0, None))
        return best


def layout(job: dict[str, Any]) -> Layout:
    """The layout of the job's paradigm, from its pinned parameters and calibration."""
    run, calibration = job["run"], job.get("calibration") or {}
    key = run.get("paradigmKey")
    p = run.get("parameters") or {}

    def param(name: str, default: float | None) -> float | None:
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
            tracking=WATER, radius=radius,
        )
    if key in CALIBRATED_ZONE_PARADIGMS:
        transform = [float(v) for v in calibration.get("transform") or []]
        zones = calibrated_zones(calibration.get("zones"), transform) if len(transform) == 9 else []
        points = [pt for z in zones for pt in z.cm.outline()]
        zones_cm: list[tuple[str, Shape, bool]] = []
        strip, tracking = None, FLOOR
        if key == "NOVEL_OBJECT":
            width, height = param("arena_width_cm", None), param("arena_height_cm", None)
            if width and height:
                box = ((0.0, 0.0), (width, 0.0), (width, height), (0.0, height))
                points += list(box)
                zones_cm.append(("arena", Shape(polygon=box), False))
        elif key == "POLE":
            tracking = POLE
        elif key == "TREADMILL":
            tracking = BELT
            lane = param("lane_length_cm", None)
            if lane:
                strip = (0.0, lane)
        return ZonedLayout(
            key, "CALIBRATION_CM", zones_cm=zones_cm, zones_px=[(z.key, z.pixels) for z in zones], tracking=tracking,
            hull=convex_hull(points), strip=strip, zones=zones,
        )
    raise ValueError(f"no layout for {key}")


# Keys under which a zone entry may carry its shape in each unit; the claim
# sends "pixels" and "centimeters".
_PIXEL_KEYS = ("pixels", "px")
_CM_KEYS = ("centimeters", "cm")


def calibrated_zones(raw: Any, transform: Sequence[float]) -> list[CalibratedZone]:
    """Reads the claim's calibration zones: a list of
    {key, pixels: {circle: {x, y, radius}} | {polygon: [{x, y}, ...]}, centimeters: {...}}.
    A key may appear several times (one entry per shape). A shape known in one
    unit only is converted with the calibration transform, so drawing and the
    inside check always agree with the samples. Malformed entries are skipped."""
    if not isinstance(raw, list):
        return []
    to_px = inverse(transform)
    out = []
    for entry in raw:
        if not isinstance(entry, dict) or not entry.get("key"):
            continue
        px, cm = _shape(_first(entry, _PIXEL_KEYS)), _shape(_first(entry, _CM_KEYS))
        if px is None and cm is None:
            continue
        if cm is None:
            cm = px.mapped(transform)
        if px is None:
            px = cm.mapped(to_px)
        out.append(CalibratedZone(str(entry["key"]), cm, px))
    return out


def _first(entry: dict[str, Any], keys: Sequence[str]) -> Any:
    return next((entry[k] for k in keys if k in entry), None)


def _shape(value: Any) -> Shape | None:
    if not isinstance(value, dict):
        return None
    try:
        circle = value.get("circle")
        if isinstance(circle, dict):
            radius = float(circle.get("radius", 0))
            return Shape(circle=(float(circle["x"]), float(circle["y"]), radius)) if radius > 0 else None
        polygon = value.get("polygon")
        if isinstance(polygon, list) and len(polygon) >= 3:
            return Shape(polygon=tuple((float(v["x"]), float(v["y"])) for v in polygon))
    except (KeyError, TypeError, ValueError):
        return None
    return None
