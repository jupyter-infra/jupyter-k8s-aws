/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Internal TLS", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	renderInternal := func(extraArgs ...string) string {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(), extraArgs...)
		helmTemplate(chartDir, outputDir, args...)
		return filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
	}

	read := func(dir, rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		Expect(err).NotTo(HaveOccurred())
		return string(data)
	}

	on := []string{helmSetFlag, "internalTls.enabled=true"}

	Context("disabled (the default)", func() {
		It("leaves every component on plaintext and creates no ServersTransport", func() {
			dir := renderInternal()
			Expect(read(dir, "dex/configmap.yaml")).To(ContainSubstring("http: 0.0.0.0:5556"))
			Expect(read(dir, "dex/configmap.yaml")).NotTo(ContainSubstring("tlsCert"))
			Expect(read(dir, "traefik/middlewares.yaml")).To(ContainSubstring("address: http://oauth2-proxy"))
			_, err := os.Stat(filepath.Join(dir, "traefik/servers-transport.yaml"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	Context("enabled", func() {
		It("has dex serve HTTPS with the plaintext listener bound to localhost only", func() {
			cfg := read(renderInternal(on...), "dex/configmap.yaml")
			// Localhost, not 0.0.0.0: the probes need it, nothing on the network should.
			Expect(cfg).To(ContainSubstring("http: 127.0.0.1:5556"))
			Expect(cfg).To(ContainSubstring("https: 0.0.0.0:5554"))
			Expect(cfg).To(ContainSubstring("tlsCert: /etc/dex/tls/tls.crt"))
			// The issuer must stay the public URL — token `iss` claims and the EKS OIDC
			// provider config both depend on it.
			Expect(cfg).To(ContainSubstring("issuer: https://"))
		})

		It("probes dex over localhost rather than the TLS port", func() {
			dep := read(renderInternal(on...), "dex/deployment.yaml")
			Expect(dep).To(ContainSubstring("http://127.0.0.1:5556/dex/healthz"))
			Expect(dep).To(ContainSubstring("containerPort: 5554"))
			Expect(dep).To(ContainSubstring("secretName: dex-internal-tls"))
		})

		It("drops oauth2-proxy's plaintext listener and probes it over HTTPS", func() {
			// The image is distroless, so exec probes are impossible; that forces
			// scheme: HTTPS, which in turn leaves the HTTP listener with nothing to serve.
			dep := read(renderInternal(on...), oauth2ProxyDeployFile)
			Expect(dep).To(ContainSubstring("--https-address=0.0.0.0:4443"))
			Expect(dep).To(ContainSubstring("--http-address="))
			Expect(dep).NotTo(ContainSubstring("--http-address=0.0.0.0:4180"))
			Expect(dep).To(ContainSubstring("scheme: HTTPS"))
		})

		It("pins serverName on the IngressRoute path but not on ForwardAuth", func() {
			dir := renderInternal(on...)

			// IngressRoute services resolve to pod IPs, so serverName is mandatory —
			// without it the handshake targets an IP no SAN matches and every request 502s.
			transports := read(dir, "traefik/servers-transport.yaml")
			Expect(transports).To(ContainSubstring("serverName: dex."))
			Expect(transports).To(ContainSubstring("serverName: oauth2-proxy."))
			Expect(transports).To(ContainSubstring("insecureSkipVerify: false"))

			// ForwardAuth takes SNI from the URL hostname, so it needs a CA but no serverName.
			mw := read(dir, "traefik/middlewares.yaml")
			Expect(mw).To(ContainSubstring("caSecret: oauth2-proxy-internal-tls"))
			Expect(mw).NotTo(ContainSubstring("serverName:"))
		})

		It("gives each certificate every hostname form Traefik dials", func() {
			// The two oauth2-proxy ForwardAuth middlewares use DIFFERENT name forms: one the
			// FQDN, one the short form. A cert with only the FQDN passes /oauth2/auth and
			// fails every login redirect.
			certs := read(renderInternal(on...), "cert-manager/certificate.yaml")
			for _, name := range []string{
				"- oauth2-proxy\n", "- oauth2-proxy.jupyter-k8s-router\n",
				"- oauth2-proxy.jupyter-k8s-router.svc.cluster.local\n",
				"- dex\n", "- dex.jupyter-k8s-router\n",
				"- dex.jupyter-k8s-router.svc.cluster.local\n",
			} {
				Expect(certs).To(ContainSubstring(name))
			}
		})

		It("moves the NetworkPolicy ports along with the listeners", func() {
			dir := renderInternal(on...)
			// A netpol left on the old port silently blocks the new one.
			Expect(read(dir, "dex/network-policy.yaml")).To(ContainSubstring("port: 5554"))
			Expect(read(dir, "dex/network-policy.yaml")).NotTo(ContainSubstring("port: 5556"))
			Expect(read(dir, "oauth2-proxy/network-policy.yaml")).To(ContainSubstring("port: 4443"))
			Expect(read(dir, "authmiddleware/network-policy.yaml")).To(ContainSubstring("port: 5554"))
		})

		It("keeps the dex wait gates pointing at the port dex actually serves", func() {
			dir := renderInternal(on...)
			for _, f := range []string{oauth2ProxyDeployFile, authmiddlewareDeployFile} {
				body := read(dir, f)
				Expect(body).To(ContainSubstring("https://dex."), f)
				Expect(body).NotTo(ContainSubstring("http://dex."), f)
				Expect(strings.Contains(body, ":5554/dex/healthz")).To(BeTrue(), f)
			}
		})

		It("still routes authmiddleware over plaintext until its image supports TLS", func() {
			// jupyter-infra/jupyter-k8s#479. Rendering https:// here would 502 every request.
			am := read(renderInternal(on...), "traefik/auth-middlewares.yaml")
			Expect(am).To(ContainSubstring("http://authmiddleware."))
			Expect(am).NotTo(ContainSubstring("https://authmiddleware."))
		})

		It("exposes the TLS port on each Service", func() {
			dir := renderInternal(on...)
			for rel, want := range map[string]int32{
				"dex/service.yaml":          5554,
				"oauth2-proxy/service.yaml": 4443,
			} {
				var svc corev1.Service
				Expect(yaml.Unmarshal([]byte(read(dir, rel)), &svc)).To(Succeed())
				Expect(svc.Spec.Ports).To(HaveLen(1), rel)
				Expect(svc.Spec.Ports[0].Port).To(Equal(want), rel)
				Expect(svc.Spec.Ports[0].Name).To(Equal("https"), rel)
			}
		})
	})
})
