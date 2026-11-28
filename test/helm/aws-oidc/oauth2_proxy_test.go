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
	batchv1 "k8s.io/api/batch/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

const (
	certRestartCronJobFile = "oauth2-proxy/cert-restart-cronjob.yaml"
	certRestartRBACFile    = "oauth2-proxy/cert-restart-rbac.yaml"
)

// renderOAuth2ProxyTemplates renders the chart and returns its templates directory.
func renderOAuth2ProxyTemplates(rootDir string, extraArgs ...string) string {
	outputDir := GinkgoT().TempDir()
	chartDir := GinkgoT().TempDir()
	copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
	helmTemplate(chartDir, outputDir, append(oidcRequiredArgs(), extraArgs...)...)
	return filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
}

var _ = Describe("OAuth2 Proxy", func() {
	Context("network policy", func() {
		var np networkingv1.NetworkPolicy

		BeforeEach(func() {
			rootDir, err := filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
			np = renderNetworkPolicy(rootDir, "oauth2-proxy/network-policy.yaml")
		})

		It("should allow :443 egress for Dex via the public domain", func() {
			Expect(egressPorts(np)).To(HaveKey(443), "oauth2-proxy needs :443 egress to reach Dex via the public domain")
		})

		// oauth2-proxy's wait-for-dex initContainer connects to
		// dex.<ns>:5554 (internalTls is on by default). That
		// egress must be scoped to the dex pods in this namespace, not left open.
		It("should scope :5554 egress to dex pods in this namespace", func() {
			Expect(egressScopedToPodOnPort(np, 5554, "dex")).To(BeTrue(),
				":5554 egress should target dex pods, namespace-scoped")
		})

		// :80 plaintext egress was unused boilerplate (issuer/redirect are https,
		// upstream is static://); dropping it removes needless attack surface.
		It("should not allow unused :80 plaintext egress", func() {
			Expect(egressPorts(np)).NotTo(HaveKey(80), "oauth2-proxy makes no plaintext :80 egress; the rule should be removed")
		})
	})

	// oauth2-proxy cannot reload a rotated certificate, so the chart restarts it on a schedule
	// shorter than tls.internalCa.renewBefore (30d); without it, logins fail once the old leaf
	// expires.
	Context("certificate restart CronJob", func() {
		var rootDir string

		BeforeEach(func() {
			var err error
			rootDir, err = filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())
		})

		readCronJob := func(dir string) batchv1.CronJob {
			data, err := os.ReadFile(filepath.Join(dir, certRestartCronJobFile))
			Expect(err).NotTo(HaveOccurred())
			var cj batchv1.CronJob
			Expect(yaml.Unmarshal(data, &cj)).To(Succeed())
			return cj
		}

		It("rolls oauth2-proxy weekly with a pinned, hardened kubectl", func() {
			cj := readCronJob(renderOAuth2ProxyTemplates(rootDir))
			Expect(cj.Spec.Schedule).To(Equal("0 3 * * 0"))
			Expect(cj.Spec.ConcurrencyPolicy).To(Equal(batchv1.ForbidConcurrent))

			pod := cj.Spec.JobTemplate.Spec.Template.Spec
			Expect(pod.ServiceAccountName).To(Equal("oauth2-proxy-cert-restart"))
			Expect(pod.Containers).To(HaveLen(1))
			c := pod.Containers[0]
			Expect(c.Image).To(Equal("registry.k8s.io/kubectl:v1.37.3"))
			Expect(c.Args).To(HaveExactElements(
				"rollout", "restart", "deployment/oauth2-proxy",
				"--namespace=jupyter-k8s-router", "--cache-dir=/tmp/kube-cache"))
			Expect(*c.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(*pod.SecurityContext.RunAsNonRoot).To(BeTrue())
			// The discovery cache needs a writable path under the read-only root.
			Expect(c.VolumeMounts).To(ContainElement(HaveField("MountPath", "/tmp")))
		})

		// A pod labelled app=oauth2-proxy would join the oauth2-proxy Service and match the
		// dex NetworkPolicy's health-gate rule.
		It("does not carry the oauth2-proxy app label", func() {
			cj := readCronJob(renderOAuth2ProxyTemplates(rootDir))
			Expect(cj.Spec.JobTemplate.Spec.Template.Labels).To(HaveKeyWithValue("app", "oauth2-proxy-cert-restart"))
		})

		It("can only get and patch the oauth2-proxy Deployment", func() {
			data, err := os.ReadFile(filepath.Join(renderOAuth2ProxyTemplates(rootDir), certRestartRBACFile))
			Expect(err).NotTo(HaveOccurred())
			var role *rbacv1.Role
			for _, doc := range strings.Split(string(data), "\n---\n") {
				var r rbacv1.Role
				if yaml.Unmarshal([]byte(doc), &r) == nil && r.Kind == "Role" {
					role = &r
				}
			}
			Expect(role).NotTo(BeNil())
			Expect(role.Rules).To(HaveExactElements(rbacv1.PolicyRule{
				APIGroups:     []string{"apps"},
				Resources:     []string{"deployments"},
				ResourceNames: []string{componentOAuth2Proxy},
				Verbs:         []string{"get", "patch"},
			}))
		})

		It("runs where oauth2-proxy runs", func() {
			cj := readCronJob(renderOAuth2ProxyTemplates(rootDir,
				helmSetFlag, "oauth2Proxy.nodeSelector.tier=components"))
			Expect(cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("tier", "components"))
		})

		// The CronJob must run inside the renewBefore window, or oauth2-proxy serves an expired
		// leaf. The chart bounds a schedule's longest gap and rejects what it cannot bound.
		Context("schedule validation", func() {
			render := func(extraArgs ...string) (string, error) {
				chartDir := GinkgoT().TempDir()
				copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
				args := append([]string{helmTemplateCmd, helmReleaseName, chartDir}, oidcRequiredArgs()...)
				out, err := exec.Command("helm", append(args, extraArgs...)...).CombinedOutput()
				return string(out), err
			}
			withSchedule := func(schedule string, extraArgs ...string) (string, error) {
				return render(append([]string{"--set-string", "oauth2Proxy.certRestart.schedule=" + schedule}, extraArgs...)...)
			}

			It("accepts daily and weekly schedules", func() {
				for _, schedule := range []string{"0 3 * * 0", "0 3 * * *", "@weekly", "@daily", "@hourly", "0 3 1 * 1"} {
					out, err := withSchedule(schedule)
					Expect(err).NotTo(HaveOccurred(), "%q should render:\n%s", schedule, out)
				}
			})

			It("rejects schedules that can leave gaps over 30 days", func() {
				for schedule, want := range map[string]string{
					"0 3 1 * *": "restricts only the day of month",
					"@monthly":  "is not supported",
					"0 3 * 1 *": "restricts the month field",
					"0 3 * *":   "must be a 5-field cron expression",
				} {
					out, err := withSchedule(schedule)
					Expect(err).To(HaveOccurred(), "%q should be rejected", schedule)
					Expect(out).To(ContainSubstring(want), schedule)
				}
			})

			It("rejects a schedule whose longest gap is not under renewBefore", func() {
				out, err := render(helmSetFlag, "tls.internalCa.renewBefore=120h")
				Expect(err).To(HaveOccurred())
				Expect(out).To(ContainSubstring("can leave up to 168h between restarts"))

				out, err = render(helmSetFlag, "tls.internalCa.renewBefore=169h")
				Expect(err).NotTo(HaveOccurred(), out)
			})

			It("skips the check when oauth2-proxy serves plaintext", func() {
				out, err := withSchedule("@monthly", helmSetFlag, "internalTls.oauth2Proxy=false")
				Expect(err).NotTo(HaveOccurred(), out)
			})
		})

		It("is not rendered when oauth2-proxy serves plaintext", func() {
			for _, flag := range []string{"internalTls.enabled=false", "internalTls.oauth2Proxy=false"} {
				dir := renderOAuth2ProxyTemplates(rootDir, helmSetFlag, flag)
				for _, f := range []string{certRestartCronJobFile, certRestartRBACFile} {
					_, err := os.Stat(filepath.Join(dir, f))
					Expect(os.IsNotExist(err)).To(BeTrue(), "%s should not render with %s", f, flag)
				}
			}
		})
	})
})
