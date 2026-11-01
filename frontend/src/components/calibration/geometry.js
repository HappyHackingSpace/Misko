// Pure helpers of the calibrator: point presets per paradigm, the request
// body, and reading stored zones. Pixels are full-frame source pixels; world
// coordinates are centimetres on the measurement plane.

// Paradigms whose arena is a rectangle from (0, 0) to (width, depth) and the
// parameters that give its sides.
const BOXES = {
  NOVEL_OBJECT: ["arena_width_cm", "arena_height_cm"],
};

// Side views: the pole seen from the side (x horizontal, y the height above the
// base) and the treadmill lane seen from the side (x along the lane from the
// shock grid, y vertical). Each names the parameter that gives its length.
const SIDE_VIEWS = {
  POLE: "pole_length_cm",
  TREADMILL: "lane_length_cm",
};

// Zone keys whose shapes swap places, for counterbalanced sessions.
export const SWAPPABLE_ZONES = [["novel_object", "familiar_object"]];

// The shapes offered for a zone, the default first. Object zones are circles
// around the object; the side view zones are outlines of a part of the rig.
const ZONE_TOOLS = {
  novel_object: ["circle", "polygon"],
  familiar_object: ["circle", "polygon"],
  pole: ["polygon"],
  base: ["polygon"],
  shock_grid: ["polygon"],
};
export const zoneTools = (key) => ZONE_TOOLS[key] || ["polygon", "circle"];

export const emptyPoint = (label = "") => ({ label, pixelX: null, pixelY: null, worldX: null, worldY: null });

const round = (value, digits = 2) => Math.round(value * 10 ** digits) / 10 ** digits;

/**
 * The preset fit and check points of a paradigm, with world coordinates and
 * no pixels, or null when the paradigm has no preset (free entry).
 * `label` is a message key under calibration.presets.points, and `t` its translator.
 * `extra.laneHeight` is the height of the treadmill lane wall in cm, which no
 * parameter gives; points at that height carry `heightOf` (the share of the
 * wall height they sit at) so a later height fills them in.
 */
export function presetPoints(paradigmKey, parameters, t, extra = {}) {
  const named = (key, x, y) => ({ ...emptyPoint(t(`calibration.presets.points.${key}`)), worldX: x === null ? null : round(x), worldY: y === null ? null : round(y) });
  const box = BOXES[paradigmKey];
  if (box) {
    const w = Number(parameters?.[box[0]]);
    const d = Number(parameters?.[box[1]]);
    if (!(w > 0 && d > 0)) return null;
    return {
      fit: [named("cornerOrigin", 0, 0), named("cornerX", w, 0), named("cornerXY", w, d), named("cornerY", 0, d)],
      // Edge midpoints check the fit away from the corners it was made from.
      check: [named("edgeMidY0", w / 2, 0), named("edgeMidX", w, d / 2), named("edgeMidYD", w / 2, d)],
    };
  }
  if (paradigmKey === "POLE") {
    const l = Number(parameters?.[SIDE_VIEWS.POLE]);
    if (!(l > 0)) return null;
    // The base and the top of the pole lie on one vertical line, so two marks
    // of the rig away from the pole complete the fit; their place is measured.
    return {
      fit: [named("poleBase", 0, 0), named("poleTop", 0, l), named("rigMark1", null, null), named("rigMark2", null, null)],
      check: [named("poleMiddle", 0, l / 2), named("rigMark3", null, null), named("rigMark4", null, null)],
    };
  }
  if (paradigmKey === "TREADMILL") {
    const l = Number(parameters?.[SIDE_VIEWS.TREADMILL]);
    if (!(l > 0)) return null;
    const h = Number(extra.laneHeight) > 0 ? Number(extra.laneHeight) : null;
    const high = (key, x, share) => ({ ...named(key, x, h === null ? null : h * share), heightOf: share });
    // The lane rectangle: x from the grid end (0) to the front, y up from the belt.
    return {
      fit: [named("gridBelt", 0, 0), named("frontBelt", l, 0), high("frontTop", l, 1), high("gridTop", 0, 1)],
      check: [named("middleBelt", l / 2, 0), high("middleTop", l / 2, 1), high("gridMiddle", 0, 0.5)],
    };
  }
  if (paradigmKey === "MWM") {
    const r = Number(parameters?.tank_diameter_cm) / 2;
    if (!(r > 0)) return null;
    const diagonal = r * Math.SQRT1_2;
    // The tank centre is the origin; x grows east and y north.
    return {
      fit: [named("north", 0, r), named("east", r, 0), named("south", 0, -r), named("west", -r, 0)],
      check: [named("northEast", diagonal, diagonal), named("southEast", diagonal, -diagonal), named("southWest", -diagonal, -diagonal)],
    };
  }
  return null;
}

// The preset's sizes, for the button that applies it.
export function presetSize(paradigmKey, parameters) {
  const box = BOXES[paradigmKey];
  if (box) return { kind: "box", width: Number(parameters?.[box[0]]), depth: Number(parameters?.[box[1]]) };
  if (paradigmKey === "MWM") return { kind: "tank", diameter: Number(parameters?.tank_diameter_cm) };
  if (paradigmKey === "POLE") return { kind: "pole", length: Number(parameters?.[SIDE_VIEWS.POLE]) };
  if (paradigmKey === "TREADMILL") return { kind: "lane", length: Number(parameters?.[SIDE_VIEWS.TREADMILL]) };
  return null;
}

/**
 * Whether the points' apparatus positions include four of which no three lie
 * on one line, which a perspective fit needs. Points without both world
 * coordinates are ignored.
 */
export function generalPosition(points) {
  const list = points.filter((p) => filled(p.worldX) && filled(p.worldY)).map((p) => ({ x: Number(p.worldX), y: Number(p.worldY) }));
  if (list.length < 4) return false;
  let extent = 0;
  for (const a of list) for (const b of list) extent = Math.max(extent, Math.abs(a.x - b.x), Math.abs(a.y - b.y));
  const tolerance = 1e-6 * extent * extent;
  const line = (a, b, c) => Math.abs((b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x)) <= tolerance;
  const n = list.length;
  for (let i = 0; i < n; i++)
    for (let j = i + 1; j < n; j++)
      for (let k = j + 1; k < n; k++)
        for (let m = k + 1; m < n; m++) {
          const [a, b, c, d] = [list[i], list[j], list[k], list[m]];
          if (!line(a, b, c) && !line(a, b, d) && !line(a, c, d) && !line(b, c, d)) return true;
        }
  return false;
}

export const defaultPlane = (paradigmKey) => (paradigmKey === "MWM" ? "WATER_SURFACE" : "ARENA_FLOOR");

const filled = (value) => value !== null && value !== undefined && value !== "" && Number.isFinite(Number(value));
export const placed = (p) => filled(p.pixelX) && filled(p.pixelY);
export const complete = (p) => placed(p) && filled(p.worldX) && filled(p.worldY);

const roundPx = (value) => Math.round(Number(value) * 100) / 100;

// A zone shape in the API's form, pixels rounded to a hundredth.
export function zoneBody(shape) {
  if (shape.circle) return { key: shape.key, circle: { x: roundPx(shape.circle.x), y: roundPx(shape.circle.y), radius: roundPx(shape.circle.radius) } };
  return { key: shape.key, polygon: shape.polygon.map((p) => ({ x: roundPx(p.x), y: roundPx(p.y) })) };
}

/**
 * Reads a stored zone of a calibration into { key, pixels, centimetres }, each
 * a { circle } or { polygon } shape. Pixels may sit on the zone itself or
 * under `pixels`; centimetres under `centimeters`, `centimetres` or `cm`.
 */
export function storedZone(zone) {
  const shapeOf = (value) => (value && (value.circle || value.polygon) ? { circle: value.circle || null, polygon: value.polygon || null } : null);
  return {
    key: zone.key,
    pixels: shapeOf(zone.pixels) || shapeOf(zone.pixel) || shapeOf(zone),
    centimetres: shapeOf(zone.centimeters) || shapeOf(zone.centimetres) || shapeOf(zone.cm) || shapeOf(zone.world),
  };
}

// The area enclosed by a polygon, by the shoelace formula.
export function polygonArea(points) {
  let sum = 0;
  for (let i = 0; i < points.length; i++) {
    const a = points[i];
    const b = points[(i + 1) % points.length];
    sum += a.x * b.y - b.x * a.y;
  }
  return Math.abs(sum) / 2;
}

export const shapeArea = (shape) => (shape?.circle ? Math.PI * shape.circle.radius ** 2 : shape?.polygon ? polygonArea(shape.polygon) : 0);

export const clamp = (value, min, max) => Math.min(max, Math.max(min, value));
