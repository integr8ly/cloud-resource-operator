package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"

	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildTestPostgres() *v1alpha1.Postgres {
	postgres := buildTestPostgresWithoutAnnotation()
	postgres.Annotations = map[string]string{
		ResourceIdentifierAnnotation: "testcloudsqldb-id",
	}
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
			"user":     []byte("postgres"),
			"password": []byte("test"),
		},
	}
}

func TestPostgresProvider_DeleteCloudSQLInstance(t *testing.T) {

	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		Logger            *logrus.Entry
	}
	type args struct {
		ctx             context.Context
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  "testcloudsqldb-id",
									State: "PENDING_DELETE",
								},
							},
						}, nil
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "postgres instance testcloudsqldb-id is already deleting",
			wantErr: false,
		},

		{
			name: "if instance is not nil, delete is not in progress delete function returns error",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:     "testcloudsqldb-id",
									State:    "RUNNABLE",
									Settings: &sqladmin.Settings{DeletionProtectionEnabled: false},
								},
							},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return nil, errors.New("failed to delete cloudSQL instance: testcloudsqldb-id")
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "failed to delete postgres instance: testcloudsqldb-id",
			wantErr: true,
		},
		{
			name: "error when getting cloud sql instances",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{},
							},
						}, fmt.Errorf("cannot retrieve sql instances from gcp")
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "cannot retrieve sql instances from gcp",
			wantErr: true,
		},
		{
			name: "failed to retrieve postgres strategy config",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestGcpInfrastructure(nil))
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						return fmt.Errorf("failed to retrieve postgres strategy config")
					}
					return mc
				}(),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:             context.TODO(),
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:             context.TODO(),
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:             context.TODO(),
				p:               buildTestPostgres(),
				networkManager:  buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(nil),
				isLastResource:  false,
				projectID:       gcpTestProjectId,
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "successful run of delete function when cloudsql object is not already deleted",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:     "testcloudsqldb-id",
									State:    "RUNNABLE",
									Settings: &sqladmin.Settings{DeletionProtectionEnabled: false},
								},
							},
						}, nil
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "delete detected, Instances.Delete() started",
			wantErr: false,
		},
		{
			name: "want error when running delete function when cloudsql object is not already deleted but delete errors",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:     "testcloudsqldb-id",
									State:    "RUNNABLE",
									Settings: &sqladmin.Settings{DeletionProtectionEnabled: false},
								},
							},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return nil, errors.New("delete error")
					}
				}),
				isLastResource: false,
				projectID:      gcpTestProjectId,
			},
			want:    "failed to delete postgres instance: testcloudsqldb-id",
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:            context.TODO(),
				p:              buildTestPostgres(),
				networkManager: buildMockNetworkManager(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:     "testcloudsqldb-id",
									State:    "RUNNABLE",
									Settings: &sqladmin.Settings{DeletionProtectionEnabled: true},
								},
							},
						}, nil
					}
					sqlClient.ModifyInstanceFn = func(ctx context.Context, s string, s2 string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, fmt.Errorf("failed to modify cloudsql instance")
					}
				}),
				isLastResource: false,
			},
			want:    "failed to modify cloudsql instance: testcloudsqldb-id",
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return &sqladmin.Operation{}, nil
					}
				}),
				isLastResource: true,
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return &sqladmin.Operation{}, nil
					}
				}),
				isLastResource: true,
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return &sqladmin.Operation{}, nil
					}
				}),
				isLastResource: true,
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return &sqladmin.Operation{}, nil
					}
				}),
				isLastResource: true,
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
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							Region:         defaultGCPCloudSQLRegion,
							ProjectID:      "project-id",
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
					sqlClient.DeleteInstanceFn = func(ctx context.Context, s string, s2 string) (*sqladmin.Operation, error) {
						return &sqladmin.Operation{}, nil
					}
				}),
				isLastResource: true,
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
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := pp.deleteCloudSQLInstance(tt.args.ctx, tt.args.networkManager, tt.args.sqladminService, tt.args.p, tt.args.isLastResource)
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
		ctx             context.Context
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
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					},
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "failed to get cluster id while exposing information metric for testcloudsqldb-id",
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
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
							Labels: map[string]string{
								"clusterID": "cluster",
							},
						},
						Status: types.ResourceTypeStatus{
							Phase: types.PhaseComplete,
						},
					},
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
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
			pp.setPostgresDeletionTimestampMetric(tt.args.ctx, tt.args.p)
		})
	}
}

func TestPostgresProvider_DeletePostgres(t *testing.T) {

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
		ctx             context.Context
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
			name: "failed to reconcile gcp postgres provider credentials for postgres instance",
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
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					},
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return nil, fmt.Errorf("failed to reconcile gcp postgres provider credentials for postgres instance")
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "failed to reconcile gcp postgres provider credentials for postgres instance gcp-cloudsql",
			wantErr: true,
		},
		{
			name: "error building cloudSQL admin service",
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
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					},
					&v1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Status: v1.InfrastructureStatus{
							InfrastructureName: "cluster",
						},
					},
				),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
						DeletionTimestamp: &metav1.Time{Time: now},
					},
				},
				sqladminService: gcpiface.GetMockSQLClient(nil),
			},
			want:    "error building cloudSQL admin service",
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
			}
			got, err := pp.DeletePostgres(tt.args.ctx, tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeletePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DeletePostgres() got = %v, want %v", got, tt.want)
			}
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
		ctx                  context.Context
		p                    *v1alpha1.Postgres
		sqladminService      gcpiface.SQLAdminService
		cloudSQLCreateConfig *sqladmin.DatabaseInstance
		strategyConfig       *StrategyConfig
		maintenanceWindow    bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "error when retrieving cloudSQL instances",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, errors.New("cannot retrieve sql instances from gcp")
					}
				}),
				cloudSQLCreateConfig: nil,
				strategyConfig:       &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow:    false,
			},
			want:    "cannot retrieve sql instances from gcp",
			wantErr: true,
		},
		{
			name: "error when retrieving cloudSQL secrets",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSecret(), buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  "testcloudsqldb-id",
									State: "RUNNABLE",
								},
							},
						}, nil
					}
				}),
				cloudSQLCreateConfig: nil,
				strategyConfig:       &StrategyConfig{ProjectID: "test-project-id"},
				maintenanceWindow:    false,
			},
			want:    "failed to retrieve cloudSQL credential secret",
			wantErr: true,
		},
		{
			name: "error when retrieving postgres password",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{},
						}, nil
					}
				}),
				cloudSQLCreateConfig: nil,
				strategyConfig:       &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow:    false,
			},
			want:    "unable to retrieve postgres password",
			wantErr: true,
		},
		{
			name: "success finding instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
					Data: map[string][]byte{
						defaultPostgresUserKey:     []byte("user"),
						defaultPostgresPasswordKey: []byte("password"),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  "testcloudsqldb-id",
									State: "RUNNABLE",
								},
							},
						}, nil
					}
				}),
				cloudSQLCreateConfig: &sqladmin.DatabaseInstance{
					Name:  "testcloudsqldb-id",
					State: "RUNNABLE",
				},
				strategyConfig:    &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow: false,
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
						defaultPostgresUserKey:     []byte("user"),
						defaultPostgresPasswordKey: []byte("password"),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  "testcloudsqldb-id",
									State: "PENDING_CREATE",
								},
							},
						}, nil
					}
				}),
				cloudSQLCreateConfig: &sqladmin.DatabaseInstance{
					Name:  "testcloudsqldb-id",
					State: "RUNNABLE",
				},
				strategyConfig:    &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow: false,
			},
			want:    "creation of testcloudsqldb-id cloudSQL instance in progress",
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
						defaultPostgresUserKey:     []byte("user"),
						defaultPostgresPasswordKey: []byte("password"),
					},
				}, buildTestPostgres(), buildTestGcpInfrastructure(nil)),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.CreateInstanceFn = func(ctx context.Context, s string, instance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
						return nil, errors.New("failed to create cloudSQL instance")
					}
				}),
				cloudSQLCreateConfig: &sqladmin.DatabaseInstance{
					Name:  "testcloudsqldb-id",
					State: "RUNNABLE",
				},
				strategyConfig:    &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow: false,
			},
			want:    "failed to create cloudSQL instance",
			wantErr: true,
		},
		{
			name: "failed to add annotation to postgres CR",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName + defaultCredSecSuffix,
						Namespace: testNs,
					},
						Data: map[string][]byte{
							defaultPostgresUserKey:     []byte("user"),
							defaultPostgresPasswordKey: []byte("password"),
						},
					}, buildTestGcpInfrastructure(nil))
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("failed to add annotation")
					}
					return mc
				}(),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     nil,
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
				sqladminService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.InstancesListFn = func(s string) (*sqladmin.InstancesListResponse, error) {
						return &sqladmin.InstancesListResponse{
							Items: []*sqladmin.DatabaseInstance{
								{
									Name:  "testcloudsqldb-id",
									State: "RUNNABLE",
								},
							},
						}, nil
					}
				}),
				cloudSQLCreateConfig: &sqladmin.DatabaseInstance{},
				strategyConfig:       &StrategyConfig{ProjectID: "sample-project-id"},
				maintenanceWindow:    false,
			},
			want:    "failed to add annotation",
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
			}
			_, got1, err := pp.reconcileCloudSQLInstance(tt.args.ctx, tt.args.p, tt.args.sqladminService, tt.args.cloudSQLCreateConfig, tt.args.strategyConfig, tt.args.maintenanceWindow)
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

func Test_getFoundInstance(t *testing.T) {
	type args struct {
		instances    []*sqladmin.DatabaseInstance
		instanceName string
	}
	tests := []struct {
		name string
		args args
		want *sqladmin.DatabaseInstance
	}{
		{
			name: "successfully found instance",
			args: args{
				instances: []*sqladmin.DatabaseInstance{
					{
						Name:  "testcloudsqldb-id",
						State: "RUNNABLE",
					},
				},
				instanceName: "testcloudsqldb-id",
			},
			want: &sqladmin.DatabaseInstance{
				Name:  "testcloudsqldb-id",
				State: "RUNNABLE",
			},
		},
		{
			name: "no instance found",
			args: args{
				instances:    nil,
				instanceName: "",
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFoundInstance(tt.args.instances, tt.args.instanceName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFoundInstance() = %v, want %v", got, tt.want)
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
		ctx context.Context
		p   *v1alpha1.Postgres
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
				ctx: context.TODO(),
				p:   buildTestPostgres(),
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
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"test\": \"test\" }"),
							DeleteStrategy: json.RawMessage("{ \"test\": \"test\" }"),
						}, nil
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
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
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				p:   buildTestPostgres(),
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
			}
			got, statusMessage, err := pp.ReconcilePostgres(tt.args.ctx, tt.args.p)
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

func TestPostgresProvider_buildPostgresConfig(t *testing.T) {
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
		p              *v1alpha1.Postgres
		strategyConfig *StrategyConfig
	}
	tests := []struct {
		name                  string
		fields                fields
		args                  args
		createInstanceRequest *sqladmin.DatabaseInstance
		deleteInstanceRequest *sqladmin.DatabaseInstance
		wantErr               bool
	}{
		{
			name: "success building create instance request",
			fields: fields{
				Client: nil,
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"name": "testcloudsqldb-id", "region": "us-central1"}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
			},
			createInstanceRequest: &sqladmin.DatabaseInstance{
				Name:   "testcloudsqldb-id",
				Region: defaultGCPCloudSQLRegion,
			},
			deleteInstanceRequest: &sqladmin.DatabaseInstance{},
			wantErr:               false,
		},
		{
			name: "failure building create instance request",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return errors.New("failed to unmarshal gcp postgres create request")
					}
					return mc
				}(),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: nil,
					DeleteStrategy: nil,
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			wantErr:               true,
		},
		{
			name: "failed to find postgres name from annotation - for create request",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						return errors.New("generic error")
					}
					return mc
				}(),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{"name": "", "region": "us-central1"}`),
					DeleteStrategy: json.RawMessage(`{}`),
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			wantErr:               true,
		},
		{
			name: "failed to find postgres name from annotation - for delete request",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						return errors.New("generic error")
					}
					return mc
				}(),
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: json.RawMessage(`{"name": "", "region": "us-central1"}`),
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			wantErr:               true,
		},
		{
			name: "success building delete instance request",
			fields: fields{
				Client: nil,
			},
			args: args{
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: json.RawMessage(`{}`),
					DeleteStrategy: json.RawMessage(`{"name": "testcloudsqldb-id", "region": "us-central1"}`),
				},
			},
			createInstanceRequest: &sqladmin.DatabaseInstance{},
			deleteInstanceRequest: &sqladmin.DatabaseInstance{
				Name:   "testcloudsqldb-id",
				Region: defaultGCPCloudSQLRegion,
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
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         defaultGCPCloudSQLRegion,
					ProjectID:      "sample-project-id",
					CreateStrategy: nil,
					DeleteStrategy: nil,
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			wantErr:               true,
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
			got, got1, err := pp.buildPostgresConfig(tt.args.p, tt.args.strategyConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildPostgresConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.createInstanceRequest) {
				t.Errorf("buildPostgresConfig() got = %v, want %v", got, tt.createInstanceRequest)
			}
			if !reflect.DeepEqual(got1, tt.deleteInstanceRequest) {
				t.Errorf("buildPostgresConfig() got1 = %v, want %v", got1, tt.deleteInstanceRequest)
			}
		})
	}
}
