package gcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	"github.com/sirupsen/logrus"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gcpTestServiceAccountEmail  = "test.user@redhat.com"
	gcpTestPostgresSnapshotName = "example-postgressnapshot"
)

func buildTestPostgresSnapshot() *v1alpha1.PostgresSnapshot {
	return &v1alpha1.PostgresSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            gcpTestPostgresSnapshotName,
			Namespace:       testNs,
			ResourceVersion: "1000",
		},
		Spec: v1alpha1.PostgresSnapshotSpec{
			ResourceName: postgresProviderName,
		},
		Status: croType.ResourceTypeSnapshotStatus{
			Phase: croType.PhaseComplete,
		},
	}
}

func buildTestLatestPostgresSnapshot(name string) *v1alpha1.PostgresSnapshot {
	snap := buildTestPostgresSnapshot()
	snap.Spec.SkipDelete = true
	if name != "" {
		snap.ObjectMeta.Name = name
	}
	return snap
}

func TestPostgresProvider_reconcilePostgresSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            k8sclient.Client
		CredentialManager CredentialManager
		Logger            *logrus.Entry
	}
	type args struct {
		snap           *v1alpha1.PostgresSnapshot
		p              *v1alpha1.Postgres
		strategyConfig *StrategyConfig
		storageService *gcpiface.MockStorageClient
		sqlService     *gcpiface.MockSqlClient
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.PostgresSnapshotInstance
		status  croType.StatusMessage
		wantErr bool
	}{
		{
			name: "error resource identifier annotation missing",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresWithoutAnnotation(),
			},
			want:    nil,
			status:  "failed to find " + ResourceIdentifierAnnotation + " annotation for postgres cr " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "error updating snapshot status",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgres(),
			},
			want:    nil,
			status:  "failed to update snapshot " + gcpTestPostgresSnapshotName + " in namespace " + testNs,
			wantErr: true,
		},
		{
			name: "error retrieving object metadata for snapshot",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgres(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return nil, errors.New("generic error")
					}
				}),
			},
			want:    nil,
			status:  croType.StatusMessage(fmt.Sprintf("failed to retrieve object metadata for bucket %s and object %s", testName, gcpTestPostgresSnapshotName)),
			wantErr: true,
		},
		{
			name: "success creating postgres snapshot",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return nil, storage.ErrObjectNotExist
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    nil,
			status:  "snapshot creation started for " + gcpTestPostgresSnapshotName,
			wantErr: false,
		},
		{
			name: "error parsing postgres snapshot retention time",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestInvalidSnapshotTime
					postgres.Spec.SnapshotFrequency = gcpTestSnapshotFrequency
					return postgres
				}(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return &storage.ObjectAttrs{
							Name:   testName,
							Bucket: testName,
						}, nil
					}
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return []*storage.ObjectAttrs{
							{
								Name:   testName,
								Bucket: testName,
							},
						}, nil
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    nil,
			status:  croType.StatusMessage(fmt.Sprintf("failed to parse \"%s\" into go duration", gcpTestInvalidSnapshotTime)),
			wantErr: true,
		},
		{
			name: "error retrieving object lifecycle for bucket",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestSnapshotRetention
					postgres.Spec.SnapshotFrequency = gcpTestSnapshotFrequency
					return postgres
				}(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return &storage.ObjectAttrs{
							Name:   testName,
							Bucket: testName,
						}, nil
					}
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return []*storage.ObjectAttrs{
							{
								Name:   testName,
								Bucket: testName,
							},
						}, nil
					}
					storageClient.HasBucketLifecycleFn = func(ctx context.Context, bucket string, days int64) (bool, error) {
						return false, errors.New("generic error")
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    nil,
			status:  "failed to check object lifecycle for bucket " + testName,
			wantErr: true,
		},
		{
			name: "error setting object lifecycle for bucket",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestSnapshotRetention
					postgres.Spec.SnapshotFrequency = gcpTestSnapshotFrequency
					return postgres
				}(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return &storage.ObjectAttrs{
							Name:   testName,
							Bucket: testName,
						}, nil
					}
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return []*storage.ObjectAttrs{
							{
								Name:   testName,
								Bucket: testName,
							},
						}, nil
					}
					storageClient.SetBucketLifecycleFn = func(ctx context.Context, bucket string, days int64) error {
						return errors.New("generic error")
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    nil,
			status:  "failed to set object lifecycle for bucket " + testName,
			wantErr: true,
		},
		{
			name: "success reconciling postgres snapshot",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p: func() *v1alpha1.Postgres {
					postgres := buildTestPostgres()
					postgres.Spec.SnapshotRetention = gcpTestSnapshotRetention
					postgres.Spec.SnapshotFrequency = gcpTestSnapshotFrequency
					return postgres
				}(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetObjectMetadataFn = func(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
						return &storage.ObjectAttrs{
							Name:   testName,
							Bucket: testName,
						}, nil
					}
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return []*storage.ObjectAttrs{
							{
								Name:   testName,
								Bucket: testName,
							},
						}, nil
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want: &providers.PostgresSnapshotInstance{
				Name: testName,
			},
			status:  "snapshot " + gcpTestPostgresSnapshotName + " successfully reconciled",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PostgresSnapshotProvider{
				client: tt.fields.Client,
				logger: tt.fields.Logger,
			}
			got, status, err := p.reconcilePostgresSnapshot(context.TODO(), tt.args.snap, tt.args.p, tt.args.strategyConfig, tt.args.storageService, tt.args.sqlService)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcilePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reconcilePostgresSnapshot() PostgresSnapshotInstance = %v, want %v", got, tt.want)
				return
			}
			if status != tt.status {
				t.Errorf("reconcilePostgresSnapshot() statusMessage = %v, want %v", status, tt.status)
			}
		})
	}
}

func TestPostgresProvider_createPostgresSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            k8sclient.Client
		CredentialManager CredentialManager
		Logger            *logrus.Entry
	}
	type args struct {
		snap           *v1alpha1.PostgresSnapshot
		p              *v1alpha1.Postgres
		strategyConfig *StrategyConfig
		storageService *gcpiface.MockStorageClient
		sqlService     *gcpiface.MockSqlClient
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "error waiting for postgres snapshot creation",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseInProgress),
			},
			want:    croType.StatusMessage("waiting for postgres instance " + testName + " to be complete, status " + croType.PhaseInProgress),
			wantErr: true,
		},
		{
			name: "error getting bucket for snapshots",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.GetBucketFn = func(ctx context.Context, bucket string) (*storage.BucketAttrs, error) {
						return nil, errors.New("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to retrieve bucket metadata for bucket " + testName,
			wantErr: true,
		},
		{
			name: "error creating bucket for snapshots",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.CreateBucketFn = func(ctx context.Context, bucket, projectID string, attrs *storage.BucketAttrs) error {
						return errors.New("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to create bucket with name " + testName,
			wantErr: true,
		},
		{
			name: "error retrieving gcp postgres instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap:           buildTestPostgresSnapshot(),
				p:              buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(nil),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return nil, errors.New("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to find postgres instance with name " + testName,
			wantErr: true,
		},
		{
			name: "error retrieving bucket policy",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.HasBucketPolicyFn = func(ctx context.Context, bucket, identity, role string) (bool, error) {
						return false, errors.New("generic error")
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to check bucket policy for " + testName,
			wantErr: true,
		},
		{
			name: "error setting gcp bucket policy",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				p:    buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.SetBucketPolicyFn = func(ctx context.Context, bucket, identity, role string) error {
						return errors.New("generic error")
					}
				}),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to set policy on bucket " + testName,
			wantErr: true,
		},
		{
			name: "error exporting postgres database",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap:           buildTestPostgresSnapshot(),
				p:              buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(nil),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
					sqlClient.ExportDatabaseFn = func(ctx context.Context, projectID, instanceName string, req *sqladmin.InstancesExportRequest) (*sqladmin.Operation, error) {
						return nil, errors.New("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "failed to export database from postgres instance " + testName,
			wantErr: true,
		},
		{
			name: "success creating snapshot of postgres instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap:           buildTestPostgresSnapshot(),
				p:              buildTestPostgresPhase(croType.PhaseComplete),
				storageService: gcpiface.GetMockStorageClient(nil),
				sqlService: gcpiface.GetMockSQLClient(func(sqlClient *gcpiface.MockSqlClient) {
					sqlClient.GetInstanceFn = func(ctx context.Context, projectID, instanceName string) (*sqladmin.DatabaseInstance, error) {
						return &sqladmin.DatabaseInstance{
							ServiceAccountEmailAddress: gcpTestServiceAccountEmail,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
				},
			},
			want:    "snapshot creation started for " + gcpTestPostgresSnapshotName,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PostgresSnapshotProvider{
				client: tt.fields.Client,
				logger: tt.fields.Logger,
			}
			got, err := p.createPostgresSnapshot(context.TODO(), tt.args.snap, tt.args.p, tt.args.strategyConfig, tt.args.storageService, tt.args.sqlService)
			if (err != nil) != tt.wantErr {
				t.Errorf("createPostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("createPostgresSnapshot() statusMessage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_reconcileSkipDelete(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            k8sclient.Client
		CredentialManager CredentialManager
		Logger            *logrus.Entry
	}
	type args struct {
		snap *v1alpha1.PostgresSnapshot
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "error determining latest snapshot",
			fields: fields{
				Client: func() k8sclient.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot())
					mc.ListFunc = func(ctx context.Context, list k8sclient.ObjectList, opts ...k8sclient.ListOption) error {
						return errors.New("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
			},
			want:    "failed to determine latest snapshot for " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "success no complete snapshots found",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
			},
			want:    "no complete snapshots found for " + postgresProviderName,
			wantErr: false,
		},
		{
			name: "error updating skipDelete on latest snapshot",
			fields: fields{
				Client: func() k8sclient.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot())
					mc.UpdateFunc = func(ctx context.Context, obj k8sclient.Object, opts ...k8sclient.UpdateOption) error {
						return errors.New("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
			},
			want:    "failed to update postgres snapshot " + gcpTestPostgresSnapshotName,
			wantErr: true,
		},
		{
			name: "success reconciling snapshot labels",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
			},
			want:    "snapshot " + gcpTestPostgresSnapshotName + " successfully reconciled",
			wantErr: false,
		},
		{
			name: "success reconciling snapshot removing existing latest",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresSnapshot(), buildTestLatestPostgresSnapshot(gcpTestPostgresSnapshotName+"2")),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
			},
			want:    "snapshot " + gcpTestPostgresSnapshotName + " successfully reconciled",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PostgresSnapshotProvider{
				client: tt.fields.Client,
				logger: tt.fields.Logger,
			}
			got, err := p.reconcileSkipDelete(context.TODO(), tt.args.snap)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcilePostgresSnapshotLabels() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("reconcilePostgresSnapshotLabels() statusMessage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_deletePostgresSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            k8sclient.Client
		CredentialManager CredentialManager
		Logger            *logrus.Entry
	}
	type args struct {
		snap           *v1alpha1.PostgresSnapshot
		pg             *v1alpha1.Postgres
		storageService *gcpiface.MockStorageClient
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "error removing finalizer from snapshot",
			fields: fields{
				Client: func() k8sclient.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestLatestPostgresSnapshot(gcpTestPostgresSnapshotName))
					mc.UpdateFunc = func(ctx context.Context, obj k8sclient.Object, opts ...k8sclient.UpdateOption) error {
						return errors.New("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestLatestPostgresSnapshot(gcpTestPostgresSnapshotName),
			},
			want:    "failed to update snapshot as part of finalizer reconcile",
			wantErr: true,
		},
		{
			name: "error deleting gcp snapshot object",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.DeleteObjectFn = func(ctx context.Context, bucket, object string) error {
						return errors.New("generic error")
					}
				}),
				pg: buildTestPostgres(),
			},
			want:    "failed to delete snapshot " + gcpTestPostgresSnapshotName + " from bucket " + testName,
			wantErr: true,
		},
		{
			name: "error listing gcp snapshot objects",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return nil, errors.New("generic error")
					}
				}),
				pg: buildTestPostgres(),
			},
			want:    "failed to list objects from bucket " + testName,
			wantErr: true,
		},
		{
			name: "success deletion in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.ListObjectsFn = func(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
						return []*storage.ObjectAttrs{
							{
								Name: gcpTestPostgresSnapshotName,
							},
						}, nil
					}
				}),
				pg: buildTestPostgres(),
			},
			want:    "object " + gcpTestPostgresSnapshotName + " deletion in progress",
			wantErr: false,
		},
		{
			name: "error deleting snapshot bucket",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap: buildTestPostgresSnapshot(),
				storageService: gcpiface.GetMockStorageClient(func(storageClient *gcpiface.MockStorageClient) {
					storageClient.DeleteBucketFn = func(ctx context.Context, bucket string) error {
						return errors.New("generic error")
					}
				}),
				pg: buildTestPostgres(),
			},
			want:    "failed to delete bucket " + testName,
			wantErr: true,
		},
		{
			name: "success removing snapshot resources",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestPostgresSnapshot()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				snap:           buildTestPostgresSnapshot(),
				storageService: gcpiface.GetMockStorageClient(nil),
				pg:             buildTestPostgres(),
			},
			want:    "snapshot " + gcpTestPostgresSnapshotName + " deleted",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PostgresSnapshotProvider{
				client: tt.fields.Client,
				logger: tt.fields.Logger,
			}
			got, err := p.deletePostgresSnapshot(context.TODO(), tt.args.snap, tt.args.pg, tt.args.storageService)
			if (err != nil) != tt.wantErr {
				t.Errorf("deletePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("deletePostgresSnapshot() statusMessage = %v, want %v", got, tt.want)
			}
		})
	}
}
