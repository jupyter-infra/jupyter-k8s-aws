/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"fmt"
	"os"
	"path/filepath"
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
