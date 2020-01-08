package aws

import (
	"context"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"

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

type mockStsClient struct {
	stsiface.STSAPI
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

// mock elasticache ModifyReplicationGroup output
func (m *mockElasticacheClient) ModifyReplicationGroup(*elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
	return &elasticache.ModifyReplicationGroupOutput{}, nil
}

// mock elasticache AddTagsToResource output
func (m *mockElasticacheClient) AddTagsToResource(*elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
	return &elasticache.TagListMessage{}, nil
}

// mock elasticache DescribeSnapshots
func (m *mockElasticacheClient) DescribeSnapshots(*elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
	return &elasticache.DescribeSnapshotsOutput{}, nil
}

// mock sts get caller identity
func (m *mockStsClient) GetCallerIdentity(*sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String("test"),
	}, nil
}

func buildTestRedisCR() *v1alpha1.Redis {
	return &v1alpha1.Redis{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
	}
}

func buildReplicationGroupPending() []*elasticache.ReplicationGroup {
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
			ReplicationGroupId:     aws.String("test-id"),
			Status:                 aws.String("available"),
			CacheNodeType:          aws.String("test"),
			SnapshotRetentionLimit: aws.Int64(20),
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
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx         context.Context
		r           *v1alpha1.Redis
		stsSvc      stsiface.STSAPI
		cacheSvc    elasticacheiface.ElastiCacheAPI
		redisConfig *elasticache.CreateReplicationGroupInput
		stratCfg    *StrategyConfig
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	tests := []struct {
		name    string
		args    args
		fields  fields
		want    *providers.RedisCluster
		wantErr bool
	}{
		{
			name: "test elasticache buildReplicationGroupPending is called",
			args: args{
				ctx:         context.TODO(),
				cacheSvc:    &mockElasticacheClient{replicationGroups: []*elasticache.ReplicationGroup{}},
				r:           buildTestRedisCR(),
				stsSvc:      &mockStsClient{},
				redisConfig: &elasticache.CreateReplicationGroupInput{},
				stratCfg:    &StrategyConfig{Region: "test"},
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is not available",
			args: args{
				ctx:         context.TODO(),
				cacheSvc:    &mockElasticacheClient{replicationGroups: buildReplicationGroupPending()},
				r:           buildTestRedisCR(),
				stsSvc:      &mockStsClient{},
				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:    &StrategyConfig{Region: "test"},
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and needs to be modified",
			args: args{
				ctx:         context.TODO(),
				cacheSvc:    &mockElasticacheClient{replicationGroups: buildReplicationGroupReady()},
				r:           buildTestRedisCR(),
				stsSvc:      &mockStsClient{},
				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:    &StrategyConfig{Region: "test"},
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and does not need to be modified",
			args: args{
				ctx:      context.TODO(),
				cacheSvc: &mockElasticacheClient{replicationGroups: buildReplicationGroupReady()},
				r:        buildTestRedisCR(),
				stsSvc:   &mockStsClient{},
				redisConfig: &elasticache.CreateReplicationGroupInput{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("test"),
					SnapshotRetentionLimit: aws.Int64(20),
				},
				stratCfg: &StrategyConfig{Region: "test"},
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
			},
			want:    buildTestRedisCluster(),
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
			}
			got, _, err := p.createElasticacheCluster(tt.args.ctx, tt.args.r, tt.args.cacheSvc, tt.args.stsSvc, tt.args.redisConfig, tt.args.stratCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("createElasticacheCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createElasticacheCluster() got = %v, want %v", got, tt.want)
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
		cacheSvc          elasticacheiface.ElastiCacheAPI
		redisCreateConfig *elasticache.CreateReplicationGroupInput
		redisDeleteConfig *elasticache.DeleteReplicationGroupInput
		ctx               context.Context
		redis             *v1alpha1.Redis
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
				redisCreateConfig: &elasticache.CreateReplicationGroupInput{},
				redisDeleteConfig: &elasticache.DeleteReplicationGroupInput{},
				redis:             buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
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

				redisCreateConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig: &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:             buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc:          &mockElasticacheClient{replicationGroups: buildReplicationGroupPending()},
			},
			wantErr: false,
		},
		{
			name: "test successful delete with existing available redis",
			args: args{

				redisCreateConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig: &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:             buildTestRedisCR(),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
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
			if _, err := p.deleteElasticacheCluster(tt.fields.CacheSvc, tt.args.redisCreateConfig, tt.args.redisDeleteConfig, tt.args.ctx, tt.args.redis); (err != nil) != tt.wantErr {
				t.Errorf("deleteElasticacheCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAWSRedisProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		r *v1alpha1.Redis
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "test short reconcile when the cr is not complete",
			args: args{
				r: &v1alpha1.Redis{
					Status: v1alpha1.RedisStatus{
						Phase: types.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				r: &v1alpha1.Redis{
					Status: v1alpha1.RedisStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSRedisProvider{}
			if got := p.GetReconcileTime(tt.args.r); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSRedisProvider_TagElasticache(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		CacheSvc          elasticacheiface.ElastiCacheAPI
	}
	type args struct {
		ctx      context.Context
		cacheSvc elasticacheiface.ElastiCacheAPI
		stsSvc   stsiface.STSAPI
		r        *v1alpha1.Redis
		stratCfg StrategyConfig
		cache    *elasticache.NodeGroupMember
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "test tags reconcile completes successfully",
			args: args{
				ctx:      context.TODO(),
				r:        buildTestRedisCR(),
				cacheSvc: &mockElasticacheClient{replicationGroups: buildReplicationGroupReady()},
				stsSvc:   &mockStsClient{},
				stratCfg: StrategyConfig{Region: "test"},
				cache: &elasticache.NodeGroupMember{
					CacheClusterId:            aws.String("test"),
					CacheNodeId:               aws.String("test"),
					PreferredAvailabilityZone: aws.String("test"),
				},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
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
			got, err := p.TagElasticacheNode(tt.args.ctx, tt.args.cacheSvc, tt.args.stsSvc, tt.args.r, tt.args.stratCfg, tt.args.cache)
			if (err != nil) != tt.wantErr {
				t.Errorf("TagElasticache() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TagElasticache() got = %v, want %v", got, tt.want)
			}
		})
	}
}
