# Command Deck — Docker Runbook

Operational reference for running Command Deck via Docker Compose on the Hostinger VPS.
Covers start, stop, rebuild, logs, health, pull, rollback, inspect, volume safety, daemon check, Hub auth, Scout, and Buildx.

---

## Prerequisites

- Docker 29+ and Docker Compose v2 installed on the host
- User is a member of the `docker` group (can run `docker` without sudo)
- `.env` file exists in the same directory as `compose.yml` (copy from `.env.example`)
- Docker Hub credentials configured if pushing images

---

## Docker Daemon Health Check

```bash
# Verify Docker daemon is responsive
docker info | head -5

# Quick liveness test
docker run --rm alpine echo "DOCKER_OK"

# Check Docker context
docker context show   # should print "default"
docker ps             # should show running containers (or empty list if none)
```

If `docker ps` fails with permission denied: `sudo usermod -aG docker $USER` then re-login.

---

## Start / Stop / Restart

```bash
# Start the stack (detached)
docker compose up -d

# Start with dev override (local source builds instead of pulling from registry)
docker compose -f compose.yml -f compose.dev.yml up -d --build

# Stop all services
docker compose down

# Stop and remove named volumes (DESTRUCTIVE — see Volume Safety below)
docker compose down -v

# Restart all services
docker compose restart

# Restart a specific service
docker compose restart commanddeck-api
```

---

## Rebuild

```bash
# Rebuild images from Dockerfile (no cache)
docker compose build --no-cache

# Rebuild a single service
docker compose build commanddeck-web

# Rebuild with inline cache (for Buildx push)
docker compose build --build-arg BUILDKIT_INLINE_CACHE=1

# Up after rebuild
docker compose up -d --force-recreate
```

---

## Logs

```bash
# Tail all services
docker compose logs -f

# Tail a specific service
docker compose logs -f commanddeck-api
docker compose logs -f commanddeck-web

# Last 100 lines all services
docker compose logs --tail=100

# Last 50 lines specific service
docker compose logs --tail=50 commanddeck-db
```

---

## Health Checks

```bash
# Show container status and health
docker compose ps

# Test postgres health
docker exec commanddeck-db-1 pg_isready -U commanddeck

# Test redis health
docker exec commanddeck-redis-1 redis-cli ping

# Test API health
curl -s http://localhost:8080/health

# Test web frontend
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000
```

---

## Pull Latest Images

```bash
# Pull all service images
docker compose pull

# Pull a specific service
docker compose pull commanddeck-api

# Pull then restart
docker compose pull && docker compose up -d
```

---

## Rollback to Previous Tag

```bash
# 1. Pin the desired image tag
export COMMANDDECK_IMAGE_TAG="<previous-sha>"   # e.g., 24a5909

# 2. Pull the pinned image
docker compose -f compose.yml -f compose.prod.yml pull

# 3. Recreate containers with the older image
docker compose -f compose.yml -f compose.prod.yml up -d --force-recreate

# To revert back to :latest, set COMMANDDECK_IMAGE_TAG=latest and repeat steps 2–3.
```

---

## Inspect Containers

```bash
# List all containers (running and stopped)
docker compose ps -a

# Inspect a container's config
docker inspect commanddeck-api-1

# Follow a container's stdout/stderr live
docker logs -f commanddeck-api-1

# Show container resource usage
docker stats

# Show port mappings
docker port commanddeck-api-1
```

---

## Inspect Volumes

```bash
# List all volumes
docker volume ls | grep commanddeck

# Inspect a volume's metadata
docker volume inspect commanddeck_pgdata

# List contents of a volume (postgres data dir)
docker run --rm -v commanddeck_pgdata:/data alpine ls /data

# WARNING: Never run commands that modify volume contents unless diagnosing data loss
```

---

## Volume Safety Rules

> **NEVER run `docker compose down -v` against a production database.**
> Named volumes (`pgdata`, `redisdata`, `backend_uploads`) persist data across `down` and `up` cycles.
> Only use `-v` flag when you intentionally want to destroy all persistent data.

Safe operations that preserve volumes:
- `docker compose stop` — stops containers, keeps volumes
- `docker compose down` — removes containers and networks, **keeps volumes**
- `docker compose restart` — restarts containers, keeps volumes

Destructive operations that delete volumes:
- `docker compose down -v` — removes containers, networks, **and all named volumes**
- `docker volume rm <volume>` — permanently deletes the volume

If you accidentally delete `pgdata`:
1. Stop the stack: `docker compose down`
2. The volume may still exist; check: `docker volume ls | grep pgdata`
3. If the volume still exists, you can restart safely
4. If the volume is truly gone, you will need to restore from a backup or reinitialize the database

---

## Docker Hub Authentication

```bash
# Check if already logged in
docker info | grep -i Username
# Expected: Username: sleeper0

# Login using an access token (secure method — does not print token):
read -s DOCKER_TOKEN
echo "$DOCKER_TOKEN" | docker login -u sleeper0 --password-stdin
unset DOCKER_TOKEN

# Verify login:
docker pull alpine:latest

# Logout when done (not needed for push/pull, but good practice on shared machines)
docker logout
```

> **Never print or log access tokens.** Use Docker Hub access tokens (not your account password).

---

## Build, Tag, and Push Images to Docker Hub

```bash
# Set variables
export DOCKER_NAMESPACE="sleeper0"
export SHORT_SHA=$(git rev-parse --short HEAD)

# Build API image with three tags
docker build \
  -t "$DOCKER_NAMESPACE/commanddeck-api:dev" \
  -t "$DOCKER_NAMESPACE/commanddeck-api:latest" \
  -t "$DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA" \
  -f apps/api/Dockerfile \
  .

# Build Web image with three tags
docker build \
  -t "$DOCKER_NAMESPACE/commanddeck-web:dev" \
  -t "$DOCKER_NAMESPACE/commanddeck-web:latest" \
  -t "$DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA" \
  -f apps/web/Dockerfile \
  .

# Push API tags
docker push "$DOCKER_NAMESPACE/commanddeck-api:dev"
docker push "$DOCKER_NAMESPACE/commanddeck-api:latest"
docker push "$DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA"

# Push Web tags
docker push "$DOCKER_NAMESPACE/commanddeck-web:dev"
docker push "$DOCKER_NAMESPACE/commanddeck-web:latest"
docker push "$DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA"
```

## Pull Verification

After pushing, verify images can be pulled back from Docker Hub:

```bash
# Pull all API tags
docker pull "$DOCKER_NAMESPACE/commanddeck-api:dev"
docker pull "$DOCKER_NAMESPACE/commanddeck-api:latest"
docker pull "$DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA"

# Pull all Web tags
docker pull "$DOCKER_NAMESPACE/commanddeck-web:dev"
docker pull "$DOCKER_NAMESPACE/commanddeck-web:latest"
docker pull "$DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA"

# Verify each image is inspectable
docker image inspect "$DOCKER_NAMESPACE/commanddeck-api:dev" >/dev/null && echo "API_DEV_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-api:latest" >/dev/null && echo "API_LATEST_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA" >/dev/null && echo "API_SHA_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-web:dev" >/dev/null && echo "WEB_DEV_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-web:latest" >/dev/null && echo "WEB_LATEST_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA" >/dev/null && echo "WEB_SHA_PULL_OK"
```

## Registry-Based Deployment (compose.prod.yml)

Use `compose.prod.yml` to deploy from Docker Hub images instead of building locally:

```bash
# Pull and start from Docker Hub images using a specific tag
export COMMANDDECK_IMAGE_TAG="$SHORT_SHA"

docker compose -f compose.yml -f compose.prod.yml pull
docker compose -f compose.yml -f compose.prod.yml up -d
docker compose -f compose.yml -f compose.prod.yml ps

# Verify runtime
curl -i http://127.0.0.1:8080/health
curl -I http://127.0.0.1:3000

# To use :latest instead, omit or set COMMANDDECK_IMAGE_TAG=latest
```

`compose.prod.yml` overrides `commanddeck-api` and `commanddeck-web` services to pull pre-built images from Docker Hub instead of building from Dockerfiles. It uses `!reset null` on the `build` key to disable local builds.

---

## Docker Scout (Security Scanning)

> Requires Docker Scout to be enabled on your Docker Hub account.

```bash
# Check if Scout is available
docker scout version || true

# Quick overview of an image
docker scout quickview sleeper0/commanddeck-api:dev || true

# Show CVEs for an image
docker scout cves sleeper0/commanddeck-api:dev || true

# Compare two image versions
docker scout compare sleeper0/commanddeck-api:dev sleeper0/commanddeck-api:$SHORT_SHA || true
```

If Scout is not available, perform manual CVE checks:
1. Note the base image tags (alpine:3.21, node:22-alpine, golang:1.26-alpine)
2. Check Alpine security advisories: https://security.alpinelinux.org
3. Check Go vulnerability database: https://vuln.go.dev
4. Check Node.js security advisories: https://nodejs.org/en/advisories

---

## Docker Build Cloud / Cloud Builder

> Requires Docker Desktop with Build Cloud enabled (Docker Pro subscription).
> Cloud builders are tied to a specific Docker Desktop installation — they do NOT sync across machines.

### Prerequisites: Docker Desktop + WSL2 Integration

```bash
# Verify Docker CLI is reachable from WSL2
docker --version
docker compose version
docker buildx version

# Verify Docker Desktop WSL runtime
docker run --rm alpine echo "DOCKER_DESKTOP_WSL_RUNTIME_OK"

# Check current context
docker context show
```

If Docker daemon is unreachable from WSL2:
```
BLOCKED — Docker Desktop WSL integration not enabled for this distro.
Fix: Docker Desktop → Settings → Resources → WSL Integration → enable this distro → Apply & Restart
```

### List Builders

```bash
# List all builders and their drivers
docker buildx ls

# Expected output for cloud builder (name may vary):
# cloud-sleeper0-commanddeck-cloud   cloud  +sleeper0/commanddeck-cloud   running
# ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^  ^^^^^
# Name shown in docker buildx ls    Must be "cloud", not "docker" or "docker-container"
```

### Inspect a Cloud Builder

```bash
# Inspect cloud builder details and node status
docker buildx inspect cloud-sleeper0-commanddeck-cloud

# Bootstrap if builder exists but shows no nodes
docker buildx inspect cloud-sleeper0-commanddeck-cloud --bootstrap
```

### Create / Select Cloud Builder

```bash
# If cloud builder does NOT appear in docker buildx ls:
# Create it using the name from Docker Build Cloud dashboard
docker buildx create --driver cloud sleeper0/commanddeck-cloud --name cloud-sleeper0-commanddeck-cloud

# Then bootstrap
docker buildx inspect cloud-sleeper0-commanddeck-cloud --bootstrap

# Select the cloud builder (required before building)
docker buildx use cloud-sleeper0-commanddeck-cloud

# Verify selected builder
docker buildx ls
# Current builder should show: cloud-sleeper0-commanddeck-cloud   cloud
```

### What To Do If Cloud Builder Does Not Appear

Classification of failures and required action:

| Error / Symptom | Likely Cause | Fix |
|---|---|---|
| `docker buildx ls` shows no cloud builder | Builder not created in Docker Dashboard | Create builder at hub.docker.com/settings/builders |
| `docker: "buildx create" driver not available` | Docker Build Cloud not enabled | Upgrade to Docker Pro; enable Build Cloud |
| `docker: buildx create: unauthorized` | Not signed in to Docker Desktop as correct account | Docker Desktop → Sign in as correct Docker Hub account |
| `Error: driver name cloud is not supported` | Buildx cloud plugin missing | Reinstall Docker Desktop with Build Cloud plugin |
| Builder appears but nodes show `offline` | Builder needs bootstrap | `docker buildx inspect <name> --bootstrap` |

### Build API Image with Cloud Builder

```bash
# Set variables
export DOCKER_NAMESPACE="sleeper0"
export SHORT_SHA="$(git rev-parse --short HEAD)"
export CLOUD_BUILDER="cloud-sleeper0-commanddeck-cloud"

# Verify builder is selected
docker buildx use "$CLOUD_BUILDER"

# Build and push API image through cloud builder
docker buildx build \
  --builder "$CLOUD_BUILDER" \
  --platform linux/amd64 \
  -t "$DOCKER_NAMESPACE/commanddeck-api:cloud-dev" \
  -t "$DOCKER_NAMESPACE/commanddeck-api:cloud-$SHORT_SHA" \
  -f apps/api/Dockerfile \
  --push \
  .
```

### Build Web Image with Cloud Builder

```bash
# Build and push Web image through cloud builder
docker buildx build \
  --builder "$CLOUD_BUILDER" \
  --platform linux/amd64 \
  -t "$DOCKER_NAMESPACE/commanddeck-web:cloud-dev" \
  -t "$DOCKER_NAMESPACE/commanddeck-web:cloud-$SHORT_SHA" \
  -f apps/web/Dockerfile \
  --push \
  .
```

### Pull-Verify Cloud-Built Images

```bash
export DOCKER_NAMESPACE="sleeper0"
export SHORT_SHA="$(git rev-parse --short HEAD)"

# Pull all 4 cloud tags
docker pull "$DOCKER_NAMESPACE/commanddeck-api:cloud-dev"
docker pull "$DOCKER_NAMESPACE/commanddeck-api:cloud-$SHORT_SHA"
docker pull "$DOCKER_NAMESPACE/commanddeck-web:cloud-dev"
docker pull "$DOCKER_NAMESPACE/commanddeck-web:cloud-$SHORT_SHA"

# Inspect-verify each tag
docker image inspect "$DOCKER_NAMESPACE/commanddeck-api:cloud-dev" > /dev/null 2>&1 && echo "API_CLOUD_DEV_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-api:cloud-$SHORT_SHA" > /dev/null 2>&1 && echo "API_CLOUD_SHA_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-web:cloud-dev" > /dev/null 2>&1 && echo "WEB_CLOUD_DEV_PULL_OK"
docker image inspect "$DOCKER_NAMESPACE/commanddeck-web:cloud-$SHORT_SHA" > /dev/null 2>&1 && echo "WEB_CLOUD_SHA_PULL_OK"
```

### Safe Token Login (if needed)

```bash
# Read token securely — do NOT echo or print the token
read -s DOCKER_TOKEN
echo "$DOCKER_TOKEN" | docker login -u sleeper0 --password-stdin
unset DOCKER_TOKEN

# Verify login succeeded (token not echoed)
docker info | grep -i Username
```

---

## Full Deployment Sequence (VPS)

```bash
# 1. SSH to VPS as myles
ssh myles@<vps-ip>

# 2. Navigate to deployment directory
cd /opt/commanddeck

# 3. Pull latest images
docker compose pull

# 4. Validate compose config
docker compose config

# 5. Stop old containers
docker compose down

# 6. Start new containers
docker compose up -d

# 7. Verify all services running
docker compose ps

# 8. Check logs for errors
docker compose logs --tail=50

# 9. Verify health endpoints
curl http://localhost:8080/health
curl http://localhost:3000
```

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| `docker ps` permission denied | User not in docker group | `sudo usermod -aG docker $USER` then re-login |
| API returns 502 | Backend not connected to DB | Check `docker compose logs commanddeck-api` |
| Web returns 502 | Frontend can't reach API | Check `docker compose logs commanddeck-web`; verify `REMOTE_API_URL` env var |
| Postgres not healthy | DB not initialized | Wait 10s; check `docker exec commanddeck-db-1 pg_isready` |
| Redis not healthy | Redis not initialized | Wait 5s; check `docker exec commanddeck-redis-1 redis-cli ping` |
| Port already in use | Conflicting service | Edit compose.yml ports or stop conflicting service |
| Image pull fails | Not logged into Docker Hub | `docker login` then retry |
| Volume data missing | Accidental `down -v` | Restore from backup if available; otherwise reinitialize |

---

*Maintained by: Mr.R9 (Primary Builder), Mr.Commander (VPS Coordinator)*
*Last updated: 2026-05-23 (COMMANDDECK-CLOUD-BUILDER-RECOVERY-001)*
*File: docs/agent-brain/runbooks/DOCKER-RUNBOOK.md*