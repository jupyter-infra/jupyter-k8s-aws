/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// minimalOIDCArgs are the required values for rendering the aws-oidc chart.
var minimalOIDCArgs = []string{
	helmSetFlag, "domain=test.example.com",
	helmSetFlag, "certManager.email=admin@example.com",
	helmSetFlag, "github.clientId=cid",
	helmSetFlag, "github.clientSecret=csec",
	helmSetFlag, "github.orgs[0].name=some-org",
	helmSetFlag, "github.orgs[0].teams[0]=devs",
	helmSetFlag, "oauth2Proxy.cookieSecret=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
}

var _ = Describe("Access Strategy", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	Context("with default values (createOAuth=true, createBearer=false)", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			helmTemplate(chartDir, outputDir, minimalOIDCArgs...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should render the oauth access strategy", func() {
			path := filepath.Join(templatesDir, oauthStrategyFile)
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			content := string(data)
			Expect(content).To(ContainSubstring("kind: WorkspaceAccessStrategy"))
			Expect(content).To(ContainSubstring("name: oauth-access-strategy"))
			Expect(content).To(ContainSubstring("namespace: jupyter-k8s-system"))
			Expect(content).To(ContainSubstring("test.example.com"))
			Expect(content).To(ContainSubstring("oauth-auth-redirect"))
			Expect(content).To(ContainSubstring(
				"applicationBasePathTemplate: \"/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/\""),
				"oauth strategy should set applicationBasePathTemplate for idle-detection path resolution")
		})

		It("should not render the bearer access strategy", func() {
			path := filepath.Join(templatesDir, bearerStrategyFile)
			_, err := os.ReadFile(path)
			Expect(os.IsNotExist(err)).To(BeTrue(),
				"bearer-access-strategy.yaml should not be rendered when createBearer=false")
		})
	})

	Context("with custom namespace", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.namespace=jupyter-workspaces",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should render the oauth strategy in the custom namespace", func() {
			path := filepath.Join(templatesDir, oauthStrategyFile)
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("namespace: jupyter-workspaces"))
		})
	})

	Context("with bearer enabled", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should render both access strategies", func() {
			oauthPath := filepath.Join(templatesDir, oauthStrategyFile)
			data, err := os.ReadFile(oauthPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: oauth-access-strategy"))

			bearerPath := filepath.Join(templatesDir, bearerStrategyFile)
			data, err = os.ReadFile(bearerPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("name: bearer-access-strategy"))
			Expect(content).To(ContainSubstring("createConnectionHandler: \"k8s-native\""))
			Expect(content).To(ContainSubstring("bearerAuthURLTemplate"))
			Expect(content).To(ContainSubstring("test.example.com"))
			Expect(content).To(ContainSubstring(
				"applicationBasePathTemplate: \"/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/\""),
				"bearer strategy should set applicationBasePathTemplate for idle-detection path resolution")
		})
	})

	Context("with both strategies disabled", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createOAuth=false",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should not render any access strategy", func() {
			oauthPath := filepath.Join(templatesDir, oauthStrategyFile)
			_, err := os.ReadFile(oauthPath)
			Expect(os.IsNotExist(err)).To(BeTrue(),
				"oauth-access-strategy.yaml should not be rendered when createOAuth=false")

			bearerPath := filepath.Join(templatesDir, bearerStrategyFile)
			_, err = os.ReadFile(bearerPath)
			Expect(os.IsNotExist(err)).To(BeTrue(),
				"bearer-access-strategy.yaml should not be rendered when createBearer=false")
		})
	})

	Context("with workspace template variables", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should preserve Go template variables in rendered output", func() {
			strategies := []string{
				oauthStrategyFile,
				bearerStrategyFile,
			}
			for _, s := range strategies {
				data, err := os.ReadFile(filepath.Join(templatesDir, s))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", s)
				content := string(data)
				Expect(content).To(ContainSubstring("{{ .Workspace.Name }}"), "%s should contain workspace name template var", s)
				Expect(content).To(ContainSubstring("{{ .Workspace.Namespace }}"), "%s should contain workspace namespace template var", s)
				Expect(content).To(ContainSubstring("{{ .Service.Name }}"), "%s should contain service name template var", s)
				Expect(content).To(ContainSubstring("{{ .Service.Namespace }}"), "%s should contain service namespace template var", s)
			}
		})
	})

	Context("startup probe paths", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should probe the /auth path for the oauth strategy", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, oauthStrategyFile))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("/auth"))
			Expect(content).To(ContainSubstring("additionalSuccessStatusCodes: [302]"))
			Expect(content).NotTo(ContainSubstring("additionalSuccessStatusCodes: [401]"))
		})

		It("should probe the /bearer-auth path for the bearer strategy", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, bearerStrategyFile))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("/bearer-auth"))
			Expect(content).To(ContainSubstring("additionalSuccessStatusCodes: [400]"))
		})
	})

	Context("with createNetworkPolicy=true (default)", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should include NetworkPolicy in both access strategies", func() {
			strategies := []string{
				oauthStrategyFile,
				bearerStrategyFile,
			}
			for _, s := range strategies {
				data, err := os.ReadFile(filepath.Join(templatesDir, s))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", s)
				content := string(data)
				Expect(content).To(ContainSubstring("kind: NetworkPolicy"), "%s should contain NetworkPolicy", s)
				Expect(content).To(ContainSubstring("kind: IngressRoute"), "%s should contain IngressRoute", s)
			}
		})
	})

	Context("with createNetworkPolicy=false", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
				helmSetFlag, "accessStrategy.createNetworkPolicy=false",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should not include NetworkPolicy but still have IngressRoute", func() {
			strategies := []string{
				oauthStrategyFile,
				bearerStrategyFile,
			}
			for _, s := range strategies {
				data, err := os.ReadFile(filepath.Join(templatesDir, s))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", s)
				content := string(data)
				Expect(content).NotTo(ContainSubstring("kind: NetworkPolicy"), "%s should NOT contain NetworkPolicy", s)
				Expect(content).To(ContainSubstring("kind: IngressRoute"), "%s should still contain IngressRoute", s)
			}
		})
	})

	Context("bearer validation", func() {
		It("should fail when createBearer is true but enableBearerAuth is false", func() {
			rootDir, err := filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())

			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			out, err := exec.Command("helm", "dependency", "build", chartDir).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "helm dependency build failed: %s", string(out))

			args := append([]string{helmTemplateCmd, helmReleaseName, chartDir, "--output-dir", outputDir},
				minimalOIDCArgs[:]...,
			)
			args = append(args,
				helmSetFlag, "accessStrategy.createBearer=true",
				helmSetFlag, "authmiddleware.enableBearerAuth=false",
			)
			out, err = exec.Command("helm", args...).CombinedOutput()
			Expect(err).To(HaveOccurred(), "helm template should have failed")
			Expect(string(out)).To(ContainSubstring(
				"accessStrategy.createBearer requires authmiddleware.enableBearerAuth"))
		})
	})
})
