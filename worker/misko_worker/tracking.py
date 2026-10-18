"""Single-subject detection by background subtraction.

The background is a per-pixel percentile of frames sampled evenly over the
clip: for a dark subject on a light floor the 90th percentile, so the floor
stays in the background even where the subject rests for up to about 90% of
the clip. Only pixels darker than the background (lighter for a light subject)
are foreground, so the spot a resting subject leaves behind is not detected. It
assumes a fixed camera, stable lighting and one subject.

Water (MWM) adds ripples, moving reflections and low contrast. There the
foreground threshold grows with each pixel's own temporal noise (a robust
standard deviation over the sampled frames), so rippling or glinting pixels need
a larger difference while calm water keeps a low threshold; detection only
looks at pixels near the apparatus (a mask from the paradigm layout); and among
several blobs the one near the previous position wins over a larger one far
away, so a reflection does not pull the track across the tank. A treadmill
belt uses the same noise-aware threshold: its moving texture is noise that a
pixel the belt covers always shows. Pixel coordinates are
continuous, with the top-left corner of the frame at (0, 0); a pixel's center is
at (column + 0.5, row + 0.5), the convention calibration points use.
"""

from __future__ import annotations

import math
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
    # Per-pixel threshold = threshold + noise_factor x the pixel's robust
    # temporal standard deviation; 0 keeps the fixed threshold.
    noise_factor: float = 0.0
    # A blob within this share of the crop diagonal from the previous position
    # is preferred over larger blobs farther away.
    max_jump_fraction: float = 0.15


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

    def model(self, settings: TrackerSettings, crop: tuple[int, int, int, int], mask: np.ndarray | None = None) -> Model:
        """Returns the background, the subject polarity ("dark" or "light") and
        the per-pixel threshold of the crop."""
        if not self._frames:
            raise ValueError("no frames")
        stack = np.stack(self._frames)
        x0, y0, width, height = crop
        region = stack[:, y0 : y0 + height, x0 : x0 + width]
        threshold: np.ndarray | float = float(settings.threshold)
        if settings.noise_factor > 0:
            median = np.median(region, axis=0)
            sigma = 1.4826 * np.median(np.abs(region - median), axis=0)
            threshold = settings.threshold + settings.noise_factor * sigma
        if settings.subject in ("dark", "light"):
            polarity = settings.subject
        else:
            polarity = self.polarity(stack, crop, threshold, mask)
        percentile = settings.background_percentile if polarity == "dark" else 100 - settings.background_percentile
        return Model(np.percentile(stack, percentile, axis=0).astype(np.int16), polarity, threshold, mask)

    @staticmethod
    def polarity(stack: np.ndarray, crop: tuple[int, int, int, int], threshold: np.ndarray | float, mask: np.ndarray | None = None) -> str:
        """A moving subject leaves pixels that were briefly much darker (or
        lighter) than their median; the path is far larger than any resting
        spot, so the side with more such pixels is the subject's. Isolated
        pixels (sensor noise, small glints) are opened away first. Ties, such
        as a subject that never moves, count as dark."""
        x0, y0, width, height = crop
        region = stack[:, y0 : y0 + height, x0 : x0 + width]
        median = np.median(region, axis=0)
        opening = np.ones((3, 3), dtype=bool)
        dark_pixels = ndimage.binary_opening(median - region.min(axis=0) > threshold, structure=opening)
        light_pixels = ndimage.binary_opening(region.max(axis=0) - median > threshold, structure=opening)
        if mask is not None:
            dark_pixels, light_pixels = dark_pixels & mask, light_pixels & mask
        dark, light = int(np.count_nonzero(dark_pixels)), int(np.count_nonzero(light_pixels))
        return "dark" if dark >= light else "light"


@dataclass(frozen=True)
class Model:
    """The background of the full frame and, for the crop, the threshold
    (a number or a per-pixel array) and the pixels detection may use."""

    background: np.ndarray
    polarity: str
    threshold: np.ndarray | float
    mask: np.ndarray | None = None


def detect(
    gray: np.ndarray,
    model: Model,
    crop: tuple[int, int, int, int],
    settings: TrackerSettings,
    previous: tuple[float, float] | None = None,
) -> Detection:
    """Finds the subject's blob darker (or lighter) than the background inside
    crop (x, y, width, height): the largest one, or the largest one near the
    previous position (full-frame pixels) when there is one."""
    x0, y0, width, height = crop
    diff = model.background[y0 : y0 + height, x0 : x0 + width] - gray[y0 : y0 + height, x0 : x0 + width]
    if model.polarity == "light":
        diff = -diff
    diff = np.clip(diff, 0, None)
    foreground = diff > model.threshold
    if model.mask is not None:
        foreground &= model.mask
    mask = ndimage.binary_opening(foreground, structure=np.ones((3, 3), dtype=bool))
    labels, count = ndimage.label(mask)
    if count == 0:
        return Detection(None, None, 0.0, 0)
    index = np.arange(1, count + 1)
    areas = ndimage.sum_labels(mask, labels, index=index)
    plausible = (areas >= settings.min_area_px) & (areas <= settings.max_area_fraction * mask.size)
    best = int(np.argmax(areas)) + 1
    if previous is not None and np.count_nonzero(plausible) > 1:
        centers = np.array(ndimage.center_of_mass(mask, labels, index))
        distance = np.hypot(centers[:, 1] + x0 + 0.5 - previous[0], centers[:, 0] + y0 + 0.5 - previous[1])
        near = plausible & (distance <= settings.max_jump_fraction * math.hypot(width, height))
        if near.any():
            best = int(np.argmax(np.where(near, areas, -1))) + 1
    area = int(areas[best - 1])
    if not plausible[best - 1]:
        return Detection(None, None, 0.0, area)
    blob = labels == best
    weights = np.where(blob, diff, 0).astype(np.float64)
    row, column = ndimage.center_of_mass(weights)
    contrast = float(diff[blob].mean())
    confidence = min(1.0, contrast / settings.full_contrast) * (area / float(areas.sum()))
    return Detection(x0 + column + 0.5, y0 + row + 0.5, confidence, area)
