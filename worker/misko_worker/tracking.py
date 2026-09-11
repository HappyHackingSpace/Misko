"""Single-subject detection by background subtraction.

The background is a per-pixel percentile of frames sampled evenly over the
clip: for a dark subject on a light floor the 90th percentile, so the floor
stays in the background even where the subject rests for up to about 90% of
the clip. Only pixels darker than the background (lighter for a light subject)
are foreground, so the spot a resting subject leaves behind is not detected. It
assumes a fixed camera, stable lighting and one subject. Pixel coordinates are
continuous, with the top-left corner of the frame at (0, 0); a pixel's center is
at (column + 0.5, row + 0.5), the convention calibration points use.
"""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np
from scipy import ndimage

_LUMA = np.array([0.299, 0.587, 0.114])


@dataclass(frozen=True)
class TrackerSettings:
    # "dark" for a subject darker than the floor, "light" for a lighter one, or
    # "auto" to decide from the sampled frames (see Background.polarity).
    subject: str = "auto"
    # Frames kept for the background and the percentile of a light floor.
    background_frames: int = 31
    background_percentile: float = 90.0
    # Luminance difference toward the subject that marks a foreground pixel.
    threshold: int = 30
    # Blobs smaller than this, or covering more than max_area_fraction of the
    # crop, are not the subject.
    min_area_px: int = 12
    max_area_fraction: float = 0.2
    # Mean difference inside the blob that counts as full contrast.
    full_contrast: float = 80.0


@dataclass(frozen=True)
class Detection:
    """A subject position in full-frame pixels, or None when nothing was found.

    Confidence is the blob's contrast (mean difference / full_contrast, at most
    1) times its share of all foreground pixels, so faint subjects and competing
    blobs both lower it.
    """

    px: float | None
    py: float | None
    confidence: float
    area: int


def luminance(rgb: np.ndarray) -> np.ndarray:
    return (rgb @ _LUMA).astype(np.int16)


class Background:
    """Keeps every stride-th frame and doubles the stride when the buffer
    exceeds twice the capacity, so memory stays bounded for any clip length."""

    def __init__(self, capacity: int) -> None:
        self._capacity, self._stride, self._seen = capacity, 1, 0
        self._frames: list[np.ndarray] = []

    def add(self, gray: np.ndarray) -> None:
        if self._seen % self._stride == 0:
            self._frames.append(gray)
            if len(self._frames) > 2 * self._capacity:
                self._frames, self._stride = self._frames[::2], self._stride * 2
        self._seen += 1

    def model(self, settings: TrackerSettings, crop: tuple[int, int, int, int]) -> tuple[np.ndarray, str]:
        """Returns the background and the subject polarity, "dark" or "light"."""
        if not self._frames:
            raise ValueError("no frames")
        stack = np.stack(self._frames)
        polarity = settings.subject if settings.subject in ("dark", "light") else self.polarity(stack, crop, settings.threshold)
        percentile = settings.background_percentile if polarity == "dark" else 100 - settings.background_percentile
        return np.percentile(stack, percentile, axis=0).astype(np.int16), polarity

    @staticmethod
    def polarity(stack: np.ndarray, crop: tuple[int, int, int, int], threshold: int) -> str:
        """A moving subject leaves pixels that were briefly much darker (or
        lighter) than their median; the path is far larger than any resting
        spot, so the side with more such pixels is the subject's. Ties, such as
        a subject that never moves, count as dark."""
        x0, y0, width, height = crop
        region = stack[:, y0 : y0 + height, x0 : x0 + width]
        median = np.median(region, axis=0)
        dark = int(np.count_nonzero(median - region.min(axis=0) > threshold))
        light = int(np.count_nonzero(region.max(axis=0) - median > threshold))
        return "dark" if dark >= light else "light"


def detect(gray: np.ndarray, background: np.ndarray, crop: tuple[int, int, int, int], settings: TrackerSettings, polarity: str = "dark") -> Detection:
    """Finds the largest blob darker (or lighter) than the background inside crop (x, y, width, height)."""
    x0, y0, width, height = crop
    diff = background[y0 : y0 + height, x0 : x0 + width] - gray[y0 : y0 + height, x0 : x0 + width]
    if polarity == "light":
        diff = -diff
    diff = np.clip(diff, 0, None)
    mask = ndimage.binary_opening(diff > settings.threshold, structure=np.ones((3, 3), dtype=bool))
    labels, count = ndimage.label(mask)
    if count == 0:
        return Detection(None, None, 0.0, 0)
    areas = ndimage.sum_labels(mask, labels, index=np.arange(1, count + 1))
    best = int(np.argmax(areas)) + 1
    area = int(areas[best - 1])
    if area < settings.min_area_px or area > settings.max_area_fraction * mask.size:
        return Detection(None, None, 0.0, area)
    blob = labels == best
    weights = np.where(blob, diff, 0).astype(np.float64)
    row, column = ndimage.center_of_mass(weights)
    contrast = float(diff[blob].mean())
    confidence = min(1.0, contrast / settings.full_contrast) * (area / float(areas.sum()))
    return Detection(x0 + column + 0.5, y0 + row + 0.5, confidence, area)
