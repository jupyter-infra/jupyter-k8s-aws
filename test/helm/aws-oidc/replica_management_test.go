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

	// scaledObjectFile maps component to its ScaledObject template filename.
	scaledObjectFile := map[string]string{
		componentTraefik:        "hpa.yaml",
		componentAuthmiddleware: "hpa.yaml",
		componentWebApp:         "scaledobject.yaml",
	}

	scaledObjectExists := func(component string, extraArgs ...string) bool {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(), extraArgs...)
		helmTemplate(chartDir, outputDir, args...)
		path := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates", component, scaledObjectFile[component])
		_, err := os.Stat(path)
		return err == nil
	}

	// Per-component config drives the shared spec table below. enableArgs turns
	// the component on (traefik is on by default; the others need a flag), and
	// valuesKey is the top-level values.yaml key for the keda.enabled toggle.
	for _, tc := range []struct {
		component  string
		valuesKey  string
		enableArgs []string
	}{
		{componentTraefik, "traefik", nil},
		{componentAuthmiddleware, "authmiddleware", []string{helmSetFlag, "authmiddleware.enabled=true"}},
		{componentWebApp, "webApp", []string{helmSetFlag, "webApp.enabled=true"}},
	} {
		kedaEnabled := tc.valuesKey + ".keda.enabled=true"

		enabledNoCRD := append(append([]string{}, tc.enableArgs...), helmSetFlag, kedaEnabled)
		enabledWithCRD := append(append([]string{}, enabledNoCRD...), "--api-versions", "keda.sh/v1alpha1")

		Context(tc.component, func() {
			It("should set replicas when keda.enabled=false (default)", func() {
				dep := renderDeployment(tc.component, tc.enableArgs...)
				Expect(dep.Spec.Replicas).NotTo(BeNil(),
					"%s Deployment should set replicas when keda is disabled (default)", tc.component)
				Expect(*dep.Spec.Replicas).To(BeEquivalentTo(2))
			})

			It("should omit replicas when keda.enabled=true and CRD is installed", func() {
				dep := renderDeployment(tc.component, enabledWithCRD...)
				Expect(dep.Spec.Replicas).To(BeNil(),
					"%s Deployment must not set replicas when keda.enabled=true and CRD is present so KEDA owns the count", tc.component)
			})

			It("should set replicas when keda.enabled=true but CRD is absent", func() {
				dep := renderDeployment(tc.component, enabledNoCRD...)
				Expect(dep.Spec.Replicas).NotTo(BeNil(),
					"%s Deployment must set replicas when keda.enabled=true but CRD is absent to avoid defaulting to 1", tc.component)
			})

			It("should render ScaledObject when keda.enabled=true and CRD is installed", func() {
				Expect(scaledObjectExists(tc.component, enabledWithCRD...)).To(BeTrue(),
					"%s ScaledObject must be rendered when keda.enabled=true and CRD is present", tc.component)
			})

			It("should not render ScaledObject when keda.enabled=false", func() {
				Expect(scaledObjectExists(tc.component, tc.enableArgs...)).To(BeFalse(),
					"%s ScaledObject must not be rendered when keda is disabled", tc.component)
			})

			It("should not render ScaledObject when keda.enabled=true but CRD is absent", func() {
				Expect(scaledObjectExists(tc.component, enabledNoCRD...)).To(BeFalse(),
					"%s ScaledObject must not be rendered when CRD is absent", tc.component)
			})
		})
	}
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
