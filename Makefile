GOLANGCI_LINT_VERSION ?= v2.4.0

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
IMAGE_NAME     ?= jk8s-aws-plugin
IMAGE_TAG      ?= latest
IMG            ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Helm chart paths
CHARTS_DIR        := charts
CHART_HYPERPOD    := $(CHARTS_DIR)/aws-hyperpod
CHART_TRAEFIK_DEX := $(CHARTS_DIR)/aws-traefik-dex

# Guided deployment
OAUTH2P_COOKIE_SECRET := $(shell openssl rand -base64 32 | tr -- '+/' '-_')

# AWS configuration
# Resolution order (highest priority first):
#   1. Command line: make deploy-aws-traefik-dex AWS_REGION=us-east-2
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
endif
AWS_REGION ?= us-west-2
EKS_CLUSTER_NAME ?= jupyter-k8s-cluster
AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query "Account" --output text)
ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
ECR_REPOSITORY_AWS_PLUGIN := jupyter-k8s-aws-plugin
ECR_REPOSITORY_AUTH := jupyter-k8s-auth
ECR_REPOSITORY_ROTATOR := jupyter-k8s-rotator
EKS_CONTEXT := arn:aws:eks:$(AWS_REGION):$(AWS_ACCOUNT_ID):cluster/$(EKS_CLUSTER_NAME)

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
	helm lint $(CHART_TRAEFIK_DEX) \
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

.PHONY: helm-test-aws-traefik-dex
helm-test-aws-traefik-dex: ## Render and test aws-traefik-dex chart
	rm -rf dist/test-output/aws-traefik-dex
	rm -rf /tmp/helm-test-chart
	cp -r $(CHART_TRAEFIK_DEX) /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template jk8s /tmp/helm-test-chart --output-dir dist/test-output/aws-traefik-dex \
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
		--set authmiddleware.enableBearerAuth=true
	rm -rf /tmp/helm-test-chart
	go test ./test/helm/aws-traefik-dex -v

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
helm-test: helm-test-aws-traefik-dex helm-test-aws-hyperpod ## Run all Helm tests

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

.PHONY: deploy-aws-traefik-dex
deploy-aws-traefik-dex: ## Deploy aws-traefik-dex chart from .env config
	@if [ ! -f .env ]; then \
		echo ".env file not found. Copy .env.example to .env and edit the values."; \
		echo "Required: TRAEFIK_DEX_DOMAIN, LETSENCRYPT_EMAIL, GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, GITHUB_ORG_NAME"; \
		exit 1; \
	fi
	@echo "Loading configuration from .env file and deploying..."
	@( \
		set -e; \
		. ./.env; \
		rm -rf /tmp/jk8s-aws-traefik-dex; \
		mkdir /tmp/jk8s-aws-traefik-dex; \
		cp -r $(CHART_TRAEFIK_DEX)/ /tmp/jk8s-aws-traefik-dex/; \
		echo 'Deploying AWS traefik dex helm chart'; \
		HELM_ARGS="--set domain=$$TRAEFIK_DEX_DOMAIN \
			--set certManager.email=$$LETSENCRYPT_EMAIL \
			--set storageClass.efs.parameters.fileSystemId=$$EFS_ID \
			--set github.clientId=$$GITHUB_CLIENT_ID \
			--set github.clientSecret=$$GITHUB_CLIENT_SECRET \
			--set github.orgs[0].name=$$GITHUB_ORG_NAME \
			--set github.orgs[0].teams[0]=$$GITHUB_TEAM \
			--set githubRbac.orgs[0].name=$$GITHUB_ORG_NAME \
			--set githubRbac.orgs[0].teams[0]=$$GITHUB_TEAM \
			--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET) \
			--set authmiddleware.repository=$(ECR_REGISTRY) \
			--set authmiddleware.imageName=$(ECR_REPOSITORY_AUTH) \
			--set authmiddleware.enableBearerAuth=true \
			--set rotator.repository=$(ECR_REGISTRY) \
			--set rotator.imageName=$(ECR_REPOSITORY_ROTATOR)"; \
		if [ ! -z "$$DEX_OAUTH2_SECRET" ]; then \
			HELM_ARGS="$$HELM_ARGS --set dex.oauth2ProxyClientSecret=$$DEX_OAUTH2_SECRET"; \
		fi; \
		\
		if [ ! -z "$$DEX_K8S_SECRET" ]; then \
			HELM_ARGS="$$HELM_ARGS --set dex.kubernetesClientSecret=$$DEX_K8S_SECRET"; \
		fi; \
		\
		if [ ! -z "$$KUBECTL_REDIRECT_PORTS" ]; then \
			IFS=',' read -ra PORTS <<< "$$KUBECTL_REDIRECT_PORTS"; \
			PORT_INDEX=0; \
			for PORT in "$${PORTS[@]}"; do \
				HELM_ARGS="$$HELM_ARGS --set dex.kubernetesClientRedirectPorts[$${PORT_INDEX}]=$${PORT}"; \
				PORT_INDEX=$$((PORT_INDEX + 1)); \
			done; \
		fi; \
		\
		helm upgrade --install jk8-aws-traefik-dex /tmp/jk8s-aws-traefik-dex/aws-traefik-dex \
			-n jupyter-k8s-router \
			--create-namespace \
			--force \
			$$HELM_ARGS; \
		\
		$(SHELL) scripts/aws-traefik-dex/generate-client.sh \
			$(EKS_CLUSTER_NAME) \
			https://$$TRAEFIK_DEX_DOMAIN/dex \
			$(AWS_REGION) \
			9800 \
			dist/users-scripts/set-kubeconfig.sh \
			"$$(kubectl get configmap dex-config -n jupyter-k8s-router -o jsonpath='{.data.config\.yaml}' | awk '/id: kubectl-oidc/{found=1} found && /secret:/{print $$2; exit}')"; \
	)
	@echo "Restarting deployments to use new images..."
	kubectl rollout restart deployment -n jupyter-k8s-router \
		traefik oauth2-proxy dex authmiddleware
	rm -rf /tmp/jk8s-aws-traefik-dex
	@echo "Bash script for end-users to set their kubeconfig available at: dist/users-scripts"

.PHONY: deploy-aws-hyperpod
deploy-aws-hyperpod: ## Deploy aws-hyperpod chart from .env config
	@if [ ! -f .env ]; then \
		echo ".env file not found. Copy .env.example to .env and edit the values."; \
		echo "Required: HYPERPOD_DOMAIN, ACM_CERT_ARN"; \
		exit 1; \
	fi
	@echo "Loading configuration from .env file and deploying aws-hyperpod..."
	@( \
		set -e; \
		. ./.env; \
		HP_DOMAIN=$$HYPERPOD_DOMAIN; \
		if [ -z "$$HP_DOMAIN" ]; then \
			echo "HYPERPOD_DOMAIN must be set in .env"; \
			exit 1; \
		fi; \
		echo "Deploying AWS HyperPod helm chart (domain=$$HP_DOMAIN)"; \
		HELM_ARGS="--set aws.region=$(AWS_REGION) \
			--set clusterWebUI.enabled=true \
			--set clusterWebUI.domain=$$HP_DOMAIN \
			--set clusterWebUI.auth.repository=$(ECR_REGISTRY) \
			--set clusterWebUI.auth.imageName=$(ECR_REPOSITORY_AUTH) \
			--set clusterWebUI.rotator.repository=$(ECR_REGISTRY) \
			--set clusterWebUI.rotator.imageName=$(ECR_REPOSITORY_ROTATOR)"; \
		if [ -n "$$ACM_CERT_ARN" ]; then \
			HELM_ARGS="$$HELM_ARGS --set clusterWebUI.awsCertificateArn=$$ACM_CERT_ARN"; \
		fi; \
		if [ -n "$$SSM_SIDECAR_REGISTRY" ] && [ -n "$$SSM_SIDECAR_REPO" ] && [ -n "$$SSM_SIDECAR_TAG" ]; then \
			HELM_ARGS="$$HELM_ARGS --set remoteAccess.enabled=true \
				--set remoteAccess.ssmSidecarImage.containerRegistry=$$SSM_SIDECAR_REGISTRY \
				--set remoteAccess.ssmSidecarImage.repository=$$SSM_SIDECAR_REPO \
				--set remoteAccess.ssmSidecarImage.tag=$$SSM_SIDECAR_TAG"; \
			if [ -n "$$SSM_MANAGED_NODE_ROLE" ]; then \
				HELM_ARGS="$$HELM_ARGS --set remoteAccess.ssmManagedNodeRole=$$SSM_MANAGED_NODE_ROLE"; \
			fi; \
		fi; \
		helm upgrade --install aws-hyperpod ./$(CHART_HYPERPOD) \
			--create-namespace --namespace jupyter-k8s-system \
			$$HELM_ARGS; \
	)
	@echo "Restarting authmiddleware deployment to use new images..."
	-kubectl rollout restart deployment -n jupyter-k8s-system workspace-auth-middleware 2>/dev/null || true

.PHONY: deploy-aws
deploy-aws: deploy-aws-traefik-dex deploy-aws-hyperpod ## Deploy both charts (aws-traefik-dex + aws-hyperpod)

.PHONY: undeploy-aws-hyperpod
undeploy-aws-hyperpod: ## Remove aws-hyperpod chart
	helm uninstall aws-hyperpod --namespace jupyter-k8s-system

.PHONY: undeploy-aws
undeploy-aws: ## Uninstall all Helm charts from remote cluster
	@echo "Undeploying Helm charts from remote AWS cluster..."
	helm uninstall jk8-aws-traefik-dex -n jupyter-k8s-router --ignore-not-found
	helm uninstall aws-hyperpod -n jupyter-k8s-system --ignore-not-found
	helm uninstall traefik-crd --ignore-not-found
