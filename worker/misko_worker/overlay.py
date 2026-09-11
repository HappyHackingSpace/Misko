"""Annotated video: the source frames with the center zone, the recent path and
the tracked position drawn on them, encoded as H.264 MP4.

Each output frame keeps its recording time as its presentation timestamp, so
the output timeline equals the recording timeline (identity-v1): output time =
recording time, source time = clip start + recording time.
"""

from __future__ import annotations

from collections import deque
from collections.abc import Sequence
from fractions import Fraction
from pathlib import Path

import av
import numpy as np

from .geometry import to_pixels

MICROSECOND = Fraction(1, 1_000_000)
ZONE = (255, 215, 0)
PATH = (0, 200, 255)
MARKER = (0, 230, 0)
LOST = (230, 0, 0)
TRAIL = 45


class Overlay:
    def __init__(self, path: Path, width: int, height: int, transform: Sequence[float], parameters: dict[str, float]) -> None:
        self._width, self._height = width, height
        self._container = av.open(str(path), mode="w", format="mp4", options={"movflags": "faststart"})
        self._stream = self._container.add_stream("libx264", rate=30)
        # H.264 in 4:2:0 needs even dimensions; the right and bottom edge are padded.
        self._stream.width, self._stream.height = width + width % 2, height + height % 2
        self._stream.pix_fmt = "yuv420p"
        self._stream.time_base = MICROSECOND
        self._stream.codec_context.time_base = MICROSECOND
        self._stream.options = {"preset": "veryfast", "crf": "20"}
        side = parameters.get("center_fraction", 0.5)
        arena_w, arena_h = parameters.get("arena_width_cm", 50.0), parameters.get("arena_height_cm", 50.0)
        left, top = arena_w * (1 - side) / 2, arena_h * (1 - side) / 2
        corners = [(left, top), (arena_w - left, top), (arena_w - left, arena_h - top), (left, arena_h - top)]
        self._zone = [to_pixels(transform, x, y) for x, y in corners]
        self._arena = [to_pixels(transform, x, y) for x, y in [(0, 0), (arena_w, 0), (arena_w, arena_h), (0, arena_h)]]
        self._trail: deque[tuple[float, float]] = deque(maxlen=TRAIL)

    def write(self, t_us: int, rgb: np.ndarray, position: tuple[float, float] | None) -> None:
        image = np.zeros((self._stream.height, self._stream.width, 3), dtype=np.uint8)
        image[: self._height, : self._width] = rgb
        _polygon(image, self._arena, ZONE, 1)
        _polygon(image, self._zone, ZONE, 2)
        if position is None:
            _border(image, LOST, 4)
        else:
            self._trail.append(position)
        points = list(self._trail)
        for a, b in zip(points, points[1:]):
            _line(image, a, b, PATH, 2)
        if position is not None:
            _disc(image, position, 5, MARKER)
        frame = av.VideoFrame.from_ndarray(image, format="rgb24")
        frame.pts, frame.time_base = t_us, MICROSECOND
        for packet in self._stream.encode(frame):
            self._container.mux(packet)

    def close(self) -> None:
        for packet in self._stream.encode():
            self._container.mux(packet)
        self._container.close()


def _line(image: np.ndarray, a: tuple[float, float], b: tuple[float, float], color: tuple[int, int, int], thickness: int) -> None:
    steps = int(max(abs(b[0] - a[0]), abs(b[1] - a[1]))) + 1
    if steps > 4 * max(image.shape):
        return
    xs = np.linspace(a[0], b[0], steps)
    ys = np.linspace(a[1], b[1], steps)
    half = thickness // 2
    for dx in range(-half, thickness - half):
        for dy in range(-half, thickness - half):
            cols = np.floor(xs).astype(int) + dx
            rows = np.floor(ys).astype(int) + dy
            keep = (cols >= 0) & (cols < image.shape[1]) & (rows >= 0) & (rows < image.shape[0])
            image[rows[keep], cols[keep]] = color


def _polygon(image: np.ndarray, points: list[tuple[float, float]], color: tuple[int, int, int], thickness: int) -> None:
    for a, b in zip(points, points[1:] + points[:1]):
        _line(image, a, b, color, thickness)


def _disc(image: np.ndarray, center: tuple[float, float], radius: int, color: tuple[int, int, int]) -> None:
    cx, cy = center
    x0, x1 = max(int(cx) - radius, 0), min(int(cx) + radius + 1, image.shape[1])
    y0, y1 = max(int(cy) - radius, 0), min(int(cy) + radius + 1, image.shape[0])
    if x0 >= x1 or y0 >= y1:
        return
    rows, cols = np.mgrid[y0:y1, x0:x1]
    inside = (cols + 0.5 - cx) ** 2 + (rows + 0.5 - cy) ** 2 <= radius**2
    image[rows[inside], cols[inside]] = color


def _border(image: np.ndarray, color: tuple[int, int, int], width: int) -> None:
    image[:width], image[-width:], image[:, :width], image[:, -width:] = color, color, color, color
