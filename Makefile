WEB_DIR := cmd/local-action/web
WEB_SRC := $(shell find $(WEB_DIR)/src -type f) $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.js $(WEB_DIR)/index.html

.PHONY: build run dev test fmt clean

build: $(WEB_DIR)/dist/index.html
	go build -o local-action ./cmd/local-action

$(WEB_DIR)/dist/index.html: $(WEB_SRC)
	cd $(WEB_DIR) && npm install && npm run build

run: build
	./local-action

# Runs backend + frontend dev server together. Ctrl-C stops both.
dev:
	@go run ./cmd/local-action & \
	BACKEND=$$!; \
	trap "kill $$BACKEND 2>/dev/null" EXIT INT TERM; \
	(cd $(WEB_DIR) && npm run dev); \
	kill $$BACKEND 2>/dev/null

test:
	go test ./...

fmt:
	gofmt -l .
	go vet ./...

clean:
	rm -f local-action local-action.db
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules
