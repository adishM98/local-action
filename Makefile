WEB_DIR := web
WEB_SRC := $(shell find $(WEB_DIR)/src -type f) $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.js $(WEB_DIR)/index.html

.PHONY: bootstrap build run dev test lint fmt install db-reset clean release-macos package-macos-app

bootstrap:
	./scripts/bootstrap.sh

build: $(WEB_DIR)/dist/index.html
	go build -o local-action ./cmd/local-action

$(WEB_DIR)/dist/index.html: $(WEB_SRC)
	cd $(WEB_DIR) && npm install && npm run build

run: build
	./local-action

# Runs backend + frontend dev server together. Ctrl-C stops both. The trap
# is installed before backgrounding the backend, using single-quoted
# (deferred-expansion) syntax so `$BACKEND` is read at signal-time rather than
# trap-install-time — safe even though $BACKEND isn't set yet when the trap is
# installed. It's the only cleanup — no separate trailing `kill`, since the
# trap already fires on normal exit too.
dev:
	@trap 'kill $${BACKEND:-0} 2>/dev/null' EXIT INT TERM; \
	go run ./cmd/local-action & \
	BACKEND=$$!; \
	cd $(WEB_DIR) && npm run dev

test:
	go test ./...
	cd $(WEB_DIR) && npm test

lint:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "unformatted files; run 'make fmt'"; exit 1; }
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
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules build/

# make release-macos VERSION=0.1.0
release-macos:
	@test -n "$(VERSION)" || { echo "usage: make release-macos VERSION=0.1.0"; exit 1; }
	./scripts/release-macos.sh $(VERSION)

# make package-macos-app VERSION=0.1.0
package-macos-app:
	@test -n "$(VERSION)" || { echo "usage: make package-macos-app VERSION=0.1.0"; exit 1; }
	./scripts/package-macos-app.sh $(VERSION)
