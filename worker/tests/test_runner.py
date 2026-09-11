"""The job loop against an in-process fake of the worker API and of Cloud
Storage signed URLs. This proves the protocol, not real GCS behavior."""

from __future__ import annotations

import base64
import json
import logging
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import google_crc32c
import pytest

from misko_worker import ALGORITHM_VERSION, storage
from misko_worker.api import WorkerApi
from misko_worker.runner import run_once
from tests import synthetic

TOKEN = "mw_secret-token-value"


class FakeCloud:
    def __init__(self, source: bytes, jobs: list[dict]) -> None:
        self.source, self.jobs = source, jobs
        self.declared: dict[str, dict] = {}
        self.objects: dict[str, bytearray] = {}
        self.results: list[dict] = []
        self.failures: list[dict] = []
        self.heartbeats = 0
        self.stale = False
        self.source_status = 0
        server = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args) -> None:
                pass

            def reply(self, status: int, body: dict | None = None, headers: dict | None = None) -> None:
                data = b"" if body is None else json.dumps(body).encode()
                self.send_response(status)
                for key, value in (headers or {}).items():
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def body(self) -> bytes:
                return self.rfile.read(int(self.headers.get("Content-Length") or 0))

            def do_GET(self) -> None:
                if server.source_status:
                    self.reply(server.source_status)
                elif self.path == "/gcs/source?generation=42":
                    self.reply_bytes(server.source)
                else:
                    self.reply(404)

            def reply_bytes(self, data: bytes) -> None:
                self.send_response(200)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_PUT(self) -> None:
                name = self.path.removeprefix("/gcs/session/")
                data = self.body()
                first, rest = self.headers["Content-Range"].removeprefix("bytes ").split("-")
                last, total = (int(v) for v in rest.split("/"))
                assert int(first) == len(server.objects[name])
                server.objects[name] += data
                if last + 1 < total:
                    self.reply(308, headers={"Range": f"bytes=0-{last}"})
                else:
                    self.reply(200, {})

            def do_POST(self) -> None:
                raw = self.body()
                if self.path.startswith("/gcs/start/"):
                    name = self.path.removeprefix("/gcs/start/")
                    assert self.headers["x-goog-resumable"] == "start"
                    server.objects[name] = bytearray()
                    self.reply(201, headers={"Location": f"http://127.0.0.1:{server.port}/gcs/session/{name}"})
                    return
                if self.headers.get("Authorization") != "Worker " + TOKEN:
                    self.reply(401, {"code": "worker.unauthenticated", "error": "worker authentication required"})
                    return
                body = json.loads(raw)
                parts = self.path.strip("/").split("/")
                if self.path == "/api/worker/claim":
                    if server.jobs:
                        self.reply(200, server.jobs.pop(0))
                    else:
                        self.reply(204)
                elif parts[-1] == "heartbeat":
                    server.heartbeats += 1
                    if server.stale:
                        self.reply(409, {"code": "analysis.staleAttempt", "error": "stale"})
                    else:
                        self.reply(200, {})
                elif parts[-1] == "source-url":
                    self.reply(200, {"url": f"http://127.0.0.1:{server.port}/gcs/source?generation=42", "expiresAt": "2026-09-12T00:00:00Z"})
                elif parts[-1] == "outputs":
                    name = f"runs/{parts[3]}/attempts/{body['attempt']}/{body['fileName']}"
                    server.declared[name] = body
                    key = name.replace("/", "_")
                    self.reply(201, {"objectName": name, "upload": {"method": "POST", "url": f"http://127.0.0.1:{server.port}/gcs/start/{key}",
                                                                     "headers": {"Content-Type": body["contentType"], "x-goog-resumable": "start"}}})
                elif parts[-1] == "result":
                    server.results.append(body)
                    self.reply(200, {"status": "SUCCEEDED"})
                elif parts[-1] == "failure":
                    server.failures.append(body)
                    self.reply(200, {"status": "FAILED"})
                else:
                    self.reply(404)

        self.http = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.http.server_address[1]
        threading.Thread(target=self.http.serve_forever, daemon=True).start()

    def uploaded(self, name: str) -> bytes:
        return bytes(self.objects[name.replace("/", "_")])


@pytest.fixture
def walk(tmp_path_factory) -> bytes:
    path = tmp_path_factory.mktemp("fixture") / "walk.mp4"
    synthetic.render(path, synthetic.regular(3.0))
    return path.read_bytes()


def crc(data: bytes) -> str:
    return base64.b64encode(google_crc32c.Checksum(data).digest()).decode()


def test_a_job_uploads_verified_outputs_and_submits_no_metrics(tmp_path: Path, walk: bytes, caplog) -> None:
    job = synthetic.job(clip_start_us=500_000, clip_end_us=2_500_000)
    cloud = FakeCloud(walk, [job])
    api = WorkerApi(f"http://127.0.0.1:{cloud.port}", TOKEN)
    caplog.set_level(logging.DEBUG)
    assert run_once(api, tmp_path, heartbeat_s=0.05) is True
    assert run_once(api, tmp_path) is False

    assert not cloud.failures and len(cloud.results) == 1
    result = cloud.results[0]
    prefix = job["outputPrefix"]
    assert result == {
        "attempt": 1,
        "modelVersion": ALGORITHM_VERSION,
        "artifacts": [{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, {"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}],
        "pair": {"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 500_000, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"},
    }
    for name, declared in cloud.declared.items():
        data = cloud.uploaded(name)
        assert declared["sizeBytes"] == len(data) and declared["crc32c"] == crc(data)
    trajectory = json.loads(cloud.uploaded(prefix + "trajectory.json"))
    assert trajectory["runId"] == job["run"]["id"] and trajectory["durationUs"] == 2_000_000
    # 25 fps frames: the first frame at or after the 0.5 s clip start is at 0.52 s.
    assert trajectory["samples"]["tUs"][0] == 20_000 and trajectory["samples"]["tUs"][-1] < 2_000_000
    assert cloud.declared[prefix + "trajectory.json"]["contentType"] == "application/json"
    # Neither the token nor signed URLs reach the logs.
    assert TOKEN not in caplog.text and "gcs/" not in caplog.text


def test_open_clips_report_their_measured_duration(tmp_path: Path, walk: bytes) -> None:
    job = synthetic.job(clip_start_us=1_000_000, clip_end_us=None)
    cloud = FakeCloud(walk, [job])
    run_once(WorkerApi(f"http://127.0.0.1:{cloud.port}", TOKEN), tmp_path)
    trajectory = json.loads(cloud.uploaded(job["outputPrefix"] + "trajectory.json"))
    assert cloud.results[0]["recordingDurationUs"] == trajectory["durationUs"] > 1_900_000


def test_reanalysis_writes_a_separate_output_pair(tmp_path: Path, walk: bytes) -> None:
    first, second = synthetic.job(run_id="run-one"), synthetic.job(run_id="run-two", attempt=2)
    cloud = FakeCloud(walk, [first, second])
    api = WorkerApi(f"http://127.0.0.1:{cloud.port}", TOKEN)
    run_once(api, tmp_path)
    run_once(api, tmp_path)
    pairs = [r["pair"]["analyzedObjectName"] for r in cloud.results]
    assert pairs == ["runs/run-one/attempts/1/overlay.mp4", "runs/run-two/attempts/2/overlay.mp4"]


def test_failures_are_reported_with_their_retry_policy(tmp_path: Path, walk: bytes) -> None:
    corrupt = FakeCloud(walk[: len(walk) // 4], [synthetic.job()])
    run_once(WorkerApi(f"http://127.0.0.1:{corrupt.port}", TOKEN), tmp_path)
    assert corrupt.failures == [{"attempt": 1, "reason": "UNREADABLE_VIDEO", "retryable": False}] and not corrupt.results

    job = synthetic.job()
    job["calibration"]["frameWidth"] = 1920
    mismatch = FakeCloud(walk, [job])
    run_once(WorkerApi(f"http://127.0.0.1:{mismatch.port}", TOKEN), tmp_path)
    assert mismatch.failures[0]["retryable"] is False and mismatch.failures[0]["reason"].startswith("FRAME_SIZE_MISMATCH")

    # An expired signature is a transient storage fault; the reason names the status, never the URL.
    expired = FakeCloud(walk, [synthetic.job()])
    expired.source_status = 403
    run_once(WorkerApi(f"http://127.0.0.1:{expired.port}", TOKEN), tmp_path)
    assert expired.failures == [{"attempt": 1, "reason": "SOURCE_DOWNLOAD_FAILED: HTTP 403", "retryable": True}]


def test_a_lost_lease_stops_without_a_report(tmp_path: Path, walk: bytes) -> None:
    long = tmp_path / "long.mp4"
    synthetic.render(long, synthetic.regular(8.0))
    cloud = FakeCloud(long.read_bytes(), [synthetic.job(clip_end_us=8_000_000)])
    cloud.stale = True
    run_once(WorkerApi(f"http://127.0.0.1:{cloud.port}", TOKEN), tmp_path, heartbeat_s=0.01)
    assert cloud.heartbeats >= 1 and not cloud.results and not cloud.failures


def test_uploads_resume_in_chunks(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setattr(storage, "CHUNK", 256 * 1024)
    data = bytes(range(256)) * 3000
    path = tmp_path / "blob.bin"
    path.write_bytes(data)
    cloud = FakeCloud(b"", [])
    signed = {"method": "POST", "url": f"http://127.0.0.1:{cloud.port}/gcs/start/blob", "headers": {"x-goog-resumable": "start"}}
    storage.upload(signed, path)
    assert bytes(cloud.objects["blob"]) == data
    assert storage.crc32c_base64(path) == crc(data)
