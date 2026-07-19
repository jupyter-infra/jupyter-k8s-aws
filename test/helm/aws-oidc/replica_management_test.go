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
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Replica management", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	renderDeployment := func(component string, extraArgs ...string) appsv1.Deployment {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(), extraArgs...)
		helmTemplate(chartDir, outputDir, args...)
		data, err := os.ReadFile(filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates", component, "deployment.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var dep appsv1.Deployment
		Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
		return dep
	}

	Context("traefik", func() {
		It("should set replicas when keda.enabled=false (default)", func() {
			dep := renderDeployment("traefik")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"traefik Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true", func() {
			dep := renderDeployment("traefik", helmSetFlag, "traefik.keda.enabled=true")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"traefik Deployment must not set replicas when keda.enabled=true so KEDA owns the count")
		})
	})

	Context("authmiddleware", func() {
		It("should set replicas when keda.enabled=false (default)", func() {
			dep := renderDeployment("authmiddleware",
				helmSetFlag, "authmiddleware.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"authmiddleware Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true", func() {
			dep := renderDeployment("authmiddleware",
				helmSetFlag, "authmiddleware.enabled=true",
				helmSetFlag, "authmiddleware.keda.enabled=true")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"authmiddleware Deployment must not set replicas when keda.enabled=true so KEDA owns the count")
		})
	})

	Context("web-app", func() {
		It("should set replicas when keda.enabled=false (default)", func() {
			dep := renderDeployment("web-app",
				helmSetFlag, "webApp.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"web-app Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true", func() {
			dep := renderDeployment("web-app",
				helmSetFlag, "webApp.enabled=true",
				helmSetFlag, "webApp.keda.enabled=true")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"web-app Deployment must not set replicas when keda.enabled=true so KEDA owns the count")
		})
	})
})
