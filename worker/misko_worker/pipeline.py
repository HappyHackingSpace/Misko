"""Turns a source video and a claimed job into a trajectory and an annotated video."""

from __future__ import annotations

import json
from dataclasses import dataclass, field, replace
from pathlib import Path
from typing import Any

from . import ALGORITHM_VERSION, TRAJECTORY_SCHEMA, video
from .geometry import to_arena, to_pixels
from .overlay import Overlay
from .paradigms import SUPPORTED, Layout, layout
from .tracking import Background, TrackerSettings, detect, luminance

# Water: a lower base threshold plus each pixel's noise (see tracking).
WATER_THRESHOLD = 18
WATER_NOISE_FACTOR = 4.0
# Opaque water gives less contrast than a lit floor; this many grey levels
# above the background count as full contrast there.
WATER_FULL_CONTRAST = 50.0


class NonRetryable(Exception):
    """The job cannot succeed with another attempt. The message is the failure reason."""


@dataclass(frozen=True)
class Sample:
    t_us: int
    x_cm: float | None
    y_cm: float | None
    tracked: bool
    confidence: float
    px: float | None = None
    py: float | None = None
    # Not seen but placed by inference. No supported paradigm infers positions
    # today; the flag stays part of the trajectory format.
    inferred: bool = False


@dataclass
class Analysis:
    duration_us: int
    samples: list[Sample] = field(default_factory=list)
    # The settings actually used (tuned per apparatus) and the subject polarity.
    settings: TrackerSettings | None = None
    polarity: str | None = None

    @property
    def lost_ratio(self) -> float:
        if not self.samples:
            return 1.0
        return sum(not s.tracked for s in self.samples) / len(self.samples)

    @property
    def inferred_ratio(self) -> float:
        if not self.samples:
            return 0.0
        return sum(s.inferred for s in self.samples) / len(self.samples)

    @property
    def mean_confidence(self) -> float:
        """Mean confidence of the samples actually seen."""
        observed = [s.confidence for s in self.samples if s.tracked and not s.inferred]
        return sum(observed) / len(observed) if observed else 0.0


def check_supported(job: dict[str, Any]) -> None:
    """Rejects a job this worker cannot measure, before anything is downloaded."""
    run = job["run"]
    key, version = run.get("paradigmKey"), run.get("paradigmVersion")
    if (key, version) not in SUPPORTED:
        supported = ", ".join(f"{k} v{v}" for k, v in sorted(SUPPORTED))
        raise NonRetryable(f"UNSUPPORTED_PARADIGM: {key} v{version}; this worker analyzes {supported}")


def settings_for(apparatus: Layout, settings: TrackerSettings) -> TrackerSettings:
    """Tracker settings tuned for the apparatus. Water gets a noise-aware
    threshold and a median background: a swimming animal never rests long at
    one spot, while a high percentile would take a shimmering reflection as
    the background and turn the water around it into foreground."""
    if apparatus.water and settings.noise_factor == 0:
        return replace(
            settings, threshold=min(settings.threshold, WATER_THRESHOLD), noise_factor=WATER_NOISE_FACTOR, background_percentile=50.0,
            full_contrast=min(settings.full_contrast, WATER_FULL_CONTRAST),
        )
    return settings


def analyze(source: Path, job: dict[str, Any], overlay_path: Path, settings: TrackerSettings = TrackerSettings()) -> Analysis:
    """Measures the one animal of the job inside its arena's crop of the shared video."""
    check_supported(job)
    run, calibration = job["run"], job.get("calibration")
    if calibration is None:
        raise NonRetryable("MISSING_CALIBRATION: this worker needs a pixel to centimeter calibration")
    info = video.probe(source)
    if (info.width, info.height) != (calibration["frameWidth"], calibration["frameHeight"]):
        raise NonRetryable(
            f"FRAME_SIZE_MISMATCH: video {info.width}x{info.height}, calibration {calibration['frameWidth']}x{calibration['frameHeight']}"
        )
    start, end = int(run["clipStartUs"]), run["clipEndUs"]
    if end is None:
        if info.duration_us is None:
            raise NonRetryable("UNKNOWN_DURATION: the clip has no end and the video reports no duration")
        duration = info.duration_us - start
    else:
        duration = int(end) - start
    if duration <= 0:
        raise NonRetryable("CLIP_OUTSIDE_VIDEO")
    crop = calibration["crop"]
    region = (int(crop["x"]), int(crop["y"]), int(crop["width"]), int(crop["height"]))
    transform = [float(v) for v in calibration["transform"]]
    apparatus = layout(job)
    settings = settings_for(apparatus, settings)

    background = Background(settings.background_frames)
    for _, rgb in video.frames(source, start, end):
        background.add(luminance(rgb))
    try:
        model = background.model(settings, region, apparatus.detection_mask(transform, region))
    except ValueError:
        raise NonRetryable("NO_FRAMES_IN_CLIP") from None

    analysis = Analysis(duration_us=duration, settings=settings, polarity=model.polarity)
    previous: tuple[float, float] | None = None
    for t, rgb in video.frames(source, start, end):
        # The recording is the half-open clip [start, end).
        if t >= duration:
            break
        found = detect(luminance(rgb), model, region, settings, previous)
        sample = Sample(t, None, None, False, found.confidence, found.px, found.py)
        if found.px is not None:
            previous = (found.px, found.py)
            x, y = (float(v) for v in to_arena(transform, found.px, found.py))
            # The engine treats positions outside the apparatus as lost; so does QC.
            sample = Sample(t, x, y, apparatus.inside(x, y), float(found.confidence), float(found.px), float(found.py))
        analysis.samples.append(sample)

    # The overlay is drawn in a second pass, once every sample is settled.
    overlay = Overlay(overlay_path, info.width, info.height, apparatus.drawings(transform))
    try:
        samples = iter(analysis.samples)
        for t, rgb in video.frames(source, start, end):
            if t >= duration:
                break
            sample = next(samples)
            position = None
            if sample.inferred:
                position = to_pixels(transform, sample.x_cm, sample.y_cm)
            elif sample.tracked:
                position = (sample.px, sample.py)
            overlay.write(t, rgb, position, sample.inferred)
    finally:
        overlay.close()
    return analysis


def trajectory_document(job: dict[str, Any], analysis: Analysis, settings: TrackerSettings = TrackerSettings()) -> bytes:
    """misko.trajectory.v1 JSON. Everything under "worker" is diagnostic; the API reads only the samples."""
    run = job["run"]
    samples = analysis.samples

    def rounded(value: float | None, digits: int) -> float | None:
        return None if value is None else round(value, digits)

    try:
        frame = layout(job).coordinate_frame
    except ValueError:
        frame = "CALIBRATION_CM"
    document = {
        "schema": TRAJECTORY_SCHEMA,
        "runId": run["id"],
        "attempt": run["attempt"],
        "durationUs": analysis.duration_us,
        "coordinateFrame": frame,
        "samples": {
            "tUs": [s.t_us for s in samples],
            "xCm": [rounded(s.x_cm, 4) if s.tracked else None for s in samples],
            "yCm": [rounded(s.y_cm, 4) if s.tracked else None for s in samples],
            "tracked": [s.tracked for s in samples],
            "confidence": [round(s.confidence, 4) for s in samples],
            # Optional: inferred samples are tracked with a position that was not seen.
            "inferred": [s.inferred for s in samples],
        },
        "worker": {
            "algorithm": ALGORITHM_VERSION,
            "settings": (analysis.settings or settings).__dict__,
            "polarity": analysis.polarity,
            "calibrationId": (job.get("calibration") or {}).get("id"),
            "testSubjectId": (job.get("testSubject") or {}).get("id"),
            "pixels": {"x": [rounded(s.px, 2) for s in samples], "y": [rounded(s.py, 2) for s in samples]},
            "paradigm": {"key": run.get("paradigmKey"), "version": run.get("paradigmVersion")},
            "qc": {
                "lostRatio": round(analysis.lost_ratio, 6),
                "inferredRatio": round(analysis.inferred_ratio, 6),
                "meanTrackedConfidence": round(analysis.mean_confidence, 6),
            },
        },
    }
    return json.dumps(document, separators=(",", ":"), allow_nan=False).encode()
