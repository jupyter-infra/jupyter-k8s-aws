/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
)

var _ = Describe("Auth Middleware", func() {
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
