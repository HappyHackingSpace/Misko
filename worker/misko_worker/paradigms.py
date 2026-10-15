"""Where the subject may be, per paradigm, in the calibration's centimeter frame.

Every sample is reported in the frame the calibration's world points define:

- OPEN_FIELD: arena-local, origin at one arena corner, x in [0, width], y in [0, height];
- LIGHT_DARK: box-local, origin at the light-side corner, the light compartment
  is 0 <= x < box_width_cm x light_fraction;
- MWM: tank-centred, x east and y north, the tank is hypot(x, y) <= diameter / 2;
- EPM, Y_MAZE, BARNES_MAZE, THREE_CHAMBER: whatever frame the calibration
  chose; their zones are drawn on the frame and come with the calibration.

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

from .geometry import Point, Shape, convex_hull, distance_to_polygon, grid_to_arena, inverse

SUPPORTED: frozenset[tuple[str, int]] = frozenset(
    {("OPEN_FIELD", 1), ("EPM", 1), ("Y_MAZE", 1), ("BARNES_MAZE", 1), ("THREE_CHAMBER", 1), ("MWM", 1), ("LIGHT_DARK", 1)}
)
CALIBRATED_ZONE_PARADIGMS = frozenset({"EPM", "Y_MAZE", "BARNES_MAZE", "THREE_CHAMBER"})

# A position this close to the outline of the calibrated zones still counts as
# inside: an animal's centroid reaches past a drawn arm edge when it leans out.
ZONE_MARGIN_CM = 3.0
# Detection looks this far beyond the apparatus, so a subject on its edge is
# seen whole and a subject past it is measured (and reported untracked).
DETECTION_MARGIN_CM = 6.0


@dataclass(frozen=True)
class Drawing:
    """A zone outline for the overlay, in full-frame pixels."""

    key: str
    points: tuple[Point, ...]
    emphasis: bool = False


@dataclass(frozen=True)
class CalibratedZone:
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
        out = [Drawing(key, tuple(shape.mapped(back).outline()), emphasis) for key, shape, emphasis in self.zones_cm]
        out += [Drawing(key, tuple(shape.outline()), True) for key, shape in self.zones_px]
        return out


@dataclass
class BoxLayout(Layout):
    """An axis-aligned box from the origin: OPEN_FIELD and LIGHT_DARK."""

    width: float = 50.0
    height: float = 50.0

    def inside(self, x: float, y: float) -> bool:
        return bool(0 <= x <= self.width and 0 <= y <= self.height)


@dataclass
class LightDarkLayout(BoxLayout):
    split: float = 20.0
    # A subject last seen this close to the partition on the light side is at the doorway.
    doorway_cm: float = 5.0

    def hidden_side(self, x: float, y: float) -> bool:
        """True in the dark compartment or at its doorway, where the subject may vanish."""
        return self.inside(x, y) and x >= self.split - self.doorway_cm

    def carried(self, x: float, y: float) -> Point:
        """Where a subject that vanished at (x, y) is carried while hidden: the
        same place when it was already in the dark compartment, else just inside
        the doorway."""
        if x >= self.split:
            return x, y
        inset = min(2.0, (self.width - self.split) / 2)
        return self.split + inset, min(max(y, 0.0), self.height)


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
    plus ZONE_MARGIN_CM, so the junctions between zones (the Y maze center, the
    Barnes platform between its holes) count as inside. Without zones the crop
    is the only limit."""

    hull: list[Point] = field(default_factory=list)

    def inside(self, x: float, y: float) -> bool:
        if len(self.hull) < 3:
            return True
        return distance_to_polygon(self.hull, x, y) <= ZONE_MARGIN_CM

    def distance_outside(self, xs: np.ndarray, ys: np.ndarray) -> np.ndarray | None:
        if len(self.hull) < 3:
            return None
        return _hull_distance(self.hull, xs, ys)


def layout(job: dict[str, Any]) -> Layout:
    """The layout of the job's paradigm, from its pinned parameters and calibration."""
    run, calibration = job["run"], job.get("calibration") or {}
    key = run.get("paradigmKey")
    p = run.get("parameters") or {}

    def param(name: str, default: float) -> float:
        value = p.get(name)
        return default if value is None else float(value)

    if key == "OPEN_FIELD":
        width, height, side = param("arena_width_cm", 50.0), param("arena_height_cm", 50.0), param("center_fraction", 0.5)
        left, top = width * (1 - side) / 2, height * (1 - side) / 2
        return BoxLayout(
            key, "ARENA_CM",
            zones_cm=[("arena", _rect(0, 0, width, height), False), ("center", _rect(left, top, width - left, height - top), True)],
            width=width, height=height,
        )
    if key == "LIGHT_DARK":
        width, height = param("box_width_cm", 40.0), param("box_height_cm", 20.0)
        split = width * param("light_fraction", 0.5)
        return LightDarkLayout(
            key, "BOX_CM",
            zones_cm=[("light", _rect(0, 0, split, height), True), ("dark", _rect(split, 0, width, height), True)],
            width=width, height=height, split=split,
        )
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
    if key in CALIBRATED_ZONE_PARADIGMS:
        transform = [float(v) for v in calibration.get("transform") or []]
        zones = calibrated_zones(calibration.get("zones"), transform) if len(transform) == 9 else []
        hull = convex_hull([pt for z in zones for pt in z.cm.outline()])
        return ZonedLayout(key, "CALIBRATION_CM", zones_px=[(z.key, z.pixels) for z in zones], hull=hull)
    raise ValueError(f"no layout for {key}")


# Keys under which a zone entry may carry its shape in each unit.
_PIXEL_KEYS = ("pixels", "px", "pixel")
_CM_KEYS = ("cm", "centimeters", "centimetres", "world")


def calibrated_zones(raw: Any, transform: Sequence[float]) -> list[CalibratedZone]:
    """Reads the claim's calibration zones.

    The expected form is {key: [shape, ...]} where each shape carries both
    units, {"pixels": {"circle"|"polygon": ...}, "cm": {...}}. Also accepted:
    a list of {key, ...} entries, {"pixels": {key: [...]}, "cm": {key: [...]}},
    {key: {"pixels": [...], "cm": [...]}}, and bare shapes, which are pixels as
    drawn. A shape known in one unit only is converted with the calibration
    transform, so drawing and the inside check always agree with the samples.
    """
    if not raw:
        return []
    pairs: list[tuple[str, Shape | None, Shape | None]] = []
    if isinstance(raw, dict) and raw and set(raw) <= set(_PIXEL_KEYS + _CM_KEYS) and all(isinstance(v, dict) for v in raw.values()):
        pixels = next((raw[k] for k in _PIXEL_KEYS if k in raw), {})
        cm = next((raw[k] for k in _CM_KEYS if k in raw), {})
        for key in list(dict.fromkeys([*pixels, *cm])):
            pairs += _zip_units(key, _as_list(pixels.get(key)), _as_list(cm.get(key)))
    elif isinstance(raw, dict):
        for key, value in raw.items():
            if isinstance(value, dict) and any(k in value for k in _PIXEL_KEYS + _CM_KEYS) and not _is_shape(value):
                pairs += _zip_units(key, _as_list(_first(value, _PIXEL_KEYS)), _as_list(_first(value, _CM_KEYS)))
            else:
                pairs += [(key, *_units(entry)) for entry in _as_list(value)]
    elif isinstance(raw, list):
        for entry in raw:
            if isinstance(entry, dict) and entry.get("key"):
                pairs.append((str(entry["key"]), *_units(entry)))

    to_px = inverse(transform)
    out = []
    for key, px, cm in pairs:
        if px is None and cm is None:
            continue
        if cm is None:
            cm = px.mapped(transform)
        if px is None:
            px = cm.mapped(to_px)
        out.append(CalibratedZone(key, cm, px))
    return out


def _units(entry: Any) -> tuple[Shape | None, Shape | None]:
    if not isinstance(entry, dict):
        return None, None
    px, cm = _shape(_first(entry, _PIXEL_KEYS)), _shape(_first(entry, _CM_KEYS))
    if px is None and cm is None:
        px = _shape(entry)
    return px, cm


def _zip_units(key: str, pixels: list[Any], cm: list[Any]) -> list[tuple[str, Shape | None, Shape | None]]:
    count = max(len(pixels), len(cm))
    return [(key, _shape(pixels[i]) if i < len(pixels) else None, _shape(cm[i]) if i < len(cm) else None) for i in range(count)]


def _first(entry: dict[str, Any], keys: Sequence[str]) -> Any:
    return next((entry[k] for k in keys if k in entry), None)


def _as_list(value: Any) -> list[Any]:
    if value is None:
        return []
    return value if isinstance(value, list) else [value]


def _is_shape(value: dict[str, Any]) -> bool:
    return "circle" in value or "polygon" in value


def _shape(value: Any) -> Shape | None:
    if not isinstance(value, dict):
        return None
    circle = value.get("circle")
    if isinstance(circle, dict):
        radius = float(circle.get("radius", 0))
        return Shape(circle=(float(circle["x"]), float(circle["y"]), radius)) if radius > 0 else None
    polygon = value.get("polygon")
    if isinstance(polygon, list) and len(polygon) >= 3:
        vertices = tuple((float(v["x"]), float(v["y"])) if isinstance(v, dict) else (float(v[0]), float(v[1])) for v in polygon)
        return Shape(polygon=vertices)
    return None


def _rect(x0: float, y0: float, x1: float, y1: float) -> Shape:
    return Shape(polygon=((x0, y0), (x1, y0), (x1, y1), (x0, y1)))


def _hull_distance(hull: Sequence[Point], xs: np.ndarray, ys: np.ndarray) -> np.ndarray:
    """Vectorized distance to a convex polygon, 0 inside."""
    n = len(hull)
    best = np.full(xs.shape, np.inf)
    # Inside a convex polygon every edge sees the point on the same side.
    signs = []
    for i in range(n):
        (ax, ay), (bx, by) = hull[i], hull[(i + 1) % n]
        dx, dy = bx - ax, by - ay
        signs.append(dx * (ys - ay) - dy * (xs - ax))
        length = dx * dx + dy * dy
        t = np.clip(((xs - ax) * dx + (ys - ay) * dy) / length, 0, 1) if length else np.zeros(xs.shape)
        best = np.minimum(best, np.hypot(xs - (ax + t * dx), ys - (ay + t * dy)))
    stacked = np.stack(signs)
    inside = np.all(stacked >= 0, axis=0) | np.all(stacked <= 0, axis=0)
    return np.where(inside, 0.0, best)
