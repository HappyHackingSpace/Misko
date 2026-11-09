"""Tracking jitter on a stretch where the subject is still (issue #242).

Replays the worker's tracker over one recording and reports, for a stretch
where the subject is visibly still, what the pipeline would count as movement:
the distribution of step lengths, the apparent path per minute and speed, and
how often the position flips in or out of each zone. Three centroid methods
are compared on the same detections:

- weighted: the contrast-weighted blob centroid the worker publishes;
- plain: the unweighted centroid of the thresholded blob;
- median3: the per-axis median of the last three weighted positions.

The background, the polarity and the tracker settings come from the whole clip
exactly as in an analysis, so a subject that rests for the whole stretch still
stands out. The input is the job as GET /api/analysis/jobs/{jobId} returns it
(or a worker claim), so the recording, clip and calibration are named by the
job, and the report can be repeated.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from collections.abc import Sequence
from pathlib import Path
from typing import Any

import numpy as np

from . import video
from .geometry import Shape, to_arena
from .paradigms import layout
from .pipeline import settings_for
from .tracking import Background, TrackerSettings, detect, luminance

METHODS = ("weighted", "plain", "median3")
PERCENTILES = (50, 90, 99)


def claim_shape(job: dict[str, Any]) -> dict[str, Any]:
    """A job read from the API carries the run's fields at the top level; the
    worker's claim nests them under run."""
    return job if "run" in job else {"run": job, "calibration": job.get("calibration")}


def zones_of(job: dict[str, Any]) -> list[tuple[str, Shape]]:
    apparatus = layout(job)
    zones = [(key, shape) for key, shape, _ in apparatus.zones_cm]
    return zones + [(z.key, z.cm) for z in getattr(apparatus, "zones", [])]


def positions(source: Path, job: dict[str, Any], still_start_us: int, still_end_us: int, settings: TrackerSettings = TrackerSettings()):
    """Times and positions in cm per method for every frame of the still
    stretch (times from the clip start), None where nothing was detected."""
    run, calibration = job["run"], job["calibration"]
    start, end = int(run["clipStartUs"]), run["clipEndUs"]
    crop = calibration["crop"]
    region = (int(crop["x"]), int(crop["y"]), int(crop["width"]), int(crop["height"]))
    transform = [float(v) for v in calibration["transform"]]
    apparatus = layout(job)
    settings = settings_for(apparatus, settings)
    background = Background(settings.background_frames)
    for _, rgb in video.frames(source, start, end):
        background.add(luminance(rgb))
    model = background.model(settings, region, apparatus.detection_mask(transform, region))

    times: list[int] = []
    tracks: dict[str, list[tuple[float, float] | None]] = {m: [] for m in METHODS}
    recent: list[tuple[float, float]] = []
    previous = None
    for t, rgb in video.frames(source, start, end):
        if t >= still_end_us:
            break
        found = detect(luminance(rgb), model, region, settings, previous)
        if found.px is not None:
            previous = (found.px, found.py)
        if t < still_start_us:
            continue
        times.append(t)
        if found.px is None:
            for m in METHODS:
                tracks[m].append(None)
            recent.clear()
            continue
        weighted = to_arena(transform, found.px, found.py)
        recent = (recent + [weighted])[-3:]
        tracks["weighted"].append(weighted)
        tracks["plain"].append(to_arena(transform, *found.plain))
        tracks["median3"].append((float(np.median([p[0] for p in recent])), float(np.median([p[1] for p in recent]))))
    return times, tracks


def summarize(times: Sequence[int], track: Sequence[tuple[float, float] | None], zones: Sequence[tuple[str, Shape]]) -> dict[str, Any]:
    """Steps between consecutive detected frames, scaled to path per minute
    of detected time, and zone membership changes between them."""
    steps, seconds = [], 0.0
    changes = {key: 0 for key, _ in zones}
    for i in range(1, len(track)):
        a, b = track[i - 1], track[i]
        if a is None or b is None:
            continue
        steps.append(math.hypot(b[0] - a[0], b[1] - a[1]))
        seconds += (times[i] - times[i - 1]) / 1e6
        for key, shape in zones:
            if shape.contains(*a) != shape.contains(*b):
                changes[key] += 1
    path = float(sum(steps))
    out: dict[str, Any] = {"steps": len(steps), "detectedSeconds": round(seconds, 3), "pathCm": round(path, 3)}
    if steps:
        values = np.array(steps)
        out["stepCm"] = {f"p{p}": round(float(np.percentile(values, p)), 4) for p in PERCENTILES} | {
            "mean": round(float(values.mean()), 4), "max": round(float(values.max()), 4),
        }
        out["histogramCm"] = histogram(values)
    if seconds > 0:
        out["pathCmPerMinute"] = round(path / seconds * 60, 2)
        out["meanSpeedCmS"] = round(path / seconds, 3)
    out["zoneChanges"] = changes
    return out


def histogram(values: np.ndarray) -> list[dict[str, Any]]:
    """Step counts in bins doubling from 0.05 cm, so a distribution of
    sub-millimetre jitter and the odd large jump both stay readable."""
    edges = [0.0, 0.05, 0.1, 0.2, 0.4, 0.8, 1.6, 3.2, math.inf]
    counts, _ = np.histogram(values, bins=edges)
    return [{"fromCm": lo, "toCm": None if math.isinf(hi) else hi, "count": int(n)} for lo, hi, n in zip(edges, edges[1:], counts)]


def report(source: Path, job: dict[str, Any], still_start_s: float, still_end_s: float) -> dict[str, Any]:
    job = claim_shape(job)
    if not job.get("calibration"):
        raise ValueError("the job has no calibration")
    start_us, end_us = round(still_start_s * 1e6), round(still_end_s * 1e6)
    if end_us <= start_us:
        raise ValueError("the still stretch must end after it starts")
    times, tracks = positions(source, job, start_us, end_us)
    zones = zones_of(job)
    run = job["run"]
    return {
        "video": source.name,
        "job": run.get("id"),
        "paradigm": f"{run.get('paradigmKey')} v{run.get('paradigmVersion')}",
        "calibration": job["calibration"].get("id"),
        "clipStartUs": run.get("clipStartUs"),
        "stillStretchS": [still_start_s, still_end_s],
        "frames": len(times),
        "methods": {m: summarize(times, tracks[m], zones) for m in METHODS},
    }


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m misko_worker.jitter",
        description="Report the apparent movement of a still subject, per centroid method, as JSON.",
    )
    parser.add_argument("video", type=Path, help="the job's source video")
    parser.add_argument("job", type=Path, help="the job as JSON, from GET /api/analysis/jobs/{jobId}")
    parser.add_argument("--still", type=float, nargs=2, required=True, metavar=("START_S", "END_S"),
                        help="the still stretch, in seconds from the clip start")
    args = parser.parse_args(argv)
    try:
        result = report(args.video, json.loads(args.job.read_text()), *args.still)
    except (video.CorruptVideo, ValueError, KeyError, OSError) as error:
        json.dump({"error": str(error)}, sys.stdout)
        print()
        return 2
    json.dump(result, sys.stdout, indent=2)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
