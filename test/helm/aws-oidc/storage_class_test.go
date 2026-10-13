/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/yaml"
)

// The EBS CSI driver tags volumes with only its own keys, so a consumer's tag-based sweep or
// cost attribution never finds workspace volumes (#101). The tag scheme is the deployment's
// call, so nothing is rendered by default.
var _ = Describe("EBS StorageClass", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	renderEBS := func(extraArgs ...string) storagev1.StorageClass {
		outputDir := GinkgoT().TempDir()
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		helmTemplate(chartDir, outputDir, append(oidcRequiredArgs(), extraArgs...)...)

		data, err := os.ReadFile(filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates/ebs-storageclass.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var sc storagev1.StorageClass
		Expect(yaml.Unmarshal(data, &sc)).To(Succeed())
		return sc
	}

	expectRejected := func(want string, extraArgs ...string) {
		chartDir := GinkgoT().TempDir()
		copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
		args := append([]string{helmTemplateCmd, helmReleaseName, chartDir}, oidcRequiredArgs()...)
		out, err := exec.Command("helm", append(args, extraArgs...)...).CombinedOutput()
		Expect(err).To(HaveOccurred(), "expected rejection, got:\n%s", string(out))
		Expect(string(out)).To(ContainSubstring(want))
	}

	Context("additional resource tags", func() {
		It("adds no tagSpecification parameters when unset", func() {
			sc := renderEBS()
			for k := range sc.Parameters {
				Expect(k).NotTo(HavePrefix("tagSpecification"))
			}
			Expect(sc.Parameters).To(HaveKeyWithValue("type", "gp3"))
		})

		// StorageClass parameters are immutable, so a numbering that moved between renders
		// would fail every helm upgrade.
		It("numbers tags in sorted key order, alongside the other parameters", func() {
			sc := renderEBS(
				helmSetFlag, "storageClass.ebs.additionalResourceTags.Owner=team-a",
				helmSetFlag, "storageClass.ebs.additionalResourceTags.DeploymentId=abc-123",
			)
			Expect(sc.Parameters).To(HaveKeyWithValue("tagSpecification_1", "DeploymentId=abc-123"))
			Expect(sc.Parameters).To(HaveKeyWithValue("tagSpecification_2", "Owner=team-a"))
			Expect(sc.Parameters).To(HaveKeyWithValue("type", "gp3"))
			Expect(sc.Parameters).To(HaveKeyWithValue("fsType", "ext4"))
		})

		It("renders tags when parameters is emptied", func() {
			sc := renderEBS(
				helmSetFlag, "storageClass.ebs.parameters=null",
				helmSetFlag, "storageClass.ebs.additionalResourceTags.Owner=team-a",
			)
			Expect(sc.Parameters).To(Equal(map[string]string{"tagSpecification_1": "Owner=team-a"}))
		})

		// The driver interpolates values with text/template at CreateVolume; the chart must
		// pass placeholders through verbatim rather than evaluate or mangle them.
		It("passes driver placeholders and \"=\" in values through verbatim", func() {
			// --set reads "{...}" as a list, so the placeholder goes through a values file,
			// as a consumer would write it.
			valuesFile := filepath.Join(GinkgoT().TempDir(), "tags.yaml")
			Expect(os.WriteFile(valuesFile, []byte(
				"storageClass:\n  ebs:\n    additionalResourceTags:\n"+
					"      Namespace: \"{{ .PVCNamespace }}\"\n"+
					"      Query: \"a=b\"\n"), 0o644)).To(Succeed())
			sc := renderEBS("--values", valuesFile)
			Expect(sc.Parameters).To(HaveKeyWithValue("tagSpecification_1", "Namespace={{ .PVCNamespace }}"))
			Expect(sc.Parameters).To(HaveKeyWithValue("tagSpecification_2", "Query=a=b"))
		})

		It("renders a slash- or colon-bearing tag key as-is", func() {
			sc := renderEBS(helmSetFlag, `storageClass.ebs.additionalResourceTags.example\.com/team:name=a`)
			Expect(sc.Parameters).To(HaveKeyWithValue("tagSpecification_1", "example.com/team:name=a"))
		})

		// The driver splits on the first "=", so an "=" in a key silently moves the boundary.
		It("rejects a key containing \"=\"", func() {
			expectRejected("key/value separator",
				helmSetFlag, `storageClass.ebs.additionalResourceTags.a\=b=c`)
		})

		// A reserved key fails CreateVolume, leaving every PVC on this class Pending.
		It("rejects keys the driver reserves", func() {
			for _, key := range []string{
				"CSIVolumeName",
				"CSIVolumeSnapshotName",
				`kubernetes\.io/cluster/x`,
				`ebs\.csi\.aws\.com/cluster`,
			} {
				expectRejected("is reserved by the EBS CSI driver",
					helmSetFlag, "storageClass.ebs.additionalResourceTags."+key+"=v")
			}
		})

		It("rejects tagSpecification parameters set alongside it", func() {
			expectRejected("set tags in one place only",
				helmSetFlag, "storageClass.ebs.parameters.tagSpecification_1=Owner=x",
				helmSetFlag, "storageClass.ebs.additionalResourceTags.Owner=team-a",
			)
		})

		It("skips validation when the EBS StorageClass is not created", func() {
			outputDir := GinkgoT().TempDir()
			chartDir := GinkgoT().TempDir()
			copyDir(filepath.Join(rootDir, "charts/aws-oidc"), chartDir)
			helmTemplate(chartDir, outputDir, append(oidcRequiredArgs(),
				helmSetFlag, "storageClass.ebs.create=false",
				helmSetFlag, "storageClass.ebs.additionalResourceTags.CSIVolumeName=v",
			)...)
			_, err := os.Stat(filepath.Join(outputDir, "jupyter-k8s-aws-oidc/templates/ebs-storageclass.yaml"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})
})
