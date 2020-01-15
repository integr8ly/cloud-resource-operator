package postgressnapshot

import (
	"context"
	"testing"

	v12 "github.com/openshift/api/config/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"

	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	croAws "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testLogger = logrus.WithFields(logrus.Fields{"testing": "true"})

type mockRdsClient struct {
	rdsiface.RDSAPI
	wantErrList   bool
	wantErrCreate bool
	wantErrDelete bool
	dbSnapshots   []*rds.DBSnapshot
	dbSnapshot    *rds.DBSnapshot
}

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestInfrastructure() *v12.Infrastructure {
	return &v12.Infrastructure{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Status: v12.InfrastructureStatus{
			InfrastructureName: "test",
		},
	}
}

func (m *mockRdsClient) DescribeDBSnapshots(*rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
	return &rds.DescribeDBSnapshotsOutput{
		DBSnapshots: m.dbSnapshots,
	}, nil
}

func (m *mockRdsClient) CreateDBSnapshot(*rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
	return &rds.CreateDBSnapshotOutput{
		DBSnapshot: m.dbSnapshot,
	}, nil
}

func TestReconcilePostgresSnapshot_createSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		client            client.Client
		scheme            *runtime.Scheme
		logger            *logrus.Entry
		ConfigManager     croAws.ConfigManager
		CredentialManager croAws.CredentialManager
	}
	type args struct {
		ctx      context.Context
		rdsSvc   rdsiface.RDSAPI
		snapshot *integreatlyv1alpha1.PostgresSnapshot
		postgres *integreatlyv1alpha1.Postgres
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusPhase
		wantErr bool
	}{
		{
			name: "test successful snapshot create",
			args: args{
				ctx: context.TODO(),
				rdsSvc: &mockRdsClient{dbSnapshot: &rds.DBSnapshot{
					DBInstanceIdentifier: aws.String("rds-db"),
				}},
				snapshot: &integreatlyv1alpha1.PostgresSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
				postgres: &integreatlyv1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			fields: fields{
				client:            fake.NewFakeClientWithScheme(scheme, buildTestInfrastructure()),
				scheme:            scheme,
				logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: types.PhaseInProgress,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcilePostgresSnapshot{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				logger:            tt.fields.logger,
				ConfigManager:     tt.fields.ConfigManager,
				CredentialManager: tt.fields.CredentialManager,
			}
			got, _, err := r.createSnapshot(tt.args.ctx, tt.args.rdsSvc, tt.args.snapshot, tt.args.postgres)
			if (err != nil) != tt.wantErr {
				t.Errorf("createSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("createSnapshot() got = %v, want %v", got, tt.want)
			}
		})
	}
}
