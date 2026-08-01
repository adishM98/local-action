WEB_DIR := web
WEB_SRC := $(shell find $(WEB_DIR)/src -type f) $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.js $(WEB_DIR)/index.html

.PHONY: build run dev test lint fmt install db-reset clean

build: $(WEB_DIR)/dist/index.html
	go build -o local-action ./cmd/local-action

$(WEB_DIR)/dist/index.html: $(WEB_SRC)
	cd $(WEB_DIR) && npm install && npm run build

run: build
	./local-action

# Runs backend + frontend dev server together. Ctrl-C stops both. The trap
# is installed immediately after backgrounding the backend (before the
# long-running `npm run dev` starts), and is the only cleanup — no separate
# trailing `kill`, since the trap already fires on normal exit too.
dev:
	@go run ./cmd/local-action & \
	BACKEND=$$!; \
	trap "kill $$BACKEND 2>/dev/null" EXIT INT TERM; \
	cd $(WEB_DIR) && npm run dev

test:
	go test ./...
	cd $(WEB_DIR) && npm test

lint:
	gofmt -l .
	go vet ./...

fmt:
	gofmt -w .

install:
	go mod download
	cd $(WEB_DIR) && npm install

db-reset:
	rm -f local-action.db

clean:
	rm -f local-action local-action.db
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules
