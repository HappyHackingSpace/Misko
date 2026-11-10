"""Automatic calibration proposal for a rectangular arena of known size.

The apparatus of NOVEL_OBJECT is a box of arena_width_cm x arena_height_cm seen
from above, so the arena floor is the largest high-contrast quadrilateral of
the frame. Detection takes a few frames spread over the video, removes the
moving subject with a pixelwise median, and finds that quadrilateral: smooth,
threshold (Otsu, both polarities), largest connected region, convex hull, and
the four hull vertices spanning the largest quadrilateral. The corners are
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
import base64
import io
import itertools
import json
import math
import sys
from collections.abc import Sequence
from dataclasses import dataclass
from pathlib import Path

import av
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
# The panel sends frames of at most 1920 px; this leaves room for a 4K frame.
MAX_FRAME_PIXELS = 3840 * 2160
# Hull vertices kept before the exhaustive corner search.
_MAX_HULL_VERTICES = 12
# Hull points this close to the frame edge count as cut off by the camera.
_BORDER_PX = 2.0


@dataclass(frozen=True)
class ArenaDetection:
    """The detected arena: corners in full-frame pixels, ordered top-left,
    top-right, bottom-right, bottom-left, and the scores behind the confidence.

    rectangularity is the corner quadrilateral's share of the region's convex
    hull (1 when the region is a quadrilateral), solidity the region's share of
    the quadrilateral (occlusions lower it), border_clearance the share of hull
    points away from the frame edge (a cropped arena lowers it). confidence is
    their product.
    """

    corners: tuple[Point, Point, Point, Point]
    confidence: float
    rectangularity: float
    solidity: float
    border_clearance: float


def detect_arena(images: Sequence[np.ndarray]) -> ArenaDetection | None:
    """The arena in a set of frames of one video, or None when no candidate is
    convincing. Frames are RGB or grayscale arrays of one size; several frames
    let the median remove the moving subject."""
    if not images:
        return None
    background = _static_background(images)
    smooth = ndimage.gaussian_filter(background, 2.0)
    threshold = _otsu(smooth)
    best = None
    for mask in (smooth > threshold, smooth <= threshold):
        candidate = _detect_in_mask(mask)
        if candidate is not None and (best is None or candidate.confidence > best.confidence):
            best = candidate
    if best is None or best.confidence < MIN_CONFIDENCE:
        return None
    return best


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
    detection = detect_arena(sample_frames(video, samples))
    if detection is None:
        return None
    return _arena_result(info.width, info.height, detection, width_cm, height_cm)


def propose_from_frame(proposal: dict) -> dict:
    """The answer to a calibration proposal the API queued: detection on the
    one reference frame the panel uploaded, in that frame's pixels (the panel
    scales them to the video). {"detected": false, "reason": ...} when the
    apparatus is not found or has no detector."""
    try:
        with av.open(io.BytesIO(base64.b64decode(proposal["frame"]))) as container:
            frame = next(container.decode(video=0))
            # A small file can declare huge dimensions; checked before the pixels are allocated.
            if frame.width * frame.height > MAX_FRAME_PIXELS:
                return {"detected": False, "reason": "FRAME_TOO_LARGE"}
            image = frame.to_ndarray(format="rgb24")
    except (av.error.FFmpegError, StopIteration, KeyError, ValueError):
        return {"detected": False, "reason": "UNREADABLE_FRAME"}
    height, width = image.shape[:2]
    parameters = proposal.get("parameters") or {}
    if proposal.get("paradigmKey") == "NOVEL_OBJECT":
        box = parameters.get("arena_width_cm"), parameters.get("arena_height_cm")
        detection = detect_arena([image]) if all(box) else None
        if detection is None:
            return {"detected": False, "reason": "NO_ARENA_DETECTED"}
        return {"detected": True, **_arena_result(width, height, detection, *box)}
    return {"detected": False, "reason": "UNSUPPORTED_APPARATUS"}


def _arena_result(frame_width: int, frame_height: int, detection: ArenaDetection, width_cm: float, height_cm: float) -> dict:
    return {
        "frameWidth": frame_width,
        "frameHeight": frame_height,
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


def _detect_in_mask(mask: np.ndarray) -> ArenaDetection | None:
    filled = ndimage.binary_fill_holes(mask)
    labels, count = ndimage.label(filled)
    if count == 0:
        return None
    sizes = ndimage.sum_labels(np.ones_like(labels), labels, index=np.arange(1, count + 1))
    region = labels == (int(np.argmax(sizes)) + 1)
    area = float(sizes.max())
    if area < MIN_AREA_FRACTION * mask.size:
        return None
    boundary = region & ~ndimage.binary_erosion(region)
    rows, cols = np.nonzero(boundary)
    hull = convex_hull([(float(x) + 0.5, float(y) + 0.5) for y, x in zip(rows, cols)])
    if len(hull) < 4:
        return None
    corners = _largest_quad(_pruned(hull))
    quad_area = _polygon_area(corners)
    hull_area = _polygon_area(hull)
    if quad_area <= 0 or hull_area <= 0:
        return None
    height, width = mask.shape
    edge = sum(
        1 for x, y in hull if x < _BORDER_PX or y < _BORDER_PX or x > width - _BORDER_PX or y > height - _BORDER_PX
    )
    rectangularity = min(quad_area / hull_area, 1.0)
    solidity = min(area / quad_area, 1.0)
    clearance = 1.0 - edge / len(hull)
    return ArenaDetection(
        corners=_ordered(corners),
        confidence=rectangularity * solidity * clearance,
        rectangularity=rectangularity,
        solidity=solidity,
        border_clearance=clearance,
    )


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
