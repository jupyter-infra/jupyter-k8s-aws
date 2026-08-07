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
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Traefik", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	// render templates the chart into a temp dir and returns the templates path.
	render := func(extraArgs ...string) string {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(), extraArgs...)
		helmTemplate(chartDir, outputDir, args...)
		return filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates")
	}

	readDeployment := func(templatesDir string) appsv1.Deployment {
		data, err := os.ReadFile(filepath.Join(templatesDir, "traefik/deployment.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var dep appsv1.Deployment
		Expect(yaml.Unmarshal(data, &dep)).To(Succeed())
		return dep
	}

	// containerArgs returns the traefik container's args (flattened for substring checks).
	containerArgs := func(dep appsv1.Deployment) string {
		return strings.Join(dep.Spec.Template.Spec.Containers[0].Args, "\n")
	}

	Context("compression", func() {
		It("renders a compress Middleware that excludes SSE", func() {
			data, err := os.ReadFile(filepath.Join(render(), "traefik/compress-middleware.yaml"))
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("kind: Middleware"))
			Expect(content).To(ContainSubstring("name: compress"))
			// Intent: SSE must not be compressed (would break JupyterLab live events).
			Expect(content).To(ContainSubstring("excludedContentTypes"))
			Expect(content).To(ContainSubstring("text/event-stream"))
		})

		It("attaches compress to the web-app routes", func() {
			data, err := os.ReadFile(filepath.Join(render(), "web-app/ingressroute.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: compress"),
				"web-app IngressRoutes should attach the compress middleware")
		})
	})

	Context("access logs + extraArgs", func() {
		It("emits no access-log args by default", func() {
			Expect(containerArgs(readDeployment(render()))).NotTo(ContainSubstring("--accesslog"))
		})

		It("emits JSON access logs with Authorization redacted when enabled", func() {
			args := containerArgs(readDeployment(render(helmSetFlag, "traefik.accessLog.enabled=true")))
			Expect(args).To(ContainSubstring("--accesslog=true"))
			Expect(args).To(ContainSubstring("--accesslog.format=json"))
			Expect(args).To(ContainSubstring("--accesslog.fields.headers.names.Authorization=redact"))
		})

		It("appends extraArgs verbatim", func() {
			args := containerArgs(readDeployment(render(helmSetFlag, "traefik.extraArgs[0]=--foo=bar")))
			Expect(args).To(ContainSubstring("--foo=bar"))
		})
	})

	Context("affinity & topologySpreadConstraints", func() {
		It("renders the default hostname podAntiAffinity when affinity is unset", func() {
			spec := readDeployment(render()).Spec.Template.Spec
			Expect(spec.Affinity).NotTo(BeNil())
			Expect(spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(spec.TopologySpreadConstraints).To(BeEmpty())
		})

		It("replaces the default affinity entirely when affinity is set", func() {
			spec := readDeployment(render(
				helmSetFlag, "traefik.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key=disktype",
				helmSetFlag, "traefik.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator=Exists",
			)).Spec.Template.Spec
			Expect(spec.Affinity.NodeAffinity).NotTo(BeNil())
			Expect(spec.Affinity.PodAntiAffinity).To(BeNil(),
				"setting traefik.affinity must drop the chart's default podAntiAffinity")
		})

		It("renders topologySpreadConstraints when set", func() {
			spec := readDeployment(render(
				helmSetFlag, "traefik.topologySpreadConstraints[0].maxSkew=1",
				helmSetFlag, "traefik.topologySpreadConstraints[0].topologyKey=zone",
				helmSetFlag, "traefik.topologySpreadConstraints[0].whenUnsatisfiable=DoNotSchedule",
			)).Spec.Template.Spec
			Expect(spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(spec.TopologySpreadConstraints[0].TopologyKey).To(Equal("zone"))
		})
	})
})

var _ = Describe("Traefik compression (bearer strategy)", func() {
	It("attaches compress to bearer workspace routes when createBearer is enabled", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append(oidcRequiredArgs(),
			helmSetFlag, "accessStrategy.createBearer=true",
			helmSetFlag, "authmiddleware.enableBearerAuth=true")
		helmTemplate(chartDir, outputDir, args...)
		data, err := os.ReadFile(filepath.Join(outputDir,
			"jupyter-k8s-aws-oidc/templates/access-strategy/bearer-access-strategy.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("name: compress"),
			"bearer workspace routes should attach the compress middleware")
	})
})
