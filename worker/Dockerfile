FROM python:3.15.2-slim@sha256:65a65fc7b30b06cb6df4930fd4650697cb23f4c2698ab81fc87e4b031bc36928
WORKDIR /worker
COPY requirements.txt ./
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY misko_worker ./misko_worker
USER 65532:65532
ENV MISKO_WORK_DIR=/tmp
ENTRYPOINT ["python", "-m", "misko_worker"]
