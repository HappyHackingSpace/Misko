"""Synthetic Open Field recordings with known positions.

A 50 x 50 cm arena is seen by a tilted camera: a homography maps arena
centimeters to a 320 x 240 frame. The subject is a dark disc drawn with 4x
supersampling around the projected position, over a textured floor with
per-frame sensor noise. These fixtures test the pipeline's geometry, timing
and failure handling; they are not evidence of accuracy on real animals.
"""

from __future__ import annotations

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


def project(x: float, y: float) -> tuple[float, float]:
    u, v, w = CM_TO_PX @ np.array([x, y, 1.0])
    return u / w, v / w


def calibration(frame_width: int = WIDTH, frame_height: int = HEIGHT) -> dict:
    return {
        "id": "01a00000-0000-7000-8000-00000000ca11",
        "frameWidth": frame_width,
        "frameHeight": frame_height,
        "crop": {"x": 20, "y": 16, "width": 290, "height": 216},
        "measurementPlane": "ARENA_FLOOR",
        "transform": [float(v) for v in PX_TO_CM.reshape(9)],
    }


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
) -> list[Truth]:
    rng = np.random.default_rng(seed)
    rows, cols = np.mgrid[0:HEIGHT, 0:WIDTH]
    texture = rng.normal(0, 4, (HEIGHT, WIDTH))
    floor = np.full((HEIGHT, WIDTH), 120.0) + texture
    # Pixels inside the arena get the floor level, outside stay darker.
    inside = np.zeros((HEIGHT, WIDTH), dtype=bool)
    pts = np.array(CORNERS_PX)
    for i in range(4):
        (x0, y0), (x1, y1) = pts[i], pts[(i + 1) % 4]
        edge = (x1 - x0) * (rows + 0.5 - y0) - (y1 - y0) * (cols + 0.5 - x0)
        inside = edge >= 0 if i == 0 else inside & (edge >= 0)
    floor[inside] = floor_level + texture[inside]

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
            cx, cy = project(x, y)
            ex, _ = project(x + radius_cm, y)
            radius_px = abs(ex - cx)
            coverage = _disc_coverage(cx, cy, radius_px)
            frame = frame * (1 - coverage) + subject_level * coverage
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
    return {
        "run": {
            "id": run_id,
            "attempt": attempt,
            "clipStartUs": clip_start_us,
            "clipEndUs": clip_end_us,
            "paradigmKey": "OPEN_FIELD",
            "paradigmVersion": 1,
            "parameters": {"arena_width_cm": ARENA, "arena_height_cm": ARENA, "center_fraction": 0.5},
        },
        "outputPrefix": f"runs/{run_id}/attempts/{attempt}/",
        "trajectorySchema": "misko.trajectory.v1",
        "calibration": calibration(),
    }
