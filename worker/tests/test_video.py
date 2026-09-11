"""Frame selection by presentation timestamp."""

from __future__ import annotations

from pathlib import Path

from misko_worker import video
from tests import synthetic


def test_frames_cover_the_half_open_clip(tmp_path: Path) -> None:
    source = tmp_path / "walk.mp4"
    synthetic.render(source, synthetic.regular(1.0))  # frames every 40 ms from 0 to 960 ms

    # Recording time is video time minus the clip start; the frame at the clip
    # start is included and the frame exactly at the clip end is not.
    times = [t for t, _ in video.frames(source, 200_000, 480_000)]
    assert times == [0, 40_000, 80_000, 120_000, 160_000, 200_000, 240_000]

    # Between frames, the clip starts at the next frame.
    times = [t for t, _ in video.frames(source, 210_000, 330_000)]
    assert times == [30_000, 70_000, 110_000]

    # Without an end, the clip runs to the last frame.
    times = [t for t, _ in video.frames(source, 800_000, None)]
    assert times == [0, 40_000, 80_000, 120_000, 160_000]

    info = video.probe(source)
    assert (info.width, info.height) == (synthetic.WIDTH, synthetic.HEIGHT)
