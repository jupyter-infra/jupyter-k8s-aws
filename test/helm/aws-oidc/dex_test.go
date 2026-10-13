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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const dexConfigChecksumKey = "checksum/config"

// renderDexConfigChecksum renders the chart and returns the dex pod template's
// config checksum annotation. It pins dex.oauth2ProxyClientSecret: left unset,
// the chart generates a fresh one per render, so the checksum would never match.
func renderDexConfigChecksum(rootDir string, extraArgs ...string) string {
	outputDir := GinkgoT().TempDir()
	chartDir := GinkgoT().TempDir()
	copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

	args := append(oidcRequiredArgs(), helmSetFlag, "dex.oauth2ProxyClientSecret=pinned")
	helmTemplate(chartDir, outputDir, append(args, extraArgs...)...)
	data, err := os.ReadFile(filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates/dex/deployment.yaml"))
	Expect(err).NotTo(HaveOccurred())

	var dep appsv1.Deployment
	Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
	Expect(dep.Spec.Template.Annotations).To(HaveKey(dexConfigChecksumKey))
	return dep.Spec.Template.Annotations[dexConfigChecksumKey]
}

var _ = Describe("Dex", func() {
	// Regression guard for #100: dex reads its config only at startup, so a
	// config change must alter the pod template or helm upgrade never rolls dex.
	Context("config checksum", func() {
		var rootDir, baseline string

		BeforeEach(func() {
			var err error
			rootDir, err = filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
			baseline = renderDexConfigChecksum(rootDir)
		})

		It("should be a sha256 hex digest", func() {
			Expect(baseline).To(MatchRegexp(`^[0-9a-f]{64}$`))
		})

		It("should be stable across renders with the same values", func() {
			Expect(renderDexConfigChecksum(rootDir)).To(Equal(baseline))
		})

		It("should change when a team is added to github.orgs", func() {
			Expect(renderDexConfigChecksum(rootDir,
				helmSetFlag, "github.orgs[0].teams[1]=t2",
			)).NotTo(Equal(baseline))
		})

		It("should change when an org is added to github.orgs", func() {
			Expect(renderDexConfigChecksum(rootDir,
				helmSetFlag, "github.orgs[1].name=org2",
				helmSetFlag, "github.orgs[1].teams[0]=t",
			)).NotTo(Equal(baseline))
		})

		// oauth2-proxy rolls on a client secret change (it is a container arg), so
		// dex must roll with it or the token exchange fails on the stale secret.
		It("should change when the oauth2-proxy client secret changes", func() {
			Expect(renderDexConfigChecksum(rootDir,
				helmSetFlag, "dex.oauth2ProxyClientSecret=rotated",
			)).NotTo(Equal(baseline))
		})

		It("should not change when only a non-dex value changes", func() {
			Expect(renderDexConfigChecksum(rootDir,
				helmSetFlag, "githubRbac.orgs[0].teams[1]=t2",
			)).To(Equal(baseline))
		})
	})

	Context("network policy", func() {
		var np networkingv1.NetworkPolicy

		BeforeEach(func() {
			rootDir, err := filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
			np = renderNetworkPolicy(rootDir, "dex/network-policy.yaml")
		})

		// Regression guard for #49: both oauth2-proxy and authmiddleware run a
		// wait-for-dex initContainer that probes http://dex:5556/dex/healthz
		// directly in-cluster. Under NetworkPolicy enforcement, dex must allow
		// that in-cluster ingress or the init gate hangs on every clean deploy.
		It("should allow in-cluster ingress on :5556 from oauth2-proxy and authmiddleware", func() {
			var gate *networkingv1.NetworkPolicyIngressRule
			for i := range np.Spec.Ingress {
				rule := &np.Spec.Ingress[i]
				for _, peer := range rule.From {
					if peer.PodSelector == nil {
						continue
					}
					for _, expr := range peer.PodSelector.MatchExpressions {
						if expr.Key == "app" && expr.Operator == metav1.LabelSelectorOpIn &&
							containsAll(expr.Values, "oauth2-proxy", componentAuthmiddleware) {
							gate = rule
						}
					}
				}
			}
			Expect(gate).NotTo(BeNil(),
				"dex must allow the in-cluster wait-for-dex gate from oauth2-proxy/authmiddleware (#49)")

			// The gate must be namespace-scoped and target the dex port.
			nsScoped := false
			for _, peer := range gate.From {
				if peer.NamespaceSelector != nil &&
					peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "" {
					nsScoped = true
				}
			}
			Expect(nsScoped).To(BeTrue(), "the dex gate ingress rule should be namespace-scoped")

			gatePort := false
			for _, p := range gate.Ports {
				if p.Port != nil && p.Port.IntValue() == 5556 {
					gatePort = true
				}
			}
			Expect(gatePort).To(BeTrue(), "the dex gate ingress rule should target :5556")
		})

		It("should allow :443 egress for the Kubernetes API server", func() {
			Expect(egressPorts(np)).To(HaveKey(443), "dex needs :443 egress for the Kubernetes API server")
		})

		// Regression guard: dex's wait-for-traefik initContainer probes
		// http://traefik-healthcheck.<ns>:9000/ping. Without :9000 egress that
		// gate is silently denied under enforcement, so dex hangs until timeout
		// then starts anyway — defeating the gate.
		It("should allow :9000 egress for the wait-for-traefik init gate", func() {
			Expect(egressPorts(np)).To(HaveKey(9000), "dex needs :9000 egress to reach traefik-healthcheck/ping")
		})

		// :9000 is plaintext, so it must be scoped to the traefik router pods in
		// this namespace rather than allowed to any destination.
		It("should scope :9000 egress to traefik router pods in this namespace", func() {
			Expect(egressScopedToPodOnPort(np, 9000, "traefik")).To(BeTrue(),
				":9000 egress should target traefik router pods, namespace-scoped")
		})

		// :80 plaintext egress was unused boilerplate; dropping it removes
		// needless plaintext attack surface.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "dex makes no plaintext :80 egress; the rule should be removed")
		})
	})
})
