/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitHub RBAC", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	Context("namespace configurability (#26)", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				"--set", "githubRbac.namespace=jupyter-workspaces",
				"--set", "githubRbac.orgs[0].name=some-org",
				"--set", "githubRbac.orgs[0].teams[0]=devs",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should render Role and RoleBinding in the custom namespace", func() {
			rbacFiles := []string{
				"github-rbac/group-role.yaml",
				"github-rbac/group-rolebinding.yaml",
			}
			for _, f := range rbacFiles {
				data, err := os.ReadFile(filepath.Join(templatesDir, f))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", f)
				Expect(string(data)).To(ContainSubstring("namespace: jupyter-workspaces"),
					"%s should use the configured namespace", f)
			}
		})

		It("should not affect ClusterRole and ClusterRoleBinding (cluster-scoped)", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "github-rbac/group-clusterrole.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("namespace:"))

			data, err = os.ReadFile(filepath.Join(templatesDir, "github-rbac/group-clusterrolebinding.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("namespace:"))
		})
	})

	Context("orgs defaulting from github.orgs (#27)", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			// Only set github.orgs — do NOT set githubRbac.orgs
			helmTemplate(chartDir, outputDir, minimalOIDCArgs...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should use github.orgs for RoleBinding subjects when githubRbac.orgs is not set", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "github-rbac/group-rolebinding.yaml"))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("github:some-org:devs"),
				"RoleBinding should contain subjects from github.orgs")
		})

		It("should use github.orgs for ClusterRoleBinding subjects when githubRbac.orgs is not set", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "github-rbac/group-clusterrolebinding.yaml"))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("github:some-org:devs"),
				"ClusterRoleBinding should contain subjects from github.orgs")
		})
	})

	Context("orgs explicit override", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				"--set", "githubRbac.orgs[0].name=rbac-org",
				"--set", "githubRbac.orgs[0].teams[0]=rbac-team",
			)
			helmTemplate(chartDir, outputDir, args...)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should use githubRbac.orgs when explicitly set", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "github-rbac/group-rolebinding.yaml"))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("github:rbac-org:rbac-team"))
			Expect(content).NotTo(ContainSubstring("github:some-org:devs"),
				"Should use githubRbac.orgs, not github.orgs, when explicitly set")
		})
	})
})
