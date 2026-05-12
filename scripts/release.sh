#!/usr/bin/env bash
# AITask → Lazycat MicroServer release pipeline.
#
# Steps:
#   1. Build linux/amd64 images for backend, web, migrate.
#   2. Push to Docker Hub.
#   3. Bridge each image to registry.lazycat.cloud via `lzc-cli appstore copy-image`.
#      Also bridge base images used by manifest services (postgres, dragonfly).
#   4. Render lzc-manifest.yml → .lzc-manifest.rendered.yml with secrets + final image refs.
#   5. lzc-cli project build → aitask.lpk
#   6. lzc-cli app install ./aitask.lpk
#
# Usage:  bash scripts/release.sh [VERSION] [DOCKER_USER]
#   VERSION       defaults to v0.1.0
#   DOCKER_USER   defaults to iwen-conf

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-${VERSION:-v0.1.0}}"
DOCKER_USER="${2:-${DOCKER_USER:-iwen-conf}}"

SECRETS_FILE=".lzc-secrets.yml.local"
SOURCE_MANIFEST="lzc-manifest.yml"
RENDERED_MANIFEST=".lzc-manifest.rendered.yml"
LPK_OUT="aitask.lpk"

require() {
  command -v "$1" >/dev/null 2>&1 || { echo "[release] missing: $1" >&2; exit 1; }
}
require docker
require yq
require lzc-cli

[ -f "$SECRETS_FILE" ] || { echo "[release] missing $SECRETS_FILE — generate it first" >&2; exit 1; }

AGENT_TOKEN_SECRET=$(yq -r '.agent_token_secret' "$SECRETS_FILE")
OPENVIKING_SETTINGS_KEY=$(yq -r '.openviking_settings_key' "$SECRETS_FILE")
POSTGRES_PASSWORD=$(yq -r '.postgres_password' "$SECRETS_FILE")
DRAGONFLY_PASSWORD=$(yq -r '.dragonfly_password' "$SECRETS_FILE")

for v in AGENT_TOKEN_SECRET OPENVIKING_SETTINGS_KEY POSTGRES_PASSWORD DRAGONFLY_PASSWORD; do
  [ -n "${!v}" ] || { echo "[release] empty secret: $v" >&2; exit 1; }
done

if ! docker info >/dev/null 2>&1; then
  echo "[release] docker daemon not running. Start OrbStack/Docker Desktop first." >&2
  exit 1
fi

echo "==> [1/6] buildx amd64 images at $VERSION"
declare -a IMAGES=(
  "aitask-backend:./core"
  "aitask-openviking:./core"
  "aitask-web:./web"
  "aitask-migrate:./migrations"
)
for spec in "${IMAGES[@]}"; do
  name="${spec%%:*}"
  ctx="${spec#*:}"
  tag="docker.io/$DOCKER_USER/$name:$VERSION"
  echo "    building $tag from $ctx"
  docker buildx build --platform linux/amd64 -t "$tag" "$ctx" --load
done

echo "==> [2/6] docker push to Docker Hub"
for spec in "${IMAGES[@]}"; do
  name="${spec%%:*}"
  tag="docker.io/$DOCKER_USER/$name:$VERSION"
  docker push "$tag"
done

echo "==> [3/6] lzc-cli appstore copy-image to registry.lazycat.cloud"
copy_image() {
  local source="$1"
  local out
  out=$(lzc-cli appstore copy-image "$source" 2>&1)
  echo "$out" >&2
  echo "$out" | grep -oE 'registry\.lazycat\.cloud/[^ ]+' | tail -1
}

BACKEND_REF=$(copy_image "docker.io/$DOCKER_USER/aitask-backend:$VERSION")
OPENVIKING_REF=$(copy_image "docker.io/$DOCKER_USER/aitask-openviking:$VERSION")
WEB_REF=$(copy_image "docker.io/$DOCKER_USER/aitask-web:$VERSION")
MIGRATE_REF=$(copy_image "docker.io/$DOCKER_USER/aitask-migrate:$VERSION")
POSTGRES_REF=$(copy_image "docker.io/postgres:18.3")
DRAGONFLY_REF=$(copy_image "docker.dragonflydb.io/dragonflydb/dragonfly:v1.38.1")

for v in BACKEND_REF OPENVIKING_REF WEB_REF MIGRATE_REF POSTGRES_REF DRAGONFLY_REF; do
  [ -n "${!v}" ] || { echo "[release] copy-image did not return a registry.lazycat.cloud ref for $v — inspect the lzc-cli output above and patch this script's regex if the format changed." >&2; exit 1; }
done
echo "    backend  → $BACKEND_REF"
echo "    openviking → $OPENVIKING_REF"
echo "    web      → $WEB_REF"
echo "    migrate  → $MIGRATE_REF"
echo "    postgres → $POSTGRES_REF"
echo "    dragonfly → $DRAGONFLY_REF"

echo "==> [4/6] render manifest"
cp "$SOURCE_MANIFEST" "$RENDERED_MANIFEST"

# Use an alternate sed delimiter; secret values can contain '/', '+', '='.
render_replace() {
  local placeholder="$1"
  local value="$2"
  local escaped
  escaped=$(printf '%s' "$value" | sed -e 's/[\\&|]/\\&/g')
  sed -i.bak "s|$placeholder|$escaped|g" "$RENDERED_MANIFEST"
  rm -f "$RENDERED_MANIFEST.bak"
}

render_replace "__POSTGRES_PASSWORD__"        "$POSTGRES_PASSWORD"
render_replace "__DRAGONFLY_PASSWORD__"       "$DRAGONFLY_PASSWORD"
render_replace "__OPENVIKING_SETTINGS_KEY__"  "$OPENVIKING_SETTINGS_KEY"
render_replace "__AGENT_TOKEN_SECRET__"       "$AGENT_TOKEN_SECRET"
render_replace "__POSTGRES_IMAGE__"           "$POSTGRES_REF"
render_replace "__DRAGONFLY_IMAGE__"          "$DRAGONFLY_REF"

# Backwrite the registry.lazycat.cloud refs over the docker.io placeholders.
yq -i ".services.backend.image = \"$BACKEND_REF\""             "$RENDERED_MANIFEST"
yq -i ".services.openviking.image = \"$OPENVIKING_REF\""       "$RENDERED_MANIFEST"
yq -i ".services.web.image = \"$WEB_REF\""                     "$RENDERED_MANIFEST"
yq -i ".services[\"migrate-init\"].image = \"$MIGRATE_REF\""   "$RENDERED_MANIFEST"

if grep -qE '__[A-Z_]+__' "$RENDERED_MANIFEST"; then
  echo "[release] rendered manifest still has placeholders — aborting" >&2
  grep -nE '__[A-Z_]+__' "$RENDERED_MANIFEST" >&2 || true
  exit 1
fi

echo "==> [5/6] lzc-cli project build"
rm -f "$LPK_OUT"
lzc-cli project build -o "$LPK_OUT"

echo "==> [6/6] lzc-cli app install $LPK_OUT"
lzc-cli app install "./$LPK_OUT"

echo
echo "✓ release done"
echo "  app:      cloud.lazycat.app.aitask"
echo "  url:      https://aitask.$(lzc-cli box default 2>/dev/null || echo '<box>').heiyu.space"
echo "  version:  $VERSION"
