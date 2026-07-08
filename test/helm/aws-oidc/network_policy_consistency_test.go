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
	"sigs.k8s.io/yaml"
)

// behindTraefikPolicy describes a component whose NetworkPolicy scopes ingress
// to the traefik router. traefik itself is intentionally excluded: it is the
// edge router terminating external traffic and cannot be "ingress from traefik only".
type behindTraefikPolicy struct {
	component    string // human label for spec descriptions
	file         string // path under templates/ to the rendered policy
	app          string // expected podSelector app label
	componentLbl string // expected podSelector component label
	ingressPort  int    // service port traefik must be allowed to reach
}

var _ = Describe("Network Policy Consistency", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	const webAppComponent = "web-app"

	// The full set of behind-traefik components. Adding a fifth component that
	// sits behind traefik should mean adding a row here — that is intentional
	// friction so the shared invariants are a conscious decision.
	components := []behindTraefikPolicy{
		{"dex", "dex/network-policy.yaml", "dex", "oauth", 5556},
		{"authmiddleware", "authmiddleware/network-policy.yaml", "authmiddleware", "auth", 8080},
		{"oauth2-proxy", "oauth2-proxy/network-policy.yaml", "oauth2-proxy", "auth", 4180},
		{webAppComponent, webAppComponent + "/network-policy.yaml", webAppComponent, webAppComponent, 8090},
	}

	var policies map[string]networkingv1.NetworkPolicy

	BeforeEach(func() {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

		// Render with every behind-traefik component enabled.
		args := append(oidcRequiredArgs(),
			helmSetFlag, "webApp.enabled=true",
			helmSetFlag, "authmiddleware.enabled=true",
		)
		helmTemplate(chartDir, outputDir, args...)
		templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

		policies = map[string]networkingv1.NetworkPolicy{}
		for _, c := range components {
			data, err := os.ReadFile(filepath.Join(templatesDir, c.file))
			Expect(err).NotTo(HaveOccurred(), "%s policy should render", c.component)

			var np networkingv1.NetworkPolicy
			Expect(yaml.Unmarshal(data, &np)).To(Succeed())
			policies[c.component] = np
		}
	})

	// hasTraefikIngressOnPort reports whether the policy allows ingress from the
	// traefik router (namespace-scoped) on the given port.
	hasTraefikIngressOnPort := func(np networkingv1.NetworkPolicy, port int) bool {
		for _, rule := range np.Spec.Ingress {
			fromTraefik := false
			for _, peer := range rule.From {
				if peer.PodSelector == nil || peer.NamespaceSelector == nil {
					continue
				}
				if peer.PodSelector.MatchLabels["app"] == "traefik" &&
					peer.PodSelector.MatchLabels["component"] == "router" &&
					peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "" {
					fromTraefik = true
				}
			}
			if !fromTraefik {
				continue
			}
			for _, p := range rule.Ports {
				if p.Port != nil && p.Port.IntValue() == port {
					return true
				}
			}
		}
		return false
	}

	for _, c := range components {
		// capture per iteration
		Context(c.component, func() {
			It("should select its own pods", func() {
				sel := policies[c.component].Spec.PodSelector.MatchLabels
				Expect(sel).To(HaveKeyWithValue("app", c.app))
				Expect(sel).To(HaveKeyWithValue("component", c.componentLbl))
			})

			// Guards against the #48 class of bug: declaring Egress in policyTypes
			// but leaving the egress block empty, which denies all egress.
			It("should declare both Ingress and Egress with a non-empty egress block", func() {
				Expect(policies[c.component].Spec.PolicyTypes).To(ConsistOf(
					networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress),
					"%s must enforce both directions", c.component)
				Expect(policies[c.component].Spec.Egress).NotTo(BeEmpty(),
					"%s declares Egress but has no egress rules — all egress would be denied", c.component)
			})

			It("should allow ingress from traefik on its service port", func() {
				Expect(hasTraefikIngressOnPort(policies[c.component], c.ingressPort)).To(BeTrue(),
					"%s should allow traefik ingress on :%d", c.component, c.ingressPort)
			})

			It("should allow DNS egress", func() {
				Expect(egressPorts(policies[c.component])).To(HaveKey(53),
					"%s must allow DNS (:53) egress", c.component)
			})
		})
	}
})
