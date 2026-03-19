SHELL := /bin/bash
export PATH := /usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:$(PATH)
GO    ?= $(shell which go 2>/dev/null || echo /usr/local/go/bin/go)

SERVICE_NAME      := ember-claw-sidecar
DOCKERFILE        := Dockerfile
IMAGE_REGISTRY    := reg.r.lastbot.com
EMBER_VERSION     ?=
BUILD_NUMBER_FILE := .ember-build-numbers
K8S_NAMESPACE     := picoclaw
KUBECONFIG_PATH   ?=

.PHONY: help build-eclaw build-picoclaw push-picoclaw build-push-picoclaw deploy-picoclaw
.DEFAULT_GOAL := help

help: ## Show this help menu
	@echo '╔══════════════════════════════════════════════════════════════╗'
	@echo '║  Ember-Claw - Build & Deploy                                 ║'
	@echo '╚══════════════════════════════════════════════════════════════╝'
	@echo ''
	@echo 'Usage: make [target] [EMBER_VERSION=x.y]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-40s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ''
	@echo 'Examples:'
	@echo '  make build-eclaw'
	@echo '  make build-picoclaw EMBER_VERSION=0.1'
	@echo '  make push-picoclaw EMBER_VERSION=0.1'
	@echo '  make build-push-picoclaw EMBER_VERSION=0.1'
	@echo '  make deploy-picoclaw'
	@echo '  make deploy-picoclaw NAME=alice PROVIDER=anthropic API_KEY=sk-xxx MODEL=claude-sonnet-4-20250514'
	@echo ''
	@echo 'Notes:'
	@echo '  - Set EMBER_VERSION for versioned builds (e.g. 0.1). Omit for "production" tag.'
	@echo '  - Authenticate to $(IMAGE_REGISTRY) with "docker login $(IMAGE_REGISTRY)" before push.'

build-eclaw: ## Build eclaw CLI binary to ./bin/eclaw
	@mkdir -p bin
	@$(GO) build -o bin/eclaw ./cmd/eclaw
	@echo "Built bin/eclaw"

build-picoclaw: ## Build Docker image for ember-claw-sidecar (use EMBER_VERSION=x.y for versioned tag)
	@if [ -n "$(EMBER_VERSION)" ]; then \
		SERVICE_NAME="$(SERVICE_NAME)"; \
		if [ -f $(BUILD_NUMBER_FILE) ]; then \
			CURRENT=$$(grep "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE) | head -1 | cut -d: -f2 || echo "0"); \
		else \
			CURRENT="0"; \
		fi; \
		BUILD_NUMBER=$$((CURRENT + 1)); \
		if [ -f $(BUILD_NUMBER_FILE) ]; then \
			if grep -q "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE); then \
				sed -i.bak "s/^$$SERVICE_NAME:.*/$$SERVICE_NAME:$$BUILD_NUMBER/" $(BUILD_NUMBER_FILE) && rm -f $(BUILD_NUMBER_FILE).bak; \
			else \
				echo "$$SERVICE_NAME:$$BUILD_NUMBER" >> $(BUILD_NUMBER_FILE); \
			fi; \
		else \
			echo "$$SERVICE_NAME:$$BUILD_NUMBER" > $(BUILD_NUMBER_FILE); \
		fi; \
		IMAGE_TAG="$(EMBER_VERSION).$$BUILD_NUMBER"; \
		echo "Building Docker image: $(IMAGE_REGISTRY)/$$SERVICE_NAME:$$IMAGE_TAG"; \
	else \
		IMAGE_TAG="production"; \
		echo "Building Docker image: $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG"; \
	fi; \
	docker buildx build --platform linux/amd64 \
		-f $(DOCKERFILE) \
		-t $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG \
		.

push-picoclaw: ## Push Docker image to reg.r.lastbot.com (run build-picoclaw first)
	@if [ -n "$(EMBER_VERSION)" ]; then \
		if [ ! -f $(BUILD_NUMBER_FILE) ]; then \
			echo "Error: run make build-picoclaw EMBER_VERSION=$(EMBER_VERSION) first"; exit 1; \
		fi; \
		SERVICE_NAME="$(SERVICE_NAME)"; \
		BUILD_NUMBER=$$(grep "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE) | head -1 | cut -d: -f2 || echo ""); \
		if [ -z "$$BUILD_NUMBER" ]; then \
			echo "Error: no build number for $$SERVICE_NAME -- run make build-picoclaw EMBER_VERSION=$(EMBER_VERSION) first"; exit 1; \
		fi; \
		IMAGE_TAG="$(EMBER_VERSION).$$BUILD_NUMBER"; \
	else \
		IMAGE_TAG="production"; \
	fi; \
	echo "Pushing $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG"; \
	docker push $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG

build-push-picoclaw: build-picoclaw push-picoclaw ## Build and push Docker image in one step

deploy-picoclaw: build-eclaw ## Deploy PicoClaw instance via interactive wizard (or override: NAME=x PROVIDER=y API_KEY=z MODEL=m)
	@echo "=== PicoClaw Instance Deployment ==="
	@NAME=$${NAME:-}; \
	if [ -z "$$NAME" ]; then read -p "Instance name: " NAME; fi; \
	PROVIDER=$${PROVIDER:-}; \
	if [ -z "$$PROVIDER" ]; then read -p "AI provider (anthropic/openai/gemini/copilot): " PROVIDER; fi; \
	API_KEY=$${API_KEY:-}; \
	if [ -z "$$API_KEY" ]; then read -s -p "API key: " API_KEY; echo; fi; \
	MODEL=$${MODEL:-}; \
	if [ -z "$$MODEL" ]; then read -p "Model name: " MODEL; fi; \
	CPU_REQ=$${CPU_REQ:-100m}; \
	CPU_LIM=$${CPU_LIM:-500m}; \
	MEM_REQ=$${MEM_REQ:-128Mi}; \
	MEM_LIM=$${MEM_LIM:-512Mi}; \
	IMAGE=$${IMAGE:-}; \
	if [ -z "$$IMAGE" ] && [ -n "$(EMBER_VERSION)" ] && [ -f $(BUILD_NUMBER_FILE) ]; then \
		SERVICE_NAME="$(SERVICE_NAME)"; \
		BUILD_NUMBER=$$(grep "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE) | head -1 | cut -d: -f2 || echo ""); \
		if [ -n "$$BUILD_NUMBER" ]; then \
			IMAGE="$(IMAGE_REGISTRY)/$$SERVICE_NAME:$(EMBER_VERSION).$$BUILD_NUMBER"; \
		fi; \
	fi; \
	IMAGE_FLAG=""; \
	if [ -n "$$IMAGE" ]; then IMAGE_FLAG="--image $$IMAGE"; fi; \
	KUBECONFIG_FLAG=""; \
	if [ -n "$(KUBECONFIG_PATH)" ]; then KUBECONFIG_FLAG="--kubeconfig $(KUBECONFIG_PATH)"; fi; \
	./bin/eclaw deploy $$NAME \
		--provider $$PROVIDER \
		--api-key $$API_KEY \
		--model $$MODEL \
		--cpu-request $$CPU_REQ \
		--cpu-limit $$CPU_LIM \
		--memory-request $$MEM_REQ \
		--memory-limit $$MEM_LIM \
		--namespace $(K8S_NAMESPACE) \
		$$KUBECONFIG_FLAG \
		$$IMAGE_FLAG
