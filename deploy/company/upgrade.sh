#!/usr/bin/env bash
# Upgrade the company offline installation to the images in this release directory.
# Run as root from the uploaded release directory:
#   sudo bash upgrade.sh [install-dir]
# Default install dir: /opt/capability-hub-company-20261101.
# Steps: checksum -> load -> back up database and config -> switch image tags ->
# disposable migration check -> start backend -> refresh base services -> report.
# Nothing is deleted; on failure before the backend starts, the old config is restored.
set -euo pipefail

export PATH=/usr/local/bin:$PATH
release=$(cd "$(dirname -- "$0")" && pwd)
install=${1:-/opt/capability-hub-company-20261101}
backups=/opt/capability-hub-backups
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || fail "run as root (sudo bash upgrade.sh)"
[[ -f "$install/config/compose.env" && -f "$install/scripts/start-backend.sh" ]] || fail "install dir not found: $install"
command -v docker >/dev/null || fail "docker not found in /usr/local/bin"

cd "$release"
sha256sum -c SHA256SUMS
tag=$(cat TAG)
expected=$(cat MIGRATION_VERSION)
backend="capability-hub-backend:$tag"
frontend="capability-hub-frontend:$tag"

echo "== Loading images $tag"
docker load -i "capability-hub-images-$tag.tar.gz"
docker image inspect "$backend" "$frontend" >/dev/null

cd "$install"
pg=$(docker ps --filter label=com.docker.compose.service=postgres --filter "label=com.docker.compose.project.working_dir=$install" --format '{{.Names}}' | head -1)
[[ -n "$pg" ]] || fail "postgres container of $install is not running"
current=$(docker exec "$pg" psql -U hub_bootstrap -d capability_hub -Atc "select max(version_id) from goose_db_version where is_applied")
old_backend=$(grep '^BACKEND_IMAGE=' config/compose.env | cut -d= -f2-)
echo "== Current: database version $current, backend $old_backend; target: $tag (migrates to $expected)"
[[ "$old_backend" != "$backend" ]] || fail "already running $backend"
if [[ "${ASSUME_YES:-}" != 1 ]]; then
  read -r -p "Continue? [y/N] " answer
  [[ "$answer" == y || "$answer" == Y ]] || fail "aborted"
fi

stamp=$(date +%Y%m%d%H%M%S)
mkdir -p -m 700 "$backups"
dump="$backups/capability_hub-before-$tag-$stamp.dump"
echo "== Backing up database to $dump"
docker exec "$pg" pg_dump -U hub_bootstrap -Fc capability_hub > "$dump"
[[ -s "$dump" ]] || fail "empty database backup"
toc=$(docker exec -i "$pg" pg_restore -l < "$dump") || fail "backup is not readable"
[[ "$toc" == *"dbname: capability_hub"* ]] || fail "backup is not a capability_hub dump"
chmod 600 "$dump"
cp -p config/compose.env "config/compose.env.before-$tag-$stamp"
[[ -f config/migration-verified ]] && cp -p config/migration-verified "config/migration-verified.before-$tag-$stamp"

restore_config() {
  cp -p "config/compose.env.before-$tag-$stamp" config/compose.env
  if [[ -f "config/migration-verified.before-$tag-$stamp" ]]; then
    cp -p "config/migration-verified.before-$tag-$stamp" config/migration-verified
  fi
  echo "Config restored; the application database was not changed." >&2
}

sed -i "s|^BACKEND_IMAGE=.*|BACKEND_IMAGE=$backend|; s|^FRONTEND_IMAGE=.*|FRONTEND_IMAGE=$frontend|" config/compose.env
grep -n '_IMAGE=' config/compose.env

echo "== Verifying migrations on a disposable database"
if ! bash scripts/verify-migrations.sh; then
  restore_config
  fail "migration check failed"
fi

echo "== Starting backend (migrates the application database)"
if ! bash scripts/start-backend.sh; then
  echo "Backend failed to start. The database may already be migrated." >&2
  echo "Do NOT start the old image on a migrated database. Logs: docker logs <backend container>" >&2
  echo "Rollback: restore $dump with pg_restore, then config/compose.env.before-$tag-$stamp." >&2
  exit 1
fi
echo "== Refreshing frontend and gateway"
bash scripts/start-base.sh

now=$(docker exec "$pg" psql -U hub_bootstrap -d capability_hub -Atc "select max(version_id) from goose_db_version where is_applied")
docker ps --filter "label=com.docker.compose.project.working_dir=$install" --format '{{.Names}}  {{.Image}}  {{.Status}}'
bind=$(grep '^HUB_BIND_ADDRESS=' config/compose.env | cut -d= -f2- || true)
port=$(grep '^HUB_HTTP_PORT=' config/compose.env | cut -d= -f2- || true)
if [[ -n "$bind" && -n "$port" ]]; then
  for path in / /healthz; do
    printf '%s %s\n' "$path" "$(curl -s -o /dev/null -m 10 -w '%{http_code}' "http://$bind:$port$path" || true)"
  done
fi
[[ "$now" == "$expected" ]] || fail "database version is $now, expected $expected"
echo "== Upgraded to $tag; database version $now. Backup: $dump"
