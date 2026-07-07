# uniqlo-deal-hunter Makefile.
# `make help` lists targets. Every target has a `##` comment used to render
# the help table by help.mk (portable across Windows / macOS / Linux).

MODULE          := github.com/jo-hoe/uniqlo-deal-hunter
BINARY          := uniqlo-deal-hunter
IMAGE_NAME      := uniqlo-deal-hunter
IMAGE_TAG       ?= local
REGISTRY_LOCAL  := localhost:5000
CHART_DIR       := charts/uniqlo-deal-hunter
K3D_CLUSTER     := uniqlo-deal-hunter
DEV_CONFIG      := dev/config.yaml

.DEFAULT_GOAL := help

include help.mk

.PHONY: init
init: ## Download Go module dependencies
	go mod download

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: build
build: ## Build the binary
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(BINARY) ./cmd/uniqlo-deal-hunter

.PHONY: test
test: ## Run unit tests (no race — CI runs with race on linux)
	go test -count=1 ./...

.PHONY: test-integration
test-integration: ## Run integration tests (build tag: integration)
	go test -count=1 -tags=integration ./test/integration/...

.PHONY: cover
cover: ## Run unit tests with coverage and print a summary
	go test -count=1 -covermode=atomic -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out | tail -1

.PHONY: lint
lint: ## Run golangci-lint (must be installed on PATH)
	golangci-lint run

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format Go sources
	gofmt -w .

.PHONY: run
run: ## Run the app locally against dev/config.yaml
	CONFIG_PATH=$(DEV_CONFIG) go run ./cmd/uniqlo-deal-hunter --config=$(DEV_CONFIG)

.PHONY: docker-build
docker-build: ## Build the container image (local tag)
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

.PHONY: generate-helm-docs
generate-helm-docs: ## Regenerate charts/*/README.md via helm-docs
	docker run --rm --volume "$(CURDIR)/charts:/helm-docs" jnorwood/helm-docs:latest

.PHONY: helm-lint
helm-lint: ## helm lint the chart
	helm lint $(CHART_DIR)

.PHONY: helm-template
helm-template: ## Render the chart to stdout
	helm template test $(CHART_DIR)

.PHONY: start-cluster
start-cluster: ## Create the k3d cluster with a local registry
	k3d cluster create --config k3d/clusterconfig.yaml

.PHONY: push-k3d
push-k3d: docker-build ## Push the image to the local k3d registry
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY_LOCAL)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(REGISTRY_LOCAL)/$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: deploy-mailhog
deploy-mailhog: ## Deploy Mailhog mail sink into the cluster (dev only)
	kubectl run mailhog --image=mailhog/mailhog --port=1025 --port=8025 \
		--labels=app=mailhog --restart=Never 2>/dev/null || true
	kubectl expose pod mailhog --name=mailhog --port=1025 --target-port=1025 \
		--selector=app=mailhog 2>/dev/null || true
	kubectl wait pod/mailhog --for=condition=Ready --timeout=60s

.PHONY: mailhog-ui
mailhog-ui: ## Open Mailhog web UI via port-forward (http://localhost:8025)
	kubectl port-forward pod/mailhog 8025:8025

.PHONY: start-k3d
start-k3d: start-cluster push-k3d deploy-mailhog ## Full local install: cluster + image + Mailhog + helm install
	helm upgrade --install $(IMAGE_NAME) $(CHART_DIR) \
		--set image.repository=registry.localhost:5000/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		--set notifier.smtp.host=mailhog \
		--set notifier.smtp.port=1025 \
		--set notifier.smtp.startTLS=false \
		--set notifier.smtp.from=deals@local \
		--set "notifier.smtp.to[0]=me@local" \
		--set notifier.smtp.auth.enabled=false

.PHONY: stop-k3d
stop-k3d: ## Delete the k3d cluster
	k3d cluster delete $(K3D_CLUSTER)

.PHONY: restart-k3d
restart-k3d: stop-k3d start-k3d ## Recreate the k3d cluster from scratch

.PHONY: clean
clean: ## Remove local build artifacts
	rm -rf bin coverage.out
