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

3. Install the schema into the empty database. On later updates run the same
   command after `pull` and before `up`; it applies only pending migrations:

   ```bash
   docker compose -f docker-compose.prod.yml --profile setup run --rm migrate
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

### HTTPS (optional)

For a public hostname, the `https` profile adds Caddy, which obtains and renews
a Let's Encrypt certificate on its own. Before starting it:

- The hostname's `A` record must already point at this machine.
- Ports 80 and 443 must be open in the firewall (Let's Encrypt validates over 80).

```bash
export SITE_ADDRESS=misko.example.org
FRONTEND_BIND=127.0.0.1 FRONTEND_PORT=8080 \
  docker compose -f docker-compose.prod.yml --profile https up -d
```

Caddy takes 80 and 443, so the panel itself moves to `127.0.0.1:8080` and is no
longer reachable over plain HTTP from outside. Keep the `caddy-data` volume
(`down -v` deletes it) so the certificate is reused instead of re-issued. If
the hostname is behind Cloudflare's proxy, set the SSL/TLS mode to Full (strict).

The `DEPLOY` workflow does this when the `SITE_ADDRESS` repository variable
(Settings > Secrets and variables > Actions > Variables) is set.

### Video storage

Videos and analysis outputs are kept on this machine, in the `videos` Docker
volume; nothing needs to be configured. They share the disk with the database,
so watch the free space (`df -h`) and back up both somewhere else. `down -v`
deletes the videos together with the database.

To use a private Google Cloud Storage bucket instead, add a `storage.env` file
(see `storage.env.example`) and the credentials the bucket needs. Videos already
on the volume are not moved to the bucket.

## Deploy to a server with GitHub Actions

The `DEPLOY` workflow (`.github/workflows/deploy.yml`) updates a Linux server
over SSH. It is started by hand from the Actions tab, on `main`, after
`Release images`.

The server needs Docker Engine with Compose v2, a user in the `docker` group
that accepts the deploy SSH key, and ports 22, 80 and 443 open.

| Name | Kind | Value |
|---|---|---|
| `VPS_HOST` | secret | server IP address or hostname |
| `VPS_USER` | secret | SSH user |
| `VPS_SSH_PRIVATE_KEY` | secret | private key without a passphrase |
| `APP_POSTGRES_PASSWORD` | secret | letters and digits only; keep it unchanged |
| `APP_JWT_SECRET` | secret | at least 32 random bytes |
| `APP_WORKER_TOKEN` | secret | empty until the worker is registered |
| `SITE_ADDRESS` | variable | public hostname; enables HTTPS |

The workflow writes `~/misko/.env` only when it does not exist; later runs change
only `IMAGE_TAG` in it.

After the first successful run, sign in to the server and, in `~/misko`, create
the administrator and register the worker. Both secrets are printed once:

```bash
docker compose -f docker-compose.prod.yml --profile setup run --rm setup
docker compose -f docker-compose.prod.yml --profile setup run --rm register-worker \
  -name worker-1 -model-version "misko-tracker-bgsub 2.2.0" \
  -paradigm MWM -paradigm NOVEL_OBJECT -paradigm POLE -paradigm TREADMILL
```

Save the worker token as `APP_WORKER_TOKEN` and run `DEPLOY` again.

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
