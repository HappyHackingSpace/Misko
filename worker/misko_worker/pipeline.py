"""Turns a source video and a claimed job into a trajectory and an annotated video."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from . import ALGORITHM_VERSION, TRAJECTORY_SCHEMA, video
from .geometry import to_arena
from .overlay import Overlay
from .tracking import Background, TrackerSettings, detect, luminance


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


@dataclass
class Analysis:
    duration_us: int
    samples: list[Sample] = field(default_factory=list)

    @property
    def lost_ratio(self) -> float:
        if not self.samples:
            return 1.0
        return sum(not s.tracked for s in self.samples) / len(self.samples)

    @property
    def mean_confidence(self) -> float:
        tracked = [s.confidence for s in self.samples if s.tracked]
        return sum(tracked) / len(tracked) if tracked else 0.0


def analyze(source: Path, job: dict[str, Any], overlay_path: Path, settings: TrackerSettings = TrackerSettings()) -> Analysis:
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
    parameters = run.get("parameters") or {}
    width, height = parameters.get("arena_width_cm", 50.0), parameters.get("arena_height_cm", 50.0)

    background = Background(settings.background_frames)
    for _, rgb in video.frames(source, start, end):
        background.add(luminance(rgb))
    try:
        model, polarity = background.model(settings, region)
    except ValueError:
        raise NonRetryable("NO_FRAMES_IN_CLIP") from None

    analysis = Analysis(duration_us=duration)
    overlay = Overlay(overlay_path, info.width, info.height, transform, parameters)
    try:
        for t, rgb in video.frames(source, start, end):
            # The recording is the half-open clip [start, end).
            if t >= duration:
                break
            found = detect(luminance(rgb), model, region, settings, polarity)
            sample = Sample(t, None, None, False, found.confidence, found.px, found.py)
            if found.px is not None:
                x, y = (float(v) for v in to_arena(transform, found.px, found.py))
                # The engine treats positions outside the arena as lost; so does QC.
                inside = bool(0 <= x <= width and 0 <= y <= height)
                sample = Sample(t, x, y, inside, float(found.confidence), float(found.px), float(found.py))
            analysis.samples.append(sample)
            overlay.write(t, rgb, (found.px, found.py) if sample.tracked else None)
    finally:
        overlay.close()
    return analysis


def trajectory_document(job: dict[str, Any], analysis: Analysis, settings: TrackerSettings = TrackerSettings()) -> bytes:
    """misko.trajectory.v1 JSON. Everything under "worker" is diagnostic; the API reads only the samples."""
    run = job["run"]
    samples = analysis.samples

    def rounded(value: float | None, digits: int) -> float | None:
        return None if value is None else round(value, digits)

    document = {
        "schema": TRAJECTORY_SCHEMA,
        "runId": run["id"],
        "attempt": run["attempt"],
        "durationUs": analysis.duration_us,
        "coordinateFrame": "ARENA_CM",
        "samples": {
            "tUs": [s.t_us for s in samples],
            "xCm": [rounded(s.x_cm, 4) if s.tracked else None for s in samples],
            "yCm": [rounded(s.y_cm, 4) if s.tracked else None for s in samples],
            "tracked": [s.tracked for s in samples],
            "confidence": [round(s.confidence, 4) for s in samples],
        },
        "worker": {
            "algorithm": ALGORITHM_VERSION,
            "settings": settings.__dict__,
            "calibrationId": (job.get("calibration") or {}).get("id"),
            "pixels": {"x": [rounded(s.px, 2) for s in samples], "y": [rounded(s.py, 2) for s in samples]},
            "qc": {"lostRatio": round(analysis.lost_ratio, 6), "meanTrackedConfidence": round(analysis.mean_confidence, 6)},
        },
    }
    return json.dumps(document, separators=(",", ":"), allow_nan=False).encode()
