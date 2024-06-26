package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"
	str2duration "github.com/xhit/go-str2duration/v2"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	utils "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gcpTestPostgresInstanceName = "gcptestclustertestNsgcpcloudsql"
	testInfrastructureName      = "cluster"
	testUser                    = "user"
	testPassword                = "password"
	gcpTestSnapshotFrequency    = "1h"
	gcpTestSnapshotRetention    = "30d"
	gcpTestInvalidSnapshotTime  = "invalid"
)

func buildTestPostgres() *v1alpha1.Postgres {
	postgres := buildTestPostgresWithoutAnnotation()
	postgres.Annotations = map[string]string{
		ResourceIdentifierAnnotation: testName,
	}
	return postgres
}

func buildTestPostgresPhase(phase types.StatusPhase) *v1alpha1.Postgres {
	postgres := buildTestPostgres()
	postgres.Status.Phase = phase
	return postgres
}

func buildTestPostgresWithoutAnnotation() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresProviderName,
			Namespace: testNs,
			Labels: map[string]string{
				"productName": "test_product",
			},
			ResourceVersion: "1000",
		},
	}
}

func buildTestPostgresSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresProviderName,
			Namespace: testNs,
		},
		Data: map[string][]byte{
			defaultPostgresUserKey:     []byte(testUser),
			defaultPostgresPasswordKey: []byte(testPassword),
		},
	}
}

func buildTestPostgresWithSnapshot() *v1alpha1.Postgres {
	postgres := buildTestPostgres()
	postgres.Spec.SnapshotRetention = gcpTestSnapshotRetention
	postgres.Spec.SnapshotFrequency = gcpTestSnapshotFrequency
	return postgres
}

func TestPostgresProvider_deleteCloudSQLInstance(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		Logger            *logrus.Entry
	}
	type args struct {
		strategyConfig  *StrategyConfig
		p               *v1alpha1.Postgres
		sqladminService *gcpiface.MockSqlClient
		networkManager  NetworkManager
		isLastResource  bool
		projectID       string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "if instance is not nil and state is PENDING_DELETE return status message",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "PENDING_DELETE",
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "postgres instance " + gcpTestPostgresInstanceName + " is already deleting",
			wantErr: false,
		},

		{
			name: "if instance is not nil, delete is not in progress delete function returns error",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "RUNNABLE",
							Settings:    &sqladmin.Settings{DeletionProtectionEnabled: false},
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return nil, errors.New("failed to delete cloudSQL instance: " + gcpTestPostgresInstanceName)
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "failed to delete cloudsql instance: " + gcpTestPostgresInstanceName,
			wantErr: true,
		},
		{
			name: "error when getting cloud sql instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return nil, fmt.Errorf("cannot retrieve sql instance from gcp")
					}
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, fmt.Errorf("cannot retrieve sql instances from gcp")
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "cannot retrieve sql instance from gcp",
			wantErr: true,
		},
		{
			name: "failed to retrieve postgres strategy config",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestGcpInfrastructure(nil))
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("failed to retrieve postgres strategy config")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:               buildTestPostgres(),
				networkManager:  buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  false,
			},
			want:    "failed to retrieve postgres strategy config",
			wantErr: true,
		},
		{
			name: "Error deleting cloudSQL secrets",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestGcpInfrastructure(nil))
					mc.DeleteFunc = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:               buildTestPostgres(),
				networkManager:  buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  false,
				projectID:       gcpTestProjectId,
			},
			want:    "failed to delete cloudSQL secrets",
			wantErr: true,
		},
		{
			name: "successful run of delete function when cloudsql object is already deleted",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:               buildTestPostgres(),
				networkManager:  buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  false,
				projectID:       gcpTestProjectId,
			},
			want:    "successfully deleted gcp postgres instance gcptestclustertestNsgcpcloudsql",
			wantErr: false,
		},
		{
			name: "successful run of delete function when cloudsql object is not already deleted",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "RUNNABLE",
							Settings:    &sqladmin.Settings{DeletionProtectionEnabled: false},
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "deletion in progress for cloudsql instance gcptestclustertestNsgcpcloudsql",
			wantErr: false,
		},
		{
			name: "want error when running delete function when cloudsql object is not already deleted but delete errors",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "RUNNABLE",
							Settings:    &sqladmin.Settings{DeletionProtectionEnabled: false},
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return nil, errors.New("delete error")
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "failed to delete cloudsql instance: " + gcpTestPostgresInstanceName,
			wantErr: true,
		},
		{
			name: "Error failed to update instance as part of finalizer reconcile",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil))
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager:  buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  false,
				projectID:       gcpTestProjectId,
			},
			want:    "failed to update instance as part of finalizer reconcile",
			wantErr: true,
		},
		{
			name: "error when modifying cloud sql instances",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "RUNNABLE",
							Settings:    &sqladmin.Settings{DeletionProtectionEnabled: true},
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
					sqlClient.ModifyInstanceFn = func(ctx context.Context, s string, s2 string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, fmt.Errorf("failed to modify cloudsql instance")
					}
				}),
				isLastResource: false,
			},
			want:    "failed to disable deletion protection for cloudsql instance: " + gcpTestPostgresInstanceName,
			wantErr: true,
		},
		{
			name: "failed to delete cluster network peering",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  true,
			},
			want:    "failed to delete cluster network peering",
			wantErr: true,
		},
		{
			name: "failed to delete cluster network service",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  true,
			},
			want:    "failed to delete cluster network service",
			wantErr: true,
		},
		{
			name: "failed to delete network IP range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  true,
			},
			want:    "failed to delete network IP range",
			wantErr: true,
		},
		{
			name: "when network component deletion in progress return status message",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					ComponentsExistFunc: func(contextMoqParam context.Context) (bool, error) {
						return true, nil
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  true,
			},
			want:    "network component deletion in progress",
			wantErr: false,
		},
		{
			name: "failed to check if components exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					ComponentsExistFunc: func(contextMoqParam context.Context) (bool, error) {
						return false, fmt.Errorf("generic error")
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  true,
			},
			want:    "failed to check if components exist",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				TCPPinger:         resources.BuildMockConnectionTester(),
			}
			got, err := pp.deleteCloudSQLInstance(context.TODO(), tt.args.networkManager, tt.args.sqladminService, tt.args.strategyConfig, tt.args.p, tt.args.isLastResource)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteCloudSQLInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DeleteCloudSQLInstance() statusMessage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_GetName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "success getting postgres provider name",
			want: postgresProviderName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{}
			if got := pp.GetName(); got != tt.want {
				t.Errorf("GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_SupportsStrategy(t *testing.T) {
	type args struct {
		deploymentStrategy string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "postgres provider supports strategy",
			args: args{
				deploymentStrategy: providers.GCPDeploymentStrategy,
			},
			want: true,
		},
		{
			name: "postgres provider does not support strategy",
			args: args{
				deploymentStrategy: providers.AWSDeploymentStrategy,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{}
			if got := pp.SupportsStrategy(tt.args.deploymentStrategy); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "get postgres default reconcile time",
			args: args{
				p: &v1alpha1.Postgres{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
		{
			name: "get postgres non-default reconcile time",
			args: args{
				p: &v1alpha1.Postgres{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{}
			if got := pp.GetReconcileTime(tt.args.p); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_setPostgresDeletionTimestampMetric(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	now := time.Now()
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		p               *v1alpha1.Postgres
		sqladminService *gcpiface.MockSqlClient
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "Deletion timestamp does exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: testInfrastructureName,
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "want error when no annotation on postgres cr",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: testInfrastructureName,
						},
						Status: configv1.InfrastructureStatus{
							InfrastructureName: testInfrastructureName,
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:              postgresProviderName,
						Namespace:         testNs,
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "unable to find instance name from annotation",
			wantErr: true,
		},
		{
			name: "annotation found on postgres cr",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: testInfrastructureName,
						},
						Status: configv1.InfrastructureStatus{
							InfrastructureName: testInfrastructureName,
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "successfully retrieved cluster ID",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&v1alpha1.Postgres{
						ObjectMeta: metav1.ObjectMeta{
							Name:      postgresProviderName,
							Namespace: testNs,
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: testName,
							},
						},
					},
					&configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: testInfrastructureName,
						},
						Status: configv1.InfrastructureStatus{
							InfrastructureName: testInfrastructureName,
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "failed to get cluster ID",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&v1alpha1.Postgres{
						ObjectMeta: metav1.ObjectMeta{
							Name:      postgresProviderName,
							Namespace: testNs,
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: testName,
							},
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "failed to get cluster id while exposing information metric for " + gcpTestPostgresInstanceName,
			wantErr: true,
		},
		{
			name: "build postgres status metrics label successful",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&v1alpha1.Postgres{
						ObjectMeta: metav1.ObjectMeta{
							Name:      postgresProviderName,
							Namespace: testNs,
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: testName,
							},
							Labels: map[string]string{
								resources.LabelClusterIDKey: "cluster",
							},
						},
						Status: types.ResourceTypeStatus{
							Phase: types.PhaseComplete,
						},
					},
					&configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: testInfrastructureName,
						},
						Status: configv1.InfrastructureStatus{
							InfrastructureName: testInfrastructureName,
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			pp.setPostgresDeletionTimestampMetric(context.TODO(), tt.args.p)
		})
	}
}

func TestPostgresProvider_reconcileCloudSQLInstance(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		p               *v1alpha1.Postgres
		sqladminService gcpiface.SQLAdminService
		strategyConfig  *StrategyConfig
		address         *computepb.Address
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "error when retrieving cloudSQL instance",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return nil, errors.New("cannot retrieve sql instance from gcp")
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"name":"gcptestclustertestNsgcpcloudsql","settings":{"backupConfiguration":{"backupRetentionSettings":{}}}}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "cannot retrieve sql instance from gcp",
			wantErr: true,
		},
		{
			name: "success building cloudSQL create strategy using defaults when settings object is not nil",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p:               buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"settings":{"backupConfiguration":{"backupRetentionSettings":{}}}}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "started cloudSQL provision",
			wantErr: false,
		},
		{
			name: "success building cloudSQL create strategy using defaults when settings object is nil",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p:               buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "started cloudSQL provision",
			wantErr: false,
		},
		{
			name: "success building cloudSQL create strategy using defaults when backup config object is nil",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p:               buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"settings":{}}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "started cloudSQL provision",
			wantErr: false,
		},
		{
			name: "success finding instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  gcpTestPostgresInstanceName,
									State: "RUNNABLE",
								},
							},
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"name":"gcptestclustertestNsgcpcloudsql"}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "started cloudSQL provision",
			wantErr: false,
		},
		{
			name: "if found instance state is PENDING_CREATE return StatusMessage",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:        gcpTestPostgresInstanceName,
							State:       "PENDING_CREATE",
							IpAddresses: []*sqladmin.IpMapping{{}},
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"name":"gcptestclustertestNsgcpcloudsql"}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "creation of " + gcpTestPostgresInstanceName + " cloudSQL instance in progress",
			wantErr: false,
		},
		{
			name: "error creating cloudSQL instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.CreateInstanceFn = func(ctx context.Context, s string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, errors.New("failed to create cloudSQL instance")
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"name":"gcptestclustertestNsgcpcloudsql"}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "failed to create cloudSQL instance",
			wantErr: true,
		},
		{
			name: "failure to add annotation when creating instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName + defaultCredSecSuffix,
						Namespace: testNs,
					},
						Data: map[string][]byte{
							defaultPostgresUserKey:     []byte(testUser),
							defaultPostgresPasswordKey: []byte(testPassword),
						},
					}, buildTestPostgres(), buildTestGcpInfrastructure(nil))
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return errors.New("failed to add annotation")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p:               buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "failed to add annotation",
			wantErr: true,
		},
		{
			name: "failure to add annotation when instance already exists",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName + defaultCredSecSuffix,
						Namespace: testNs,
					},
						Data: map[string][]byte{
							defaultPostgresUserKey:     []byte(testUser),
							defaultPostgresPasswordKey: []byte(testPassword),
						},
					}, buildTestPostgres(), buildTestGcpInfrastructure(nil))
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return errors.New("failed to add annotation")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: &v1alpha1.Postgres{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Type: "postgres",
						Tier: "development",
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  gcpTestPostgresInstanceName,
									State: "RUNNABLE",
								},
							},
						}, nil
					}
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:            gcpTestPostgresInstanceName,
							State:           "RUNNABLE",
							DatabaseVersion: defaultGCPCLoudSQLDatabaseVersion,
							Settings: &sqladmin.Settings{
								BackupConfiguration: &sqladmin.BackupConfiguration{
									BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
										RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
										RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
									},
								},
							},
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "failed to add annotation to postgres cr",
			wantErr: true,
		},
		{
			name: "error when modifying cloud sql instances",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:            gcpTestPostgresInstanceName,
							State:           "RUNNABLE",
							DatabaseVersion: defaultGCPCLoudSQLDatabaseVersion,
							Settings: &sqladmin.Settings{
								BackupConfiguration: &sqladmin.BackupConfiguration{
									BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
										RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
										RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
									},
								},
							},
						}, nil
					}
					sqlClient.ModifyInstanceFn = func(ctx context.Context, s string, s2 string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"settings":{"backupConfiguration":{"backupRetentionSettings":{}}}}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "failed to modify cloudsql instance: " + gcpTestPostgresInstanceName,
			wantErr: true,
		},
		{
			name: "success when modifying cloud sql instances",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte(testUser),
						defaultPostgresPasswordKey: []byte(testPassword),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							Name:            gcpTestPostgresInstanceName,
							State:           "RUNNABLE",
							DatabaseVersion: defaultGCPCLoudSQLDatabaseVersion,
							IpAddresses: []*sqladmin.IpMapping{
								{
									IpAddress: "",
								},
							},
							Settings: &sqladmin.Settings{
								BackupConfiguration: &sqladmin.BackupConfiguration{
									Enabled:                    defaultDeleteProtectionEnabled,
									PointInTimeRecoveryEnabled: defaultPointInTimeRecoveryEnabled,
									BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
										RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
										RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
									},
								},
								DeletionProtectionEnabled: defaultDeleteProtectionEnabled,
								StorageAutoResize:         utils.To(defaultStorageAutoResize),
								IpConfiguration: &sqladmin.IpConfiguration{
									Ipv4Enabled: defaultIPConfigIPV4Enabled,
								},
								MaintenanceWindow: &sqladmin.MaintenanceWindow{
									Day:  1,
									Hour: 10,
								},
								UserLabels: map[string]string{},
							},
						}, nil
					}
					sqlClient.ModifyInstanceFn = func(ctx context.Context, s string, s2 string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"instance":{"settings":{"deletionProtectionEnabled":false,"storageAutoResize":false,"userLabels":{"same-label":"same-value"},"ipConfiguration":{"ipv4Enabled":true},"maintenanceWindow":{"day": 7, "hour": 0},"backupConfiguration":{"enabled":false,"pointInTimeRecoveryEnabled":false,"backupRetentionSettings":{"retentionUnit":"RETENTION_UNIT_UNSPECIFIED","retainedBackups":20}}}}}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    "successfully reconciled cloudsql instance gcptestclustertestNsgcpcloudsql",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         resources.BuildMockConnectionTester(),
			}
			_, got1, err := pp.reconcileCloudSQLInstance(context.TODO(), tt.args.p, tt.args.sqladminService, tt.args.strategyConfig, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileCloudSQLInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got1 != tt.want {
				t.Errorf("reconcileCloudSQLInstance() got1 = %v, want %v", got1, tt.want)
			}
		})
	}
}

func TestPostgresProvider_reconcileCloudSqlInstanceSnapshots(t *testing.T) {
	now := time.Now()
	retentionDuration, err := str2duration.ParseDuration(gcpTestSnapshotRetention)
	if err != nil {
		t.Fatalf("failed to convert test retention time %s", gcpTestSnapshotRetention)
	}
	frequencyDuration, err := str2duration.ParseDuration(gcpTestSnapshotFrequency)
	if err != nil {
		t.Fatalf("failed to convert test frequency time %s", gcpTestSnapshotRetention)
	}
	// retention elapsed
	expiredTime := now.Add(-retentionDuration)
	// frequency elapsed
	elapsedTime := now.Add(-frequencyDuration)
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "error parsing retention time",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestInvalidSnapshotTime
					return postgres
				}(),
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to parse \"%s\" into go duration", gcpTestInvalidSnapshotTime)),
			wantErr: true,
		},
		{
			name: "error retrieving snapshots",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "failed to fetch all snapshots associated with postgres instance " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "success creating initial snapshot",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "created postgres snapshot CR for instance " + postgresProviderName,
			wantErr: false,
		},
		{
			name: "error creating initial snapshot",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "failed to create postgres snapshot for " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "latest snapshot creation in progress",
			fields: fields{
				Client: func() client.Client {
					snap := buildTestPostgresSnapshot()
					snap.Status.Phase = types.PhaseInProgress
					return moqClient.NewSigsClientMoqWithScheme(scheme, snap)
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "latest snapshot creation in progress for instance " + postgresProviderName,
			wantErr: false,
		},
		{
			name: "remove expired snapshot CR for GCP object",
			fields: fields{
				Client: func() client.Client {
					snap1 := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName,
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(now),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					snap2 := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName + "2",
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(expiredTime),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						snap1,
						snap2,
					)
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "successfully reconciled postgres instance " + postgresProviderName + " snapshots",
			wantErr: false,
		},
		{
			name: "error removing expired snapshot CR for GCP object",
			fields: fields{
				Client: func() client.Client {
					snap1 := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName,
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(now),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					snap2 := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName + "2",
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(expiredTime),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						snap1,
						snap2,
					)
					mc.DeleteFunc = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "failed to delete postgres snapshot " + gcpTestPostgresSnapshotName + "2",
			wantErr: true,
		},
		{
			name: "error parsing snapshot frequency",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestPostgresSnapshot()),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestSnapshotRetention
					postgres.Spec.SnapshotFrequency = gcpTestInvalidSnapshotTime
					return postgres
				}(),
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to parse \"%s\" into go duration", gcpTestInvalidSnapshotTime)),
			wantErr: true,
		},
		{
			name: "create new snapshot as frequency elapsed",
			fields: fields{
				Client: func() client.Client {
					snap := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName,
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(elapsedTime),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					return moqClient.NewSigsClientMoqWithScheme(scheme, snap)
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "successfully reconciled postgres instance " + postgresProviderName + " snapshots",
			wantErr: false,
		},
		{
			name: "error creating new snapshot after frequency elapsed",
			fields: fields{
				Client: func() client.Client {
					snap := &v1alpha1.PostgresSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:              gcpTestPostgresSnapshotName,
							Namespace:         testNs,
							CreationTimestamp: metav1.NewTime(elapsedTime),
						},
						Spec: v1alpha1.PostgresSnapshotSpec{
							ResourceName: postgresProviderName,
						},
						Status: types.ResourceTypeSnapshotStatus{
							Phase: types.PhaseComplete,
						},
					}
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, snap)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				p: buildTestPostgresWithSnapshot(),
			},
			want:    "failed to create postgres snapshot for " + postgresProviderName,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         resources.BuildMockConnectionTester(),
			}
			got, err := pp.reconcileCloudSqlInstanceSnapshots(context.TODO(), tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileCloudSqlInstanceSnapshots() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("reconcileCloudSqlInstanceSnapshots() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_ReconcilePostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		want          *providers.PostgresInstance
		statusMessage types.StatusMessage
		wantErr       bool
	}{
		{
			name: "failed to set finalizer",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return errors.New("failed to set finalizer")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				p: buildTestPostgres(),
			},
			want:          nil,
			statusMessage: "failed to set finalizer",
			wantErr:       true,
		},
		{
			name: "failed to retrieve postgres strategy config",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgres()),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, fmt.Errorf("generic error")
					},
				},
			},
			args: args{
				p: buildTestPostgres(),
			},
			want:          nil,
			statusMessage: "failed to retrieve postgres strategy config",
			wantErr:       true,
		},
		{
			name: "failed to reconcile gcp postgres provider credentials for postgres instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return nil, errors.New("generic error")
					},
				},
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage(`{"instance": {"Name": "gcptestclustertestNsgcpcloudsql"}}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
			},
			args: args{
				p: buildTestPostgres(),
			},
			want:          nil,
			statusMessage: "failed to reconcile gcp postgres provider credentials for postgres instance gcp-cloudsql",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         resources.BuildMockConnectionTester(),
			}
			got, statusMessage, err := pp.ReconcilePostgres(context.TODO(), tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcilePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcilePostgres() got = %v, want %v", got, tt.want)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("ReconcilePostgres() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestPostgresProvider_buildCloudSQLCreateStrategy(t *testing.T) {
	type fields struct {
		Client client.Client
	}
	type args struct {
		pg             *v1alpha1.Postgres
		strategyConfig *StrategyConfig
		sec            *corev1.Secret
		address        *computepb.Address
	}
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *gcpiface.DatabaseInstance
		wantErr bool
	}{
		{
			name: "success building default postgres tags and password",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				pg: &v1alpha1.Postgres{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Type: "postgres",
						Tier: "development",
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"Name":"gcptestclustertestNsgcpcloudsql","Settings": {"userLabels":{"integreatly-org_clusterid":"gcp-test-cluster","integreatly-org_resource-name":"testName","integreatly-org_resource-type":"","red-hat-managed":"true"}}}}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
				sec: &corev1.Secret{
					Data: map[string][]byte{
						defaultPostgresPasswordKey: []byte("secret"),
					},
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want: &gcpiface.DatabaseInstance{
				RootPassword: "secret",
				Settings: &gcpiface.Settings{
					UserLabels: map[string]string{
						"integreatly-org_clusterid":     gcpTestClusterName,
						"integreatly-org_resource-name": "gcp-cloudsql",
						"integreatly-org_resource-type": "postgres",
						"red-hat-managed":               "true",
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "fail to unmarshal gcp postgres create strategy",
			fields: fields{},
			args: args{
				pg: nil,
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: nil,
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to build postgres instance id from object",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				pg: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				address: buildValidGcpAddressRange(gcpTestIpRangeName),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresProvider{
				Client: tt.fields.Client,
			}
			got, err := p.buildCloudSQLCreateStrategy(context.TODO(), tt.args.pg, tt.args.strategyConfig, tt.args.sec, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCloudSQLCreateStrategy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && tt.want != nil {
				if got.RootPassword != tt.want.RootPassword || !reflect.DeepEqual(got.Settings.UserLabels, tt.want.Settings.UserLabels) {
					t.Errorf("buildCloudSQLCreateStrategy() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestPostgresProvider_buildCloudSQLDeleteStrategy(t *testing.T) {
	type fields struct {
		Client client.Client
	}
	type args struct {
		pg             *v1alpha1.Postgres
		strategyConfig *StrategyConfig
	}
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *sqladmin.DatabaseInstance
		wantErr bool
	}{
		{
			name: "success building delete instance request",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				pg: &v1alpha1.Postgres{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Type: "postgres",
						Tier: "development",
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: json.RawMessage(`{"instance":{}}`),
				},
			},
			want: &sqladmin.DatabaseInstance{
				Name: "gcptestclustertestNsgcpcloudsql",
			},
			wantErr: false,
		},
		{
			name: "failure building delete instance request",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.DeleteFunc = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
						return errors.New("failed to unmarshal gcp postgres create request")
					}
					return mc
				}(),
			},
			args: args{
				pg: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: json.RawMessage(`{"instance":{}}`),
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "failure building delete instance request - unmarshalling",
			fields: fields{},
			args: args{
				pg: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: nil,
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresProvider{
				Client: tt.fields.Client,
			}
			got, err := p.buildCloudSQLDeleteStrategy(context.TODO(), tt.args.pg, tt.args.strategyConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCloudSQLDeleteStrategy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildCloudSQLDeleteStrategy() got = %v, want %v", got, tt.want)
			}
		})
	}
}
