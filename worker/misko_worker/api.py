"""Client for the worker routes of the Misko API. The worker token is sent only in
the Authorization header and never logged; signed URLs are never logged either."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Any


class ApiError(Exception):
    def __init__(self, status: int, code: str, message: str) -> None:
        super().__init__(f"HTTP {status} {code}")
        self.status, self.code, self.message = status, code, message


class WorkerApi:
    def __init__(self, base_url: str, token: str, timeout: float = 30.0) -> None:
        self._base, self._token, self._timeout = base_url.rstrip("/"), token, timeout

    def claim(self) -> dict[str, Any] | None:
        return self._post("/api/worker/claim", {})

    def heartbeat(self, run_id: str, attempt: int) -> dict[str, Any]:
        return self._post(f"/api/worker/runs/{run_id}/heartbeat", {"attempt": attempt})

    def source_url(self, run_id: str, attempt: int) -> str:
        return self._post(f"/api/worker/runs/{run_id}/source-url", {"attempt": attempt})["url"]

    def request_output(self, run_id: str, attempt: int, kind: str, file_name: str, content_type: str, size: int, crc32c: str) -> dict[str, Any]:
        return self._post(
            f"/api/worker/runs/{run_id}/outputs",
            {"attempt": attempt, "kind": kind, "fileName": file_name, "contentType": content_type, "sizeBytes": size, "crc32c": crc32c},
        )

    def submit(self, run_id: str, body: dict[str, Any]) -> dict[str, Any]:
        return self._post(f"/api/worker/runs/{run_id}/result", body)

    def fail(self, run_id: str, attempt: int, reason: str, retryable: bool) -> dict[str, Any]:
        return self._post(f"/api/worker/runs/{run_id}/failure", {"attempt": attempt, "reason": reason[:2000], "retryable": retryable})

    def _post(self, path: str, body: dict[str, Any]) -> Any:
        request = urllib.request.Request(
            self._base + path,
            data=json.dumps(body).encode(),
            method="POST",
            headers={"Authorization": "Worker " + self._token, "Content-Type": "application/json", "Accept": "application/json"},
        )
        try:
            with urllib.request.urlopen(request, timeout=self._timeout) as response:
                if response.status == 204:
                    return None
                return json.loads(response.read())
        except urllib.error.HTTPError as error:
            try:
                problem = json.loads(error.read())
            except ValueError:
                problem = {}
            raise ApiError(error.code, problem.get("code", "unknown"), problem.get("error", "")) from None
