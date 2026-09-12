# Deploy - pull-based (no source checkout)

This is the fastest way to run Mişko on a machine that does not have the source
code or git: pull prebuilt images from the GitHub Container Registry (GHCR) and
start them with Docker Compose.

## Prerequisites

- Docker Desktop (or Docker Engine + Compose v2). Nothing else: no Go, no Node, no git.

## One-time publish (maintainer)

The images are published by the `Release images` GitHub Actions workflow:

- Push a version tag: `git tag v1.0.0 && git push origin v1.0.0`, or
- Run it manually from the Actions tab (publishes `:latest`).

After the first publish, open the repo Packages settings and set both
`misko-backend` and `misko-frontend` to **Public** so they can be pulled without
logging in. (If they stay private, the reviewer must `docker login ghcr.io` with
a personal access token that has `read:packages`.)

## Run (reviewer / professor)

1. Create a folder and put two files in it:
   - `docker-compose.prod.yml`
   - `.env` (copied from `.env.prod.example`)

   In `.env`, `POSTGRES_PASSWORD` and `JWT_SECRET` are **required** - compose
   refuses to start with them unset. Pin `IMAGE_TAG` to a released version
   (e.g. `1.0.0`) instead of `latest` for a reproducible deploy.

2. Pull the images:

   ```bash
   docker compose -f docker-compose.prod.yml pull
   ```

3. Install the schema into the empty database. This runs once:

   ```bash
   docker compose -f docker-compose.prod.yml --profile setup run --rm schema
   ```

4. Create the laboratory and the first administrator. This also runs once, and
   it prints the generated password to your terminal:

   ```bash
   docker compose -f docker-compose.prod.yml --profile setup run --rm setup
   ```

   The password is shown **once** and is never written to a log. Copy it before
   closing the terminal. If you need it in a file instead, add
   `-credentials-file /out/admin.txt` and mount a folder at `/out`.

5. Start the system:

   ```bash
   docker compose -f docker-compose.prod.yml up -d
   ```

6. Open the UI at http://localhost:8080 and sign in with the email from `.env`
   (`ADMIN_EMAIL`) and the password from step 4. Change the password after
   signing in.

The API answers `/api/health` and `/api/ready`, so
`curl http://localhost:8080/api/health` is a quick check that the panel reaches
the backend through nginx.

### Video storage

Uploads and playback need a private Google Cloud Storage bucket. Without
`GCS_BUCKET` the video upload and read routes answer 503 and everything else
works normally, which is enough to review the workflow up to the point where a
recording is uploaded.

## Stop / reset

```bash
docker compose -f docker-compose.prod.yml down        # stop, keep data
docker compose -f docker-compose.prod.yml down -v      # stop and wipe the database
```

After a `down -v` the database is empty again, so repeat steps 3 and 4 before
starting the stack.

## Offline alternative (no registry)

If the machine has no registry access, the maintainer can export the images and
hand them over on a drive:

```bash
docker save \
  ghcr.io/happyhackingspace/misko-backend:latest \
  ghcr.io/happyhackingspace/misko-frontend:latest \
  postgres:18.6-alpine -o misko-images.tar
```

The reviewer then runs `docker load -i misko-images.tar` and continues from
step 2 above.
