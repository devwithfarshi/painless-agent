#!/usr/bin/env bash
# Seed the database with sample tasks and skills for development / demo purposes.
# Safe to run multiple times — upserts on name where applicable.
#
# Usage: ./infra/scripts/seed.sh
#
# Env vars:
#   DATABASE_URL  — required

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
  exit 1
fi

if ! command -v psql &>/dev/null; then
  echo "ERROR: psql is not installed. Install postgresql-client and retry." >&2
  exit 1
fi

echo "Seeding database …"

psql "$DATABASE_URL" <<'SQL'
-- Demo tasks
INSERT INTO tasks (goal, status) VALUES
  ('Research the latest Go 1.26 release notes and summarise key changes', 'pending'),
  ('Write a hello-world HTTP server in Python and execute it', 'pending')
ON CONFLICT DO NOTHING;

-- Demo skill: generic web research workflow
INSERT INTO skills (name, description, steps_json, usage_count)
VALUES (
  'web-research',
  'Search the web for information, browse key pages, and produce a structured summary.',
  '[
    {"description": "Use web_search to find the top 5 results for the query", "tool_hint": "web_search"},
    {"description": "Visit the most relevant URL and extract the main content", "tool_hint": "browser"},
    {"description": "Summarise the findings into a concise report", "tool_hint": "summarizer"}
  ]',
  0
)
ON CONFLICT (name) DO NOTHING;
SQL

echo "Seed complete."
