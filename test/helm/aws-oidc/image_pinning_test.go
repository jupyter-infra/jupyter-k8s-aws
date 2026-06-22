/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Image Pinning", func() {
	var rootDir string

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not use :latest tag in any deployment image", func() {
		testOutputDir := filepath.Join(
			rootDir, "dist/test-output/aws-oidc/jupyter-k8s-aws-oidc/templates")

		var violations []string

		err := filepath.Walk(testOutputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return err
			}

			rel, _ := filepath.Rel(testOutputDir, path)

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			for _, doc := range strings.Split(string(data), "---") {
				trimmed := strings.TrimSpace(doc)
				if trimmed == "" {
					continue
				}

				if strings.Contains(trimmed, "kind: Deployment") {
					var dep appsv1.Deployment
					if err := yaml.Unmarshal([]byte(trimmed), &dep); err != nil {
						continue
					}
					violations = append(violations,
						checkPodSpec(dep.Spec.Template.Spec, rel, dep.Name)...)
				}

				if strings.Contains(trimmed, "kind: CronJob") {
					var cj batchv1.CronJob
					if err := yaml.Unmarshal([]byte(trimmed), &cj); err != nil {
						continue
					}
					violations = append(violations,
						checkPodSpec(cj.Spec.JobTemplate.Spec.Template.Spec, rel, cj.Name)...)
				}

				if strings.Contains(trimmed, "kind: Job") && !strings.Contains(trimmed, "kind: CronJob") {
					var job batchv1.Job
					if err := yaml.Unmarshal([]byte(trimmed), &job); err != nil {
						continue
					}
					violations = append(violations,
						checkPodSpec(job.Spec.Template.Spec, rel, job.Name)...)
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(violations).To(BeEmpty(),
			"Found images using :latest tag:\n%s", strings.Join(violations, "\n"))
	})
})

func checkPodSpec(spec corev1.PodSpec, file, resourceName string) []string {
	var violations []string
	for _, c := range spec.InitContainers {
		if isUnpinned(c.Image) {
			violations = append(violations,
				fmt.Sprintf("  %s (%s) initContainer %q: %s", file, resourceName, c.Name, c.Image))
		}
	}
	for _, c := range spec.Containers {
		if isUnpinned(c.Image) {
			violations = append(violations,
				fmt.Sprintf("  %s (%s) container %q: %s", file, resourceName, c.Name, c.Image))
		}
	}
	return violations
}

func isUnpinned(image string) bool {
	// No tag at all (implies :latest)
	if !strings.Contains(image, ":") {
		return true
	}
	// Explicit :latest
	return strings.HasSuffix(image, ":latest")
}

var _ = Describe("In-house Image Version Governance", func() {
	inHouseImages := []string{
		"jupyter-k8s-ui",
		"jupyter-k8s-authmiddleware",
		"jupyter-k8s-rotator",
	}

	// Images released together from the jupyter-k8s repo.
	coreImages := []string{
		"jupyter-k8s-authmiddleware",
		"jupyter-k8s-rotator",
	}

	semverRegex := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

	extractTag := func(valuesContent, imageName string) string {
		re := regexp.MustCompile(`imageName:\s*` + regexp.QuoteMeta(imageName) + `\s*\n\s*imageTag:\s*(\S+)`)
		matches := re.FindStringSubmatch(valuesContent)
		ExpectWithOffset(1, matches).To(HaveLen(2),
			fmt.Sprintf("Could not find imageTag for %s in values.yaml", imageName))
		return matches[1]
	}

	It("should use release semver tags (vX.Y.Z) for all in-house images", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(filepath.Join(rootDir, "charts/aws-oidc/values.yaml"))
		Expect(err).NotTo(HaveOccurred())
		valuesContent := string(data)

		var violations []string
		for _, img := range inHouseImages {
			tag := extractTag(valuesContent, img)
			if !semverRegex.MatchString(tag) {
				violations = append(violations,
					fmt.Sprintf("  %s: %s (expected vX.Y.Z)", img, tag))
			}
		}
		Expect(violations).To(BeEmpty(),
			"In-house images must use release semver tags (no rc/alpha/beta):\n%s",
			strings.Join(violations, "\n"))
	})

	It("should use the same version for authmiddleware and rotator", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(filepath.Join(rootDir, "charts/aws-oidc/values.yaml"))
		Expect(err).NotTo(HaveOccurred())
		valuesContent := string(data)

		reference := extractTag(valuesContent, coreImages[0])
		var mismatches []string
		for _, img := range coreImages[1:] {
			tag := extractTag(valuesContent, img)
			if tag != reference {
				mismatches = append(mismatches,
					fmt.Sprintf("  %s: %s (expected %s)", img, tag, reference))
			}
		}
		Expect(mismatches).To(BeEmpty(),
			"authmiddleware and rotator are released together and must use the same tag. "+
				"Reference (%s: %s), mismatches:\n%s",
			coreImages[0], reference, strings.Join(mismatches, "\n"))
	})
})
