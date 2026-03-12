#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${AGENTHUB_REPO_URL:-https://github.com/trevorprater/agenthub}"
AGENTHUB_DIR="${AGENTHUB_DIR:-/tmp/agenthub}"
PLAYGROUND_ROOT="${AGENTHUB_PLAYGROUND_DIR:-$(mktemp -d /tmp/agenthub-playground.XXXXXX)}"
BIN_DIR="$PLAYGROUND_ROOT/bin"
DATA_DIR="${AGENTHUB_DATA_DIR:-$PLAYGROUND_ROOT/data}"
HOME_DIR="$PLAYGROUND_ROOT/home"
REPO_WORKTREE="$PLAYGROUND_ROOT/repo"
SERVER_LOG="$PLAYGROUND_ROOT/server.log"
PORT="${AGENTHUB_PORT:-18080}"
SERVER_URL="${AGENTHUB_SERVER_URL:-http://127.0.0.1:$PORT}"
ADMIN_KEY="${AGENTHUB_ADMIN_KEY:-dev-admin-key}"
AGENT_ID="${AGENTHUB_AGENT_ID:-golem-playground}"
SERVER_BIN="$BIN_DIR/agenthub-server"
CLI_BIN="$BIN_DIR/ah"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

mkdir -p "$BIN_DIR" "$DATA_DIR" "$HOME_DIR" "$REPO_WORKTREE"

if [[ ! -d "$AGENTHUB_DIR/.git" ]]; then
  git clone --depth=1 "$REPO_URL" "$AGENTHUB_DIR"
fi

(
  cd "$AGENTHUB_DIR"
  go build -o "$SERVER_BIN" ./cmd/agenthub-server
  go build -o "$CLI_BIN" ./cmd/ah
)

"$SERVER_BIN" --listen ":$PORT" --admin-key "$ADMIN_KEY" --data "$DATA_DIR" >"$SERVER_LOG" 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 40); do
  if curl -fsS "$SERVER_URL/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
curl -fsS "$SERVER_URL/api/health" >/dev/null

export HOME="$HOME_DIR"
"$CLI_BIN" join --server "$SERVER_URL" --name "$AGENT_ID" --admin-key "$ADMIN_KEY"

if [[ ! -d "$REPO_WORKTREE/.git" ]]; then
  (
    cd "$REPO_WORKTREE"
    git init -b main
    git config user.name "Golem Playground"
    git config user.email "golem-playground@example.com"
  )
fi

(
  cd "$REPO_WORKTREE"
  cat > README.md <<'EOF'
# agenthub playground

This repo was created by scripts/agenthub-playground.sh to exercise a local AgentHub server.
EOF
  git add README.md
  if git diff --cached --quiet; then
    :
  else
    git commit -m "seed playground repo"
  fi

  HEAD_HASH="$(git rev-parse HEAD)"
  "$CLI_BIN" push
  echo
  echo "== Commit graph =="
  "$CLI_BIN" log --limit 5
  echo
  echo "== Leaves =="
  "$CLI_BIN" leaves
  echo
  echo "== Lineage =="
  "$CLI_BIN" lineage "$HEAD_HASH"
)

echo
echo "AgentHub playground is up"
echo "  server:   $SERVER_URL"
echo "  data dir: $DATA_DIR"
echo "  home dir: $HOME_DIR"
echo "  repo dir: $REPO_WORKTREE"
echo "  log:      $SERVER_LOG"
