package aws

import (
	"context"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"testing"
)

var (
	testLogger  = logrus.WithFields(logrus.Fields{"testing": "true"})
	testAddress = aws.String("redis")
	testPort    = aws.Int64(6397)
)

type mockElasticacheClient struct {
	elasticacheiface.ElastiCacheAPI
	wantErrList       bool
	wantErrCreate     bool
	wantErrDelete     bool
	wantEmpty         bool
	replicationGroups []*elasticache.ReplicationGroup
}

// mock elasticache DescribeReplicationGroups output
func (m *mockElasticacheClient) DescribeReplicationGroups(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	if m.wantEmpty {
		return &elasticache.DescribeReplicationGroupsOutput{}, nil
	}
	return &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: m.replicationGroups,
	}, nil
}

// mock elasticache CreateReplicationGroup output
func (m *mockElasticacheClient) CreateReplicationGroup(*elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
	return &elasticache.CreateReplicationGroupOutput{}, nil
}

// mock elasticache DeleteReplicationGroup output
func (m *mockElasticacheClient) DeleteReplicationGroup(*elasticache.DeleteReplicationGroupInput) (*elasticache.DeleteReplicationGroupOutput, error) {
	return &elasticache.DeleteReplicationGroupOutput{}, nil
}

func buildTestRedisCR() *v1alpha1.Redis {
	return &v1alpha1.Redis{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
	}
}

func buildReplicationGroup() []*elasticache.ReplicationGroup {
	return []*elasticache.ReplicationGroup{
		{
			ReplicationGroupId: aws.String("test-id"),
			Status:             aws.String("pending"),
		},
	}
}

func buildReplicationGroupReady() []*elasticache.ReplicationGroup {
	return []*elasticache.ReplicationGroup{
		{
			ReplicationGroupId: aws.String("test-id"),
			Status:             aws.String("available"),
			NodeGroups: []*elasticache.NodeGroup{
				{
					NodeGroupId:      aws.String("primary-node"),
					NodeGroupMembers: nil,
					PrimaryEndpoint: &elasticache.Endpoint{
						Address: testAddress,
						Port:    testPort,
					},
				},
			},
		},
	}
}

func buildTestRedisCluster() *providers.RedisCluster {
	return &providers.RedisCluster{DeploymentDetails: &providers.RedisDeploymentDetails{
		URI:  *testAddress,
		Port: *testPort,
	}}
}

func Test_createRedisCluster(t *testing.T) {
	type args struct {
		cacheSvc    elasticacheiface.ElastiCacheAPI
		redisConfig *elasticache.CreateReplicationGroupInput
	}
	tests := []struct {
		name    string
		args    args
		want    *providers.RedisCluster
		wantErr bool
	}{
		{
			name: "test redis is created",
			args: args{
				cacheSvc:    &mockElasticacheClient{replicationGroups: []*elasticache.ReplicationGroup{}},
				redisConfig: &elasticache.CreateReplicationGroupInput{},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test redis already exists",
			args: args{
				cacheSvc:    &mockElasticacheClient{replicationGroups: buildReplicationGroup()},
				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test redis already exists and is ready",
			args: args{
				cacheSvc:    &mockElasticacheClient{replicationGroups: buildReplicationGroupReady()},
				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := createRedisCluster(tt.args.cacheSvc, tt.args.redisConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRedisCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createRedisCluster() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSRedisProvider_deleteRedisCluster(t *testing.T) {
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
		CacheSvc          elasticacheiface.ElastiCacheAPI
	}
	type args struct {
		cacheSvc    elasticacheiface.ElastiCacheAPI
		redisConfig *elasticache.CreateReplicationGroupInput
		ctx         context.Context
		redis       *v1alpha1.Redis
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test successful delete with no redis",
			args: args{
				redisConfig: &elasticache.CreateReplicationGroupInput{},
				redis:       buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc:          &mockElasticacheClient{replicationGroups: []*elasticache.ReplicationGroup{}},
			},
			wantErr: false,
		},
		{
			name: "test successful delete with existing unavailable redis",
			args: args{

				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:       buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc:          &mockElasticacheClient{replicationGroups: buildReplicationGroup()},
			},
			wantErr: false,
		},
		{
			name: "test successful delete with existing available redis",
			args: args{

				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:       buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc:          &mockElasticacheClient{replicationGroups: buildReplicationGroupReady()},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSRedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				CacheSvc:          tt.fields.CacheSvc,
			}
			if _, err := p.deleteRedisCluster(tt.fields.CacheSvc, tt.args.redisConfig, tt.args.ctx, tt.args.redis); (err != nil) != tt.wantErr {
				t.Errorf("deleteRedisCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
