.PHONY: install ui engine build run dev test check clean extension check-extension test-extension release test-native configure-codex test-codex test-claude
ROOT := $(CURDIR)
REPO ?= $(ROOT)/examples/sandbox
PORT ?= 4820

install:
	cd ui && npm ci
	cd daemon && go mod download

ui:
	cd ui && npm run build
	python3 scripts/embed-ui.py

engine: ui
	mkdir -p bin
	cd daemon && go build -o ../bin/stef .
	cp bin/stef bin/stefd

build: engine

run: build
	./bin/stef --repo "$(REPO)" --port "$(PORT)"

dev:
	bash scripts/dev.sh "$(REPO)" "$(PORT)"

test: engine
	cd daemon && go test -race ./...
	cd daemon && go test -race ../examples/sandbox/main.go ../examples/sandbox/main_test.go
	cd ui && npm test

check:
	cd daemon && go vet ./...
	cd ui && npm run check

clean:
	rm -rf bin ui/dist ui/test-results ui/playwright-report

extension: build
	cd integrations/vscode && npm ci
	cd integrations/vscode && npm run compile
	python3 scripts/package-extension.py
	cd integrations/vscode && npm run package

check-extension:
	cd integrations/vscode && npx tsc --noEmit

test-extension:
	cd integrations/vscode && npm run compile && npm test

test-native: build
	python3 scripts/smoke-native.py

release: build
	python3 scripts/package-release.py

configure-codex: build
	./bin/stef --setup-codex --repo "$(REPO)" --daemon "http://127.0.0.1:$(PORT)"

# Opt-in integration: this invokes the real Codex CLI using existing authentication.
test-codex: build
	python3 scripts/smoke-codex.py

# Opt-in integration: this invokes the real Claude Code CLI using existing authentication.
test-claude: build
	python3 scripts/smoke-claude.py
