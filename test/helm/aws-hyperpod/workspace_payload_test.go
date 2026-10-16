/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_hyperpod_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

type workspaceAccessStrategy struct {
	Spec struct {
		DeploymentModifications struct {
			PodModifications struct {
				Volumes                       []corev1.Volume    `json:"volumes"`
				InitContainers                []corev1.Container `json:"initContainers"`
				PrimaryContainerModifications struct {
					VolumeMounts []corev1.VolumeMount `json:"volumeMounts"`
				} `json:"primaryContainerModifications"`
			} `json:"podModifications"`
		} `json:"deploymentModifications"`
	} `json:"spec"`
}

func readWorkspaceAccessStrategy(outputDirectory string) workspaceAccessStrategy {
	rootDir, err := filepath.Abs("../../..")
	Expect(err).NotTo(HaveOccurred())

	accessStrategyPath := filepath.Join(
		rootDir,
		"dist",
		"test-output",
		outputDirectory,
		"jupyter-k8s-aws-hyperpod",
		"templates",
		"hyperpod-access-strategy.yaml",
	)

	data, err := os.ReadFile(accessStrategyPath)
	Expect(err).NotTo(HaveOccurred())

	var strategy workspaceAccessStrategy
	Expect(yaml.Unmarshal(data, &strategy)).To(Succeed())
	return strategy
}

var _ = Describe("Workspace payload injection", func() {
	DescribeTable("makes the workspace entrypoint available for every supported access mode",
		func(outputDirectory string, expectedInitContainer string) {
			strategy := readWorkspaceAccessStrategy(outputDirectory)
			podModifications := strategy.Spec.DeploymentModifications.PodModifications

			Expect(podModifications.Volumes).To(ContainElement(
				WithTransform(func(volume corev1.Volume) string { return volume.Name }, Equal("workspace-base")),
			))
			Expect(podModifications.InitContainers).To(ConsistOf(
				WithTransform(func(container corev1.Container) string { return container.Name }, Equal(expectedInitContainer)),
			))
			Expect(podModifications.PrimaryContainerModifications.VolumeMounts).To(ContainElement(
				And(
					WithTransform(func(mount corev1.VolumeMount) string { return mount.Name }, Equal("workspace-base")),
					WithTransform(func(mount corev1.VolumeMount) string { return mount.MountPath },
						Equal("/opt/amazon/sagemaker/workspace")),
				),
			))
		},
		Entry("with SSM remote access", "aws-hyperpod", "workspace-init-container"),
		Entry("with both remote access mechanisms disabled",
			"aws-hyperpod-no-remote-access", "workspace-init-container"),
		Entry("with direct SSH", "aws-hyperpod-direct-ssh", "direct-ssh-init"),
	)
})
