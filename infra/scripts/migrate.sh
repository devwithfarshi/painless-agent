#!/usr/bin/env bash
# Run database migrations via the embedded goose runner inside cmd/server.
# Migrations are applied automatically on server startup; this script provides a
# standalone way to run them (CI, one-off deploys, etc.).
#
# Usage: ./infra/scripts/migrate.sh [--down]
#
# Env vars:
#   DATABASE_URL  — required (e.g. postgres://postgres:postgres@localhost:2345/painless-agent)
#   BACKEND_DIR   — path to the backend/ directory (default: repo root / backend)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="${BACKEND_DIR:-$REPO_ROOT/backend}"

if [[ -z "${DATABASE_URL:-}" ]]; then
  ENV_FILE="$BACKEND_DIR/.env"
  if [[ -f "$ENV_FILE" ]]; then
    set -o allexport
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +o allexport
  fi
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "ERROR: DATABASE_URL is not set." >&2
  echo "  Either export it or add it to backend/.env" >&2
  exit 1
fi

# Use goose directly if available; otherwise fall back to go run.
if command -v goose &>/dev/null; then
  DIRECTION="${1:-up}"
  if [[ "$DIRECTION" == "--down" ]]; then
    echo "Running goose down …"
    goose -dir "$BACKEND_DIR/migrations" postgres "$DATABASE_URL" down
  else
    echo "Running goose up …"
    goose -dir "$BACKEND_DIR/migrations" postgres "$DATABASE_URL" up
  fi
else
  echo "goose binary not found — running migrations via server startup."
  echo "Start the server once and it will auto-apply all pending migrations."
  echo "  make -C '$BACKEND_DIR' run"
fi
