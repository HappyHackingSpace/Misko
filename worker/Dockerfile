FROM python:3.15.0rc3-slim@sha256:a3af3d230522dd52f7f2191b39e1892912dbcb0c8dee22afdec3dc83baf0f549
WORKDIR /worker
COPY requirements.txt ./
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY misko_worker ./misko_worker
USER 65532:65532
ENV MISKO_WORK_DIR=/tmp
ENTRYPOINT ["python", "-m", "misko_worker"]
