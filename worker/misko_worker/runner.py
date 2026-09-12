"""The job loop: claim, keep the lease alive, analyze, upload and submit."""

from __future__ import annotations

import logging
import tempfile
import threading
from pathlib import Path
from typing import Any

from . import ALGORITHM_VERSION, TIME_MAPPING_VERSION
from .api import ApiError, WorkerApi
from .pipeline import NonRetryable, analyze, trajectory_document
from .storage import StorageError, crc32c_base64, download, upload
from .tracking import TrackerSettings
from .video import CorruptVideo

log = logging.getLogger("misko_worker")


class LostLease(Exception):
    """Another attempt owns the run now; this attempt stops without reporting."""


class _Heartbeat(threading.Thread):
    def __init__(self, api: WorkerApi, run_id: str, attempt: int, interval: float) -> None:
        super().__init__(daemon=True)
        self._api, self._run, self._attempt, self._interval = api, run_id, attempt, interval
        self._stop = threading.Event()
        self.lost = False

    def run(self) -> None:
        while not self._stop.wait(self._interval):
            try:
                self._api.heartbeat(self._run, self._attempt)
            except ApiError as error:
                if error.code == "analysis.staleAttempt":
                    self.lost = True
                    return
                log.warning("heartbeat failed", extra={"run": self._run, "code": error.code})
            except OSError:
                log.warning("heartbeat failed", extra={"run": self._run})

    def stop(self) -> None:
        self._stop.set()


def run_once(api: WorkerApi, work_dir: Path, heartbeat_s: float = 60.0, settings: TrackerSettings = TrackerSettings()) -> bool:
    """Processes one job. Returns False when there was nothing to claim."""
    job = api.claim()
    if job is None:
        return False
    run = job["run"]
    run_id, attempt = run["id"], run["attempt"]
    beat = _Heartbeat(api, run_id, attempt, heartbeat_s)
    beat.start()
    try:
        with tempfile.TemporaryDirectory(dir=work_dir) as scratch:
            result = _process(api, job, Path(scratch), beat, settings)
        log.info("run finished", extra={"run": run_id, "attempt": attempt, "status": result.get("status")})
    except LostLease:
        log.warning("lease lost; stopping this attempt", extra={"run": run_id, "attempt": attempt})
    except (NonRetryable, CorruptVideo) as error:
        _report(api, run_id, attempt, str(error), retryable=False)
    except ApiError as error:
        if error.code == "analysis.staleAttempt":
            log.warning("attempt is no longer current", extra={"run": run_id, "attempt": attempt})
        else:
            _report(api, run_id, attempt, f"API_ERROR: {error}", retryable=error.status >= 500 or error.status == 409)
    except StorageError as error:
        _report(api, run_id, attempt, str(error), retryable=True)
    except Exception as error:  # noqa: BLE001 - any other fault is reported without its message
        log.exception("run crashed", extra={"run": run_id, "attempt": attempt})
        _report(api, run_id, attempt, f"WORKER_ERROR: {type(error).__name__}", retryable=True)
    finally:
        beat.stop()
    return True


def _process(api: WorkerApi, job: dict[str, Any], scratch: Path, beat: _Heartbeat, settings: TrackerSettings) -> dict[str, Any]:
    run = job["run"]
    run_id, attempt = run["id"], run["attempt"]

    def current() -> None:
        if beat.lost:
            raise LostLease()

    source = scratch / "source"
    download(api.source_url(run_id, attempt), source)
    current()
    overlay = scratch / "overlay.mp4"
    analysis = analyze(source, job, overlay, settings)
    current()
    trajectory = scratch / "trajectory.json"
    trajectory.write_bytes(trajectory_document(job, analysis, settings))

    artifacts = []
    for kind, path, content_type in (("ANALYZED_VIDEO", overlay, "video/mp4"), ("TRAJECTORY", trajectory, "application/json")):
        declared = api.request_output(run_id, attempt, kind, path.name, content_type, path.stat().st_size, crc32c_base64(path))
        upload(declared["upload"], path)
        artifacts.append({"kind": kind, "objectName": declared["objectName"]})
        current()
    body = {
        "attempt": attempt,
        "modelVersion": ALGORITHM_VERSION,
        "artifacts": artifacts,
        "pair": {
            "analyzedObjectName": artifacts[0]["objectName"],
            "sourceOffsetUs": run["clipStartUs"],
            "outputOffsetUs": 0,
            "timeMappingVersion": TIME_MAPPING_VERSION,
        },
    }
    if run["clipEndUs"] is None:
        body["recordingDurationUs"] = analysis.duration_us
    return api.submit(run_id, body)


def _report(api: WorkerApi, run_id: str, attempt: int, reason: str, retryable: bool) -> None:
    log.warning("reporting failure", extra={"run": run_id, "attempt": attempt, "reason": reason, "retryable": retryable})
    try:
        api.fail(run_id, attempt, reason, retryable)
    except (ApiError, OSError) as error:
        log.warning("failure report was not accepted", extra={"run": run_id, "error": type(error).__name__})
