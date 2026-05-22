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
# 1. Edit compose.yml: change image tag from :dev to :<previous-sha>
#    e.g., image: mtvalines/commanddeck-api:24a5909

# 2. Pull the pinned image
docker compose pull

# 3. Recreate containers with the older image
docker compose up -d --force-recreate

# To revert back to :dev, undo the tag change and repeat steps 2–3.
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

# Login to Docker Hub (interactive — paste token when prompted)
docker login

# After login, verify:
docker pull alpine:latest

# Logout when done (not needed for push/pull, but good practice on shared machines)
docker logout
```

> **Never print or log access tokens.** Use Docker Hub access tokens (not your account password).

---

## Build and Push Images

```bash
# Set variables
export DOCKER_NAMESPACE="mtvalines"    # confirm this with Myles
export SHORT_SHA=$(git rev-parse --short HEAD)

# Build API image with two tags
docker build -t $DOCKER_NAMESPACE/commanddeck-api:dev \
             -t $DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA \
             -f apps/api/Dockerfile .

# Build Web image with two tags
docker build -t $DOCKER_NAMESPACE/commanddeck-web:dev \
             -t $DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA \
             -f apps/web/Dockerfile .

# Push all four tags
docker push $DOCKER_NAMESPACE/commanddeck-api:dev
docker push $DOCKER_NAMESPACE/commanddeck-api:$SHORT_SHA
docker push $DOCKER_NAMESPACE/commanddeck-web:dev
docker push $DOCKER_NAMESPACE/commanddeck-web:$SHORT_SHA
```

---

## Docker Scout (Security Scanning)

> Requires Docker Scout to be enabled on your Docker Hub account.

```bash
# Check if Scout is available
docker scout version || true

# Quick overview of an image
docker scout quickview mtvalines/commanddeck-api:dev || true

# Show CVEs for an image
docker scout cves mtvalines/commanddeck-api:dev || true

# Compare two image versions
docker scout compare mtvalines/commanddeck-api:dev mtvalines/commanddeck-api:$SHORT_SHA || true
```

If Scout is not available, perform manual CVE checks:
1. Note the base image tags (alpine:3.21, node:22-alpine, golang:1.26-alpine)
2. Check Alpine security advisories: https://security.alpinelinux.org
3. Check Go vulnerability database: https://vuln.go.dev
4. Check Node.js security advisories: https://nodejs.org/en/advisories

---

## Buildx / Cloud Builder

> Requires Docker Pro with Build Cloud enabled.

```bash
# List available builders
docker buildx ls

# Create a cloud builder (replace ORG/BUILDER_NAME with actual values)
docker buildx create --driver cloud ORG/BUILDER_NAME

# Use cloud builder to build and push
docker buildx build \
  --builder cloud-ORG-BUILDER_NAME \
  --platform linux/amd64 \
  -t $DOCKER_NAMESPACE/commanddeck-api:dev \
  --push \
  -f apps/api/Dockerfile .

# Inspect cloud builder performance
docker buildx inspect --builder cloud-ORG-BUILDER_NAME
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

*Maintained by: Mr.R9 (Primary Builder)*
*Last updated: 2026-05-21*
*File: docs/agent-brain/runbooks/DOCKER-RUNBOOK.md*