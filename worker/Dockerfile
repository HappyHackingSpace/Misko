FROM python:3.14.8-slim@sha256:a2b82f3c48559aa0a8446d9af49826b6e2b2016f4cd2afabfe6013ec53729170
WORKDIR /worker
COPY requirements.txt ./
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY misko_worker ./misko_worker
USER 65532:65532
ENV MISKO_WORK_DIR=/tmp
ENTRYPOINT ["python", "-m", "misko_worker"]
