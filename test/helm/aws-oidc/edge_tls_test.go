/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// Public TLS terminates at the NLB with an ACM certificate and is re-encrypted to Traefik.
// The load-bearing invariant is that Traefik still sees HTTPS on websecure, because that is
// what leaves the whole routing layer — including the IngressRoutes embedded in
// WorkspaceAccessStrategy templates that jupyter-deploy and samples/ consume — untouched.
var _ = Describe("Edge TLS", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	renderTLS := func(extraArgs ...string) string {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(), extraArgs...)
		helmTemplate(chartDir, outputDir, args...)
		return filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
	}

	readFile := func(dir, rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		Expect(err).NotTo(HaveOccurred())
		return string(data)
	}

	// The single assertion in the Service that is a *reference* rather than a literal, and the
	// only one whose failure is silent. Everything else in these annotations is a static string
	// that a diff review already shows.
	It("only asks for a TLS listener on a port the Service actually defines", func() {
		var svc corev1.Service
		Expect(yaml.Unmarshal([]byte(readFile(renderTLS(), "traefik/service.yaml")), &svc)).To(Succeed())

		// ssl-ports names Service PORT NAMES, not listener ports. A non-numeric entry lands in
		// a names set (cloud-provider-aws getPortSets) and is matched as
		// sslPorts.names.Has(port.Name). On a mismatch the certificate condition is simply
		// skipped: FrontendProtocol stays TCP, NO TLS listener is created, and edge TLS
		// disappears with no render error and no apply error. Renaming the `websecure` port
		// without updating this annotation is all it takes.
		sslPorts := svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"]
		Expect(sslPorts).NotTo(BeEmpty(),
			"without ssl-ports every port gets the certificate, including plain HTTP on 80")

		names := make([]string, 0, len(svc.Spec.Ports))
		for _, p := range svc.Spec.Ports {
			names = append(names, p.Name)
		}
		for _, want := range strings.Split(sslPorts, ",") {
			Expect(names).To(ContainElement(strings.TrimSpace(want)),
				"ssl-ports references %q, which is not a port name on the Service (%v)", want, names)
		}
	})

	// These annotations are absent on purpose, and absence is not self-documenting: the template
	// shows no line, and nothing tells the next reader that is a decision rather than an
	// oversight. Re-adding nlb-target-type is specifically plausible — the deferred optional-LBC
	// work would introduce it naturally.
	It("asks for nothing the in-tree cloud provider would ignore", func() {
		var svc corev1.Service
		Expect(yaml.Unmarshal([]byte(readFile(renderTLS(), "traefik/service.yaml")), &svc)).To(Succeed())

		// LBC-only: in-tree hardcodes instance targets, so this would read as if the chart got
		// IP targets while it silently got instance targets.
		Expect(svc.Annotations).NotTo(HaveKey("service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"))

		// Health checking stays on the in-tree default (TCP on traffic-port). An HTTP check is
		// not expressible with instance targets: /ping on 9000 has no NodePort, and the traffic
		// port serves TLS. Pinning healthcheck-protocol would also defeat the better default
		// that externalTrafficPolicy: Local gets for free.
		for _, k := range []string{"protocol", "port", "path"} {
			Expect(svc.Annotations).NotTo(HaveKey("service.beta.kubernetes.io/aws-load-balancer-healthcheck-" + k))
		}
	})

	// Cross-file link. Traefik resolves every `tls: {}` IngressRoute through the TLSStore default
	// certificate, so if these two names drift apart Traefik falls back to its own built-in
	// self-signed certificate — no error, no crash, and the NLB will not complain because it
	// does not verify the backend. Silent, and spread across two files a review reads separately.
	It("points the TLSStore default certificate at the secret the Traefik cert issues", func() {
		dir := renderTLS()

		certSecret := ""
		for _, doc := range strings.Split(readFile(dir, "cert-manager/certificate.yaml"), "---") {
			var c struct {
				Metadata struct{ Name string }       `json:"metadata"`
				Spec     struct{ SecretName string } `json:"spec"`
			}
			if err := yaml.Unmarshal([]byte(doc), &c); err != nil || c.Metadata.Name != "traefik-cert" {
				continue
			}
			certSecret = c.Spec.SecretName
		}
		Expect(certSecret).NotTo(BeEmpty(), "no Certificate named traefik-cert was rendered")

		var store struct {
			Spec struct {
				DefaultCertificate struct{ SecretName string } `json:"defaultCertificate"`
			} `json:"spec"`
		}
		Expect(yaml.Unmarshal([]byte(readFile(dir, "traefik/tls-store.yaml")), &store)).To(Succeed())

		Expect(store.Spec.DefaultCertificate.SecretName).To(Equal(certSecret),
			"TLSStore points at %q but the Traefik Certificate issues %q — Traefik would serve its "+
				"built-in self-signed certificate instead",
			store.Spec.DefaultCertificate.SecretName, certSecret)
	})

	// Re-encryption is only invisible to the routing layer if the routes are untouched. A route
	// that lost `tls: {}` or moved to the `web` entrypoint would serve plaintext. This walks
	// every rendered file rather than naming them, so it also constrains IngressRoutes added
	// later — the point is the rule, not today's set of routes.
	It("keeps every IngressRoute on the websecure entrypoint with TLS", func() {
		dir := renderTLS()
		routes := 0
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return err
			}
			for _, doc := range strings.Split(string(mustRead(path)), "---") {
				var route struct {
					Kind string `json:"kind"`
					Spec struct {
						EntryPoints []string  `json:"entryPoints"`
						TLS         *struct{} `json:"tls"`
					} `json:"spec"`
				}
				if err := yaml.Unmarshal([]byte(doc), &route); err != nil || route.Kind != "IngressRoute" {
					continue
				}
				routes++
				// Parsed rather than substring-matched: `- web\n` also matches any list item
				// ending in "web", and misses the `[web]` flow form entirely.
				Expect(route.Spec.EntryPoints).To(ConsistOf("websecure"), path)
				Expect(route.Spec.TLS).NotTo(BeNil(), "%s: IngressRoute has no tls block", path)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(routes).To(BeNumerically(">", 0))
	})

	It("redirects HTTP to the load balancer's port, not Traefik's container port", func() {
		// Naming the websecure entrypoint as the redirect target makes Traefik emit
		// https://<host>:8443/, which no client can reach.
		dep := readFile(renderTLS(), "traefik/deployment.yaml")
		Expect(dep).To(ContainSubstring("redirections.entryPoint.to=:443"))
		Expect(dep).NotTo(ContainSubstring("redirections.entryPoint.to=websecure"))
	})

	Context("target node labels", func() {
		// Targets are node instances, so by default EVERY node joins the target group,
		// including workspace nodes that never run Traefik. Restricting to the routing tier
		// is the deployment's call, so the chart must not presume a label scheme of its own.
		It("registers every node when unset", func() {
			Expect(readFile(renderTLS(), "traefik/service.yaml")).
				NotTo(ContainSubstring("aws-load-balancer-target-node-labels"))
		})

		It("renders pairs sorted, so the Service does not churn between upgrades", func() {
			dir := renderTLS(
				helmSetFlag, `traefik.targetNodeLabels.zzz=last`,
				helmSetFlag, `traefik.targetNodeLabels.aaa=first`,
			)
			Expect(readFile(dir, "traefik/service.yaml")).To(ContainSubstring(
				`aws-load-balancer-target-node-labels: "aaa=first,zzz=last"`))
		})

		It("renders a slash-bearing label key as-is", func() {
			// jupyter-deploy uses jupyter-deploy/role=routing; --set treats "." as a path
			// separator, so this is also the escaping the deployment has to write.
			dir := renderTLS(helmSetFlag, `traefik.targetNodeLabels.jupyter-deploy/role=routing`)
			Expect(readFile(dir, "traefik/service.yaml")).To(ContainSubstring(
				`aws-load-balancer-target-node-labels: "jupyter-deploy/role=routing"`))
		})
	})

	Context("removed Let's Encrypt configuration", func() {
		// A stored value from an older release must fail loudly rather than be ignored,
		// since --reset-then-reuse-values would otherwise carry it silently forward.
		// Shells out directly instead of going through helmTemplate, which asserts success.
		It("rejects certManager.* and tls.mode", func() {
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)

			for _, tc := range []struct{ arg, wantErr string }{
				{"certManager.email=a@b.com", "certManager.* was removed"},
				{"tls.mode=letsencrypt", "tls.mode was removed"},
			} {
				args := append([]string{helmTemplateCmd, helmReleaseName, chartDir}, oidcRequiredArgs()...)
				args = append(args, helmSetFlag, tc.arg)
				out, err := exec.Command("helm", args...).CombinedOutput()
				Expect(err).To(HaveOccurred(), "expected %s to be rejected, got:\n%s", tc.arg, string(out))
				Expect(string(out)).To(ContainSubstring(tc.wantErr))
			}
		})
	})
})

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	return data
}
