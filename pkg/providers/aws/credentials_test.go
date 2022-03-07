package aws

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCredentialManager_ReconcileCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	err = v12.AddToScheme(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	cases := []struct {
		name                string
		client              client.Client
		isSTS               bool
		wantErr             bool
		expectedAccessKeyID string
		expectedSecretKey   string
		expectedRoleARN     string
		expectedTokenPath   string
		expectedErrMsg      string
	}{
		{
			name:                "credentials are reconciled successfully",
			client:              buildClient(scheme, false),
			expectedAccessKeyID: "ACCESS_KEY_ID",
			expectedSecretKey:   "SECRET_ACCESS_KEY",
		},
		{
			name:              "sts credentials are reconciled successfully",
			client:            buildClient(scheme, true, "ROLE_ARN", "TOKEN_PATH"),
			isSTS:             true,
			expectedRoleARN:   "ROLE_ARN",
			expectedTokenPath: "TOKEN_PATH",
		},
		{
			name:           "undefined role arn key in sts credentials secret",
			client:         buildClient(scheme, true, "", "TOKEN_PATH"),
			isSTS:          true,
			wantErr:        true,
			expectedErrMsg: fmt.Sprintf("%s key is undefined in secret %s", defaultRoleARNKeyName, defaultSTSCredentialSecretName),
		},
		{
			name:           "undefined token path key in sts credentials secret",
			client:         buildClient(scheme, true, "ROLE_ARN", ""),
			isSTS:          true,
			wantErr:        true,
			expectedErrMsg: fmt.Sprintf("%s key is undefined in secret %s", defaultTokenPathKeyName, defaultSTSCredentialSecretName),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewCredentialManager(tc.client)
			awsCreds, err := cm.ReconcileProviderCredentials(context.TODO(), "")
			if tc.wantErr {
				if !errorContains(err, tc.expectedErrMsg) {
					t.Fatalf("unexpected error from ReconcileProviderCredentials(): %v", err)
				}
				return
			}
			switch tc.isSTS {
			case true:
				if awsCreds.RoleArn != tc.expectedRoleARN {
					t.Fatalf("unexpected role arn, expected %s but got %s", tc.expectedRoleARN, awsCreds.RoleArn)
				}
				if awsCreds.TokenFilePath != tc.expectedTokenPath {
					t.Fatalf("unexpected toke file path, expected %s but got %s", tc.expectedTokenPath, awsCreds.TokenFilePath)
				}
			default:
				if awsCreds.AccessKeyID != tc.expectedAccessKeyID {
					t.Fatalf("unexpected access key id, expected %s but got %s", tc.expectedAccessKeyID, awsCreds.AccessKeyID)
				}
				if awsCreds.SecretAccessKey != tc.expectedSecretKey {
					t.Fatalf("unexpected secret access key, expected %s but got %s", tc.expectedSecretKey, awsCreds.SecretAccessKey)
				}
			}
		})
	}
}

func buildClient(scheme *runtime.Scheme, isSTS bool, secretValues ...string) client.Client {
	if isSTS {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v12.Secret{
			ObjectMeta: controllerruntime.ObjectMeta{
				Name: defaultSTSCredentialSecretName,
			},
			Data: map[string][]byte{
				defaultRoleARNKeyName:   []byte(secretValues[0]),
				defaultTokenPathKeyName: []byte(secretValues[1]),
			},
		}).Build()
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1.CredentialsRequest{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: defaultProviderCredentialName,
		},
		Spec: v1.CredentialsRequestSpec{
			SecretRef: v12.ObjectReference{
				Name: defaultProviderCredentialName,
			},
		},
		Status: v1.CredentialsRequestStatus{
			Provisioned: true,
			ProviderStatus: &runtime.RawExtension{
				Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
			},
		},
	}, &v12.Secret{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: defaultProviderCredentialName,
		},
		Data: map[string][]byte{
			defaultCredentialsKeyIDName:     []byte("ACCESS_KEY_ID"),
			defaultCredentialsSecretKeyName: []byte("SECRET_ACCESS_KEY"),
		},
	}).Build()
}
