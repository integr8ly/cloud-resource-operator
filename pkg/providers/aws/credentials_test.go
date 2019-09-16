package aws

import (
	"context"
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
		credName            string
		credNS              string
		entries             []v1.StatementEntry
		expectedAccessKeyID string
		expectedSecretKey   string
		client              client.Client
	}{
		{
			name:                "test credentials are reconciled successfully",
			credName:            "test",
			credNS:              "test",
			entries:             []v1.StatementEntry{},
			expectedAccessKeyID: "testkey",
			expectedSecretKey:   "testsecret",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Spec: v1.CredentialsRequestSpec{
					SecretRef: v12.ObjectReference{
						Name:      "test",
						Namespace: "test",
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
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte("testkey"),
					defaultCredentialsSecretKeyName: []byte("testsecret"),
				},
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewCredentialMinterCredentialManager(tc.client)
			_, awsCreds, err := cm.ReconcileCredentials(context.TODO(), tc.credName, tc.credNS, tc.entries)
			if err != nil {
				t.Fatal("unexpected error", err)
			}
			if awsCreds.AccessKeyID != tc.expectedAccessKeyID {
				t.Fatalf("unexpected access key id, expected %s but got %s", tc.expectedAccessKeyID, awsCreds.AccessKeyID)
			}
			if awsCreds.SecretAccessKey != tc.expectedSecretKey {
				t.Fatalf("unexpected secret access key, expected %s but got %s", tc.expectedSecretKey, awsCreds.SecretAccessKey)
			}
		})
	}
}
