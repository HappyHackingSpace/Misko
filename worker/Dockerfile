FROM python:3.14.7-slim@sha256:51dafde81dbdb6ebde285137a295cf18a47ca95234fe388a343719cb97305b3d
WORKDIR /worker
COPY requirements.txt ./
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY misko_worker ./misko_worker
USER 65532:65532
ENV MISKO_WORK_DIR=/tmp
ENTRYPOINT ["python", "-m", "misko_worker"]
