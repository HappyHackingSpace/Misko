"""Signed URL transfers with Cloud Storage: download of the pinned source and
resumable uploads of outputs. URLs and upload session URIs are never logged."""

from __future__ import annotations

import base64
import shutil
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

import google_crc32c

CHUNK = 8 * 1024 * 1024  # a multiple of 256 KiB, as resumable uploads require


class StorageError(Exception):
    """A transfer failed. The message carries the HTTP status, never the URL."""


def crc32c_base64(path: Path) -> str:
    checksum = google_crc32c.Checksum()
    with path.open("rb") as f:
        while block := f.read(CHUNK):
            checksum.update(block)
    return base64.b64encode(checksum.digest()).decode()


def download(url: str, destination: Path, timeout: float = 60.0) -> None:
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response, destination.open("wb") as out:
            shutil.copyfileobj(response, out, CHUNK)
    except urllib.error.HTTPError as error:
        raise StorageError(f"SOURCE_DOWNLOAD_FAILED: HTTP {error.code}") from None
    except urllib.error.URLError as error:
        raise StorageError(f"SOURCE_DOWNLOAD_FAILED: {type(error.reason).__name__}") from None


def upload(signed: dict[str, Any], path: Path, timeout: float = 60.0) -> None:
    """Starts a resumable upload with the signed POST and sends the file in chunks."""
    start = urllib.request.Request(signed["url"], data=b"", method=signed.get("method", "POST"), headers=dict(signed.get("headers") or {}))
    try:
        with urllib.request.urlopen(start, timeout=timeout) as response:
            session = response.headers["Location"]
    except urllib.error.HTTPError as error:
        raise StorageError(f"UPLOAD_START_FAILED: HTTP {error.code}") from None
    if not session:
        raise StorageError("UPLOAD_START_FAILED: no session")
    size = path.stat().st_size
    offset = 0
    with path.open("rb") as f:
        while offset < size:
            f.seek(offset)
            block = f.read(CHUNK)
            last = offset + len(block) - 1
            request = urllib.request.Request(
                session, data=block, method="PUT", headers={"Content-Length": str(len(block)), "Content-Range": f"bytes {offset}-{last}/{size}"}
            )
            try:
                with urllib.request.urlopen(request, timeout=timeout):
                    return
            except urllib.error.HTTPError as error:
                if error.code != 308:
                    raise StorageError(f"UPLOAD_FAILED: HTTP {error.code}") from None
                received = error.headers.get("Range")
                offset = int(received.split("-")[1]) + 1 if received else 0
