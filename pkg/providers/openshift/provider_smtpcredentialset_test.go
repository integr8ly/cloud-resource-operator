package openshift

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"
)

func TestSMTPCredentialProvider_CreateSMTPCredentials(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		ctx       context.Context
		smtpCreds *v1alpha1.SMTPCredentialSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.SMTPCredentialSetInstance
		wantErr bool
	}{
		{
			name: "test placeholders used when secret does not exist",
			fields: fields{
				Client: fake.NewFakeClient(),
				Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				smtpCreds: &v1alpha1.SMTPCredentialSet{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			want: &providers.SMTPCredentialSetInstance{
				DeploymentDetails: &aws.SMTPCredentialSetDetails{
					Username: varPlaceholder,
					Password: varPlaceholder,
					Port:     smtpPortPlaceholder,
					Host:     varPlaceholder,
					TLS:      smtpTLSPlaceholder,
				},
			},
			wantErr: false,
		},
		{
			name: "test existing secret values are used if exist",
			fields: fields{
				Client: fake.NewFakeClient(&v12.Secret{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						aws.DetailsSMTPUsernameKey: []byte("test"),
						aws.DetailsSMTPPasswordKey: []byte("test"),
						aws.DetailsSMTPHostKey:     []byte("test"),
						aws.DetailsSMTPTLSKey:      []byte("false"),
						aws.DetailsSMTPPortKey:     []byte("123"),
					},
				}),
				Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				smtpCreds: &v1alpha1.SMTPCredentialSet{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: v1alpha1.SMTPCredentialSetStatus{
						Phase: v1alpha1.PhaseComplete,
						SecretRef: &v1alpha1.SecretRef{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			want: &providers.SMTPCredentialSetInstance{
				DeploymentDetails: &aws.SMTPCredentialSetDetails{
					Username: "test",
					Password: "test",
					Port:     123,
					Host:     "test",
					TLS:      false,
				},
			},
			wantErr: false,
		},
		{
			name: "test missing values are replaced with placeholders",
			fields: fields{
				Client: fake.NewFakeClient(&v12.Secret{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						aws.DetailsSMTPPortKey: []byte("123"),
					},
				}), Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				smtpCreds: &v1alpha1.SMTPCredentialSet{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: v1alpha1.SMTPCredentialSetStatus{
						Phase: v1alpha1.PhaseComplete,
						SecretRef: &v1alpha1.SecretRef{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			want: &providers.SMTPCredentialSetInstance{
				DeploymentDetails: &aws.SMTPCredentialSetDetails{
					Username: varPlaceholder,
					Password: varPlaceholder,
					Port:     123,
					Host:     varPlaceholder,
					TLS:      smtpTLSPlaceholder,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SMTPCredentialProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			got, _, err := s.CreateSMTPCredentials(tt.args.ctx, tt.args.smtpCreds)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSMTPCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateSMTPCredentials() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSMTPCredentialProvider_GetReconcileTime(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		smtpCreds *v1alpha1.SMTPCredentialSet
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   time.Duration
	}{
		{
			name: "test expected time for regression",
			want: time.Second * 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SMTPCredentialProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			if got := s.GetReconcileTime(tt.args.smtpCreds); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSMTPCredentialProvider_SupportsStrategy(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		str string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "test supported",
			args: args{str: "openshift"},
			want: true,
		},
		{
			name: "test unsupported",
			args: args{str: "test"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SMTPCredentialProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			if got := s.SupportsStrategy(tt.args.str); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}
