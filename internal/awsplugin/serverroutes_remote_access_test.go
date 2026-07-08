/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pluginapi "github.com/jupyter-infra/jupyter-k8s-plugin/api"
)

const (
	testRemotePodUID    = "pod-uid-123"
	testRemoteWorkspace = "my-workspace"
	testRemoteNamespace = "default"
	testVscodeRemote    = "vscode-remote"
	testKiroRemote      = "kiro-remote"
	testSSMDocName      = "my-ssm-document"
	testSSMDocKey       = "ssmDocumentName"
)

func newTestSSMClient(mockSSM *MockSSMClient) *SSMClient {
	return NewSSMClientWithMock(mockSSM, "us-west-2")
}

func TestAWSRemoteAccessRoutes_RegisterNodeAgent_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)
	mockSSM.On("CreateActivation", mock.Anything, mock.Anything).Return(
		&ssm.CreateActivationOutput{
			ActivationId:   aws.String("act-123"),
			ActivationCode: aws.String("code-456"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:        testRemotePodUID,
		WorkspaceName: testRemoteWorkspace,
		Namespace:     testRemoteNamespace,
		PodEventsContext: map[string]string{
			"ssmManagedNodeRole": "arn:aws:iam::123456789012:role/SSMRole",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, "act-123", resp.ActivationID)
	assert.Equal(t, "code-456", resp.ActivationCode)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_RegisterNodeAgent_MissingRole(t *testing.T) {
	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(new(MockSSMClient)))

	_, err := handler.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:           testRemotePodUID,
		WorkspaceName:    testRemoteWorkspace,
		Namespace:        testRemoteNamespace,
		PodEventsContext: map[string]string{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssmManagedNodeRole")
}

func TestAWSRemoteAccessRoutes_DeregisterNodeAgent_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock CleanupManagedInstancesByPodUID flow
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{},
		}, nil,
	)

	// Mock CleanupActivationsByPodUID flow
	mockSSM.On("DescribeActivations", mock.Anything, mock.Anything).Return(
		&ssm.DescribeActivationsOutput{
			ActivationList: []types.Activation{},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{
		PodUID: testRemotePodUID,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock FindInstanceByPodUID
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)

	// Mock checkNumActiveSessions
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)

	// Mock StartSession
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-abc"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-xyz"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: testVscodeRemote,
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "sess-abc")
	assert.Contains(t, resp.ConnectionURL, connectionScheme(testVscodeRemote).Default)
	assert.Contains(t, resp.ConnectionURL, "streamUrl=wss://stream.example.com")
	assert.Contains(t, resp.ConnectionURL, "sessionToken=token-xyz")
	assert.Contains(t, resp.ConnectionURL, "workspaceName="+testRemoteWorkspace)
	assert.Contains(t, resp.ConnectionURL, "namespace="+testRemoteNamespace)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_MissingDocumentName(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock FindInstanceByPodUID
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:            testRemotePodUID,
		WorkspaceName:     testRemoteWorkspace,
		Namespace:         testRemoteNamespace,
		ConnectionType:    testVscodeRemote,
		ConnectionContext: map[string]string{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), testSSMDocKey)
}

func TestAWSRemoteAccessRoutes_CreateSession_InstanceNotFound(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: testVscodeRemote,
		ConnectionContext: map[string]string{
			testSSMDocKey: "my-doc",
		},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no managed instance found")
}

func TestAWSRemoteAccessRoutes_CreateSession_KiroRemote(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-kiro"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-kiro"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: testKiroRemote,
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "kiro://amazonwebservices.aws-toolkit-vscode/connect/workspace")
	assert.Contains(t, resp.ConnectionURL, "sess-kiro")
	assert.NotContains(t, resp.ConnectionURL, "vscode://")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_CursorRemote(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-cursor"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-cursor"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: "cursor-remote",
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "cursor://amazonwebservices.aws-toolkit-vscode/connect/workspace")
	assert.NotContains(t, resp.ConnectionURL, "vscode://")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_UnknownRemoteType(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-windsurf"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-windsurf"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: "windsurf-remote",
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "windsurf://amazonwebservices.aws-toolkit-vscode/connect/workspace")
	assert.NotContains(t, resp.ConnectionURL, "vscode://")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_SchemeOverrideViaContext(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-custom"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-custom"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: testKiroRemote,
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
			"kiroScheme":  "kiro://custom.kiro.dev/workspace",
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "kiro://custom.kiro.dev/workspace")
	assert.NotContains(t, resp.ConnectionURL, "aws-toolkit-vscode")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_EmptyConnectionType(t *testing.T) {
	mockSSM := new(MockSSMClient)
	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:         testRemotePodUID,
		WorkspaceName:  testRemoteWorkspace,
		Namespace:      testRemoteNamespace,
		ConnectionType: "",
		ConnectionContext: map[string]string{
			testSSMDocKey: testSSMDocName,
		},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectionType is required")
}

func TestConnectionScheme(t *testing.T) {
	tests := []struct {
		connectionType string
		expectedKey    string
		expectedScheme string
	}{
		{testVscodeRemote, "vscodeScheme", "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace"},
		{testKiroRemote, "kiroScheme", "kiro://amazonwebservices.aws-toolkit-vscode/connect/workspace"},
		{"cursor-remote", "cursorScheme", "cursor://amazonwebservices.aws-toolkit-vscode/connect/workspace"},
		{"windsurf-remote", "windsurfScheme", "windsurf://amazonwebservices.aws-toolkit-vscode/connect/workspace"},
	}

	for _, tt := range tests {
		t.Run(tt.connectionType, func(t *testing.T) {
			entry := connectionScheme(tt.connectionType)
			assert.Equal(t, tt.expectedKey, entry.Key)
			assert.Equal(t, tt.expectedScheme, entry.Default)
		})
	}
}
