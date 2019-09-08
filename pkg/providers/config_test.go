package providers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	v1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewConfigManager(t *testing.T) {
	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme())
	cases := []struct {
		name              string
		cmName            string
		cmNamespace       string
		expectedName      string
		expectedNamespace string
		client            client.Client
	}{
		{
			name:              "test defaults are set if values are not provided",
			cmName:            "",
			cmNamespace:       "",
			expectedName:      DefaultProviderConfigMapName,
			expectedNamespace: DefaultConfigNamespace,
			client:            fakeClient,
		},
		{
			name:              "test defaults are not used if values are provided",
			cmName:            "test",
			cmNamespace:       "test",
			expectedName:      "test",
			expectedNamespace: "test",
			client:            fakeClient,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewConfigManager(tc.cmName, tc.cmNamespace, tc.client)
			if cm.providerConfigMapName != tc.expectedName {
				t.Fatalf("unexpected config map name, got %s but expected %s", cm.providerConfigMapName, tc.expectedName)
			}
			if cm.providerConfigMapNamespace != tc.expectedNamespace {
				t.Fatalf("unexpected config map namespace, got %s but expected %s", cm.providerConfigMapNamespace, tc.expectedNamespace)
			}
		})
	}
}

func TestConfigManager_GetStrategyMappingForDeploymentType(t *testing.T) {
	testDtc := &DeploymentStrategyMapping{
		BlobStorage: AWSDeploymentStrategy,
	}
	testDtcJSON, err := json.Marshal(testDtc)
	if err != nil {
		t.Fatal("failed to marshal test deployment config type", err)
	}
	scheme := runtime.NewScheme()
	err = v1.AddToScheme(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme, &v1.ConfigMap{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Data: map[string]string{
			ManagedDeploymentType: string(testDtcJSON),
		},
	})
	cases := []struct {
		name           string
		cmName         string
		cmNamespace    string
		client         client.Client
		expectError    bool
		validateConfig func(dtc *DeploymentStrategyMapping) error
	}{
		{
			name:        "test config is unmarshalled successfully when configmap is structured correctly",
			cmName:      "test",
			cmNamespace: "test",
			client:      fakeClient,
			validateConfig: func(dtc *DeploymentStrategyMapping) error {
				if dtc.BlobStorage != AWSDeploymentStrategy {
					return errors.New("strategy mapping has incorrect structure")
				}
				return nil
			},
		},
		{
			name:        "test error is returned when config map doesn't exist",
			cmName:      "err",
			cmNamespace: "err",
			client:      fakeClient,
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewConfigManager(tc.cmName, tc.cmNamespace, tc.client)
			dtc, err := cm.GetStrategyMappingForDeploymentType(context.TODO(), ManagedDeploymentType)
			if err != nil {
				if tc.expectError {
					return
				}
				t.Fatal("failed to read deployment type config", err)
			}
			err = tc.validateConfig(dtc)
			if err != nil {
				t.Fatal("failed to validate deployment type config", err)
			}
		})
	}
}
