# Deploy - pull-based (no source checkout)

This is the fastest way to run Mişko on a machine that does not have the source
code or git: pull prebuilt images from the GitHub Container Registry (GHCR) and
start them with Docker Compose.

## Prerequisites

- Docker Desktop (or Docker Engine + Compose v2). Nothing else: no Node, no git.

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
   refuses to start with them unset. For a real (non-demo) deployment also set
   `CORS_ORIGIN` to the UI origin instead of `*`, and pin `IMAGE_TAG` to a
   released version (e.g. `1.0.0`) instead of `latest` for a reproducible deploy.

2. Pull and start:

   ```bash
   docker compose -f docker-compose.prod.yml pull
   docker compose -f docker-compose.prod.yml up -d
   ```

   The backend container automatically applies the database migrations and
   creates the superadmin on first boot.

3. Get the one-time admin password from the log:

   ```bash
   docker compose -f docker-compose.prod.yml logs backend
   ```

   Look for the boxed line printed by the bootstrap step (email `admin@miskolab.com`
   by default).

4. Open the UI: http://localhost:8080 and sign in with that email/password.

## Stop / reset

```bash
docker compose -f docker-compose.prod.yml down        # stop, keep data
docker compose -f docker-compose.prod.yml down -v      # stop and wipe the database
```

## Offline alternative (no registry)

If the machine has no registry access, the maintainer can export the images and
hand them over on a drive:

```bash
docker save \
  ghcr.io/happyhackingspace/misko-backend:latest \
  ghcr.io/happyhackingspace/misko-frontend:latest \
  postgres:16-alpine -o misko-images.tar
```

The reviewer then runs `docker load -i misko-images.tar` and continues from
step 2 above.
