FROM python:3.14.3-slim@sha256:5e59aae31ff0e87511226be8e2b94d78c58f05216efda3b07dbbed938ec8583b
WORKDIR /worker
COPY requirements.txt ./
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY misko_worker ./misko_worker
USER 65532:65532
ENV MISKO_WORK_DIR=/tmp
ENTRYPOINT ["python", "-m", "misko_worker"]
