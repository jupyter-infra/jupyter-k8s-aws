GOLANGCI_LINT_VERSION ?= v2.12.2

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= finch
BUILD_OPTS :=

# Traefik CRD chart version — pinned because latest (1.16.0+) exceeds the 1MB
# Kubernetes Secret size limit for Helm release metadata.
TRAEFIK_CRD_CHART_VERSION ?= 1.15.0

# Use Finch as the container provider for Kind when using Finch
# Update goproxy for cloud desktop compatibility
ifeq ($(CONTAINER_TOOL),finch)
  export KIND_EXPERIMENTAL_PROVIDER=finch
  export GOPROXY=direct

  # Set BUILD_OPTS to '--network host' on cloud desktop (if /etc/os-release exists), otherwise empty
  BUILD_OPTS := $(shell if [ -f /etc/os-release ]; then echo "--network host"; else echo ""; fi)
endif

# Image settings
IMAGE_REGISTRY ?= public.ecr.aws/jupyter-infra
IMAGE_NAME     ?= jupyter-k8s-aws-plugin
IMAGE_TAG      ?= latest
IMG            ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Helm chart paths
CHARTS_DIR        := charts
CHART_HYPERPOD    := $(CHARTS_DIR)/aws-hyperpod
CHART_OIDC        := $(CHARTS_DIR)/aws-oidc

# Path to sibling jupyter-k8s checkout (for controller chart deployment)
CONTROLLER_DIR ?= ../jupyter-k8s

# Guided deployment (used by helm-lint and helm-test only)
OAUTH2P_COOKIE_SECRET := $(shell openssl rand -base64 32 | tr -- '+/' '-_')

# Extra Helm arguments for deploy targets (pass overrides on the command line)
# Example: make deploy-aws-oidc HELM_EXTRA_ARGS="--set authmiddleware.imageTag=dev"
HELM_EXTRA_ARGS ?=

# AWS configuration
# Resolution order (highest priority first):
#   1. Command line: make deploy-aws-oidc AWS_REGION=us-east-2
#   2. .env file: AWS_REGION=us-east-2
#   3. Defaults: us-west-2 / jupyter-k8s-cluster
ifneq (,$(wildcard .env))
	ifneq ($(origin AWS_REGION),command line)
		_ENV_AWS_REGION := $(shell grep -s '^AWS_REGION=' .env | cut -d= -f2)
		ifneq (,$(_ENV_AWS_REGION))
			AWS_REGION := $(_ENV_AWS_REGION)
		endif
	endif
	ifneq ($(origin EKS_CLUSTER_NAME),command line)
		_ENV_EKS_CLUSTER := $(shell grep -s '^EKS_CLUSTER_NAME=' .env | cut -d= -f2)
		ifneq (,$(_ENV_EKS_CLUSTER))
			EKS_CLUSTER_NAME := $(_ENV_EKS_CLUSTER)
		endif
	endif
	ifneq ($(origin CONTROLLER_DIR),command line)
		_ENV_CONTROLLER_DIR := $(shell grep -s '^CONTROLLER_DIR=' .env | cut -d= -f2)
		ifneq (,$(_ENV_CONTROLLER_DIR))
			CONTROLLER_DIR := $(_ENV_CONTROLLER_DIR)
		endif
	endif
endif
AWS_REGION ?= us-west-2
EKS_CLUSTER_NAME ?= jupyter-k8s-cluster
AWS_ACCOUNT_ID = $(shell aws sts get-caller-identity --query "Account" --output text)
ECR_REGISTRY = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
ECR_REPOSITORY_CONTROLLER := jupyter-k8s
ECR_REPOSITORY_AWS_PLUGIN := jupyter-k8s-aws-plugin
ECR_REPOSITORY_AUTH := jupyter-k8s-auth
ECR_REPOSITORY_ROTATOR := jupyter-k8s-rotator
ECR_REPOSITORY_WEB_APP := jk8s-application-web-app
EKS_CONTEXT = arn:aws:eks:$(AWS_REGION):$(AWS_ACCOUNT_ID):cluster/$(EKS_CLUSTER_NAME)

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

.PHONY: release
release: build lint test helm-lint helm-test ## Run all checks required before PR submission

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Go

.PHONY: build
build: ## Build all Go binaries
	go build ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --fix

.PHONY: test
test: ## Run unit tests with coverage
	go test ./internal/... -coverprofile coverage.out

.PHONY: test-functional
test-functional: ## Run functional tests (build-tagged)
	go test -tags=functional ./test/functional/ -v

.PHONY: deps
deps: ## Download and tidy Go dependencies
	go mod download
	go mod tidy

##@ Helm

.PHONY: helm-lint
helm-lint: ## Lint all Helm charts
	helm lint $(CHART_OIDC) \
		--set domain=a.fine.example.com \
		--set certManager.email=admin@example.com \
		--set storageClass.efs.parameters.fileSystemId=fs-00001111222233334 \
		--set github.clientId=some-client-id \
		--set github.clientSecret=some-github-secret \
		--set github.orgs[0].name=some-org \
		--set github.orgs[0].teams[0]=ace-devs \
		--set githubRbac.orgs[0].name=some-org \
		--set githubRbac.orgs[0].teams[0]=ace-devs \
		--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET)
	helm lint $(CHART_HYPERPOD) \
		--set clusterWebUI.enabled=true \
		--set clusterWebUI.domain=test.example.com \
		--set clusterWebUI.awsCertificateArn=arn:aws:acm:us-east-1:123456789:certificate/abc \
		--set aws.region=us-east-1 \
		--set remoteAccess.enabled=true \
		--set remoteAccess.ssmManagedNodeRole=arn:aws:iam::123456789:role/SageMakerRole \
		--set remoteAccess.ssmSidecarImage.containerRegistry=123456789.dkr.ecr.us-east-1.amazonaws.com \
		--set remoteAccess.ssmSidecarImage.repository=ssm-sidecar \
		--set remoteAccess.ssmSidecarImage.tag=latest

.PHONY: helm-test-aws-oidc
helm-test-aws-oidc: ## Render and test aws-oidc chart
	rm -rf dist/test-output/aws-oidc
	rm -rf /tmp/helm-test-chart
	cp -r $(CHART_OIDC) /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template jk8s /tmp/helm-test-chart --output-dir dist/test-output/aws-oidc \
		--set domain=a.fine.example.com \
		--set certManager.email=admin@example.com \
		--set storageClass.efs.parameters.fileSystemId=fs-00001111222233334 \
		--set github.clientId=some-client-id \
		--set github.clientSecret=some-github-secret \
		--set github.orgs[0].name=some-org \
		--set github.orgs[0].teams[0]=ace-devs \
		--set githubRbac.orgs[0].name=some-org \
		--set githubRbac.orgs[0].teams[0]=ace-devs \
		--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET) \
		--set authmiddleware.enableBearerAuth=true \
		--set accessStrategy.createBearer=true
	rm -rf /tmp/helm-test-chart
	go test ./test/helm/aws-oidc -v

.PHONY: helm-test-aws-hyperpod
helm-test-aws-hyperpod: ## Render and test aws-hyperpod chart
	rm -rf dist/test-output/aws-hyperpod
	rm -rf /tmp/helm-test-chart
	cp -r $(CHART_HYPERPOD) /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template jk8s /tmp/helm-test-chart --output-dir dist/test-output/aws-hyperpod \
		--set clusterWebUI.enabled=true \
		--set clusterWebUI.domain=test.example.com \
		--set clusterWebUI.awsCertificateArn=arn:aws:acm:us-east-1:123456789:certificate/abc \
		--set aws.region=us-east-1 \
		--set remoteAccess.enabled=true \
		--set remoteAccess.ssmManagedNodeRole=arn:aws:iam::123456789:role/SageMakerRole \
		--set remoteAccess.ssmSidecarImage.containerRegistry=123456789.dkr.ecr.us-east-1.amazonaws.com \
		--set remoteAccess.ssmSidecarImage.repository=ssm-sidecar \
		--set remoteAccess.ssmSidecarImage.tag=latest \
		--set clusterWebUI.auth.enableBearerAuth=true
	rm -rf /tmp/helm-test-chart
	go test ./test/helm/aws-hyperpod -v

.PHONY: helm-test
helm-test: helm-test-aws-oidc helm-test-aws-hyperpod ## Run all Helm tests

##@ Images

.PHONY: build-aws-plugin
build-aws-plugin: ## Build aws-plugin sidecar image for local testing
	@echo "Building aws-plugin image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t docker.io/library/aws-plugin:local -f Dockerfile .

.PHONY: image-build
image-build: ## Build aws-plugin container image
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t $(IMG) -f Dockerfile .

.PHONY: image-push
image-push: ## Push aws-plugin container image
	$(CONTAINER_TOOL) push $(IMG)

.PHONY: ecr-create
ecr-create: ## Create ECR repository for aws-plugin
	@echo "Creating ECR repository for aws-plugin if it doesn't exist..."
	@aws ecr describe-repositories --repository-names $(ECR_REPOSITORY_AWS_PLUGIN) --region $(AWS_REGION) > /dev/null 2>&1 || \
		aws ecr create-repository --repository-name $(ECR_REPOSITORY_AWS_PLUGIN) --region $(AWS_REGION)

.PHONY: ecr-push
ecr-push: ## Build and push aws-plugin image to ECR
	@aws ecr get-login-password --region $(AWS_REGION) | \
		$(CONTAINER_TOOL) login --username AWS --password-stdin $(ECR_REGISTRY)
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 \
		-t $(ECR_REGISTRY)/$(ECR_REPOSITORY_AWS_PLUGIN):latest -f Dockerfile .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY_AWS_PLUGIN):latest
	@echo "AWS plugin image pushed to $(ECR_REGISTRY)/$(ECR_REPOSITORY_AWS_PLUGIN):latest"

##@ AWS Deployment

.PHONY: setup-aws
setup-aws: ## Setup connection to remote EKS cluster
	@echo "Setting up remote cluster connection..."
	@if [ -n "$(EKS_CLUSTER_NAME)" ]; then \
		echo "Getting kubeconfig from EKS cluster $(EKS_CLUSTER_NAME)..."; \
		aws eks update-kubeconfig \
			--name $(EKS_CLUSTER_NAME) \
			--region $(AWS_REGION); \
	else \
		echo "EKS_CLUSTER_NAME not provided. Please set it when running this command."; \
		exit 1; \
	fi
	@if ! kubectl get crds | grep traefik > /dev/null 2>&1; then \
		echo "Installing traefik CRDs"; \
		helm repo add traefik https://traefik.github.io/charts; \
		helm repo update; \
		helm install traefik-crd traefik/traefik-crds \
			--namespace traefik \
			--create-namespace \
			--version $(TRAEFIK_CRD_CHART_VERSION); \
		echo "Successfully installed traefik CRDs"; \
	else \
		echo "traefik CRDs already installed, skipping"; \
	fi
	@echo "Remote AWS setup complete."

.PHONY: kubectl-aws
kubectl-aws: ## Configure kubectl to use remote cluster
	@echo "Setting up kubectl to use remote cluster..."
	@if kubectl config get-contexts | grep -q "$(EKS_CLUSTER_NAME)"; then \
		echo "Switching to EKS cluster context..."; \
		kubectl config use-context "$(EKS_CONTEXT)"; \
		echo "kubectl configured to use remote cluster. Current context: $$(kubectl config current-context)"; \
	else \
		echo "EKS cluster context not found. Try running 'make setup-aws' first."; \
		exit 1; \
	fi

.PHONY: deploy-aws-oidc
deploy-aws-oidc: setup-aws ## Deploy aws-oidc chart (reuses existing Helm values from JD)
	@echo "Upgrading aws-oidc chart with --reset-then-reuse-values..."
	helm upgrade jupyter-k8s-aws-oidc $(CHART_OIDC) \
		-n jupyter-k8s-router \
		--reset-then-reuse-values \
		$(HELM_EXTRA_ARGS)
	@( \
		set -e; \
		DOMAIN=$$(helm get values jupyter-k8s-aws-oidc -n jupyter-k8s-router -o json | jq -r '.domain'); \
		$(SHELL) scripts/aws-oidc/generate-client.sh \
			$(EKS_CLUSTER_NAME) \
			https://$$DOMAIN/dex \
			$(AWS_REGION) \
			9800 \
			dist/users-scripts/set-kubeconfig.sh \
			"$$(kubectl get configmap dex-config -n jupyter-k8s-router -o jsonpath='{.data.config\.yaml}' | awk '/id: kubectl-oidc/{found=1} found && /secret:/{print $$2; exit}')"; \
	)
	@echo "Restarting deployments to pick up chart changes..."
	kubectl rollout restart deployment -n jupyter-k8s-router \
		traefik oauth2-proxy dex authmiddleware
	-kubectl rollout restart deployment -n jupyter-k8s-router web-app 2>/dev/null || true
	@echo "Kubectl setup script available at: dist/users-scripts/set-kubeconfig.sh"

.PHONY: deploy-aws-hyperpod
deploy-aws-hyperpod: setup-aws ## Deploy aws-hyperpod chart (reuses existing Helm values from JD)
	@echo "Upgrading aws-hyperpod chart with --reset-then-reuse-values..."
	helm upgrade aws-hyperpod $(CHART_HYPERPOD) \
		-n jupyter-k8s-system \
		--reset-then-reuse-values \
		$(HELM_EXTRA_ARGS)
	@echo "Restarting authmiddleware deployment to pick up chart changes..."
	-kubectl rollout restart deployment -n jupyter-k8s-system workspace-auth-middleware 2>/dev/null || true

.PHONY: deploy-controller
deploy-controller: setup-aws ## Deploy jupyter-k8s controller (without aws-plugin sidecar)
	@if [ ! -d "$(CONTROLLER_DIR)" ]; then \
		echo "jupyter-k8s repo not found at $(CONTROLLER_DIR). Set CONTROLLER_DIR to the correct path."; \
		exit 1; \
	fi
	$(MAKE) -C $(CONTROLLER_DIR) load-images-aws helm-generate \
		AWS_REGION=$(AWS_REGION) EKS_CLUSTER_NAME=$(EKS_CLUSTER_NAME)
	@echo "Deploying jupyter-k8s controller to remote AWS cluster..."
	helm upgrade --install jupyter-k8s $(CONTROLLER_DIR)/dist/chart \
		--namespace jupyter-k8s-system --create-namespace \
		--set manager.image.pullPolicy=Always \
		--set manager.image.repository=$(ECR_REGISTRY)/$(ECR_REPOSITORY_CONTROLLER) \
		--set manager.image.tag=latest \
		--set application.imagesPullPolicy=Always \
		--set application.imagesRegistry=$(ECR_REGISTRY) \
		--set workspacePodWatching.enable=true \
		--set extensionApi.enable=true \
		--set extensionApi.jwtSecret.enable=true \
		--set extensionApi.jwtSecret.rotator.repository=$(ECR_REGISTRY) \
		--set extensionApi.jwtSecret.rotator.imageName=$(ECR_REPOSITORY_ROTATOR) \
		--set workspaceTemplates.defaultNamespace=jupyter-k8s-system
	kubectl rollout restart deployment -n jupyter-k8s-system jupyter-k8s-controller-manager
	@echo "Controller deployed successfully."

.PHONY: deploy-controller-with-plugin
deploy-controller-with-plugin: setup-aws ecr-push ## Deploy jupyter-k8s controller with aws-plugin sidecar
	@if [ ! -d "$(CONTROLLER_DIR)" ]; then \
		echo "jupyter-k8s repo not found at $(CONTROLLER_DIR). Set CONTROLLER_DIR to the correct path."; \
		exit 1; \
	fi
	$(MAKE) -C $(CONTROLLER_DIR) load-images-aws helm-generate \
		AWS_REGION=$(AWS_REGION) EKS_CLUSTER_NAME=$(EKS_CLUSTER_NAME)
	@echo "Deploying jupyter-k8s controller with aws-plugin sidecar..."
	helm upgrade --install jupyter-k8s $(CONTROLLER_DIR)/dist/chart \
		--namespace jupyter-k8s-system --create-namespace \
		--set manager.image.pullPolicy=Always \
		--set manager.image.repository=$(ECR_REGISTRY)/$(ECR_REPOSITORY_CONTROLLER) \
		--set manager.image.tag=latest \
		--set application.imagesPullPolicy=Always \
		--set application.imagesRegistry=$(ECR_REGISTRY) \
		--set workspacePodWatching.enable=true \
		--set extensionApi.enable=true \
		--set extensionApi.jwtSecret.enable=true \
		--set extensionApi.jwtSecret.rotator.repository=$(ECR_REGISTRY) \
		--set extensionApi.jwtSecret.rotator.imageName=$(ECR_REPOSITORY_ROTATOR) \
		--set workspaceTemplates.defaultNamespace=jupyter-k8s-system \
		--set controller.plugins[0].name=aws \
		--set controller.plugins[0].image.repository=$(ECR_REGISTRY)/$(ECR_REPOSITORY_AWS_PLUGIN) \
		--set controller.plugins[0].image.tag=latest \
		--set controller.plugins[0].port=8080 \
		--set controller.plugins[0].imagePullPolicy=Always \
		--set 'controller.plugins[0].healthcheckCommand[0]=/aws-plugin' \
		--set 'controller.plugins[0].healthcheckCommand[1]=--healthcheck' \
		--set controller.plugins[0].env.PLUGIN_PORT=8080 \
		--set controller.plugins[0].env.AWS_REGION=$(AWS_REGION) \
		--set controller.plugins[0].env.CLUSTER_ID=$(EKS_CONTEXT)
	kubectl rollout restart deployment -n jupyter-k8s-system jupyter-k8s-controller-manager
	@echo "Controller with aws-plugin deployed successfully."

.PHONY: redeploy-plugin
redeploy-plugin: ecr-push ## Build, push aws-plugin image and restart the controller to pick it up
	kubectl rollout restart deployment -n jupyter-k8s-system jupyter-k8s-controller-manager
	@echo "Waiting for rollout to complete..."
	kubectl rollout status deployment -n jupyter-k8s-system jupyter-k8s-controller-manager --timeout=120s

.PHONY: undeploy-aws-hyperpod
undeploy-aws-hyperpod: ## Remove aws-hyperpod chart
	helm uninstall aws-hyperpod --namespace jupyter-k8s-system

.PHONY: undeploy-aws
undeploy-aws: ## Uninstall all Helm charts from remote cluster
	@echo "Undeploying Helm charts from remote AWS cluster..."
	helm uninstall jupyter-k8s-aws-oidc -n jupyter-k8s-router --ignore-not-found
	helm uninstall aws-hyperpod -n jupyter-k8s-system --ignore-not-found
	helm uninstall traefik-crd --ignore-not-found

##@ Samples

WS_NAMESPACE ?= default

.PHONY: apply-sample-oidc
apply-sample-oidc: ## Create sample OIDC workspaces (access strategies are shipped by the aws-oidc chart)
	kubectl apply -k samples/oidc

.PHONY: delete-sample-oidc
delete-sample-oidc: ## Delete sample OIDC workspaces
	kubectl delete -k samples/oidc

.PHONY: apply-sample-hyperpod
apply-sample-hyperpod: ## Create sample hyperpod workspaces. Usage: make apply-sample-hyperpod WS_USER=alice
	@if [ -z "$(WS_USER)" ]; then echo "WS_USER is required. Usage: make apply-sample-hyperpod WS_USER=alice"; exit 1; fi
	@echo "Creating sample hyperpod workspaces for '$(WS_USER)'..."
	export WS_USER=$(WS_USER); \
	kubectl apply -k samples/hyperpod --dry-run=client -o yaml | envsubst | kubectl apply -f -

.PHONY: delete-sample-hyperpod
delete-sample-hyperpod: ## Delete sample hyperpod workspaces. Usage: make delete-sample-hyperpod WS_USER=alice
	@if [ -z "$(WS_USER)" ]; then echo "WS_USER is required. Usage: make delete-sample-hyperpod WS_USER=alice"; exit 1; fi
	@echo "Deleting sample hyperpod workspaces for '$(WS_USER)'..."
	export WS_USER=$(WS_USER); \
	kubectl apply -k samples/hyperpod --dry-run=client -o yaml | envsubst | kubectl delete -f -

.PHONY: bearer-token
bearer-token: ## Create a bearer token URL for a workspace. Usage: make bearer-token WS_NAME=<name> [WS_NAMESPACE=default]
	@bash -c '\
		RESULT=$$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/$(WS_NAMESPACE)/workspaceconnections" \
			-f <(echo '"'"'{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"$(WS_NAMESPACE)"},"spec":{"workspaceName":"$(WS_NAME)","workspaceConnectionType":"web-ui"}}'"'"') 2>&1) && \
		URL=$$(echo "$$RESULT" | jq -r ".status.workspaceConnectionUrl // empty") && \
		echo "$$URL" || \
		{ echo "$$RESULT"; exit 1; } \
	'

.PHONY: vscode-token
vscode-token: ## Create a VS Code remote connection URL. Usage: make vscode-token WS_NAME=<name> [WS_NAMESPACE=default]
	@bash -c '\
		RESULT=$$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/$(WS_NAMESPACE)/workspaceconnections" \
			-f <(echo '"'"'{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"$(WS_NAMESPACE)"},"spec":{"workspaceName":"$(WS_NAME)","workspaceConnectionType":"vscode-remote"}}'"'"') 2>&1) && \
		URL=$$(echo "$$RESULT" | jq -r ".status.workspaceConnectionUrl // empty") && \
		echo "$$URL" || \
		{ echo "$$RESULT"; exit 1; } \
	'
