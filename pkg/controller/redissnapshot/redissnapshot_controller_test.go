package redissnapshot

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v12 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	croAws "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

var testLogger = logrus.WithFields(logrus.Fields{"testing": "true"})

type mockElasticacheClient struct {
	elasticacheiface.ElastiCacheAPI
	wantErrList   bool
	wantErrCreate bool
	wantErrDelete bool
	wantEmpty     bool
	repGroups     []*elasticache.ReplicationGroup
	rep           *elasticache.ReplicationGroup
	nodeSnapshot  *elasticache.Snapshot
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

func buildSnapshot() *elasticache.Snapshot {
	return &elasticache.Snapshot{
		SnapshotName: aws.String("test"),
	}
}

func buildAvailableReplicationGroup() *elasticache.ReplicationGroup {
	return &elasticache.ReplicationGroup{
		ReplicationGroupId: aws.String("test"),
		Status:             aws.String("available"),
		NodeGroups: []*elasticache.NodeGroup{
			{
				NodeGroupId: aws.String("test"),
				NodeGroupMembers: []*elasticache.NodeGroupMember{
					{
						CacheClusterId: aws.String("test"),
						CurrentRole:    aws.String("primary"),
					},
				},
			},
		},
	}
}

func buildReplicationGroups() []*elasticache.ReplicationGroup {
	var groups []*elasticache.ReplicationGroup
	groups = append(groups, buildAvailableReplicationGroup())
	return groups
}

func (m *mockElasticacheClient) DescribeSnapshots(*elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
	return &elasticache.DescribeSnapshotsOutput{}, nil
}

func (m *mockElasticacheClient) DescribeReplicationGroups(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	return &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: m.repGroups,
	}, nil
}

func (m *mockElasticacheClient) CreateSnapshot(*elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
	return &elasticache.CreateSnapshotOutput{}, nil
}

func TestReconcileRedisSnapshot_createSnapshot(t *testing.T) {
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
		cacheSvc elasticacheiface.ElastiCacheAPI
		snapshot *integreatlyv1alpha1.RedisSnapshot
		redis    *integreatlyv1alpha1.Redis
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusPhase
		want1   types.StatusMessage
		wantErr bool
	}{
		{
			name: "test successful snapshot started",
			args: args{
				ctx:      context.TODO(),
				cacheSvc: &mockElasticacheClient{repGroups: buildReplicationGroups()},
				snapshot: &integreatlyv1alpha1.RedisSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
				redis: &integreatlyv1alpha1.Redis{
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
				ConfigManager:     nil,
				CredentialManager: nil,
			},
			want:    types.PhaseInProgress,
			want1:   "snapshot started",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileRedisSnapshot{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				logger:            tt.fields.logger,
				ConfigManager:     tt.fields.ConfigManager,
				CredentialManager: tt.fields.CredentialManager,
			}
			got, got1, err := r.createSnapshot(tt.args.ctx, tt.args.cacheSvc, tt.args.snapshot, tt.args.redis)
			if (err != nil) != tt.wantErr {
				t.Errorf("createSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("createSnapshot() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("createSnapshot() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
