"""Synthetic recordings with known positions.

The default scene is an Open Field: a 50 x 50 cm arena seen by a tilted camera,
a homography maps arena centimeters to a 320 x 240 frame. Other paradigms use
their own Scene (see tests/scenes.py): its homography, the floor the subject
moves on, an optional cover painted over the subject (the lid of a dark
compartment) and optional per-frame effects (ripples and reflections on water).
The subject is a disc drawn with 4x supersampling around the projected
position, over a textured floor with per-frame sensor noise. These fixtures
test the pipeline's geometry, timing and failure handling; they are not
evidence of accuracy on real animals.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from fractions import Fraction
from pathlib import Path

import av
import numpy as np

WIDTH, HEIGHT = 320, 240
ARENA = 50.0
# Arena corners (0,0), (50,0), (50,50), (0,50) seen by the camera.
CORNERS_PX = [(52.0, 28.0), (268.0, 36.0), (292.0, 222.0), (30.0, 214.0)]


def _homography(src: Sequence[tuple[float, float]], dst: Sequence[tuple[float, float]]) -> np.ndarray:
    rows, rhs = [], []
    for (x, y), (u, v) in zip(src, dst):
        rows.append([x, y, 1, 0, 0, 0, -u * x, -u * y])
        rows.append([0, 0, 0, x, y, 1, -v * x, -v * y])
        rhs += [u, v]
    h = np.linalg.solve(np.array(rows, dtype=np.float64), np.array(rhs, dtype=np.float64))
    return np.append(h, 1.0).reshape(3, 3)


CM_TO_PX = _homography([(0, 0), (ARENA, 0), (ARENA, ARENA), (0, ARENA)], CORNERS_PX)
PX_TO_CM = np.linalg.inv(CM_TO_PX)
PX_TO_CM = PX_TO_CM / PX_TO_CM[2, 2]


def project(x: float, y: float, cm_to_px: np.ndarray = CM_TO_PX) -> tuple[float, float]:
    u, v, w = cm_to_px @ np.array([x, y, 1.0])
    return u / w, v / w


def pixel_to_cm(cm_to_px: np.ndarray) -> np.ndarray:
    inverse = np.linalg.inv(cm_to_px)
    return inverse / inverse[2, 2]


def calibration(frame_width: int = WIDTH, frame_height: int = HEIGHT, *, transform: np.ndarray = PX_TO_CM,
                crop: tuple[int, int, int, int] = (20, 16, 290, 216)) -> dict:
    return {
        "id": "01a00000-0000-7000-8000-00000000ca11",
        "frameWidth": frame_width,
        "frameHeight": frame_height,
        "crop": {"x": crop[0], "y": crop[1], "width": crop[2], "height": crop[3]},
        "measurementPlane": "ARENA_FLOOR",
        "transform": [float(v) for v in transform.reshape(9)],
    }


Mask = Callable[[np.ndarray, np.ndarray], np.ndarray]
# effects(frame, t_s, rng) changes a frame in place after the subject is drawn.
Effects = Callable[[np.ndarray, float, np.random.Generator], None]


@dataclass(frozen=True)
class Scene:
    cm_to_px: np.ndarray
    # floor(rows, cols) marks the pixels of the surface the subject moves on.
    floor: Mask | None = None
    surround_level: float = 120.0
    # cover(rows, cols) marks pixels painted cover_level over the subject.
    cover: Mask | None = None
    cover_level: float = 30.0
    effects: Effects | None = None


def polygon_mask(points_px: Sequence[tuple[float, float]]) -> Mask:
    """Pixels whose centers are inside a convex polygon listed clockwise on screen."""

    def mask(rows: np.ndarray, cols: np.ndarray) -> np.ndarray:
        inside = np.ones(rows.shape, dtype=bool)
        n = len(points_px)
        for i in range(n):
            (x0, y0), (x1, y1) = points_px[i], points_px[(i + 1) % n]
            inside &= (x1 - x0) * (rows + 0.5 - y0) - (y1 - y0) * (cols + 0.5 - x0) >= 0
        return inside

    return mask


@dataclass(frozen=True)
class Truth:
    t_us: int
    x: float
    y: float
    visible: bool


Path2D = Callable[[float], tuple[float, float, bool]]


def walk_with_pauses(t: float) -> tuple[float, float, bool]:
    """0-2 s east along y=10, 2-3.5 s still, 3.5-5 s north-west to the center, then still."""
    if t < 2.0:
        return 10 + 15 * t, 10.0, True
    if t < 3.5:
        return 40.0, 10.0, True
    if t < 5.0:
        f = (t - 3.5) / 1.5
        return 40 - 15 * f, 10 + 15 * f, True
    return 25.0, 25.0, True


def render(
    path: Path,
    times_us: Sequence[int],
    trajectory: Path2D = walk_with_pauses,
    *,
    subject_level: int = 45,
    floor_level: int = 185,
    noise: float = 2.0,
    radius_cm: float = 2.2,
    seed: int = 7,
    scene: Scene | None = None,
) -> list[Truth]:
    scene = scene or Scene(CM_TO_PX, polygon_mask(CORNERS_PX))
    rng = np.random.default_rng(seed)
    rows, cols = np.mgrid[0:HEIGHT, 0:WIDTH]
    texture = rng.normal(0, 4, (HEIGHT, WIDTH))
    floor = np.full((HEIGHT, WIDTH), scene.surround_level) + texture
    # Pixels of the floor get the floor level, the surroundings stay darker.
    inside = scene.floor(rows, cols) if scene.floor else np.ones((HEIGHT, WIDTH), dtype=bool)
    floor[inside] = floor_level + texture[inside]
    cover = scene.cover(rows, cols) if scene.cover else None

    container = av.open(str(path), mode="w", format="mp4")
    stream = container.add_stream("libx264", rate=25)
    stream.width, stream.height, stream.pix_fmt = WIDTH, HEIGHT, "yuv420p"
    stream.time_base = stream.codec_context.time_base = Fraction(1, 1_000_000)
    stream.options = {"preset": "veryfast", "crf": "12"}
    truth = []
    for t in times_us:
        x, y, visible = trajectory(t / 1_000_000)
        frame = floor + rng.normal(0, noise, (HEIGHT, WIDTH))
        if visible:
            cx, cy = project(x, y, scene.cm_to_px)
            ex, ey = project(x + radius_cm, y, scene.cm_to_px)
            radius_px = math.hypot(ex - cx, ey - cy)
            coverage = _disc_coverage(cx, cy, radius_px)
            frame = frame * (1 - coverage) + subject_level * coverage
        if cover is not None:
            frame[cover] = scene.cover_level + texture[cover]
        if scene.effects:
            scene.effects(frame, t / 1_000_000, rng)
        truth.append(Truth(int(t), x, y, visible))
        gray = np.clip(frame, 0, 255).astype(np.uint8)
        image = av.VideoFrame.from_ndarray(np.dstack([gray, gray, gray]), format="rgb24")
        image.pts, image.time_base = int(t), Fraction(1, 1_000_000)
        for packet in stream.encode(image):
            container.mux(packet)
    for packet in stream.encode():
        container.mux(packet)
    container.close()
    return truth


def _disc_coverage(cx: float, cy: float, radius: float, samples: int = 4) -> np.ndarray:
    coverage = np.zeros((HEIGHT, WIDTH))
    x0, x1 = max(int(cx - radius) - 1, 0), min(int(cx + radius) + 2, WIDTH)
    y0, y1 = max(int(cy - radius) - 1, 0), min(int(cy + radius) + 2, HEIGHT)
    offsets = (np.arange(samples) + 0.5) / samples
    rows, cols = np.mgrid[y0:y1, x0:x1]
    hits = np.zeros(rows.shape)
    for oy in offsets:
        for ox in offsets:
            hits += (cols + ox - cx) ** 2 + (rows + oy - cy) ** 2 <= radius**2
    coverage[y0:y1, x0:x1] = hits / samples**2
    return coverage


def regular(duration_s: float, fps: float = 25.0) -> list[int]:
    step = 1_000_000 / fps
    return [int(round(i * step)) for i in range(int(duration_s * fps))]


def job(run_id: str = "01a00000-0000-7000-8000-0000000000aa", attempt: int = 1, clip_start_us: int = 0, clip_end_us: int | None = 6_000_000) -> dict:
    """An OPEN_FIELD job; other paradigms replace its run's paradigm, parameters and calibration."""
    return {
        "run": {
            "id": run_id,
            "attempt": attempt,
            "testSubjectId": "01a00000-0000-7000-8000-0000000000b1",
            "clipStartUs": clip_start_us,
            "clipEndUs": clip_end_us,
            "paradigmKey": "OPEN_FIELD",
            "paradigmVersion": 1,
            "parameters": {"arena_width_cm": ARENA, "arena_height_cm": ARENA, "center_fraction": 0.5},
        },
        "testSubject": {"id": "01a00000-0000-7000-8000-0000000000b1", "environmentRevisionId": "01a00000-0000-7000-8000-0000000000b2"},
        "outputPrefix": f"runs/{run_id}/attempts/{attempt}/",
        "trajectorySchema": "misko.trajectory.v1",
        "calibration": calibration(),
    }
