SHELL := /bin/bash
.DEFAULT_GOAL := help

# Local overrides (never committed). Exported so `go run`, `uv run`, and `npm` see them.
-include .env
export

CP  := controlplane
ML  := ml
WEB := web

PG_URL        ?= postgres://tensora:tensora@localhost:5432/tensora?sslmode=disable
GOLANGCI_LINT ?= golangci-lint
# CRD schemas (KubeVirt, KServe, Gateway API, cert-manager, ...) from the community catalog.
CRD_SCHEMAS   := https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json
KUBECONFORM   := kubeconform -strict -summary -ignore-missing-schemas -schema-location default -schema-location '$(CRD_SCHEMAS)'
IMAGE_REPO    ?= ghcr.io/your-org
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- setup
.PHONY: setup
setup: ## Install dev dependencies for all components
	cd $(CP) && go mod download
	cd $(ML) && uv sync
	cd $(WEB) && npm install
	-pre-commit install

# ---------------------------------------------------------------- local dev
.PHONY: dev dev-pg web-dev migrate
dev: ## All-in-one local stack: API+controller+usage accounting, in-memory store, fake backend (:8080; macOS or Linux)
	cd $(CP) && go run ./cmd/tensora dev

dev-pg: ## Same as dev, but against Postgres in Docker
	docker compose up -d --wait postgres
	cd $(CP) && TENSORA_DATABASE_URL='$(PG_URL)' go run ./cmd/tensora migrate
	cd $(CP) && TENSORA_DATABASE_URL='$(PG_URL)' go run ./cmd/tensora dev

web-dev: ## Dashboard dev server on :5173 (proxies /v1 to :8080)
	cd $(WEB) && npm run dev

migrate: ## Apply DB migrations to $$TENSORA_DATABASE_URL
	cd $(CP) && go run ./cmd/tensora migrate

# ---------------------------------------------------------------- test
.PHONY: test test-cp test-cp-pg test-ml test-web
test: test-cp test-ml test-web ## Run all tests

test-cp: ## Go tests (in-memory store; Postgres tests skip without TENSORA_TEST_DATABASE_URL)
	cd $(CP) && go test ./...

test-cp-pg: ## Go tests including Postgres store tests
	docker compose up -d --wait postgres
	cd $(CP) && TENSORA_TEST_DATABASE_URL='$(PG_URL)' go test ./...

test-ml: ## Python ML layer tests
	cd $(ML) && uv run pytest

test-web: ## Dashboard tests
	cd $(WEB) && npm test

# ---------------------------------------------------------------- lint / format
.PHONY: lint lint-cp lint-ml lint-web lint-yaml fmt
lint: lint-cp lint-ml lint-web lint-yaml ## Run all linters

lint-cp:
	cd $(CP) && test -z "$$(gofmt -l .)" || (gofmt -l . && echo 'run: make fmt' && exit 1)
	cd $(CP) && go vet ./...
	cd $(CP) && $(GOLANGCI_LINT) run

lint-ml:
	cd $(ML) && uv run ruff check . && uv run ruff format --check . && uv run mypy

lint-web:
	cd $(WEB) && npm run typecheck

lint-yaml:
	uvx yamllint -c .yamllint.yaml .

fmt: ## Format all code
	cd $(CP) && gofmt -w .
	cd $(ML) && uv run ruff format . && uv run ruff check --fix .

# ---------------------------------------------------------------- deploy (render only, never applies)
.PHONY: deploy-check
deploy-check: ## Lint + render charts and validate with kubeconform (no cluster needed)
	helm lint deploy/charts/tensora --set-file catalog=images/catalog.yaml
	helm lint deploy/charts/cluster-config -f deploy/environments/single-node.yaml
	helm template tensora deploy/charts/tensora --set-file catalog=images/catalog.yaml | $(KUBECONFORM)
	for env in single-node production; do \
	  helm template cluster-config deploy/charts/cluster-config -f deploy/environments/$$env.yaml | $(KUBECONFORM) || exit 1; \
	done
	$(KUBECONFORM) deploy/examples

# ---------------------------------------------------------------- cross-platform builds (AGENTS.md rule 11)
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64

.PHONY: build-all
build-all: ## Cross-compile tensora for linux/amd64, linux/arm64, darwin/arm64 into controlplane/bin/
	cd $(CP) && for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  echo "building tensora for $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -o bin/tensora-$$os-$$arch ./cmd/tensora || exit 1; \
	done

# ---------------------------------------------------------------- images
.PHONY: images
images: ## Build container images for controlplane, ml, web
	docker build -t $(IMAGE_REPO)/tensora-controlplane:$(VERSION) $(CP)
	docker build -t $(IMAGE_REPO)/tensora-ml:$(VERSION) $(ML)
	docker build -t $(IMAGE_REPO)/tensora-web:$(VERSION) $(WEB)
