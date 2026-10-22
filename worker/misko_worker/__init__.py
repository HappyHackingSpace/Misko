"""Misko analysis worker for recorded, fixed-camera, single-subject video of
the lab's paradigms: MWM (Morris water maze), NOVEL_OBJECT (novel object
recognition), POLE (pole test) and TREADMILL (forced treadmill).

The worker measures where the subject is in every frame and writes a
misko.trajectory.v1 file and an annotated video. The Go API computes every
published metric and event from that trajectory; the worker computes none.
"""

# ALGORITHM_VERSION is the model version the worker registers and submits. It
# changes whenever detection or trajectory output changes.
ALGORITHM_VERSION = "misko-tracker-bgsub 2.2.0"
TRAJECTORY_SCHEMA = "misko.trajectory.v1"
TIME_MAPPING_VERSION = "identity-v1"
# The paradigm versions this worker can measure are listed in
# paradigms.SUPPORTED; a job for any other one is failed with
# UNSUPPORTED_PARADIGM before its video is downloaded.
