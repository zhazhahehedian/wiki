#!/usr/bin/env bash
# Build the company offline release on a workstation with Docker.
# Usage: bash deploy/company/build-release.sh [output-root]
# Output: <output-root>/capability-hub-<TAG>/ with images, SHA256SUMS, TAG and upgrade.sh.
# Default output root is ../release next to the repository (never inside it).
set -euo pipefail

repo=$(cd "$(dirname -- "$0")/../.." && pwd)
out_root=${1:-"$repo/../release"}
cd "$repo"

if ! command -v docker >/dev/null 2>&1; then
  desktop="${LOCALAPPDATA:-}/Programs/DockerDesktop/resources/bin"
  [[ -n "${LOCALAPPDATA:-}" ]] && desktop=$(cygpath -u "$desktop" 2>/dev/null || printf '%s' "$desktop")
  [[ -x "$desktop/docker.exe" || -x "$desktop/docker" ]] || { echo "docker not found" >&2; exit 1; }
  export PATH="$desktop:$PATH"
fi
docker version --format '{{.Server.Os}}' >/dev/null || { echo "Docker engine is not running" >&2; exit 1; }

sha=$(git rev-parse --short HEAD)
if [[ -n "$(git status --porcelain)" ]]; then
  [[ "${ALLOW_DIRTY:-}" == 1 ]] || { echo "Working tree has uncommitted changes; commit first (or ALLOW_DIRTY=1 for a test build)." >&2; exit 1; }
  sha="$sha-dirty"
fi
tag="$(date +%Y%m%d)-$sha"
backend="capability-hub-backend:$tag"
frontend="capability-hub-frontend:$tag"
echo "== Building $tag (Docker Desktop must reach Docker Hub; enable the system proxy if needed)"

if [[ "${SKIP_BUILD:-}" == 1 ]]; then
  # Re-export images already built for this commit (e.g. after a failed export).
  docker image inspect "$backend" "$frontend" >/dev/null
else
  docker build --platform linux/amd64 -t "$backend" backend
  docker build --platform linux/amd64 --build-arg NEXT_PUBLIC_API_BASE_URL= -t "$frontend" frontend
fi

echo "== Smoke test: migrate an empty database and serve /healthz"
net="caphub-release-smoke-$$"
cleanup() { docker rm -f -v "$net-pg" "$net-minio" "$net-backend" "$net-frontend" >/dev/null 2>&1 || true; docker network rm "$net" >/dev/null 2>&1 || true; }
trap cleanup EXIT
rand() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }
pw=$(rand 16); mk=$(rand 12); ms=$(rand 24); key=$(rand 16)
docker network create "$net" >/dev/null
docker run -d --name "$net-pg" --network "$net" -e POSTGRES_PASSWORD="$pw" pgvector/pgvector:0.8.2-pg18 >/dev/null
docker run -d --name "$net-minio" --network "$net" -e MINIO_ROOT_USER="$mk" -e MINIO_ROOT_PASSWORD="$ms" bitnamilegacy/minio:2025.7.23 >/dev/null
for _ in $(seq 1 60); do docker exec "$net-pg" pg_isready -U postgres -q 2>/dev/null && break; sleep 1; done
sleep 5
docker run -d --name "$net-backend" --network "$net" \
  -e DATABASE_URL="postgres://postgres:$pw@$net-pg:5432/postgres?sslmode=disable" \
  -e S3_ENDPOINT="http://$net-minio:9000" -e S3_ACCESS_KEY="$mk" -e S3_SECRET_KEY="$ms" -e S3_BUCKET=smoke \
  -e EMBEDDING_DIM=1024 -e OAUTH_ENCRYPTION_KEY="$key" -e FEISHU_APP_ID=cli_smoke -e FEISHU_APP_SECRET=smoke \
  -e FEISHU_REDIRECT_URL=http://127.0.0.1/api/v1/auth/feishu/callback -e FRONTEND_ORIGIN=http://127.0.0.1 \
  -e FEISHU_NOTIFY_ENABLED=true "$backend" >/dev/null
docker run -d --name "$net-frontend" --network "$net" "$frontend" >/dev/null
ok=false
for _ in $(seq 1 60); do
  if docker exec "$net-backend" wget -q -O - http://127.0.0.1:8080/healthz >/dev/null 2>&1; then ok=true; break; fi
  sleep 2
done
"$ok" || { docker logs "$net-backend" 2>&1 | tail -20; echo "backend smoke failed" >&2; exit 1; }
version=$(docker exec "$net-pg" psql -U postgres -Atc "select max(version_id) from goose_db_version where is_applied")
docker exec "$net-frontend" wget -q -O - http://127.0.0.1:3000/ >/dev/null || { echo "frontend smoke failed" >&2; exit 1; }
echo "   backend healthy, migrated to $version; frontend serving"
cleanup
trap - EXIT

dir="$out_root/capability-hub-$tag"
mkdir -p "$dir"
echo "== Exporting to $dir"
docker save "$backend" "$frontend" | gzip -6 > "$dir/capability-hub-images-$tag.tar.gz"
printf '%s\n' "$tag" > "$dir/TAG"
printf '%s\n' "$version" > "$dir/MIGRATION_VERSION"
sed 's/\r$//' < "$repo/deploy/company/upgrade.sh" > "$dir/upgrade.sh"  # strip CR: Windows checkouts may add CRLF
(cd "$dir" && sha256sum "capability-hub-images-$tag.tar.gz" TAG MIGRATION_VERSION upgrade.sh | sed 's/ \*/  /' > SHA256SUMS)
ls -lh "$dir"
echo "== Done. Upload the whole directory to the server and run: sudo bash upgrade.sh"
