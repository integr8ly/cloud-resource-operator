package aws

import (
	"context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	v12 "k8s.io/api/core/v1"
	"os"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestSTSCredentialManager_ReconcileProviderCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	// Make test work locally
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	ns, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		t.Fatal("failed to get operator namespace", err)
	}
	cases := []struct {
		name              string
		client            client.Client
		wantErr           bool
		expectedRoleARN   string
		expectedTokenPath string
		expectedErrMsg    string
	}{
		{
			name: "sts credentials are reconciled successfully",
			client: fake.NewFakeClientWithScheme(scheme, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultSTSCredentialSecretName,
					Namespace: ns,
				},
				Data: map[string][]byte{
					defaultRoleARNKeyName: []byte("ROLE_ARN"),
				},
			}),
			wantErr:           false,
			expectedRoleARN:   "ROLE_ARN",
			expectedTokenPath: defaultTokenPath,
		},
		{
			name: "undefined role arn key in sts credentials secret",
			client: fake.NewFakeClientWithScheme(scheme, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultSTSCredentialSecretName,
					Namespace: ns,
				},
				Data: map[string][]byte{
					defaultRoleARNKeyName: []byte(""),
				},
			}),
			wantErr:        true,
			expectedErrMsg: fmt.Sprintf("%s key is undefined in secret %s", defaultRoleARNKeyName, defaultSTSCredentialSecretName),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm, err := NewCredentialManager(tc.client)
			awsCreds, err := cm.(*STSCredentialManager).ReconcileProviderCredentials(context.TODO(), ns)
			if tc.wantErr {
				if !errorContains(err, tc.expectedErrMsg) {
					t.Fatalf("unexpected error from STS ReconcileProviderCredentials(): %v", err)
				}
				return
			}
			if awsCreds.RoleArn != tc.expectedRoleARN {
				t.Fatalf("unexpected role arn, expected %s but got %s", tc.expectedRoleARN, awsCreds.RoleArn)
			}
			if awsCreds.TokenFilePath != tc.expectedTokenPath {
				t.Fatalf("unexpected toke file path, expected %s but got %s", tc.expectedTokenPath, awsCreds.TokenFilePath)
			}
		})
	}
}

func TestSTSCredentialManager_ReconcileBucketOwnerCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	// Make test work locally
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	ns, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		t.Fatal("failed to get operator namespace", err)
	}
	type args struct {
		ctx    context.Context
		name   string
		ns     string
		bucket string
	}
	cases := []struct {
		name    string
		client  client.Client
		args    args
		wantErr bool
	}{
		{
			name: "successfully reconciled bucket owner credentials",
			client: fake.NewFakeClientWithScheme(scheme, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultSTSCredentialSecretName,
					Namespace: ns,
				},
				Data: map[string][]byte{
					defaultRoleARNKeyName: []byte("ROLE_ARN"),
				},
			}),
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm, err := NewCredentialManager(tc.client)
			if err != nil {
				t.Fatal(err.Error())
			}
			_, err = cm.(*STSCredentialManager).ReconcileBucketOwnerCredentials(tc.args.ctx, tc.args.name, tc.args.ns, tc.args.bucket)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error from ReconcileBucketOwnerCredentials(), but got nil")
				}
				return
			}
		})
	}
}
