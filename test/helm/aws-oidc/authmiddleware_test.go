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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Auth Middleware", func() {
	Context("pod scheduling: affinity & topologySpreadConstraints", func() {
		var rootDir string

		BeforeEach(func() {
			var err error
			rootDir, err = filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
		})

		renderAuthPod := func(extraArgs ...string) corev1.PodSpec {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			args := append(oidcRequiredArgs(), extraArgs...)
			helmTemplate(chartDir, outputDir, args...)

			data, err := os.ReadFile(filepath.Join(outputDir,
				"jupyter-k8s-aws-oidc/templates/authmiddleware/deployment.yaml"))
			Expect(err).NotTo(HaveOccurred())
			var dep appsv1.Deployment
			Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
			return dep.Spec.Template.Spec
		}

		It("renders the default preferred podAntiAffinity when affinity is unset", func() {
			spec := renderAuthPod()
			Expect(spec.Affinity).NotTo(BeNil())
			Expect(spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).
				To(HaveLen(1))
			Expect(spec.TopologySpreadConstraints).To(BeEmpty())
		})

		It("replaces the default affinity entirely when affinity is set", func() {
			spec := renderAuthPod(
				helmSetFlag, "authmiddleware.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key=disktype",
				helmSetFlag, "authmiddleware.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator=Exists",
			)
			Expect(spec.Affinity).NotTo(BeNil())
			Expect(spec.Affinity.NodeAffinity).NotTo(BeNil())
			Expect(spec.Affinity.PodAntiAffinity).To(BeNil(),
				"setting affinity must drop the chart's default podAntiAffinity")
		})

		It("renders topologySpreadConstraints when set", func() {
			spec := renderAuthPod(
				helmSetFlag, "authmiddleware.topologySpreadConstraints[0].maxSkew=1",
				helmSetFlag, "authmiddleware.topologySpreadConstraints[0].topologyKey=zone",
				helmSetFlag, "authmiddleware.topologySpreadConstraints[0].whenUnsatisfiable=DoNotSchedule",
			)
			Expect(spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(spec.TopologySpreadConstraints[0].MaxSkew).To(Equal(int32(1)))
			Expect(spec.TopologySpreadConstraints[0].TopologyKey).To(Equal("zone"))
		})
	})

	Context("network policy", func() {
		var np networkingv1.NetworkPolicy

		BeforeEach(func() {
			rootDir, err := filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
			np = renderNetworkPolicy(rootDir, "authmiddleware/network-policy.yaml")
		})

		It("should allow :443 egress for the Kubernetes API server", func() {
			Expect(egressPorts(np)).To(HaveKey(443), "authmiddleware needs :443 egress for the in-cluster API server")
		})

		// authmiddleware has the same wait-for-dex initContainer as oauth2-proxy:
		// it probes http://dex.<ns>:5556/dex/healthz. That port is plaintext, so it
		// must be scoped to the dex pods in this namespace rather than open egress.
		It("should scope :5556 egress to dex pods in this namespace", func() {
			Expect(egressScopedToPodOnPort(np, 5556, "dex")).To(BeTrue(),
				":5556 egress should target dex pods, namespace-scoped")
		})

		// :80 plaintext egress was unused boilerplate (issuer/redirect are https,
		// upstream is static://); dropping it removes needless attack surface.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "authmiddleware makes no plaintext :80 egress; the rule should be removed")
		})
	})
})
