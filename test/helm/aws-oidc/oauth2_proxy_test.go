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

var _ = Describe("OAuth2 Proxy", func() {
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

			data, err := os.ReadFile(filepath.Join(templatesDir, "oauth2-proxy/network-policy.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(data, &np)).To(Succeed())
		})

		It("should allow :443 egress for Dex via the public domain", func() {
			Expect(egressPorts(np)).To(HaveKey(443), "oauth2-proxy needs :443 egress to reach Dex via the public domain")
		})

		// oauth2-proxy's wait-for-dex initContainer probes
		// http://dex.<ns>:5556/dex/healthz. That port is plaintext, so it must
		// be scoped to the dex pods in this namespace rather than open egress.
		It("should scope :5556 egress to dex pods in this namespace", func() {
			var scoped bool
			for _, rule := range np.Spec.Egress {
				targets5556 := false
				for _, p := range rule.Ports {
					if p.Port != nil && p.Port.IntValue() == 5556 {
						targets5556 = true
					}
				}
				if !targets5556 {
					continue
				}
				Expect(rule.To).NotTo(BeEmpty(), ":5556 egress must not be open to all destinations")
				for _, peer := range rule.To {
					if peer.PodSelector != nil &&
						peer.PodSelector.MatchLabels["app"] == "dex" &&
						peer.NamespaceSelector != nil &&
						peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "" {
						scoped = true
					}
				}
			}
			Expect(scoped).To(BeTrue(),
				":5556 egress should target dex pods, namespace-scoped")
		})

		// :80 plaintext egress was unused boilerplate (issuer/redirect are https,
		// upstream is static://); dropping it removes needless attack surface.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "oauth2-proxy makes no plaintext :80 egress; the rule should be removed")
		})
	})
})
