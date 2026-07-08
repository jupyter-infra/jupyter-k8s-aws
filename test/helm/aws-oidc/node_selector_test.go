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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Node Selector and Tolerations", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not include nodeSelector or tolerations with default values", func() {
		testOutputDir := filepath.Join(
			rootDir, "dist/test-output/aws-oidc/jupyter-k8s-aws-oidc/templates")

		deployments := []string{
			"traefik/deployment.yaml",
			"dex/deployment.yaml",
			"oauth2-proxy/deployment.yaml",
			"authmiddleware/deployment.yaml",
		}

		for _, d := range deployments {
			data, err := os.ReadFile(filepath.Join(testOutputDir, d))
			Expect(err).NotTo(HaveOccurred(), "Failed to read %s", d)

			var dep appsv1.Deployment
			Expect(yaml.Unmarshal(data, &dep)).To(Succeed(), "Failed to unmarshal %s", d)
			Expect(dep.Spec.Template.Spec.NodeSelector).To(BeEmpty(),
				"Expected no nodeSelector in %s with default values", d)
			Expect(dep.Spec.Template.Spec.Tolerations).To(BeEmpty(),
				"Expected no tolerations in %s with default values", d)
		}

		cronJobPath := filepath.Join(testOutputDir, "rotator/cronjob.yaml")
		data, err := os.ReadFile(cronJobPath)
		Expect(err).NotTo(HaveOccurred())

		var cj batchv1.CronJob
		Expect(yaml.Unmarshal(data, &cj)).To(Succeed())
		Expect(cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(BeEmpty(),
			"Expected no nodeSelector in cronjob with default values")
		Expect(cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations).To(BeEmpty(),
			"Expected no tolerations in cronjob with default values")
	})

	Context("with chart-level nodeSelector and tolerations", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			helmTemplate(chartDir, outputDir,
				helmSetFlag, "domain=test.example.com",
				helmSetFlag, "certManager.email=admin@example.com",
				helmSetFlag, "storageClass.efs.parameters.fileSystemId=fs-000",
				helmSetFlag, "github.clientId=cid",
				helmSetFlag, "github.clientSecret=csec",
				helmSetFlag, "github.orgs[0].name=org",
				helmSetFlag, "github.orgs[0].teams[0]=t",
				helmSetFlag, "githubRbac.orgs[0].name=org",
				helmSetFlag, "githubRbac.orgs[0].teams[0]=t",
				helmSetFlag, "oauth2Proxy.cookieSecret=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
				helmSetFlag, "webApp.enabled=true",
				helmSetFlag, "nodeSelector.jupyter-deploy/role=components",
				helmSetFlag, "tolerations[0].key=jupyter-deploy/role",
				helmSetFlag, "tolerations[0].operator=Equal",
				helmSetFlag, "tolerations[0].value=components",
				helmSetFlag, "tolerations[0].effect=NoSchedule",
			)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should set nodeSelector on all deployments", func() {
			deployments := []string{
				"traefik/deployment.yaml",
				"dex/deployment.yaml",
				"oauth2-proxy/deployment.yaml",
				"authmiddleware/deployment.yaml",
				"web-app/deployment.yaml",
			}

			for _, d := range deployments {
				data, err := os.ReadFile(filepath.Join(templatesDir, d))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", d)

				var dep appsv1.Deployment
				Expect(yaml.Unmarshal(data, &dep)).To(Succeed(), "Failed to unmarshal %s", d)
				Expect(dep.Spec.Template.Spec.NodeSelector).To(
					HaveKeyWithValue("jupyter-deploy/role", "components"),
					"Expected nodeSelector in %s", d)
				Expect(dep.Spec.Template.Spec.Tolerations).To(ContainElement(
					corev1.Toleration{
						Key:      "jupyter-deploy/role",
						Operator: corev1.TolerationOpEqual,
						Value:    "components",
						Effect:   corev1.TaintEffectNoSchedule,
					}), "Expected toleration in %s", d)
			}
		})

		It("should set nodeSelector on all cronjobs", func() {
			cronJobs := []string{
				"rotator/cronjob.yaml",
				"rotator/web-app-cronjob.yaml",
			}

			for _, c := range cronJobs {
				data, err := os.ReadFile(filepath.Join(templatesDir, c))
				Expect(err).NotTo(HaveOccurred(), "Failed to read %s", c)

				var cj batchv1.CronJob
				Expect(yaml.Unmarshal(data, &cj)).To(Succeed(), "Failed to unmarshal %s", c)
				Expect(cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(
					HaveKeyWithValue("jupyter-deploy/role", "components"),
					"Expected nodeSelector in %s", c)
				Expect(cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations).To(ContainElement(
					corev1.Toleration{
						Key:      "jupyter-deploy/role",
						Operator: corev1.TolerationOpEqual,
						Value:    "components",
						Effect:   corev1.TaintEffectNoSchedule,
					}), "Expected toleration in %s", c)
			}
		})
	})

	Context("with per-component override", func() {
		var templatesDir string

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			helmTemplate(chartDir, outputDir,
				helmSetFlag, "domain=test.example.com",
				helmSetFlag, "certManager.email=admin@example.com",
				helmSetFlag, "storageClass.efs.parameters.fileSystemId=fs-000",
				helmSetFlag, "github.clientId=cid",
				helmSetFlag, "github.clientSecret=csec",
				helmSetFlag, "github.orgs[0].name=org",
				helmSetFlag, "github.orgs[0].teams[0]=t",
				helmSetFlag, "githubRbac.orgs[0].name=org",
				helmSetFlag, "githubRbac.orgs[0].teams[0]=t",
				helmSetFlag, "oauth2Proxy.cookieSecret=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				helmSetFlag, "authmiddleware.enableBearerAuth=true",
				helmSetFlag, "nodeSelector.jupyter-deploy/role=components",
				helmSetFlag, "traefik.nodeSelector.jupyter-deploy/role=edge",
			)
			templatesDir = filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
		})

		It("should use per-component nodeSelector for traefik and chart-level for others", func() {
			data, err := os.ReadFile(filepath.Join(templatesDir, "traefik/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())
			var traefik appsv1.Deployment
			Expect(yaml.Unmarshal(data, &traefik)).To(Succeed())
			Expect(traefik.Spec.Template.Spec.NodeSelector).To(
				HaveKeyWithValue("jupyter-deploy/role", "edge"))

			data, err = os.ReadFile(filepath.Join(templatesDir, "dex/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())
			var dex appsv1.Deployment
			Expect(yaml.Unmarshal(data, &dex)).To(Succeed())
			Expect(dex.Spec.Template.Spec.NodeSelector).To(
				HaveKeyWithValue("jupyter-deploy/role", "components"))
		})
	})
})

func copyDir(src, dst string) {
	entries, err := os.ReadDir(src)
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			Expect(os.MkdirAll(dstPath, 0o755)).To(Succeed())
			copyDir(srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(dstPath, data, 0o644)).To(Succeed())
		}
	}
}

func helmTemplate(chartDir, outputDir string, extraArgs ...string) {
	out, err := exec.Command("helm", "dependency", "build", chartDir).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "helm dependency build failed: %s", string(out))

	args := append([]string{helmTemplateCmd, helmReleaseName, chartDir, "--output-dir", outputDir}, extraArgs...)
	out, err = exec.Command("helm", args...).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "helm template failed: %s", string(out))
}
