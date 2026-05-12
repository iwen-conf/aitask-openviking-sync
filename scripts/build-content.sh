#!/usr/bin/env bash
# Called by `lzc-cli project build` as the buildscript declared in lzc-build.yml.
# Keep it idempotent and minimal — release.sh handles image build/push and
# secret rendering before invoking lzc-cli.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p content
[ -f content/.keep ] || touch content/.keep

if [ ! -f .lzc-manifest.rendered.yml ]; then
  echo "[build-content] missing .lzc-manifest.rendered.yml — run 'make release' first (renders secrets + image refs)." >&2
  exit 1
fi

if [ ! -f lzc-icon.png ]; then
  echo "[build-content] missing lzc-icon.png" >&2
  exit 1
fi

echo "[build-content] ok ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
