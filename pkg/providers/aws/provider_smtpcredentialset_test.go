package aws

import (
	"bytes"
	"context"
	"errors"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	apis2 "github.com/integr8ly/cloud-resource-operator/pkg/apis"
	crov1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/config/v1"
	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"github.com/openshift/cloud-credential-operator/pkg/apis"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/sirupsen/logrus"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := apis2.AddToScheme(scheme)
	err = crov1.SchemeBuilder.AddToScheme(scheme)
	err = v12.AddToScheme(scheme)
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestCredentialsRequest() *v1.CredentialsRequest {
	return &v1.CredentialsRequest{
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
	}
}

func buildTestInfrastructure() *crov1.Infrastructure {
	return &crov1.Infrastructure{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Status: crov1.InfrastructureStatus{
			InfrastructureName: "test",
			Platform:           crov1.AWSPlatformType,
			PlatformStatus: &crov1.PlatformStatus{
				AWS: &crov1.AWSPlatformStatus{
					Region: "test",
				},
			},
		},
	}
}

func buildTestAWSCredentials() *Credentials {
	return &Credentials{
		Username:        "test",
		PolicyName:      "test",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}
}

func buildTestSMTPCredentialSet() *v1alpha1.SMTPCredentialSet {
	return &v1alpha1.SMTPCredentialSet{
		ObjectMeta: controllerruntime.ObjectMeta{},
		Spec: v1alpha1.SMTPCredentialSetSpec{
			Type:      "test",
			Tier:      "test",
			SecretRef: &types.SecretRef{Name: "test"},
		},
		Status: v1alpha1.SMTPCredentialSetStatus{},
	}
}

func Test_getSMTPPasswordFromAWSSecret(t *testing.T) {
	type args struct {
		secAccessKey string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test smtp auth password generation for ses works as expected",
			args: args{
				secAccessKey: "test",
			},
			want:    "AsuNxtdhciTpIaQYwF9CtO/nlNX2hCZkD8E+4vZzrjs0",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getSMTPPasswordFromAWSSecret(tt.args.secAccessKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSMTPPasswordFromAWSSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getSMTPPasswordFromAWSSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSMTPCredentialProvider_DeleteSMTPCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}
	testSMTPCred := &v1alpha1.SMTPCredentialSet{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Finalizers: []string{
				DefaultFinalizer,
			},
		},
		Spec: v1alpha1.SMTPCredentialSetSpec{
			SecretRef: &types.SecretRef{
				Name: "test",
			},
			Tier: "test",
			Type: "test",
		},
	}

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx       context.Context
		smtpCreds *v1alpha1.SMTPCredentialSet
	}
	tests := []struct {
		name                 string
		fields               fields
		args                 args
		wantErr              bool
		validateCredentialFn func(set *v1alpha1.SMTPCredentialSet) error
	}{
		{
			name: "test finalizer and credential request is removed successfully",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, testSMTPCred),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx:       context.TODO(),
				smtpCreds: testSMTPCred,
			},
			wantErr: false,
			validateCredentialFn: func(set *v1alpha1.SMTPCredentialSet) error {
				if len(set.Finalizers) != 0 {
					return errors.New("finalizer was not removed")
				}
				return nil
			},
		},
		{
			name: "test deletion handler completes successfully when credential request does not exist",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, testSMTPCred, buildTestCredentialsRequest()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx:       context.TODO(),
				smtpCreds: testSMTPCred,
			},
			wantErr: false,
			validateCredentialFn: func(set *v1alpha1.SMTPCredentialSet) error {
				if len(set.Finalizers) != 0 {
					return errors.New("finalizer was not removed")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SMTPCredentialProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			if _, err := p.DeleteSMTPCredentials(tt.args.ctx, tt.args.smtpCreds); (err != nil) != tt.wantErr {
				t.Errorf("DeleteSMTPCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.validateCredentialFn(tt.args.smtpCreds); err != nil {
				t.Error("unexpected error", err)
			}
			return
		})
	}
}

func TestSMTPCredentialProvider_SupportsStrategy(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		d string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "test aws strategy is supported",
			fields: fields{
				Client:            nil,
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{d: providers.AWSDeploymentStrategy},
			want: true,
		},
		{
			name: "test openshift strategy is not supported",
			fields: fields{
				Client:            nil,
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{d: "openshift"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SMTPCredentialProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			if got := p.SupportsStrategy(tt.args.d); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSMTPCredentialProvider_CreateSMTPCredentials(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx       context.Context
		smtpCreds *v1alpha1.SMTPCredentialSet
	}
	tests := []struct {
		name              string
		fields            fields
		args              args
		validateDetailsFn func(cred *v1alpha1.SMTPCredentialSet, inst *providers.SMTPCredentialSetInstance) error
		wantData          map[string][]byte
		wantErr           bool
	}{
		{
			name: "test smtp credential set details are retrieved successfully",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestSMTPCredentialSet(), buildTestInfrastructure()),
				Logger: testLogger,
				CredentialManager: &CredentialManagerMock{
					ReconcileSESCredentialsFunc: func(ctx context.Context, name string, ns string) (credentials *Credentials, e error) {
						return buildTestAWSCredentials(), nil
					},
				},
				ConfigManager: &ConfigManagerMock{
					GetDefaultRegionSMTPServerMappingFunc: func() map[string]string {
						return map[string]string{
							regionEUWest1: sesSMTPEndpointEUWest1,
						}
					},
					ReadSMTPCredentialSetStrategyFunc: func(ctx context.Context, tier string) (config *StrategyConfig, e error) {
						return &StrategyConfig{
							Region:         regionEUWest1,
							CreateStrategy: []byte("{}"),
						}, nil
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				smtpCreds: buildTestSMTPCredentialSet(),
			},
			wantErr: false,
			validateDetailsFn: func(cred *v1alpha1.SMTPCredentialSet, inst *providers.SMTPCredentialSetInstance) error {
				if len(cred.GetFinalizers()) == 0 {
					return errors.New("finalizer was not set on smtp credential resource")
				}
				return nil
			},
			wantData: map[string][]byte{
				DetailsSMTPUsernameKey: []byte("test"),
				DetailsSMTPPasswordKey: []byte("AsuNxtdhciTpIaQYwF9CtO/nlNX2hCZkD8E+4vZzrjs0"),
				DetailsSMTPPortKey:     []byte("587"),
				DetailsSMTPHostKey:     []byte(sesSMTPEndpointEUWest1),
				DetailsSMTPTLSKey:      []byte("true"),
			},
		},
		{
			name: "test fails if unsupported ses region is used",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestSMTPCredentialSet()),
				Logger: testLogger,
				CredentialManager: &CredentialManagerMock{
					ReconcileSESCredentialsFunc: func(ctx context.Context, name string, ns string) (credentials *Credentials, e error) {
						return buildTestAWSCredentials(), nil
					},
				},
				ConfigManager: &ConfigManagerMock{
					GetDefaultRegionSMTPServerMappingFunc: func() map[string]string {
						return map[string]string{}
					},
					ReadSMTPCredentialSetStrategyFunc: func(ctx context.Context, tier string) (config *StrategyConfig, e error) {
						return &StrategyConfig{
							Region:         "unsupported-region",
							CreateStrategy: []byte("{}"),
						}, nil
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				smtpCreds: buildTestSMTPCredentialSet(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SMTPCredentialProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, _, err := p.CreateSMTPCredentials(tt.args.ctx, tt.args.smtpCreds)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSMTPCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantData != nil {
				for k, v := range tt.wantData {
					if !bytes.Equal(v, got.DeploymentDetails.Data()[k]) {
						t.Errorf("CreateSMTPCredentials() data = %v, wantData %v", string(got.DeploymentDetails.Data()[k]), string(v))
						return
					}
				}
			}
			if tt.validateDetailsFn != nil {
				if err := tt.validateDetailsFn(tt.args.smtpCreds, got); err != nil {
					t.Error("error during validation", err)
					return
				}
			}
		})
	}
}

func TestSMTPCredentialProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		s *v1alpha1.SMTPCredentialSet
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "test short reconcile when the cr is not complete",
			args: args{
				s: &v1alpha1.SMTPCredentialSet{
					Status: v1alpha1.SMTPCredentialSetStatus{
						Phase: types.PhaseInProgress,
					},
				},
			},
			want: time.Second * 30,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				s: &v1alpha1.SMTPCredentialSet{
					Status: v1alpha1.SMTPCredentialSetStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SMTPCredentialProvider{}
			if got := p.GetReconcileTime(tt.args.s); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
