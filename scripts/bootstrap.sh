#!/usr/bin/env bash
# Installs everything local-action needs to build/run from source: Go, Node,
# and act. Safe to re-run — skips anything already present at a good-enough
# version. Docker is checked but never auto-installed (it's a GUI app with
# its own licensing/setup flow — not something a script should silently do).
set -euo pipefail

REQUIRED_GO_MAJOR=1
REQUIRED_GO_MINOR=25

have() { command -v "$1" >/dev/null 2>&1; }

go_version_ok() {
  have go || return 1
  local v major minor
  v=$(go version | awk '{print $3}' | sed 's/^go//')
  major=${v%%.*}
  minor=$(echo "$v" | cut -d. -f2)
  [ "$major" -gt "$REQUIRED_GO_MAJOR" ] || { [ "$major" -eq "$REQUIRED_GO_MAJOR" ] && [ "$minor" -ge "$REQUIRED_GO_MINOR" ]; }
}

echo "==> Checking local-action's build dependencies"

if go_version_ok; then
  echo "  go: OK ($(go version | awk '{print $3}'))"
else
  echo "  go: installing (need ${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}+)…"
  if have brew; then
    brew install go
  elif have apt-get; then
    sudo apt-get update && sudo apt-get install -y golang-go
  else
    echo "  go: no supported package manager (brew/apt-get) found." >&2
    echo "      Install Go ${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}+ manually: https://go.dev/dl/" >&2
    exit 1
  fi
  go_version_ok || {
    echo "  go: still not ${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}+ after install — your package manager's version may be too old." >&2
    echo "      Install manually: https://go.dev/dl/" >&2
    exit 1
  }
fi

if have node && have npm; then
  echo "  node: OK ($(node --version))"
else
  echo "  node: installing…"
  if have brew; then
    brew install node
  elif have apt-get; then
    sudo apt-get update && sudo apt-get install -y nodejs npm
  else
    echo "  node: no supported package manager (brew/apt-get) found." >&2
    echo "        Install manually: https://nodejs.org/" >&2
    exit 1
  fi
fi

if have act; then
  echo "  act: OK ($(act --version))"
else
  echo "  act: installing…"
  if have brew; then
    brew install act
  else
    # act's own official installer — handles OS/arch detection itself,
    # no point in reimplementing that here.
    curl -fsSL https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash -s -- -b /usr/local/bin
  fi
fi

if have docker && docker info >/dev/null 2>&1; then
  echo "  docker: OK (running)"
else
  echo "  docker: not installed or not running." >&2
  echo "          Install and start Docker Desktop, then re-run this script: https://docs.docker.com/get-docker/" >&2
  exit 1
fi

echo "==> All set. Run: make run"
