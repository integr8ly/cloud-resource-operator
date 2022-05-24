package aws

import (
	"context"
	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCredentialMinterManager_ReconcileProviderCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	cases := []struct {
		name                string
		client              client.Client
		wantErr             bool
		expectedAccessKeyID string
		expectedSecretKey   string
		expectedErrMsg      string
	}{
		{
			name: "credentials are reconciled successfully",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte("ACCESS_KEY_ID"),
					defaultCredentialsSecretKeyName: []byte("SECRET_ACCESS_KEY"),
				},
			}),
			wantErr:             false,
			expectedAccessKeyID: "ACCESS_KEY_ID",
			expectedSecretKey:   "SECRET_ACCESS_KEY",
		},
		{
			name: "error reconciling credentials",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}),
			wantErr:        true,
			expectedErrMsg: "failed to reconcile aws credentials from credential request cloud-resources-aws-credentials",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewCredentialManager(tc.client).(*CredentialMinterCredentialManager)
			awsCreds, err := cm.ReconcileProviderCredentials(context.TODO(), "testNamespace")
			if tc.wantErr {
				if !errorContains(err, tc.expectedErrMsg) {
					t.Fatalf("unexpected error from ReconcileProviderCredentials(): %v", err)
				}
				return
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

func TestCredentialMinterManager_ReconcileCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx     context.Context
		name    string
		ns      string
		entries []v1.StatementEntry
	}
	cases := []struct {
		name                string
		client              client.Client
		args                args
		wantErr             bool
		expectedErrMsg      string
		expectedAccessKeyID string
		expectedSecretKey   string
		mockFn              func()
	}{
		{
			name: "successfully reconciled credentials",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte("ACCESS_KEY_ID"),
					defaultCredentialsSecretKeyName: []byte("SECRET_ACCESS_KEY"),
				},
			}),
			args: args{
				ctx:     context.TODO(),
				name:    defaultProviderCredentialName,
				ns:      "testNamespace",
				entries: nil,
			},
			wantErr:             false,
			expectedAccessKeyID: "ACCESS_KEY_ID",
			expectedSecretKey:   "SECRET_ACCESS_KEY",
		},
		{
			name: "undefined aws access key id in credentials secret",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte(""),
					defaultCredentialsSecretKeyName: []byte("SECRET_ACCESS_KEY"),
				},
			}),
			args: args{
				ctx:     context.TODO(),
				name:    defaultProviderCredentialName,
				ns:      "testNamespace",
				entries: nil,
			},
			wantErr:        true,
			expectedErrMsg: "aws access key id is undefined in secret",
		},
		{
			name: "undefined aws access key id in credentials secret",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: "testNamespace",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte("ACCESS_KEY_ID"),
					defaultCredentialsSecretKeyName: []byte(""),
				},
			}),
			args: args{
				ctx:     context.TODO(),
				name:    defaultProviderCredentialName,
				ns:      "testNamespace",
				entries: nil,
			},
			wantErr:        true,
			expectedErrMsg: "aws secret access key is undefined in secret",
		},
		{
			name:   "failed to reconcile aws credential request",
			client: fake.NewFakeClientWithScheme(scheme),
			args: args{
				ctx: context.TODO(),
			},
			wantErr:        true,
			expectedErrMsg: "failed to reconcile aws credential request",
		},
		{
			name:   "failed to provision credential request (timeout)",
			client: fake.NewFakeClientWithScheme(scheme),
			args: args{
				ctx:  context.TODO(),
				name: defaultProviderCredentialName,
				ns:   "testNamespace",
			},
			wantErr:        true,
			expectedErrMsg: "timed out waiting for credential request to provision",
			mockFn: func() {
				timeOut = time.Millisecond * 10
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFn != nil {
				tc.mockFn()
				// Reset
				defer func() {
					timeOut = time.Minute * 5
				}()
			}
			cm := NewCredentialManager(tc.client).(*CredentialMinterCredentialManager)
			_, err := cm.reconcileCredentials(tc.args.ctx, tc.args.name, tc.args.ns, tc.args.entries)
			if tc.wantErr {
				if !errorContains(err, tc.expectedErrMsg) {
					t.Fatalf("unexpected error from reconcileCredentials(): %v", err)
				}
				return
			}
		})
	}
}

func TestCredentialManager_ReconcileBucketOwnerCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx    context.Context
		name   string
		ns     string
		bucket string
	}
	cases := []struct {
		name                string
		client              client.Client
		args                args
		wantErr             bool
		expectedAccessKeyID string
		expectedSecretKey   string
	}{
		{
			name: "successfully reconciled bucket owner credentials",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "testName",
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}, &v12.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "testName",
					Namespace: "testNamespace",
				},
				Data: map[string][]byte{
					defaultCredentialsKeyIDName:     []byte("ACCESS_KEY_ID"),
					defaultCredentialsSecretKeyName: []byte("SECRET_ACCESS_KEY"),
				},
			}),
			args: args{
				ctx:    context.TODO(),
				name:   "testName",
				ns:     "testNamespace",
				bucket: "testBucket",
			},
			wantErr:             false,
			expectedAccessKeyID: "ACCESS_KEY_ID",
			expectedSecretKey:   "SECRET_ACCESS_KEY",
		},
		{
			name: "failed to get aws credentials secret",
			client: fake.NewFakeClientWithScheme(scheme, &v1.CredentialsRequest{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "testName",
					Namespace: "testNamespace",
				},
				Status: v1.CredentialsRequestStatus{
					Provisioned: true,
					ProviderStatus: &runtime.RawExtension{
						Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
					},
				},
			}),
			args: args{
				ctx:    context.TODO(),
				name:   "testName",
				ns:     "testNamespace",
				bucket: "testBucket",
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewCredentialManager(tc.client)
			awsCreds, err := cm.ReconcileBucketOwnerCredentials(tc.args.ctx, tc.args.name, tc.args.ns, tc.args.bucket)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error from ReconcileBucketOwnerCredentials(), but got nil")
				}
				return
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
