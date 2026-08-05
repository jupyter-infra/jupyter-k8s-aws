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
				helmSetFlag, "githubRbac.namespace=jupyter-workspaces",
				helmSetFlag, "githubRbac.orgs[0].name=some-org",
				helmSetFlag, "githubRbac.orgs[0].teams[0]=devs",
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
				helmSetFlag, "githubRbac.orgs[0].name=rbac-org",
				helmSetFlag, "githubRbac.orgs[0].teams[0]=rbac-team",
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

	Context("shared-discovery Role (bare-install parity)", func() {
		// renderSharedDiscovery renders with createSharedDiscoveryRole enabled (the
		// feature is opt-in), returning the file content or "" if helm omitted it.
		renderSharedDiscovery := func(extraArgs ...string) string {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(minimalOIDCArgs,
				helmSetFlag, "githubRbac.createSharedDiscoveryRole=true")
			args = append(args, extraArgs...)
			helmTemplate(chartDir, outputDir, args...)
			data, err := os.ReadFile(filepath.Join(outputDir,
				"jupyter-k8s-aws-oidc/templates/github-rbac/shared-discovery-role.yaml"))
			if err != nil {
				return ""
			}
			return string(data)
		}

		It("does not render by default (createSharedDiscoveryRole off)", func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			helmTemplate(chartDir, outputDir, minimalOIDCArgs...)
			_, err := os.ReadFile(filepath.Join(outputDir,
				"jupyter-k8s-aws-oidc/templates/github-rbac/shared-discovery-role.yaml"))
			Expect(err).To(HaveOccurred(),
				"shared-discovery Role should be omitted unless createSharedDiscoveryRole=true")
		})

		It("renders Role + RoleBinding in webApp.sharedNamespace with group subjects when enabled", func() {
			out := renderSharedDiscovery()
			Expect(out).To(ContainSubstring("kind: Role"))
			Expect(out).To(ContainSubstring("kind: RoleBinding"))
			Expect(out).To(ContainSubstring("name: github-shared-discovery-reader"))
			Expect(out).To(ContainSubstring("namespace: jupyter-k8s-shared"))
			Expect(out).To(ContainSubstring("workspacetemplates"))
			Expect(out).To(ContainSubstring("workspaceaccessstrategies"))
			// minimalOIDCArgs sets github.orgs[0]={some-org, teams:[devs]}.
			Expect(out).To(ContainSubstring("github:some-org:devs"))
		})

		It("does not render when githubRbac.create is false", func() {
			Expect(renderSharedDiscovery(helmSetFlag, "githubRbac.create=false")).To(BeEmpty())
		})

		It("does not render when createOrgsRole is false", func() {
			Expect(renderSharedDiscovery(helmSetFlag, "githubRbac.createOrgsRole=false")).To(BeEmpty())
		})

		It("lands in webApp.sharedNamespace, not the router namespace or githubRbac.namespace", func() {
			out := renderSharedDiscovery(
				helmSetFlag, "webApp.sharedNamespace=my-shared",
				helmSetFlag, "githubRbac.namespace=rbac-ns",
			)
			Expect(out).To(ContainSubstring("namespace: my-shared"))
			Expect(out).NotTo(ContainSubstring("namespace: jupyter-k8s-router"))
			Expect(out).NotTo(ContainSubstring("namespace: rbac-ns"))
		})

		It("renders one Group subject per team, and the org itself when teamless", func() {
			// Two teams under one org → two team-scoped Group subjects.
			multi := renderSharedDiscovery(
				helmSetFlag, "githubRbac.orgs[0].name=acme",
				helmSetFlag, "githubRbac.orgs[0].teams[0]=t1",
				helmSetFlag, "githubRbac.orgs[0].teams[1]=t2",
			)
			Expect(multi).To(ContainSubstring("github:acme:t1"))
			Expect(multi).To(ContainSubstring("github:acme:t2"))

			// Org with no teams → a single org-level Group subject.
			teamless := renderSharedDiscovery(
				helmSetFlag, "githubRbac.orgs[0].name=acme",
			)
			Expect(teamless).To(ContainSubstring("name: github:acme"))
			Expect(teamless).NotTo(ContainSubstring("github:acme:"))
		})
	})
})
