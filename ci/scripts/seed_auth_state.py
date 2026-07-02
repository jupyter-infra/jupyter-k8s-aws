#!/usr/bin/env python3
"""One-time auth state seeding for CI.

Logs in as the bot account against a deployed cluster using automated 2FA,
then saves the Playwright storage state to .auth/github-oauth-state.json.

Usage:
    cd /path/to/jupyter-k8s-aws
    python ci/scripts/seed_auth_state.py --project-dir <path-to-deployed-jd-project>

Example:
    python ci/scripts/seed_auth_state.py --project-dir ~/ci-clusters/org-pr

After running, export the state to Secrets Manager:
    just auth-export sandbox-ci
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

import boto3


def get_secret(secret_id: str, region: str) -> str:
    client = boto3.client("secretsmanager", region_name=region)
    return client.get_secret_value(SecretId=secret_id)["SecretString"]


def get_ssm(name: str, region: str) -> str:
    client = boto3.client("ssm", region_name=region)
    return client.get_parameter(Name=name)["Parameter"]["Value"]


def get_totp(totp_secret: str) -> str:
    result = subprocess.run(
        ["oathtool", "-b", "--totp", totp_secret],
        capture_output=True, text=True, check=True
    )
    return result.stdout.strip()


def main() -> None:
    parser = argparse.ArgumentParser(description="Seed Playwright auth state for CI")
    parser.add_argument("--project-dir", type=Path,
                        help="Path to deployed JD project (e.g. ~/ci-clusters/org-pr)")
    parser.add_argument("--url",
                        help="Direct URL of deployed app (alternative to --project-dir)")
    parser.add_argument("--region", default="us-west-2")
    parser.add_argument("--storage-state", type=Path,
                        default=Path(".auth/github-oauth-state.json"))
    args = parser.parse_args()

    if not args.project_dir and not args.url:
        print("Error: must provide either --project-dir or --url")
        sys.exit(1)

    print("Reading bot credentials from Secrets Manager...")
    email = get_ssm("/jupyter-k8s-aws-ci/github-bot-account-email", args.region)
    password = get_secret("jupyter-k8s-aws-ci/github-bot-account-password", args.region)
    totp_secret = get_secret("jupyter-k8s-aws-ci/github-bot-account-totp-secret", args.region)

    print(f"Bot account: {email}")

    if args.url:
        jupyterlab_url = args.url.rstrip("/")
    else:
        if not args.project_dir.exists():
            print(f"Error: project dir does not exist: {args.project_dir}")
            sys.exit(1)
        print("Getting workspace URL from project...")
        result = subprocess.run(
            ["jd", "show", "-o", "workspace_base_url", "--text"],
            cwd=args.project_dir, capture_output=True, text=True, check=True
        )
        workspace_url = result.stdout.strip()
        jupyterlab_url = workspace_url.replace("/workspaces", "")
    print(f"Target URL: {jupyterlab_url}")

    print("Launching Playwright...")
    from playwright.sync_api import sync_playwright
    from pytest_jupyter_deploy.oauth2_proxy.github import GitHubOAuth2ProxyApplication

    args.storage_state.parent.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as p:
        browser = p.firefox.launch(headless=True)
        context = browser.new_context()
        page = context.new_page()

        # Step 1: Full GitHub login to establish logged_in=yes session cookie.
        # Must do this before OAuth flow — OAuth alone only sets logged_in=no.
        print("Step 1: GitHub direct login (username + password + TOTP)...")
        page.goto("https://github.com/login", wait_until="load", timeout=30000)
        page.fill('input[name="login"]', email)
        page.fill('input[name="password"]', password)
        page.click('input[type="submit"]')
        page.wait_for_url("**/sessions/two-factor/app**", timeout=30000)
        totp_code = get_totp(totp_secret)
        page.fill("#app_totp", totp_code)
        try:
            page.wait_for_url(lambda url: "two-factor" not in url, timeout=10000)
        except Exception:
            page.click('button[type="submit"]')
            page.wait_for_url(lambda url: "two-factor" not in url, timeout=10000)
        print(f"  GitHub login complete, now at: {page.url}")

        # Step 2: Navigate to app URL to complete OAuth flow and get app cookies.
        print("Step 2: OAuth flow via app URL...")
        oauth_app = GitHubOAuth2ProxyApplication(
            page=page,
            jupyterlab_url=jupyterlab_url,
            storage_state_path=args.storage_state,
            ci_email=email,
            ci_password=password,
            ci_totp_fn=lambda: get_totp(totp_secret),
        )
        oauth_app.ensure_authenticated()

        browser.close()

    print(f"\nAuth state saved to {args.storage_state}")
    print("\nNext step: export to Secrets Manager:")
    print("  just auth-export sandbox-ci")


if __name__ == "__main__":
    main()
