package smtpcredentialset

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/sirupsen/logrus"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	apis2 "github.com/openshift/cloud-credential-operator/pkg/apis"
	v12 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func buildTestOperatorConfigMap() *v12.ConfigMap {
	return &v12.ConfigMap{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "cloud-resource-config",
			Namespace: "test",
		},
		Data: map[string]string{
			"test": "{ \"smtpcredentials\": \"test\" }",
		},
	}
}

func buildTestSMTPCredentialSet() *v1alpha1.SMTPCredentialSet {
	return &v1alpha1.SMTPCredentialSet{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v1alpha1.SMTPCredentialSetSpec{
			Tier:      "test",
			Type:      "test",
			SecretRef: &v1alpha1.SecretRef{Name: "test"},
		},
	}
}

func TestReconcileSMTPCredentialSet_Reconcile(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Error("unexpected error while constructing test scheme", err)
	}

	type fields struct {
		client       client.Client
		scheme       *runtime.Scheme
		logger       *logrus.Entry
		ctx          context.Context
		providerList []providers.SMTPCredentialsProvider
		cfgMgr       providers.ConfigManager
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "test successful reconcile sets repeated reconciliation to true",
			fields: fields{
				client: fake.NewFakeClientWithScheme(scheme, buildTestOperatorConfigMap(), buildTestSMTPCredentialSet()),
				scheme: scheme,
				logger: logrus.WithFields(logrus.Fields{}),
				ctx:    context.TODO(),
				providerList: []providers.SMTPCredentialsProvider{
					&providers.SMTPCredentialsProviderMock{
						CreateSMTPCredentialsFunc: func(ctx context.Context, smtpCreds *v1alpha1.SMTPCredentialSet) (*providers.SMTPCredentialSetInstance, v1alpha1.StatusMessage, error) {
							return &providers.SMTPCredentialSetInstance{
								DeploymentDetails: &providers.DeploymentDetailsMock{
									DataFunc: func() map[string][]byte {
										return map[string][]byte{
											"test": []byte("test"),
										}
									},
								},
							}, "", nil
						},
						GetReconcileTimeFunc: func(smtpCreds *v1alpha1.SMTPCredentialSet) time.Duration {
							return time.Second * 10
						},
						DeleteSMTPCredentialsFunc: func(ctx context.Context, smtpCreds *v1alpha1.SMTPCredentialSet) (v1alpha1.StatusMessage, error) {
							return "", nil
						},
						GetNameFunc: func() string {
							return "test"
						},
						SupportsStrategyFunc: func(s string) bool {
							return s == "test"
						},
					},
				},
				cfgMgr: &providers.ConfigManagerMock{
					GetStrategyMappingForDeploymentTypeFunc: func(ctx context.Context, t string) (*providers.DeploymentStrategyMapping, error) {
						return &providers.DeploymentStrategyMapping{
							SMTPCredentials: "test",
						}, nil
					},
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test",
						Name:      "test",
					},
				},
			},
			want: struct {
				Requeue      bool
				RequeueAfter time.Duration
			}{Requeue: true, RequeueAfter: time.Second * 10},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileSMTPCredentialSet{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
				logger: tt.fields.logger,
				resourceProvider: &resources.ReconcileResourceProvider{
					Client: tt.fields.client,
					Scheme: tt.fields.scheme,
					Logger: tt.fields.logger,
				},
				providerList: tt.fields.providerList,
			}
			got, err := r.Reconcile(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconcile() got = %v, want %v", got, tt.want)
			}
		})
	}
}
