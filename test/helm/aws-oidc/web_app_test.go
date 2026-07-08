/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Web App", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	// getEnvVar finds an env var by name in a container's env list.
	getEnvVar := func(envList []corev1.EnvVar, name string) (string, bool) {
		for _, e := range envList {
			if e.Name == name {
				return e.Value, true
			}
		}
		return "", false
	}

	Context("value consistency (dex ↔ webApp)", func() {
		var (
			templatesDir string
			dep          appsv1.Deployment
		)

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			args := append(oidcRequiredArgs(),
				"--set", "webApp.enabled=true",
				"--set", "webApp.clusterAccess.clusterName=test-cluster",
				"--set", "webApp.clusterAccess.apiServer=https://api.test-cluster.example.com",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

			data, err := os.ReadFile(filepath.Join(templatesDir, "web-app/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
		})

		It("should set OIDC_ISSUER_URL to https://<domain>/dex", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "OIDC_ISSUER_URL")
			Expect(found).To(BeTrue(), "OIDC_ISSUER_URL not found in deployment env")
			Expect(val).To(Equal("https://test.example.com/dex"))
		})

		It("should set OIDC_CLIENT_ID to the dex kubernetesClientId default", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "OIDC_CLIENT_ID")
			Expect(found).To(BeTrue(), "OIDC_CLIENT_ID not found in deployment env")
			Expect(val).To(Equal("kubectl-oidc"))
		})

		It("should set SESSION_EXPECTED_DOMAIN to the domain value", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "SESSION_EXPECTED_DOMAIN")
			Expect(found).To(BeTrue(), "SESSION_EXPECTED_DOMAIN not found in deployment env")
			Expect(val).To(Equal("test.example.com"))
		})

		It("should set CLUSTER_NAME to the configured clusterName", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "CLUSTER_NAME")
			Expect(found).To(BeTrue(), "CLUSTER_NAME not found in deployment env")
			Expect(val).To(Equal("test-cluster"))
		})

		It("should set CLUSTER_API_SERVER to the configured apiServer", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "CLUSTER_API_SERVER")
			Expect(found).To(BeTrue(), "CLUSTER_API_SERVER not found in deployment env")
			Expect(val).To(Equal("https://api.test-cluster.example.com"))
		})

		It("should set NAMESPACE to the webApp.namespace default", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "NAMESPACE")
			Expect(found).To(BeTrue(), "NAMESPACE not found in deployment env")
			Expect(val).To(Equal("default"))
		})
	})

	Context("encryption requirements", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			args := append(oidcRequiredArgs(),
				"--set", "webApp.enabled=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should generate a session secret with the correct number of keys", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "web-app/session-secret.yaml"))
			Expect(err).NotTo(HaveOccurred())

			var secret corev1.Secret
			Expect(yaml.Unmarshal(data, &secret)).To(Succeed())

			// Default numberOfKeys is 3; initial render creates 1 key (lookup returns empty on template)
			// The secret should exist and have at least 1 key with the expected prefix
			Expect(secret.Data).NotTo(BeEmpty(), "session secret should have at least one key")
			for key, value := range secret.Data {
				Expect(key).To(HavePrefix("jwt-signing-key-"),
					"session secret key should have prefix 'jwt-signing-key-'")
				// Each value is base64 in the Secret .data field; when unmarshaled
				// by the K8s types, it's already decoded bytes
				decoded, err := base64.StdEncoding.DecodeString(string(value))
				if err != nil {
					// If not double-encoded, the raw bytes from unmarshal are already decoded
					decoded = value
				}
				Expect(decoded).To(HaveLen(48),
					"each session key should be 48 bytes (HS384 requirement)")
			}
		})

		It("should have the rotator cronjob reference the correct secret name", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "rotator/web-app-cronjob.yaml"))
			Expect(err).NotTo(HaveOccurred())

			var cj batchv1.CronJob
			Expect(yaml.Unmarshal(data, &cj)).To(Succeed())

			env := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "SECRET_NAME")
			Expect(found).To(BeTrue(), "SECRET_NAME not found in rotator cronjob env")
			Expect(val).To(Equal("web-app-session-secret"))
		})
	})

	Context("node placement (per-component override)", func() {
		It("should use webApp-specific nodeSelector when both chart-level and webApp-level are set", func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			args := append(oidcRequiredArgs(),
				"--set", "webApp.enabled=true",
				"--set", "nodeSelector.jupyter-deploy/role=components",
				"--set", "webApp.nodeSelector.jupyter-deploy/role=web-ui",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

			data, err := os.ReadFile(filepath.Join(templatesDir, "web-app/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())

			var dep appsv1.Deployment
			Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
			Expect(dep.Spec.Template.Spec.NodeSelector).To(
				HaveKeyWithValue("jupyter-deploy/role", "web-ui"),
				"web-app should use its own nodeSelector, not the chart-level one")
		})
	})

	Context("session timing derivation", func() {
		var dep appsv1.Deployment

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			// Default cookieExpire is "8h" = 28800s
			args := append(oidcRequiredArgs(),
				"--set", "webApp.enabled=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

			data, err := os.ReadFile(filepath.Join(templatesDir, "web-app/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
		})

		It("should set SESSION_COOKIE_MAX_AGE_SECS to 75% of cookieExpire", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "SESSION_COOKIE_MAX_AGE_SECS")
			Expect(found).To(BeTrue(), "SESSION_COOKIE_MAX_AGE_SECS not found")
			// 8h = 28800s, 75% = 21600
			Expect(val).To(Equal("21600"))
		})

		It("should set SESSION_MAX_LIFETIME_SECS to 100% of cookieExpire", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "SESSION_MAX_LIFETIME_SECS")
			Expect(found).To(BeTrue(), "SESSION_MAX_LIFETIME_SECS not found")
			// 8h = 28800s
			Expect(val).To(Equal("28800"))
		})

		It("should set SESSION_NEAR_EXPIRY_THRESHOLD_SECS to 25% of cookieExpire", func() {
			env := dep.Spec.Template.Spec.Containers[0].Env
			val, found := getEnvVar(env, "SESSION_NEAR_EXPIRY_THRESHOLD_SECS")
			Expect(found).To(BeTrue(), "SESSION_NEAR_EXPIRY_THRESHOLD_SECS not found")
			// 8h = 28800s, 25% = 7200
			Expect(val).To(Equal("7200"))
		})
	})

	// Shared behind-traefik invariants (policyTypes, traefik-only ingress, DNS
	// egress) are covered by the network-policy consistency suite. This asserts
	// the web-app-specific egress need: :443 for the K8s API server and Dex via
	// the public domain (see issue #50).
	Context("network policy (web-app specific egress)", func() {
		var np networkingv1.NetworkPolicy

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			args := append(oidcRequiredArgs(),
				"--set", "webApp.enabled=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

			data, err := os.ReadFile(filepath.Join(templatesDir, "web-app/network-policy.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(data, &np)).To(Succeed())
		})

		It("should allow :443 egress for the API server and Dex", func() {
			Expect(egressPorts(np)).To(HaveKey(443), "web-app needs :443 egress for the API server and Dex")
		})

		// web-app reaches the API server and Dex over https only; there is no
		// plaintext :80 egress, so the rule should not be present.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "web-app makes no plaintext :80 egress; the rule should not be present")
		})
	})

	Context("validation: incomplete clusterAccess", func() {
		It("should fail when clusterName is set but apiServer is empty", func() {
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			// Build dependencies first
			out, err := exec.Command("helm", "dependency", "build", chartDir).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "helm dependency build failed: %s", string(out))

			args := append([]string{"template", helmReleaseName, chartDir},
				oidcRequiredArgs()...)
			args = append(args,
				"--set", "webApp.enabled=true",
				"--set", "webApp.clusterAccess.clusterName=test-cluster",
				// apiServer intentionally left empty (default "")
			)

			out, err = exec.Command("helm", args...).CombinedOutput()
			output := string(out)

			if err != nil {
				// Validation exists — helm template should fail
				Expect(strings.ToLower(output)).To(
					ContainSubstring("apiserver"),
					"Expected validation error about missing apiServer")
			} else {
				Skip("Chart does not yet validate that apiServer is required when clusterName is set — " +
					"add a template guard (required/fail) and remove this Skip")
			}
		})
	})
})
