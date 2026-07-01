# OAuth app slot numbers
# 1 = pr, 2 = canary, 3 = fresh
ci_dir := "sandbox-ci"
e2e_dir := "sandbox-e2e"
fresh_app_num := "3"

# Path to jupyter-deploy repo (override with JD_DIR env var)
jd_dir := env_var_or_default("JD_DIR", "../jupyter-deploy")

# Container tool
container_tool := `command -v finch >/dev/null 2>&1 && echo "finch" || echo "docker"`

# List available commands
default:
    @just --list

# --- CI infrastructure ---

# Restore the CI project (tf-aws-iam-ci) from S3
ci-restore ci_dir=ci_dir:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/ci_restore.py {{ci_dir}}

# Restore the EKS project from S3 by OAuth app number
# Usage: just ci-restore-eks <oauth-app-num> [ci-dir] [project-dir]
ci-restore-eks oauth_app_num ci_dir=ci_dir project_dir=e2e_dir:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/ci_restore_eks.py {{ci_dir}} {{oauth_app_num}} {{project_dir}}

# Find and tear down a previous EKS deployment by OAuth app number
# Usage: just find-takedown-eks <oauth-app-num> [ci-dir] [project-dir]
find-takedown-eks oauth_app_num ci_dir=ci_dir project_dir=e2e_dir:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/find_takedown_eks.py {{ci_dir}} {{oauth_app_num}} {{project_dir}}

# Get ECR repository URL for a given OAuth app slot
ci-e2e-ecr-url oauth_app_num ci_dir=ci_dir:
    @PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} jd show -o ecr_repository_url_{{oauth_app_num}} --text -p {{ci_dir}}

# Get test results S3 bucket name
ci-test-results-bucket ci_dir=ci_dir:
    @PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} jd show -o test_results_bucket_name --text -p {{ci_dir}}

# Upload test results to S3 on failure
ci-upload-test-results oauth_app_num ci_dir=ci_dir results_dir="test-results":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -d "{{results_dir}}" ] || [ -z "$(ls -A {{results_dir}} 2>/dev/null)" ]; then
        echo "No test results to upload"
        exit 0
    fi
    TIMESTAMP=$(date -u +"%Y-%m-%d-%H-%M")
    BUCKET=$(just ci-test-results-bucket {{ci_dir}})
    S3_PATH="s3://${BUCKET}/${TIMESTAMP}/{{oauth_app_num}}/"
    echo "Uploading test results to ${S3_PATH}..."
    aws s3 cp "{{results_dir}}/" "$S3_PATH" --recursive

# --- ECR image ---

# Pull E2E image from ECR (for layer cache)
ci-e2e-pull oauth_app_num tag="latest" ci_dir=ci_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    ECR_URL=$(just ci-e2e-ecr-url {{oauth_app_num}} {{ci_dir}})
    REGISTRY=$(echo "$ECR_URL" | cut -d'/' -f1)
    REGION=$(PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} jd show -o region --text -p {{ci_dir}})
    aws ecr get-login-password --region "$REGION" | {{container_tool}} login --username AWS --password-stdin "$REGISTRY"
    {{container_tool}} pull "$ECR_URL:{{tag}}" || true

# Build E2E image
ci-e2e-build cache_from="" ci_dir=ci_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    JD={{jd_dir}}
    BASE_DOCKERFILE=$(uv run --project "$JD" python -c \
        "from pytest_jupyter_deploy.image import IMAGE_PATH; print(IMAGE_PATH / 'Dockerfile')")
    BASE_DIR=$(dirname "$BASE_DOCKERFILE")
    echo "Building base E2E image..."
    {{container_tool}} build \
        -f "$BASE_DOCKERFILE" \
        -t jupyter-k8s-aws-e2e:base \
        "$BASE_DIR"
    echo "Building CI E2E image..."
    CACHE_ARG=""
    if [ -n "{{cache_from}}" ]; then
        CACHE_ARG="--cache-from={{cache_from}}"
    fi
    {{container_tool}} build \
        -f "$JD/.github/e2e-shared/Dockerfile" \
        --build-arg BASE_IMAGE=jupyter-k8s-aws-e2e:base \
        $CACHE_ARG \
        -t jupyter-k8s-aws-e2e:latest \
        "$JD"

# Push E2E image to ECR
ci-e2e-push oauth_app_num extra_tag="" ci_dir=ci_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    ECR_URL=$(just ci-e2e-ecr-url {{oauth_app_num}} {{ci_dir}})
    REGISTRY=$(echo "$ECR_URL" | cut -d'/' -f1)
    REGION=$(PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} jd show -o region --text -p {{ci_dir}})
    aws ecr get-login-password --region "$REGION" | {{container_tool}} login --username AWS --password-stdin "$REGISTRY"
    {{container_tool}} tag jupyter-k8s-aws-e2e:latest "$ECR_URL:latest"
    {{container_tool}} push "$ECR_URL:latest"
    if [ -n "{{extra_tag}}" ]; then
        {{container_tool}} tag jupyter-k8s-aws-e2e:latest "$ECR_URL:{{extra_tag}}"
        {{container_tool}} push "$ECR_URL:{{extra_tag}}"
    fi

# --- Auth ---

# Import Playwright auth state from Secrets Manager (run ci-restore first)
auth-import ci_dir=ci_dir:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/sync_auth_state.py import {{ci_dir}}

# Export Playwright auth state to Secrets Manager
auth-export ci_dir=ci_dir:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/sync_auth_state.py export {{ci_dir}}

# Check local auth state cookie expiry
auth-check:
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/sync_auth_state.py check

# Generate a 2FA code for the bot (requires oathtool)
auth-bot-2fa ci_dir=ci_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    SEED=$(uv run --project {{jd_dir}} jd show -v github_bot_account_totp_secret --text -p {{ci_dir}})
    oathtool -b --totp "$SEED"

# --- Deploy ---

# Generate .env for a fresh EKS OIDC deploy
# Usage: just env-setup-eks <project-dir> <ci-dir> <oauth-app-num> [options]
env-setup-eks project_dir ci_dir=ci_dir oauth_app_num=fresh_app_num options="":
    PYTHONPATH={{jd_dir}}/scripts uv run --project {{jd_dir}} python {{jd_dir}}/scripts/env_setup_eks.py "{{project_dir}}" {{ci_dir}} {{oauth_app_num}} "{{options}}"

# Deploy a fresh EKS OIDC cluster inside the pre-built container
# Usage: just ci-e2e-eks-deploy <project-dir> [ci-dir]
ci-e2e-eks-deploy project_dir=e2e_dir ci_dir=ci_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    ROOT={{justfile_directory()}}
    JD={{jd_dir}}
    : "${AWS_REGION:=$(aws configure get region 2>/dev/null || true)}"
    if [ -z "${AWS_REGION:-}" ]; then
        echo "Error: AWS_REGION is not set"
        exit 1
    fi
    export AWS_REGION
    mkdir -p "$ROOT/{{project_dir}}"
    E2E_COMPOSE=$(uv run --project "$JD" python -c \
        "from pytest_jupyter_deploy.image import IMAGE_PATH; print(IMAGE_PATH / 'docker-compose.yml')")
    OVERRIDE_FILE="$ROOT/docker-compose.e2e-override.yml"
    printf 'services:\n  e2e:\n    image: jupyter-k8s-aws-e2e:latest\n    volumes:\n      - ./{{project_dir}}:/workspace/{{project_dir}}\n      - ./{{ci_dir}}:/workspace/{{ci_dir}}\n' > "$OVERRIDE_FILE"
    trap 'rm -f "$OVERRIDE_FILE"' EXIT
    mkdir -p "$HOME/.kube"
    {{container_tool}} compose --project-directory "$ROOT" -f "$E2E_COMPOSE" down
    {{container_tool}} compose --project-directory "$ROOT" -f "$E2E_COMPOSE" -f "$OVERRIDE_FILE" up -d --no-build
    EXEC="{{container_tool}} compose --project-directory $ROOT -f $E2E_COMPOSE exec -e PYTHONUNBUFFERED=1 e2e bash -c"
    echo "=== jd init ==="
    $EXEC ". .venv/bin/activate && cd /workspace && jupyter-deploy init -E terraform -P aws -I eks -T oidc {{project_dir}}"
    echo "=== render variables.yaml ==="
    set -a && source "$ROOT/.env" && set +a
    envsubst < "$ROOT/ci/variables.yaml.tmpl" > "$ROOT/{{project_dir}}/variables.yaml"
    echo "=== jd config ==="
    $EXEC ". .venv/bin/activate && cd /workspace/{{project_dir}} && jupyter-deploy config -v"
    echo "=== jd up ==="
    $EXEC ". .venv/bin/activate && cd /workspace/{{project_dir}} && jupyter-deploy up -y -v"

# Destroy a fresh EKS OIDC cluster
# Usage: just destroy-fresh [project-dir]
destroy-fresh project_dir=e2e_dir:
    #!/usr/bin/env bash
    set -euo pipefail
    ROOT={{justfile_directory()}}
    cd "$ROOT/{{project_dir}}" && uv run --project {{jd_dir}} jd down -y -v

# --- Test ---

# Run E2E tests against a deployed EKS OIDC cluster
# Usage: just test-e2e-eks-oidc <project-dir> [test-filter] [options]
test-e2e-eks-oidc project_dir=e2e_dir test_filter="" options="":
    #!/usr/bin/env bash
    set -euo pipefail
    ROOT={{justfile_directory()}}
    JD={{jd_dir}}
    FILTER_ARG=""
    if [ -n "{{test_filter}}" ]; then
        FILTER_ARG="-k {{test_filter}}"
    fi
    E2E_COMPOSE=$(uv run --project "$JD" python -c \
        "from pytest_jupyter_deploy.image import IMAGE_PATH; print(IMAGE_PATH / 'docker-compose.yml')")
    {{container_tool}} compose --project-directory "$ROOT" -f "$E2E_COMPOSE" exec -e PYTHONUNBUFFERED=1 e2e \
        bash -c ". .venv/bin/activate && cd /workspace && \
        python -m pytest libs/jupyter-deploy-tf-aws-eks-oidc/tests/e2e \
        $FILTER_ARG \
        --project-dir {{project_dir}} \
        --auth-state .auth/github-oauth-state.json \
        -v"
