package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	apis2 "github.com/integr8ly/cloud-resource-operator/pkg/apis"
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
	err = v12.AddToScheme(scheme)
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
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
			SecretRef: &v1alpha1.SecretRef{
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
				Client: fake.NewFakeClientWithScheme(scheme, testSMTPCred, &v1.CredentialsRequest{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				}),
				Logger:            logrus.WithFields(logrus.Fields{}),
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
				Client:            fake.NewFakeClientWithScheme(scheme, testSMTPCred),
				Logger:            logrus.WithFields(logrus.Fields{}),
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
			if err := p.DeleteSMTPCredentials(tt.args.ctx, tt.args.smtpCreds); (err != nil) != tt.wantErr {
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
