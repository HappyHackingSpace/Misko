"""Automatic calibration proposal for a rectangular arena of known size.

The apparatus of NOVEL_OBJECT is a box of arena_width_cm x arena_height_cm seen
from above, so the arena floor is the largest high-contrast quadrilateral of
the frame. Detection takes a few frames spread over the video, removes the
moving subject with a pixelwise median, and finds that quadrilateral: smooth,
threshold (Otsu, both polarities, on the image and on an
illumination-flattened copy for vignetting and low contrast), the largest
connected regions with interior holes filled (a subject sitting still is a
hole, not a dent in the floor), convex hull, the four hull vertices spanning
the largest quadrilateral, and corners refined as intersections of robust
line fits to the region boundary (so a shadow or a subject biting one corner
does not move it). Candidates whose aspect contradicts the known arena size
are rejected; among the convincing ones the largest wins. The corners are
ordered top-left, top-right, bottom-right, bottom-left in image coordinates,
matching the paradigm's arena frame (origin at a corner, x to the right and
y down the image), and scored with a confidence in [0, 1].

A detection is only ever a PROPOSAL: it pairs the pixel corners with the
paradigm's physical corners as FIT points for the existing calibration flow,
where a researcher confirms or adjusts every point before anything is saved.
The derived CHECK points come from the same four corners, so they cannot
validate the fit; the researcher must re-measure them (see propose).

Detection uses only the dependencies the worker already has (numpy, scipy, av);
there is no OpenCV here.
"""

from __future__ import annotations

import argparse
import itertools
import json
import math
import sys
from collections.abc import Sequence
from dataclasses import dataclass
from pathlib import Path

import numpy as np
from scipy import ndimage

from .geometry import Point, convex_hull
from .video import CorruptVideo, frames, probe

# A candidate region must cover at least this fraction of the frame: the arena
# dominates an overhead recording, while noise specks and props do not.
MIN_AREA_FRACTION = 0.05
# Detections scoring below this are discarded: the frame holds no convincing
# rectangle (round arenas, heavy occlusion, featureless frames).
MIN_CONFIDENCE = 0.6
# Hull vertices kept before the exhaustive corner search.
_MAX_HULL_VERTICES = 12
# Hull points this close to the frame edge count as cut off by the camera.
_BORDER_PX = 2.0
# Regions considered per mask, largest first: the arena is sometimes not the
# largest thresholded region (a bright desk sheet next to a low-contrast box).
_MAX_REGIONS = 3
# Observed over expected aspect ratio beyond this factor rejects a candidate:
# an overhead view distorts the aspect far less, clutter often misses by more.
_MAX_ASPECT_FACTOR = 1.6
# Corner refinement: boundary points this close to an edge line support its
# fit, tightening over the iterations; a line keeps its estimate below the
# minimum support.
_FIT_CAPS_PX = (8.0, 4.0, 2.5, 1.5)
_MIN_FIT_POINTS = 8
# Boundary points within this of an edge line count as lying on it.
_FIT_TOL_PX = 2.0
# Background pixels at or above this are saturated glare: replaced (with a
# halo for noise at the glare rim) by the median of the rest before smoothing,
# so a glare blob on an arena edge leaves a straight-chorded bite the edge
# lines bridge instead of a bulge that bends the convex hull, and leaves no
# bright smoothing halo to threshold.
_SATURATION_LEVEL = 250.0
_SATURATION_HALO_PX = 3
# Saturation covering more of the frame than this is an overexposed scene, not
# glare: cutting it would destroy the arena region itself.
_MAX_SATURATION_FRACTION = 0.25
# A real wall edge completes its intensity step within a few pixels: the step
# sampled _SHARPNESS_NEAR_PX along the edge normal must reach this share of
# the step sampled _SHARPNESS_FAR_PX out, or the boundary is a smooth
# gradient's level set (uneven lighting, a flattening moat), not an arena.
_MIN_EDGE_SHARPNESS = 0.5
_SHARPNESS_NEAR_PX = 4.0
_SHARPNESS_FAR_PX = 10.0
# A threshold is only used when the class means it separates differ by at
# least this many pooled within-class standard deviations: any split of
# unimodal noise stays near 2.8 however the threshold falls, while a real
# arena-floor edge scores far higher, so this keeps smoothed sensor noise from
# percolating into a plausible-looking region.
_MIN_SEPARATION = 4.0


@dataclass(frozen=True)
class ArenaDetection:
    """The detected arena: corners in full-frame pixels, ordered top-left,
    top-right, bottom-right, bottom-left, and the scores behind the confidence.

    rectangularity is the share of the region's boundary lying on the four
    corner-to-corner edge lines (1 when the boundary is four straight walls; a
    round arena or a bulge lowers it), solidity the region's share of the
    quadrilateral (occlusions lower it), border_clearance the share of hull
    points away from the frame edge (a cropped arena lowers it). confidence is
    their product.
    """

    corners: tuple[Point, Point, Point, Point]
    confidence: float
    rectangularity: float
    solidity: float
    border_clearance: float


def detect_arena(images: Sequence[np.ndarray], *, expected_aspect: float | None = None) -> ArenaDetection | None:
    """The arena in a set of frames of one video, or None when no candidate is
    convincing. Frames are RGB or grayscale arrays of one size; several frames
    let the median remove the moving subject. expected_aspect is the arena's
    physical width over height when known: candidates whose observed aspect
    deviates by more than _MAX_ASPECT_FACTOR are rejected, so a desk sheet or
    paper towel near a low-contrast arena cannot win with a confident wrong
    answer. Of the candidates at or above MIN_CONFIDENCE the largest wins: the
    arena dominates an overhead recording."""
    if not images:
        return None
    background = _static_background(images)
    saturated = background >= _SATURATION_LEVEL
    glare = None
    if 0 < float(saturated.mean()) <= _MAX_SATURATION_FRACTION:
        glare = ndimage.binary_dilation(saturated, iterations=_SATURATION_HALO_PX)
        keep = (~glare).astype(np.float64)
        weight = ndimage.gaussian_filter(keep, 8.0)
        local = ndimage.gaussian_filter(background * keep, 8.0) / np.maximum(weight, 1e-9)
        background = np.where(glare, local, background)
    smooth = ndimage.gaussian_filter(background, 2.0)
    flattened = smooth - ndimage.gaussian_filter(smooth, min(smooth.shape) / 4.0)
    images = (smooth, flattened)
    primaries = [_otsu(image) for image in images]
    tiers = [
        [(image, t) for image, t in zip(images, primaries) if _separation(image, t) >= _MIN_SEPARATION],
        [(image, sub) for image, first in zip(images, primaries) for sub in _subthresholds(image, first)],
    ]
    for tier in tiers:
        candidates: list[ArenaDetection] = []
        for image, threshold in tier:
            for mask in (image > threshold, image <= threshold):
                if glare is not None:
                    mask = mask & ~glare
                candidates.extend(_mask_candidates(mask, image, expected_aspect))
        eligible = [c for c in candidates if c.confidence >= MIN_CONFIDENCE]
        if eligible:
            return max(eligible, key=lambda c: _polygon_area(c.corners))
    return None


def propose(detection: ArenaDetection, width_cm: float, height_cm: float) -> dict:
    """The calibration proposal for the detected corners of an arena floor of
    the given physical size: the paradigm's frame has its origin at the
    top-left corner, x to the right and y down the image, so width maps to the
    horizontal axis of the camera image (as NOVEL_OBJECT defines its
    parameters) whatever the aspect ratio.

    fitPoints are the four corners. derivedCheckPoints are the edge midpoints
    projected through the homography those same corners define: a starting
    point for the researcher, never a validation, since their error against
    that homography is zero by construction. The researcher confirms or moves
    every point in the calibration UI; nothing here is saved on its own.
    """
    if not (math.isfinite(width_cm) and math.isfinite(height_cm)) or width_cm <= 0 or height_cm <= 0:
        raise ValueError("width_cm and height_cm must be positive")
    world = ((0.0, 0.0), (width_cm, 0.0), (width_cm, height_cm), (0.0, height_cm))
    cm_to_px = _homography(world, detection.corners)
    mids = tuple(((ax + bx) / 2, (ay + by) / 2) for (ax, ay), (bx, by) in zip(world, world[1:] + world[:1]))
    return {
        "measurementPlane": "ARENA_FLOOR",
        "confidence": detection.confidence,
        "fitPoints": [_point(px, w) for px, w in zip(detection.corners, world)],
        "derivedCheckPoints": [_point(_project(cm_to_px, x, y), (x, y)) for x, y in mids],
        "warning": "derivedCheckPoints share the proposed corners and cannot validate them; confirm or re-measure every point before saving",
    }


def run(video: Path, width_cm: float, height_cm: float, samples: int = 9) -> dict | None:
    """Detects the arena on frames sampled from the video and returns the
    detection with its proposal, or None."""
    info = probe(video)
    aspect = width_cm / height_cm if width_cm > 0 and height_cm > 0 else None
    detection = detect_arena(sample_frames(video, samples), expected_aspect=aspect)
    if detection is None:
        return None
    return {
        "frameWidth": info.width,
        "frameHeight": info.height,
        "corners": [{"x": x, "y": y} for x, y in detection.corners],
        "confidence": detection.confidence,
        "rectangularity": detection.rectangularity,
        "solidity": detection.solidity,
        "borderClearance": detection.border_clearance,
        "proposal": propose(detection, width_cm, height_cm),
    }


def sample_frames(video: Path, count: int) -> list[np.ndarray]:
    """Frames spread over the video: one seek per sample when the duration is
    known, otherwise the first count frames."""
    duration = probe(video).duration_us
    out: list[np.ndarray] = []
    if duration:
        for i in range(count):
            start = int(duration * (i + 0.5) / count)
            image = next((image for _, image in frames(video, start, None)), None)
            if image is not None:
                out.append(image)
    if not out:
        out = [image for _, image in itertools.islice(frames(video, 0, None), count)]
    return out


def _static_background(images: Sequence[np.ndarray]) -> np.ndarray:
    grays = [image.mean(axis=2) if image.ndim == 3 else image.astype(np.float64) for image in images]
    return np.median(np.stack(grays), axis=0)


def _subthresholds(image: np.ndarray, first: float) -> list[float]:
    """The Otsu thresholds of the classes on either side of the first one: a
    small very bright patch (glare, a white sheet by a low-contrast arena) or
    a very dark one can absorb the whole between-class variance and leave the
    arena unseparated from the floor; these split the remaining classes. Only
    a fallback: a sub-threshold sits away from the arena's edge midtone, so
    its boundary runs a pixel or two off along the edge blur. A sub-threshold
    that does not separate its own class clearly (_MIN_SEPARATION, judged on
    that class alone) splits mere noise and is dropped."""
    out: list[float] = []
    for part in (image[image <= first], image[image > first]):
        if part.size:
            sub = _otsu(part)
            if (
                _separation(part, sub) >= _MIN_SEPARATION
                and abs(sub - first) > 1e-9
                and all(abs(sub - t) > 1e-9 for t in out)
            ):
                out.append(sub)
    return out


def _separation(image: np.ndarray, threshold: float) -> float:
    """How far apart the classes on either side of the threshold are, in
    pooled within-class standard deviations."""
    below, above = image[image <= threshold], image[image > threshold]
    if below.size == 0 or above.size == 0:
        return 0.0
    w0 = below.size / image.size
    pooled = math.sqrt(w0 * float(below.var()) + (1 - w0) * float(above.var()))
    if pooled <= 0:
        return math.inf
    return (float(above.mean()) - float(below.mean())) / pooled


def _otsu(image: np.ndarray) -> float:
    """The threshold maximizing the between-class variance of a 256-bin histogram."""
    low, high = float(image.min()), float(image.max())
    if high <= low:
        return low
    counts, edges = np.histogram(image, bins=256, range=(low, high))
    weights = counts / counts.sum()
    centers = (edges[:-1] + edges[1:]) / 2
    w0 = np.cumsum(weights)
    w1 = 1 - w0
    m0 = np.cumsum(weights * centers)
    total = m0[-1]
    with np.errstate(divide="ignore", invalid="ignore"):
        variance = w0 * w1 * (m0 / w0 - (total - m0) / w1) ** 2
    return float(centers[np.nanargmax(variance)])


def _mask_candidates(mask: np.ndarray, image: np.ndarray, expected_aspect: float | None) -> list[ArenaDetection]:
    """Detections of the _MAX_REGIONS largest regions of the mask, opened so
    that sensor-noise speckle cannot percolate into a region, then with their
    interior holes filled: a dark subject sitting inside a bright arena (or on
    its wall) is a hole or a dent, never a reason to lose the region."""
    opened = ndimage.binary_opening(mask, structure=np.ones((3, 3), dtype=bool), iterations=2)
    filled = ndimage.binary_fill_holes(opened)
    labels, count = ndimage.label(filled)
    if count == 0:
        return []
    sizes = ndimage.sum_labels(np.ones_like(labels), labels, index=np.arange(1, count + 1))
    out: list[ArenaDetection] = []
    for index in np.argsort(sizes)[::-1][:_MAX_REGIONS]:
        area = float(sizes[index])
        if area < MIN_AREA_FRACTION * mask.size:
            break
        candidate = _region_detection(labels == int(index) + 1, area, image, expected_aspect)
        if candidate is not None:
            out.append(candidate)
    return out


def _region_detection(
    region: np.ndarray, area: float, image: np.ndarray, expected_aspect: float | None
) -> ArenaDetection | None:
    boundary = region & ~ndimage.binary_erosion(region)
    rows, cols = np.nonzero(boundary)
    points = np.column_stack((cols + 0.5, rows + 0.5)).astype(np.float64)
    hull = convex_hull([(float(x), float(y)) for x, y in points])
    if len(hull) < 4:
        return None
    corners = _refined(_initial_quad(_pruned(hull)), points)
    quad_area = _polygon_area(corners)
    hull_area = _polygon_area(hull)
    if quad_area <= 0 or hull_area <= 0:
        return None
    ordered = _ordered(corners)
    if not _aspect_plausible(ordered, expected_aspect):
        return None
    height, width = region.shape
    edge = sum(
        1 for x, y in hull if x < _BORDER_PX or y < _BORDER_PX or x > width - _BORDER_PX or y > height - _BORDER_PX
    )
    rectangularity = _boundary_fit(points, corners)
    if _edge_sharpness(image, points, corners) < _MIN_EDGE_SHARPNESS:
        return None
    solidity = min(area / quad_area, 1.0)
    clearance = 1.0 - edge / len(hull)
    return ArenaDetection(
        corners=ordered,
        confidence=rectangularity * solidity * clearance,
        rectangularity=rectangularity,
        solidity=solidity,
        border_clearance=clearance,
    )


def _aspect_plausible(ordered: Sequence[Point], expected_aspect: float | None) -> bool:
    """Whether the quadrilateral's width over height is within
    _MAX_ASPECT_FACTOR of the arena's known physical aspect."""
    if expected_aspect is None or not math.isfinite(expected_aspect) or expected_aspect <= 0:
        return True
    (ax, ay), (bx, by), (cx, cy), (dx, dy) = ordered
    width = (math.hypot(bx - ax, by - ay) + math.hypot(cx - dx, cy - dy)) / 2
    height = (math.hypot(dx - ax, dy - ay) + math.hypot(cx - bx, cy - by)) / 2
    if width <= 0 or height <= 0:
        return False
    factor = (width / height) / expected_aspect
    return 1 / _MAX_ASPECT_FACTOR <= factor <= _MAX_ASPECT_FACTOR


def _initial_quad(hull: Sequence[Point]) -> tuple[Point, Point, Point, Point]:
    """The quadrilateral bounded by the lines of the four longest hull edges,
    in hull order: an occlusion (a shadow band, a subject on a corner) cuts a
    straight chord off the region, so the cut corner is no hull vertex, but
    the walls stay the longest edges and their intersection restores it. Falls
    back to the four hull vertices spanning the largest quadrilateral."""
    n = len(hull)
    fallback = _largest_quad(hull)
    if n < 4:
        return fallback
    lengths = [math.hypot(hull[(i + 1) % n][0] - hull[i][0], hull[(i + 1) % n][1] - hull[i][1]) for i in range(n)]
    longest = sorted(sorted(range(n), key=lambda i: -lengths[i])[:4])
    lines = [_line_through(hull[i], hull[(i + 1) % n]) for i in longest]
    if any(line is None for line in lines):
        return fallback
    corners = []
    for j in range(4):
        corner = _intersection(lines[j], lines[(j + 1) % 4])  # type: ignore[arg-type]
        if corner is None:
            return fallback
        corners.append(corner)
    hull_area = _polygon_area(hull)
    if not _is_convex(corners) or not 0.5 * hull_area <= _polygon_area(corners) <= 2.0 * hull_area:
        return fallback
    a, b, c, d = corners
    return a, b, c, d


def _boundary_fit(points: np.ndarray, corners: Sequence[Point]) -> float:
    """The share of boundary points within _FIT_TOL_PX of one of the four
    corner-to-corner edge lines: 1 when the boundary is four straight walls,
    low for a round arena or an irregular blob."""
    lines = [_line_through(corners[i], corners[(i + 1) % 4]) for i in range(4)]
    if any(line is None for line in lines) or len(points) == 0:
        return 0.0
    normals = np.array([[nx, ny] for nx, ny, _ in lines])
    offsets = np.array([c for _, _, c in lines])
    distances = np.abs(points @ normals.T + offsets)
    return float(np.mean(distances.min(axis=1) <= _FIT_TOL_PX))


def _edge_sharpness(image: np.ndarray, points: np.ndarray, corners: Sequence[Point]) -> float:
    """How much of the intensity step across the region's edges happens close
    to them: the median, over boundary points lying on the four edge lines, of
    the step _SHARPNESS_NEAR_PX out along the edge normal over the step
    _SHARPNESS_FAR_PX out. Near 1 at a real wall, low on the level set of a
    smooth lighting gradient."""
    lines = [_line_through(corners[i], corners[(i + 1) % 4]) for i in range(4)]
    if any(line is None for line in lines) or len(points) == 0:
        return 0.0
    normals = np.array([[nx, ny] for nx, ny, _ in lines])
    offsets = np.array([c for _, _, c in lines])
    distances = np.abs(points @ normals.T + offsets)
    nearest = np.argmin(distances, axis=1)
    on_edge = distances[np.arange(len(points)), nearest] <= _FIT_TOL_PX
    if not np.any(on_edge):
        return 0.0
    samples = points[on_edge]
    direction = normals[nearest[on_edge]]
    height, width = image.shape

    def at(offset: float) -> np.ndarray:
        x = np.clip(np.rint(samples[:, 0] + offset * direction[:, 0]).astype(int), 0, width - 1)
        y = np.clip(np.rint(samples[:, 1] + offset * direction[:, 1]).astype(int), 0, height - 1)
        return image[y, x]

    near = np.abs(at(_SHARPNESS_NEAR_PX) - at(-_SHARPNESS_NEAR_PX))
    far = np.abs(at(_SHARPNESS_FAR_PX) - at(-_SHARPNESS_FAR_PX))
    return float(np.median(near / np.maximum(far, 1e-9)))


def _refined(corners: tuple[Point, Point, Point, Point], points: np.ndarray) -> tuple[Point, Point, Point, Point]:
    """Corners as intersections of lines fitted to the boundary points of each
    edge: hull vertices sit on noise extrema, and a shadow, glare bulge or a
    subject resting on a corner moves them, while the edges keep enough clean
    support to place the corner where the walls meet. Each pass assigns every
    boundary point to its nearest edge line within a shrinking cap and refits
    the line by total least squares; implausible results fall back to the
    hull corners."""
    lines = [_line_through(corners[i], corners[(i + 1) % 4]) for i in range(4)]
    if any(line is None for line in lines):
        return corners
    for cap in _FIT_CAPS_PX:
        normals = np.array([[nx, ny] for nx, ny, _ in lines])
        offsets = np.array([c for _, _, c in lines])
        distances = np.abs(points @ normals.T + offsets)
        nearest = np.argmin(distances, axis=1)
        smallest = distances[np.arange(len(points)), nearest]
        for i in range(4):
            support = points[(nearest == i) & (smallest <= cap)]
            if len(support) >= _MIN_FIT_POINTS:
                mean = support.mean(axis=0)
                _, _, vt = np.linalg.svd(support - mean, full_matrices=False)
                nx, ny = -vt[0][1], vt[0][0]
                lines[i] = (float(nx), float(ny), float(-(nx * mean[0] + ny * mean[1])))
    refined = []
    for i in range(4):
        corner = _intersection(lines[(i - 1) % 4], lines[i])
        if corner is None:
            return corners
        refined.append(corner)
    area = _polygon_area(refined)
    original = _polygon_area(corners)
    if not all(math.isfinite(x) and math.isfinite(y) for x, y in refined):
        return corners
    if not _is_convex(refined) or not 0.7 * original <= area <= 1.4 * original:
        return corners
    return tuple(refined)


def _line_through(a: Point, b: Point) -> tuple[float, float, float] | None:
    """The line through a and b as (nx, ny, c) with a unit normal:
    nx * x + ny * y + c = 0."""
    dx, dy = b[0] - a[0], b[1] - a[1]
    norm = math.hypot(dx, dy)
    if norm < 1e-9:
        return None
    nx, ny = -dy / norm, dx / norm
    return nx, ny, -(nx * a[0] + ny * a[1])


def _intersection(p: tuple[float, float, float], q: tuple[float, float, float]) -> Point | None:
    det = p[0] * q[1] - p[1] * q[0]
    if abs(det) < 1e-9:
        return None
    return (p[1] * q[2] - p[2] * q[1]) / det, (p[2] * q[0] - p[0] * q[2]) / det


def _is_convex(quad: Sequence[Point]) -> bool:
    signs = []
    for i in range(4):
        (ax, ay), (bx, by), (cx, cy) = quad[i], quad[(i + 1) % 4], quad[(i + 2) % 4]
        signs.append((bx - ax) * (cy - by) - (by - ay) * (cx - bx))
    return all(s > 0 for s in signs) or all(s < 0 for s in signs)


def _pruned(hull: Sequence[Point]) -> list[Point]:
    """At most _MAX_HULL_VERTICES hull vertices: the vertex whose removal loses
    the least area goes first, repeatedly, so the spanning corners survive."""
    points = list(hull)
    while len(points) > _MAX_HULL_VERTICES:
        n = len(points)
        losses = [_triangle_area(points[i - 1], points[i], points[(i + 1) % n]) for i in range(n)]
        points.pop(losses.index(min(losses)))
    return points


def _largest_quad(hull: Sequence[Point]) -> tuple[Point, Point, Point, Point]:
    """The four hull vertices spanning the largest quadrilateral; hull order is
    kept, so the quadrilateral is convex."""
    best, best_area = None, -1.0
    for combo in itertools.combinations(hull, 4):
        area = _polygon_area(combo)
        if area > best_area:
            best, best_area = combo, area
    assert best is not None
    return best


def _ordered(corners: Sequence[Point]) -> tuple[Point, Point, Point, Point]:
    """Top-left, top-right, bottom-right, bottom-left in image coordinates
    (y down): sorted by angle around the centroid, started at the corner
    nearest the frame origin."""
    cx = sum(x for x, _ in corners) / 4
    cy = sum(y for _, y in corners) / 4
    ring = sorted(corners, key=lambda p: math.atan2(p[1] - cy, p[0] - cx))
    start = min(range(4), key=lambda i: ring[i][0] + ring[i][1])
    a, b, c, d = ring[start:] + ring[:start]
    return a, b, c, d


def _polygon_area(points: Sequence[Point]) -> float:
    n = len(points)
    total = 0.0
    for i in range(n):
        (x1, y1), (x2, y2) = points[i], points[(i + 1) % n]
        total += x1 * y2 - x2 * y1
    return abs(total) / 2


def _triangle_area(a: Point, b: Point, c: Point) -> float:
    return abs((b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0])) / 2


def _homography(src: Sequence[Point], dst: Sequence[Point]) -> np.ndarray:
    """The 3x3 map taking the four src points exactly to the four dst points."""
    rows, rhs = [], []
    for (x, y), (u, v) in zip(src, dst):
        rows.append([x, y, 1, 0, 0, 0, -u * x, -u * y])
        rows.append([0, 0, 0, x, y, 1, -v * x, -v * y])
        rhs += [u, v]
    h = np.linalg.solve(np.array(rows, dtype=np.float64), np.array(rhs, dtype=np.float64))
    return np.append(h, 1.0).reshape(3, 3)


def _project(h: np.ndarray, x: float, y: float) -> Point:
    u, v, w = h @ np.array([x, y, 1.0])
    return float(u / w), float(v / w)


def _point(pixel: Point, world: Point) -> dict:
    return {"pixelX": pixel[0], "pixelY": pixel[1], "worldX": world[0], "worldY": world[1]}


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m misko_worker.autocalibrate",
        description="Detect the rectangular arena in a video and print a calibration proposal as JSON.",
    )
    parser.add_argument("video", type=Path, help="path of the video file")
    parser.add_argument("--width-cm", type=float, required=True, help="internal arena width, along the horizontal image axis")
    parser.add_argument("--height-cm", type=float, required=True, help="internal arena depth, along the vertical image axis")
    parser.add_argument("--samples", type=int, default=9, help="frames sampled over the video (default: 9)")
    args = parser.parse_args(argv)
    try:
        result = run(args.video, args.width_cm, args.height_cm, max(args.samples, 1))
    except (CorruptVideo, ValueError, OSError) as error:
        json.dump({"error": str(error)}, sys.stdout)
        print()
        return 2
    if result is None:
        json.dump({"error": "NO_ARENA_DETECTED"}, sys.stdout)
        print()
        return 1
    json.dump(result, sys.stdout, indent=2)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
