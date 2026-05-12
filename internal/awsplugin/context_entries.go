/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"strings"

	"github.com/jupyter-infra/jupyter-k8s-plugin/plugin"
)

// --- Pod event context keys ---
// These are resolved from podEventsContext (RegisterNodeAgent / DeregisterNodeAgent).

var (
	// WorkspacePodUIDTagKey is the SSM tag filter key used to identify workspace pods.
	WorkspacePodUIDTagKey = plugin.ContextEntry{
		Key:     "workspacePodUIDTagKey",
		Default: "tag:workspace-pod-uid",
	}

	// SageMakerManagedByTagKey is the tag key for SageMaker managed-by identification.
	SageMakerManagedByTagKey = plugin.ContextEntry{
		Key:     "sagemakerManagedByTagKey",
		Default: "sagemaker.amazonaws.com/managed-by",
	}

	// SageMakerManagedByTagValue is the required value for the SageMaker managed-by tag.
	SageMakerManagedByTagValue = plugin.ContextEntry{
		Key:     "sagemakerManagedByTagValue",
		Default: "amazon-sagemaker-spaces",
	}

	// SageMakerEKSClusterTagKey is the tag key for SageMaker EKS cluster ARN.
	SageMakerEKSClusterTagKey = plugin.ContextEntry{
		Key:     "sagemakerEKSClusterTagKey",
		Default: "sagemaker.amazonaws.com/eks-cluster-arn",
	}

	// SSMInstanceNamePrefix is the prefix used for SSM instance names.
	SSMInstanceNamePrefix = plugin.ContextEntry{
		Key:     "ssmInstanceNamePrefix",
		Default: "workspace",
	}
)

// --- Connection context keys ---
// These are resolved from createConnectionContext (CreateSession).

var (
	// MaxConcurrentSSMSessions is the maximum concurrent SSM sessions per managed instance.
	MaxConcurrentSSMSessions = plugin.ContextEntry{
		Key:     "maxConcurrentSSMSessions",
		Default: "10",
	}
)

// connectionScheme returns a ContextEntry for the given connection type.
// The key is derived by stripping "-remote" and appending "Scheme":
//
//	"vscode-remote" → key "vscodeScheme", default "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace"
//	"kiro-remote"   → key "kiroScheme",   default "kiro://amazonwebservices.aws-toolkit-kiro/connect/workspace"
func connectionScheme(connectionType string) plugin.ContextEntry {
	ide := strings.TrimSuffix(connectionType, "-remote")
	return plugin.ContextEntry{
		Key:     ide + "Scheme",
		Default: ide + "://amazonwebservices.aws-toolkit-" + ide + "/connect/workspace",
	}
}
