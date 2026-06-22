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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// egressPorts collects every port allowed across all egress rules of a policy.
func egressPorts(np networkingv1.NetworkPolicy) map[int]bool {
	ports := map[int]bool{}
	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			if p.Port != nil {
				ports[p.Port.IntValue()] = true
			}
		}
	}
	return ports
}

// containsAll reports whether haystack contains every needle.
func containsAll(haystack []string, needles ...string) bool {
	set := map[string]bool{}
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}

var _ = Describe("Dex", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	requiredArgs := func() []string {
		return []string{
			"--set", "domain=test.example.com",
			"--set", "certManager.email=admin@example.com",
			"--set", "storageClass.efs.parameters.fileSystemId=fs-000",
			"--set", "github.clientId=cid",
			"--set", "github.clientSecret=csec",
			"--set", "github.orgs[0].name=org",
			"--set", "github.orgs[0].teams[0]=t",
			"--set", "githubRbac.orgs[0].name=org",
			"--set", "githubRbac.orgs[0].teams[0]=t",
			"--set", "oauth2Proxy.cookieSecret=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		}
	}

	Context("network policy", func() {
		var np networkingv1.NetworkPolicy

		BeforeEach(func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			helmTemplate(chartDir, outputDir, requiredArgs()...)
			templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

			data, err := os.ReadFile(filepath.Join(templatesDir, "dex/network-policy.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(data, &np)).To(Succeed())
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
							containsAll(expr.Values, "oauth2-proxy", "authmiddleware") {
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
			var scoped bool
			for _, rule := range np.Spec.Egress {
				targets9000 := false
				for _, p := range rule.Ports {
					if p.Port != nil && p.Port.IntValue() == 9000 {
						targets9000 = true
					}
				}
				if !targets9000 {
					continue
				}
				Expect(rule.To).NotTo(BeEmpty(), ":9000 egress must not be open to all destinations")
				for _, peer := range rule.To {
					if peer.PodSelector != nil &&
						peer.PodSelector.MatchLabels["app"] == "traefik" &&
						peer.PodSelector.MatchLabels["component"] == "router" &&
						peer.NamespaceSelector != nil &&
						peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "" {
						scoped = true
					}
				}
			}
			Expect(scoped).To(BeTrue(),
				":9000 egress should target traefik router pods, namespace-scoped")
		})

		// :80 plaintext egress was unused boilerplate; dropping it removes
		// needless plaintext attack surface.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "dex makes no plaintext :80 egress; the rule should be removed")
		})
	})
})
