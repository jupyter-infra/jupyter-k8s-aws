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

// helmReleaseName is the release name used when rendering the chart in tests.
const helmReleaseName = "jk8s"

// oidcRequiredArgs returns the standard helm values needed to render the
// aws-oidc chart in tests.
func oidcRequiredArgs() []string {
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

// renderNetworkPolicy renders the chart and unmarshals the NetworkPolicy at the
// given path under templates/ (e.g. "dex/network-policy.yaml").
func renderNetworkPolicy(rootDir, policyPath string, extraArgs ...string) networkingv1.NetworkPolicy {
	outputDir := GinkgoT().TempDir()
	chartDir := GinkgoT().TempDir()
	copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

	helmTemplate(chartDir, outputDir, append(oidcRequiredArgs(), extraArgs...)...)
	templatesDir := filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")

	data, err := os.ReadFile(filepath.Join(templatesDir, policyPath))
	Expect(err).NotTo(HaveOccurred())

	var np networkingv1.NetworkPolicy
	Expect(yaml.Unmarshal(data, &np)).To(Succeed())
	return np
}

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

// egressScopedToPodOnPort reports whether the policy has an egress rule for the
// given port whose only destinations are pods carrying app=podApp, namespace-scoped.
// It also asserts (via Gomega) that such a rule does not leave its destinations open.
func egressScopedToPodOnPort(np networkingv1.NetworkPolicy, port int, podApp string) bool {
	scoped := false
	for _, rule := range np.Spec.Egress {
		targetsPort := false
		for _, p := range rule.Ports {
			if p.Port != nil && p.Port.IntValue() == port {
				targetsPort = true
			}
		}
		if !targetsPort {
			continue
		}
		Expect(rule.To).NotTo(BeEmpty(), "egress to :%d must not be open to all destinations", port)
		for _, peer := range rule.To {
			if peer.PodSelector != nil &&
				peer.PodSelector.MatchLabels["app"] == podApp &&
				peer.NamespaceSelector != nil &&
				peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "" {
				scoped = true
			}
		}
	}
	return scoped
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
