// Pure helpers of the calibrator: point presets per paradigm, the request
// body, and reading stored zones. Pixels are full-frame source pixels; world
// coordinates are centimetres on the measurement plane.

// Paradigms whose arena is a rectangle from (0, 0) to (width, depth) and the
// parameters that give its sides.
const BOXES = {
  NOVEL_OBJECT: ["arena_width_cm", "arena_height_cm"],
};

export const emptyPoint = (label = "") => ({ label, pixelX: null, pixelY: null, worldX: null, worldY: null });

const round = (value, digits = 2) => Math.round(value * 10 ** digits) / 10 ** digits;

/**
 * The preset fit and check points of a paradigm, with world coordinates and
 * no pixels, or null when the paradigm has no preset (free entry).
 * `label` is a message key under calibration.presets.points, and `t` its translator.
 */
export function presetPoints(paradigmKey, parameters, t) {
  const named = (key, x, y) => ({ ...emptyPoint(t(`calibration.presets.points.${key}`)), worldX: round(x), worldY: round(y) });
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
  return null;
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
