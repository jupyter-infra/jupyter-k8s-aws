/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"os"
	"path/filepath"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Chart Version", func() {
	var chart struct {
		Version    string `json:"version"`
		AppVersion string `json:"appVersion"`
	}

	semverRegex := regexp.MustCompile(`^\d+\.\d+\.\d+$`)

	BeforeEach(func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
		data, err := os.ReadFile(filepath.Join(rootDir, "charts/aws-oidc/Chart.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal(data, &chart)).To(Succeed())
	})

	It("uses a valid semver version", func() {
		Expect(chart.Version).To(MatchRegexp(semverRegex.String()),
			"Chart.yaml version must be X.Y.Z")
	})

	It("keeps version and appVersion in sync", func() {
		// They drifted apart historically (both 0.1.1) — keep them locked so a
		// version bump can't silently update only one.
		Expect(chart.AppVersion).To(Equal(chart.Version),
			"Chart.yaml appVersion must equal version")
	})

	// Note: the "local version must be >= the latest published OCI chart"
	// invariant is intentionally NOT a unit test — it requires a network fetch
	// from ghcr.io and is inherently non-hermetic. It lives in the
	// `make check-aws-oidc-version` target, which `deploy-aws-oidc` runs to
	// guard against a downgrade on `helm upgrade`.
})
