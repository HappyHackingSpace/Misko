"""Runs the worker until SIGTERM or SIGINT.

Environment:
  MISKO_API_URL        API base URL, for example https://misko.example.org
  MISKO_WORKER_TOKEN   token returned when the worker was registered
  MISKO_WORK_DIR       scratch directory for downloads and outputs (default: system temp)
  MISKO_POLL_INTERVAL  seconds to wait when no run is queued (default: 10)
"""

from __future__ import annotations

import json
import logging
import os
import signal
import sys
import tempfile
import threading
from pathlib import Path

from . import ALGORITHM_VERSION
from .api import ApiError, WorkerApi
from .runner import run_once


class _JsonFormatter(logging.Formatter):
    FIELDS = ("run", "attempt", "status", "code", "reason", "retryable", "error")

    def format(self, record: logging.LogRecord) -> str:
        entry = {"level": record.levelname, "message": record.getMessage()}
        entry.update({k: getattr(record, k) for k in self.FIELDS if hasattr(record, k)})
        return json.dumps(entry)


def main() -> int:
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(_JsonFormatter())
    logging.basicConfig(level=logging.INFO, handlers=[handler])
    log = logging.getLogger("misko_worker")
    base, token = os.environ.get("MISKO_API_URL", ""), os.environ.get("MISKO_WORKER_TOKEN", "")
    if not base.startswith(("http://", "https://")) or not token:
        log.error("MISKO_API_URL and MISKO_WORKER_TOKEN are required")
        return 2
    try:
        interval = float(os.environ.get("MISKO_POLL_INTERVAL", "10"))
    except ValueError:
        interval = -1
    if interval < 1:
        log.error("MISKO_POLL_INTERVAL must be at least 1 second")
        return 2
    work_dir = Path(os.environ.get("MISKO_WORK_DIR") or tempfile.gettempdir())
    stop = threading.Event()
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: stop.set())
    api = WorkerApi(base, token)
    log.info("worker started", extra={"status": ALGORITHM_VERSION})
    while not stop.is_set():
        try:
            if run_once(api, work_dir):
                continue
        except (ApiError, OSError) as error:
            log.warning("claim failed", extra={"error": type(error).__name__})
        stop.wait(interval)
    log.info("worker stopped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
