"""The jitter report on a still subject (issue #242)."""

from __future__ import annotations

import json
from pathlib import Path

from misko_worker import jitter
from misko_worker.geometry import Shape
from tests import scenes, synthetic


def test_summary_counts_steps_path_and_zone_flips():
    zone = Shape(circle=(0.0, 0.0, 1.0))
    times = [0, 40_000, 80_000, 120_000, 160_000]
    # In, out (0.2 cm step across the edge), lost, in again: one flip, two steps.
    track = [(0.9, 0.0), (1.1, 0.0), None, (0.9, 0.0), (0.9, 0.1)]
    out = jitter.summarize(times, track, [("zone", zone)])
    assert out["steps"] == 2 and out["detectedSeconds"] == 0.08
    assert abs(out["pathCm"] - 0.3) < 1e-9 and out["zoneChanges"] == {"zone": 1}
    assert abs(out["pathCmPerMinute"] - 0.3 / 0.08 * 60) < 0.01
    assert sum(b["count"] for b in out["histogramCm"]) == 2


def test_api_jobs_are_read_like_claims():
    job = {"id": "j", "paradigmKey": "NOVEL_OBJECT", "calibration": {"id": "c"}}
    assert jitter.claim_shape(job) == {"run": job, "calibration": {"id": "c"}}
    claim = {"run": {"id": "j"}, "calibration": {"id": "c"}}
    assert jitter.claim_shape(claim) is claim


# The NOVEL_OBJECT scene rests the subject on the novel object from 4.0 to
# 11.2 s; between 5 and 10 s the true movement is zero, so every step is jitter.
def test_still_subject_report(tmp_path: Path):
    scene, job = scenes.novel_object()
    source = tmp_path / "nor.mp4"
    synthetic.render(source, synthetic.regular(12.0), scenes.nor_walk, scene=scene)
    job_file = tmp_path / "job.json"
    job_file.write_text(json.dumps(job))

    result = jitter.report(source, job, 5.0, 10.0)
    assert result["frames"] == 125 and result["paradigm"] == "NOVEL_OBJECT v1"
    for method in jitter.METHODS:
        summary = result["methods"][method]
        assert summary["steps"] == 124
        assert summary["stepCm"]["p99"] < 0.5, (method, summary)
        assert set(summary["zoneChanges"]) >= {"novel_object", "familiar_object"}
    # A running median cannot step further than the raw positions it follows.
    assert result["methods"]["median3"]["pathCm"] <= result["methods"]["weighted"]["pathCm"]
    assert jitter.main([str(source), str(job_file), "--still", "5", "10"]) == 0
