.PHONY: help dev build server server-turn dev-turn webui-dev test e2e-docker

WORKER_BASE_URL ?= https://turn.runbok.com
TMUX_SOCKET ?=
WEBUI_DEV_PROXY_URL ?= http://127.0.0.1:15173
WEBUI_DIST_DIR ?= ../webui/dist

help:
	@echo "Muxt Commands:"
	@echo "  make dev         - Run backend in local mode with air (proxy -> 15173)"
	@echo "  make webui-dev   - Run Vite dev server on 127.0.0.1:15173"
	@echo "  make build   - Build CLI binary to ./tmp/termteam"
	@echo "  make server      - Run backend once in local mode (no hot reload)"
	@echo "  make server-turn - Run backend once in turn mode"
	@echo "  make dev-turn    - Run backend with air in turn mode"
	@echo "  make test    - Run CLI unit tests"
	@echo "  make e2e-docker  - Run full local e2e chain in docker compose"
	@echo ""
	@echo "Environment overrides:"
	@echo "  WORKER_BASE_URL=<url>  (default: https://turn.runbok.com)"
	@echo "  TMUX_SOCKET=<name>     (default: empty)"
	@echo "  WEBUI_DEV_PROXY_URL=<url> (default: http://127.0.0.1:15173)"
	@echo "  WEBUI_DIST_DIR=<dir>      (default: ../webui/dist)"
	@echo "  TERMTEAM_LOCAL_HOST=<host> (default for make dev: 0.0.0.0)"

build:
	@mkdir -p tmp
	cd cli && go build -o ../tmp/termteam ./cmd/termteam

server: build
	TERMTEAM_MODE=local \
	TERMTEAM_WEBUI_MODE=dev \
	TERMTEAM_WEBUI_DEV_PROXY_URL="$(WEBUI_DEV_PROXY_URL)" \
	TERMTEAM_WEBUI_DIST_DIR="$(WEBUI_DIST_DIR)" \
	TERMTEAM_TMUX_SOCKET="$(TMUX_SOCKET)" \
	./tmp/termteam

server-turn: build
	TERMTEAM_MODE=turn \
	TERMTEAM_WORKER_BASE_URL="$(WORKER_BASE_URL)" \
	TERMTEAM_TMUX_SOCKET="$(TMUX_SOCKET)" \
	./tmp/termteam

dev:
	@command -v air >/dev/null 2>&1 || (echo "air not found. Install: go install github.com/air-verse/air@latest" && exit 1)
	TERMTEAM_MODE=local \
	TERMTEAM_LOCAL_HOST="$${TERMTEAM_LOCAL_HOST:-0.0.0.0}" \
	TERMTEAM_WEBUI_MODE=dev \
	TERMTEAM_WEBUI_DEV_PROXY_URL="$(WEBUI_DEV_PROXY_URL)" \
	TERMTEAM_WEBUI_DIST_DIR="$(WEBUI_DIST_DIR)" \
	TERMTEAM_TMUX_SOCKET="$(TMUX_SOCKET)" \
	air -c .air.toml

dev-turn:
	@command -v air >/dev/null 2>&1 || (echo "air not found. Install: go install github.com/air-verse/air@latest" && exit 1)
	TERMTEAM_MODE=turn \
	TERMTEAM_WORKER_BASE_URL="$(WORKER_BASE_URL)" \
	TERMTEAM_TMUX_SOCKET="$(TMUX_SOCKET)" \
	air -c .air.toml

webui-dev:
	cd webui && npm run dev -- --host 127.0.0.1 --port 15173

test:
	cd cli && go test ./...

e2e-docker:
	@mkdir -p logs
	docker compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-runner
