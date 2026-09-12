"""Video decoding with presentation timestamps.

Frame times come from the container's timestamps, never from a frame rate, so a
variable frame rate video keeps its timeline. Video time is measured from the
stream's first timestamp.
"""

from __future__ import annotations

from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path

import av
import numpy as np


class CorruptVideo(Exception):
    """The source cannot be opened or decoded. The message is a stable reason code."""


@dataclass(frozen=True)
class VideoInfo:
    width: int
    height: int
    duration_us: int | None


def probe(path: Path) -> VideoInfo:
    try:
        with av.open(str(path)) as container:
            if not container.streams.video:
                raise CorruptVideo("NO_VIDEO_STREAM")
            stream = container.streams.video[0]
            duration = None
            if stream.duration is not None and stream.time_base is not None:
                duration = int(round(stream.duration * stream.time_base * 1_000_000))
            elif container.duration is not None:
                duration = int(container.duration)
            return VideoInfo(stream.codec_context.width, stream.codec_context.height, duration)
    except av.FFmpegError as error:
        raise CorruptVideo("UNREADABLE_VIDEO") from error


def frames(path: Path, start_us: int, end_us: int | None) -> Iterator[tuple[int, np.ndarray]]:
    """Yields (recording time, RGB image) for frames with start_us <= video time < end_us.

    Recording time is video time minus start_us, in microseconds. Frames that
    repeat an earlier timestamp are skipped.
    """
    try:
        with av.open(str(path)) as container:
            stream = container.streams.video[0]
            stream.thread_type = "AUTO"
            base = stream.time_base
            origin = stream.start_time or 0
            if start_us > 0:
                container.seek(int(origin + start_us / 1_000_000 / base), stream=stream, backward=True)
            last = None
            for frame in container.decode(stream):
                if frame.pts is None:
                    continue
                t = int(round((frame.pts - origin) * base * 1_000_000))
                if t < start_us or (last is not None and t <= last):
                    continue
                if end_us is not None and t >= end_us:
                    break
                last = t
                yield t - start_us, frame.to_ndarray(format="rgb24")
    except av.FFmpegError as error:
        raise CorruptVideo("UNDECODABLE_VIDEO") from error
