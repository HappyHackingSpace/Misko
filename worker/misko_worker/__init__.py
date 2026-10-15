"""Misko analysis worker for recorded, fixed-camera, single-subject Open Field video.

The worker measures where the subject is in every frame and writes a
misko.trajectory.v1 file and an annotated video. The Go API computes every
published metric and event from that trajectory; the worker computes none.
"""

# ALGORITHM_VERSION is the model version the worker registers and submits. It
# changes whenever detection or trajectory output changes.
ALGORITHM_VERSION = "misko-open-field-bgsub 1.0.0"
TRAJECTORY_SCHEMA = "misko.trajectory.v1"
# SUPPORTED is the only paradigm version this worker can measure. A job for any
# other one is failed with UNSUPPORTED_PARADIGM before its video is downloaded.
SUPPORTED_PARADIGM = ("OPEN_FIELD", 1)
TIME_MAPPING_VERSION = "identity-v1"
