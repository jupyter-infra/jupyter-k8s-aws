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

const (
	componentTraefik        = "traefik"
	componentAuthmiddleware = "authmiddleware"
	componentWebApp         = "web-app"
)

// kedaTimingValues mirrors the keda sub-block in values.yaml for parsing.
type kedaTimingValues struct {
	CooldownPeriodSeconds  int `yaml:"cooldownPeriodSeconds"`
	PollingIntervalSeconds int `yaml:"pollingIntervalSeconds"`
}

type kedaBlock struct {
	Keda kedaTimingValues `yaml:"keda"`
}

type kedaTimingChart struct {
	Traefik        kedaBlock `yaml:"traefik"`
	Authmiddleware kedaBlock `yaml:"authmiddleware"`
	WebApp         kedaBlock `yaml:"webApp"`
}

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
			dep := renderDeployment(componentTraefik)
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"traefik Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true and CRD is installed", func() {
			dep := renderDeployment(componentTraefik,
				helmSetFlag, "traefik.keda.enabled=true",
				"--api-versions", "keda.sh/v1alpha1")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"traefik Deployment must not set replicas when keda.enabled=true and CRD is present so KEDA owns the count")
		})

		It("should set replicas when keda.enabled=true but CRD is absent", func() {
			dep := renderDeployment(componentTraefik, helmSetFlag, "traefik.keda.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"traefik Deployment must set replicas when keda.enabled=true but CRD is absent to avoid defaulting to 1")
		})
	})

	Context("authmiddleware", func() {
		It("should set replicas when keda.enabled=false (default)", func() {
			dep := renderDeployment(componentAuthmiddleware,
				helmSetFlag, "authmiddleware.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"authmiddleware Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true and CRD is installed", func() {
			dep := renderDeployment(componentAuthmiddleware,
				helmSetFlag, "authmiddleware.enabled=true",
				helmSetFlag, "authmiddleware.keda.enabled=true",
				"--api-versions", "keda.sh/v1alpha1")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"authmiddleware Deployment must not set replicas when keda.enabled=true and CRD is present so KEDA owns the count")
		})

		It("should set replicas when keda.enabled=true but CRD is absent", func() {
			dep := renderDeployment(componentAuthmiddleware,
				helmSetFlag, "authmiddleware.enabled=true",
				helmSetFlag, "authmiddleware.keda.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"authmiddleware Deployment must set replicas when keda.enabled=true but CRD is absent to avoid defaulting to 1")
		})
	})

	Context("web-app", func() {
		It("should set replicas when keda.enabled=false (default)", func() {
			dep := renderDeployment(componentWebApp,
				helmSetFlag, "webApp.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"web-app Deployment should set replicas when keda is disabled (default)")
			Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
		})

		It("should omit replicas when keda.enabled=true and CRD is installed", func() {
			dep := renderDeployment(componentWebApp,
				helmSetFlag, "webApp.enabled=true",
				helmSetFlag, "webApp.keda.enabled=true",
				"--api-versions", "keda.sh/v1alpha1")
			Expect(dep.Spec.Replicas).To(BeNil(),
				"web-app Deployment must not set replicas when keda.enabled=true and CRD is present so KEDA owns the count")
		})

		It("should set replicas when keda.enabled=true but CRD is absent", func() {
			dep := renderDeployment(componentWebApp,
				helmSetFlag, "webApp.enabled=true",
				helmSetFlag, "webApp.keda.enabled=true")
			Expect(dep.Spec.Replicas).NotTo(BeNil(),
				"web-app Deployment must set replicas when keda.enabled=true but CRD is absent to avoid defaulting to 1")
		})
	})
})

var _ = Describe("KEDA timing parameters", func() {
	It("pollingIntervalSeconds < cooldownPeriodSeconds for all components", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(filepath.Join(rootDir, "charts/aws-oidc/values.yaml"))
		Expect(err).NotTo(HaveOccurred())

		var chart kedaTimingChart
		Expect(yaml.Unmarshal(data, &chart)).To(Succeed())

		for _, tc := range []struct {
			name     string
			polling  int
			cooldown int
		}{
			{componentTraefik, chart.Traefik.Keda.PollingIntervalSeconds, chart.Traefik.Keda.CooldownPeriodSeconds},
			{componentAuthmiddleware, chart.Authmiddleware.Keda.PollingIntervalSeconds, chart.Authmiddleware.Keda.CooldownPeriodSeconds},
			{componentWebApp, chart.WebApp.Keda.PollingIntervalSeconds, chart.WebApp.Keda.CooldownPeriodSeconds},
		} {
			Expect(tc.polling).To(BeNumerically(">", 0),
				"%s: pollingIntervalSeconds must be a positive integer", tc.name)
			Expect(tc.cooldown).To(BeNumerically(">", 0),
				"%s: cooldownPeriodSeconds must be a positive integer", tc.name)
			Expect(tc.polling).To(BeNumerically("<", tc.cooldown),
				"%s: pollingIntervalSeconds (%d) must be less than cooldownPeriodSeconds (%d)",
				tc.name, tc.polling, tc.cooldown)
		}
	})
})
