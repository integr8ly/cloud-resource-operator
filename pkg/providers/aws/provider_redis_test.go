package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/mock"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	croApis "github.com/integr8ly/cloud-resource-operator/api"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	cloudCredentialApis "github.com/openshift/cloud-credential-operator/pkg/apis"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"testing"
)

var (
	testLogger   = logrus.WithFields(logrus.Fields{"testing": "true"})
	testAddress  = aws.String("redis")
	testPort     = aws.Int32(6397)
	snapshotName = "test-snapshot"
)

type mockStsClient struct {
	mock.Mock
	sts.Client
}

type mockSnapshotNotFoundError struct{}

func (e *mockSnapshotNotFoundError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrorCode(), e.ErrorMessage())
}

func (e *mockSnapshotNotFoundError) ErrorCode() string {
	return "SnapshotNotFoundFault"
}

func (e *mockSnapshotNotFoundError) ErrorMessage() string {
	return "Snapshot not found"
}

func buildCacheClusterList(modifyFn func([]elasticachetypes.CacheCluster)) []elasticachetypes.CacheCluster {
	mock := []elasticachetypes.CacheCluster{
		{
			CacheClusterStatus: aws.String("available"),
			ReplicationGroupId: aws.String("test-id"),
			EngineVersion:      aws.String(defaultEngineVersion),
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestSchemeRedis() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := croApis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = cloudCredentialApis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = monitoringv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

// mock elasticache DescribeReplicationGroups output
func (m *mockElasticacheClient) DescribeReplicationGroups(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	callInfo := struct {
		In1 *elasticache.DescribeReplicationGroupsInput
	}{
		In1: input,
	}
	m.calls.DescribeReplicationGroups = append(m.calls.DescribeReplicationGroups, callInfo)
	return m.describeReplicationGroupsFn(ctx, input)

}

func (m *mockElasticacheClient) DescribeUpdateActions(ctx context.Context, input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
	callInfo := struct {
		In1 *elasticache.DescribeUpdateActionsInput
	}{
		In1: input,
	}
	m.calls.DescribeUpdateActions = append(m.calls.DescribeUpdateActions, callInfo)
	return m.describeUpdateActionsFn(ctx, input)

}

func (m *mockElasticacheClient) ModifyReplicationGroup(ctx context.Context, input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
	callInfo := struct {
		In1 *elasticache.ModifyReplicationGroupInput
	}{
		In1: input,
	}
	m.calls.ModifyReplicationGroup = append(m.calls.ModifyReplicationGroup, callInfo)
	return m.modifyReplicationGroupFn(ctx, input)
}

// mock elasticache CreateReplicationGroup output
func (m *mockElasticacheClient) CreateReplicationGroup(ctx context.Context, input *elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
	if m.createReplicationGroupFn == nil {
		panic("createReplicationGroupFn: method is nil but elasticacheClient.CreateReplicationGroup was just called")
	}
	callInfo := struct {
		In1 *elasticache.CreateReplicationGroupInput
	}{
		In1: input,
	}
	m.calls.CreateReplicationGroup = append(m.calls.CreateReplicationGroup, callInfo)
	return m.createReplicationGroupFn(ctx, input)
}

// mock elasticache DeleteReplicationGroup output
func (m *mockElasticacheClient) DeleteReplicationGroup(*elasticache.DeleteReplicationGroupInput) (*elasticache.DeleteReplicationGroupOutput, error) {
	return &elasticache.DeleteReplicationGroupOutput{}, nil
}

// mock elasticache AddTagsToResource output
func (m *mockElasticacheClient) AddTagsToResource(ctx context.Context, input *elasticache.AddTagsToResourceInput) (*elasticache.AddTagsToResourceOutput, error) {
	if resources.SafeStringDereference(input.ResourceName) == "arn:aws:elasticache:tes:test:cluster:test" {
		return &elasticache.AddTagsToResourceOutput{}, nil
	} else {
		return m.addTagsToResourceFn(ctx, input)
	}
}

// mock elasticache DescribeSnapshots
func (m *mockElasticacheClient) DescribeSnapshots(ctx context.Context, input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
	if m.describeSnapshotsFn == nil {
		panic("describeSnapshotsFn: method is nil but elasticacheClient.DescribeSnapshots was just called")
	}
	callInfo := struct {
		In1 *elasticache.DescribeSnapshotsInput
	}{
		In1: input,
	}
	m.calls.DescribeSnapshots = append(m.calls.DescribeSnapshots, callInfo)
	return m.describeSnapshotsFn(ctx, input)
}

func (m *mockElasticacheClient) CreateSnapshot(ctx context.Context, input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
	if m.createSnapshotFn == nil {
		panic("createSnapshotFn: method is nil but elasticacheClient.CreateSnapshot was just called")
	}
	callInfo := struct {
		In1 *elasticache.CreateSnapshotInput
	}{
		In1: input,
	}
	m.calls.CreateSnapshot = append(m.calls.CreateSnapshot, callInfo)
	return m.createSnapshotFn(ctx, input)
}

func (m *mockElasticacheClient) DeleteSnapshot(ctx context.Context, input *elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error) {
	if m.deleteSnapshotFn == nil {
		panic("deleteSnapshotFn: method is nil but elasticacheClient.DeleteSnapshot was just called")
	}
	callInfo := struct {
		In1 *elasticache.DeleteSnapshotInput
	}{
		In1: input,
	}
	m.calls.DeleteSnapshot = append(m.calls.DeleteSnapshot, callInfo)
	return m.deleteSnapshotFn(ctx, input)
}

func (m *mockElasticacheClient) DescribeCacheClusters(ctx context.Context, input *elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
	if m.describeCacheClustersFn == nil {
		panic("describeCacheClustersFn: method is nil but elasticacheClient.DescribeCacheClusters was just called")
	}
	return m.describeCacheClustersFn(ctx, input)
}

func (m *mockElasticacheClient) BatchApplyUpdateAction(ctx context.Context, input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
	if m.batchApplyUpdateActionFn == nil {
		panic("batchApplyUpdateActionFn: method is nil but elasticacheClient.batchApplyUpdateActionFn was just called")
	}
	callInfo := struct {
		In1 *elasticache.BatchApplyUpdateActionInput
	}{
		In1: input,
	}
	m.calls.BatchApplyUpdateAction = append(m.calls.BatchApplyUpdateAction, callInfo)
	return m.batchApplyUpdateActionFn(ctx, input)
}

func (m *mockElasticacheClient) DescribeCacheSubnetGroups(ctx context.Context, input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
	return m.describeCacheSubnetGroupsFn(ctx, input)
}

func (m *mockElasticacheClient) CreateCacheSubnetGroup(*elasticache.CreateCacheSubnetGroupInput) (*elasticache.CreateCacheSubnetGroupOutput, error) {
	return &elasticache.CreateCacheSubnetGroupOutput{}, nil
}

func (m *mockElasticacheClient) DeleteCacheSubnetGroup(ctx context.Context, input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
	return m.deleteCacheSubnetGroupFn(ctx, input)
}

func (m *mockElasticacheClient) ModifyCacheSubnetGroup(ctx context.Context, input *elasticache.ModifyCacheSubnetGroupInput) (*elasticache.ModifyCacheSubnetGroupOutput, error) {
	return m.modifyCacheSubnetGroupFn(ctx, input)
}

// mock sts get caller identity
func (m *mockStsClient) GetCallerIdentity(*sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String("test"),
	}, nil
}

func buildTestPrometheusRule() *monitoringv1.PrometheusRule {
	return &monitoringv1.PrometheusRule{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "availability-rule-test",
			Namespace: "test",
		},
	}
}

func buildTestRedisCR() *v1alpha1.Redis {
	return &v1alpha1.Redis{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:            "test",
			Namespace:       "test",
			ResourceVersion: fakeResourceVersion,
		},
	}
}

func buildReplicationGroup(modifyFn func(elasticachetypes.ReplicationGroup)) elasticachetypes.ReplicationGroup {
	mock := elasticachetypes.ReplicationGroup{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestRedisCluster() *providers.RedisCluster {
	return &providers.RedisCluster{DeploymentDetails: &providers.RedisDeploymentDetails{
		URI:  *testAddress,
		Port: int64(*testPort),
	}}
}

func Test_createRedisCluster(t *testing.T) {
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	secName, err := resources.BuildInfraName(context.TODO(), moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()), defaultSecurityGroupPostfix, defaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build security name", err)
	}
	type args struct {
		ctx                     context.Context
		r                       *v1alpha1.Redis
		stsClient               STSAPI
		elasticacheClient       ElastiCacheAPI
		ec2Client               EC2API
		redisConfig             *elasticache.CreateReplicationGroupInput
		stratCfg                *StrategyConfig
		standaloneNetworkExists bool
		maintenanceWindow       bool
		ServiceUpdate           *ServiceUpdate
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         resources.ConnectionTester
	}
	tests := []struct {
		name    string
		args    args
		fields  fields
		want    *providers.RedisCluster
		wantErr bool
		mockFn  func()
	}{
		{
			name: "test no error on cache clusters of type memcached with no replicationgroupid",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId: aws.String("primary-node"),
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: []elasticachetypes.CacheCluster{
							{CacheClusterId: aws.String("test-id")},
						},
					}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mockSts := new(mock_STSClient)
					return mockSts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "error getting replication groups",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.DescribeReplicationGroupsOutput)(nil), genericAWSError)
					return mockElasticache
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			wantErr: true,
			mockFn: func() {
				timeOut = time.Millisecond * 10
			},
		},
		{
			name: "error creating elasticache cluster",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.DescribeReplicationGroupsOutput)(nil), genericAWSError)
					mockElasticache.On("CreateReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.CreateReplicationGroupOutput)(nil), genericAWSError)
					return mockElasticache
				}(),
				standaloneNetworkExists: true,
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			wantErr: true,
			mockFn: func() {
				timeOut = time.Millisecond * 10
			},
		},
		{
			name: "error building subnet group name",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("CreateReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.CreateReplicationGroupOutput)(nil), genericAWSError)
					return mockElasticache
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			wantErr: true,
		},
		{
			name: "error describing subnet groups",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.DescribeCacheSubnetGroupsOutput)(nil), genericAWSError)
					return mockElasticache
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error getting vpc id from associated subnets",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeSubnetsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error getting vpc",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeVpcsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error when more than one vpc found associated with bundled subnets",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen),
							*buildValidStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error getting availability zones",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidNonTaggedStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeAvailabilityZonesOutput)(nil), genericAWSError)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error creating new subnet",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidNonTaggedStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								State:    "available",
								ZoneName: aws.String("new-zone"),
							},
						},
					}, nil)
					mockEc2.On("CreateSubnet", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateSubnetOutput)(nil), genericAWSError)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error setting up security group",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("testsubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "error creating security group",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							buildReplicationGroup(func(group elasticachetypes.ReplicationGroup) {
								group.ReplicationGroupId = aws.String("test-id")
								group.Status = aws.String("available")
								group.CacheNodeType = aws.String("test")
								group.SnapshotRetentionLimit = aws.Int32(20)
								group.NodeGroups = []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								}
							},
							)},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("testsubnetgroup"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateSecurityGroupOutput)(nil), genericAWSError)
					return mockEc2
				}(),
			},
			fields: fields{
				Logger: testLogger,
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			wantErr: true,
		},
		{
			name: "failed to describe clusters",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							{
								CacheSubnetGroupName: aws.String("testsubnetgroup"),
							},
						},
					}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.DescribeCacheClustersOutput)(nil), genericAWSError)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					sgs := buildSecurityGroups(secName)
					sgs[0].IpPermissions = []ec2types.IpPermission{
						{
							IpProtocol: aws.String("-1"),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: aws.String("10.0.0.0/16"),
								},
							},
						},
					}
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: sgs,
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				redisConfig: &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				r:           buildTestRedisCR(),
			},
			fields: fields{
				Logger:    testLogger,
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			wantErr: true,
		},
		{
			name: "test elasticache buildReplicationGroupPending is called (valid bundled subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("testtesttest"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("pending"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("CreateReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateReplicationGroupOutput{}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						UpdateActions: []elasticachetypes.UpdateAction{
							elasticachetypes.UpdateAction{
								CacheClusterId:                      nil,
								CacheNodeUpdateStatus:               nil,
								Engine:                              nil,
								EstimatedUpdateTime:                 nil,
								NodeGroupUpdateStatus:               nil,
								NodesUpdated:                        nil,
								ReplicationGroupId:                  nil,
								ServiceUpdateName:                   nil,
								ServiceUpdateRecommendedApplyByDate: nil,
								ServiceUpdateReleaseDate:            nil,
								ServiceUpdateSeverity:               "",
								ServiceUpdateStatus:                 "",
								ServiceUpdateType:                   "",
								SlaMet:                              "",
								UpdateActionAvailableDate:           nil,
								UpdateActionStatus:                  elasticachetypes.UpdateActionStatusWaitingToStart,
								UpdateActionStatusModifiedDate:      nil,
							},
						},
					}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: aws.String("test"),
								State:    "available",
							},
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is available (valid bundled subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.subnets = buildValidBundleSubnets()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: aws.String("test"),
								State:    "available",
							},
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is not available (valid bundled subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("pending"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.subnets = buildValidBundleSubnets()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: aws.String("test"),
								State:    "available",
							},
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and needs to be modified (valid bundled subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)

					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.subnets = buildValidBundleSubnets()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: aws.String("test"),
								State:    "available",
							},
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{}, nil)
					return mockEc2
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache needs to be modified error creating update strategy (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeInstanceTypeOfferingsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			wantErr: true,
		},
		{
			name: "test elasticache needs to be modified error modifying replication group (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.ModifyReplicationGroupOutput)(nil), genericAWSError)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{}, nil)
					return mockEc2
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			wantErr: true,
		},
		{
			name: "test elasticache needs to be modified service updates present (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{}, nil)
					return mockEc2
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				ServiceUpdate:           &ServiceUpdate{updates: []string{"test-service-update"}},
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache modification error applying service updates (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.DescribeUpdateActionsOutput)(nil), genericAWSError)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{}, nil)
					return mockEc2
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				ServiceUpdate:           &ServiceUpdate{updates: []string{"test-service-update"}},
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			wantErr: true,
		},
		{
			name: "test elasticache does not need to be modified maintenance window true (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and does not need to be modified (valid bundled subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: aws.String("test"),
								State:    "available",
							},
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				redisConfig: &elasticache.CreateReplicationGroupInput{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("test"),
					SnapshotRetentionLimit: aws.Int32(20),
				},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is available (valid standalone subnets)",
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{}, nil)
					return mockElasticache
				}(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					return mockEc2
				}(),
				r: buildTestRedisCR(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					return mocksts
				}(),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
				maintenanceWindow:       false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockFn != nil {
				tt.mockFn()
				// reset
				defer func() {
					timeOut = time.Minute * 5
				}()
			}
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         tt.fields.TCPPinger,
			}
			got, _, err := p.createElasticacheCluster(tt.args.ctx, tt.args.r, tt.args.elasticacheClient, tt.args.stsClient, tt.args.ec2Client, tt.args.redisConfig, tt.args.stratCfg, tt.args.ServiceUpdate, tt.args.standaloneNetworkExists, tt.args.maintenanceWindow)
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
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		ElastiCacheClient ElastiCacheAPI
		Ec2Client         EC2API
	}
	type args struct {
		networkManager          NetworkManager
		redisCreateConfig       *elasticache.CreateReplicationGroupInput
		redisDeleteConfig       *elasticache.DeleteReplicationGroupInput
		ctx                     context.Context
		redis                   *v1alpha1.Redis
		standaloneNetworkExists bool
		isLastResource          bool
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
				redisCreateConfig:       &elasticache.CreateReplicationGroupInput{},
				redisDeleteConfig:       &elasticache.DeleteReplicationGroupInput{},
				networkManager:          buildMockNetworkManager(),
				redis:                   buildTestRedisCR(),
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				ElastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{}, nil)
					return mockElasticache
				}(),
			},
			wantErr: false,
		},
		{
			name: "test successful delete with existing unavailable redis",
			args: args{
				networkManager:          buildMockNetworkManager(),
				redisCreateConfig:       &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig:       &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:                   buildTestRedisCR(),
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				ElastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{ReplicationGroups: []elasticachetypes.ReplicationGroup{
						elasticachetypes.ReplicationGroup{
							ARN:                        nil,
							AtRestEncryptionEnabled:    nil,
							AuthTokenEnabled:           nil,
							AuthTokenLastModifiedDate:  nil,
							AutoMinorVersionUpgrade:    nil,
							AutomaticFailover:          "",
							CacheNodeType:              aws.String("test"),
							ClusterEnabled:             nil,
							ClusterMode:                "",
							ConfigurationEndpoint:      nil,
							DataTiering:                "",
							Description:                nil,
							Engine:                     nil,
							GlobalReplicationGroupInfo: nil,
							IpDiscovery:                "",
							KmsKeyId:                   nil,
							LogDeliveryConfigurations:  nil,
							MemberClusters:             nil,
							MemberClustersOutpostArns:  nil,
							MultiAZ:                    "",
							NetworkType:                "",
							NodeGroups: []elasticachetypes.NodeGroup{
								{
									NodeGroupId:      aws.String("primary-node"),
									NodeGroupMembers: nil,
									PrimaryEndpoint: &elasticachetypes.Endpoint{
										Address: testAddress,
										Port:    testPort,
									},
									Status: aws.String("available"),
								},
							},
							PendingModifiedValues:      nil,
							ReplicationGroupCreateTime: nil,
							ReplicationGroupId:         aws.String("test-id"),
							SnapshotRetentionLimit:     aws.Int32(20),
							SnapshotWindow:             nil,
							SnapshottingClusterId:      nil,
							Status:                     aws.String("pending"),
							TransitEncryptionEnabled:   nil,
							TransitEncryptionMode:      "",
							UserGroupIds:               nil,
						},
					}}, nil)
					return mockElasticache
				}(),
			},
			wantErr: false,
		},
		{
			name: "test successful delete with existing available redis",
			args: args{
				networkManager:          buildMockNetworkManager(),
				redisCreateConfig:       &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig:       &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:                   buildTestRedisCR(),
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				ElastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: []elasticachetypes.ReplicationGroup{
							elasticachetypes.ReplicationGroup{
								ARN:                        nil,
								AtRestEncryptionEnabled:    nil,
								AuthTokenEnabled:           nil,
								AuthTokenLastModifiedDate:  nil,
								AutoMinorVersionUpgrade:    nil,
								AutomaticFailover:          "",
								CacheNodeType:              aws.String("test"),
								ClusterEnabled:             nil,
								ClusterMode:                "",
								ConfigurationEndpoint:      nil,
								DataTiering:                "",
								Description:                nil,
								Engine:                     nil,
								GlobalReplicationGroupInfo: nil,
								IpDiscovery:                "",
								KmsKeyId:                   nil,
								LogDeliveryConfigurations:  nil,
								MemberClusters:             nil,
								MemberClustersOutpostArns:  nil,
								MultiAZ:                    "",
								NetworkType:                "",
								NodeGroups: []elasticachetypes.NodeGroup{
									{
										NodeGroupId:      aws.String("primary-node"),
										NodeGroupMembers: nil,
										PrimaryEndpoint: &elasticachetypes.Endpoint{
											Address: testAddress,
											Port:    testPort,
										},
										Status: aws.String("available"),
									},
								},
								PendingModifiedValues:      nil,
								ReplicationGroupCreateTime: nil,
								ReplicationGroupId:         aws.String("test-id"),
								SnapshotRetentionLimit:     aws.Int32(20),
								SnapshotWindow:             nil,
								SnapshottingClusterId:      nil,
								Status:                     aws.String("available"),
								TransitEncryptionEnabled:   nil,
								TransitEncryptionMode:      "",
								UserGroupIds:               nil,
							},
						},
					}, nil)
					mockElasticache.On("DeleteReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
			},
			wantErr: false,
		},
		{
			name: "test successful delete with no existing redis but with standalone network",
			args: args{
				networkManager:          buildMockNetworkManager(),
				redisCreateConfig:       &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig:       &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:                   buildTestRedisCR(),
				standaloneNetworkExists: true,
				isLastResource:          true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				ElastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{}, nil)
					return mockElasticache
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					return mockEc2
				}(),
			},
			wantErr: false,
		}, {
			name: "test successful delete with no existing redis but with bundled network resources",
			args: args{
				networkManager:          buildMockNetworkManager(),
				redisCreateConfig:       &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redisDeleteConfig:       &elasticache.DeleteReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				redis:                   buildTestRedisCR(),
				standaloneNetworkExists: false,
				isLastResource:          true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				ElastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{}, nil)
					return mockElasticache
				}(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				CacheSvc:          tt.fields.ElastiCacheClient,
			}
			if _, err := p.deleteElasticacheCluster(tt.args.ctx, tt.args.networkManager, tt.fields.ElastiCacheClient, tt.fields.Ec2Client, tt.args.redisCreateConfig, tt.args.redisDeleteConfig, tt.args.redis, tt.args.standaloneNetworkExists, tt.args.isLastResource); (err != nil) != tt.wantErr {
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
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				r: &v1alpha1.Redis{
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{}
			if got := p.GetReconcileTime(tt.args.r); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSRedisProvider_TagElasticache(t *testing.T) {
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		ElastiCacheClient ElastiCacheAPI
	}
	type args struct {
		ctx               context.Context
		elastiCacheClient ElastiCacheAPI
		stsClient         STSAPI
		r                 *v1alpha1.Redis
		stratCfg          StrategyConfig
		cache             *elasticachetypes.NodeGroupMember
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "test tags reconcile completes successfully",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				elastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName: &snapshotName,
							},
						},
					}, nil)
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.AddTagsToResourceOutput{}, nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					return mockElasticache
				}(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					mocksts.On("GetCallerIdentity", mock.Anything, mock.Anything, mock.Anything).Return(&sts.GetCallerIdentityOutput{
						Account: aws.String("test"),
					}, nil)
					return mocksts
				}(),
				stratCfg: StrategyConfig{Region: "test"},
				cache: &elasticachetypes.NodeGroupMember{
					CacheClusterId:            aws.String("test"),
					CacheNodeId:               aws.String("test"),
					PreferredAvailabilityZone: aws.String("test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("successfully created and tagged"),
			wantErr: false,
		},
		{
			name: "test tags reconcile completes successfully with a DBClusterSnapshotNotFound error",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				elastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName: &snapshotName,
							},
						},
					}, &mockSnapshotNotFoundError{})
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.AddTagsToResourceOutput)(nil), nil)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					return mockElasticache
				}(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					mocksts.On("GetCallerIdentity", mock.Anything, mock.Anything, mock.Anything).Return(&sts.GetCallerIdentityOutput{
						Account: aws.String("test"),
					}, nil)
					return mocksts
				}(),
				stratCfg: StrategyConfig{Region: "test"},
				cache: &elasticachetypes.NodeGroupMember{
					CacheClusterId:            aws.String("test"),
					CacheNodeId:               aws.String("test"),
					PreferredAvailabilityZone: aws.String("test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("successfully created and tagged"),
			wantErr: false,
		},
		{
			name: "test tags reconcile fails with any other than expected aws error",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				elastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{}, nil)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName: &snapshotName,
							},
						},
					}, nil)
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.AddTagsToResourceOutput)(nil), nil).Once()
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.AddTagsToResourceOutput)(nil), genericAWSError)
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					return mockElasticache
				}(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					mocksts.On("GetCallerIdentity", mock.Anything, mock.Anything, mock.Anything).Return(&sts.GetCallerIdentityOutput{
						Account: aws.String("test"),
					}, nil)
					return mocksts
				}(),
				stratCfg: StrategyConfig{Region: "test"},
				cache: &elasticachetypes.NodeGroupMember{
					CacheClusterId:            aws.String("test"),
					CacheNodeId:               aws.String("test"),
					PreferredAvailabilityZone: aws.String("test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("failed to add tags to aws elasticache snapshot"),
			wantErr: true,
		},
		{
			name: "test tags reconcile fails with any other generic error",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				elastiCacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName: &snapshotName,
							},
						},
					}, errors.New("SnapshotAlreadyExistsFault"))
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.AddTagsToResourceOutput)(nil), nil).Once()
					mockElasticache.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.AddTagsToResourceOutput)(nil), errors.New("SnapshotAlreadyExistsFault")).Once()
					mockElasticache.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheClustersOutput{
						CacheClusters: buildCacheClusterList(nil),
					}, nil)
					return mockElasticache
				}(),
				stsClient: func() STSAPI {
					mocksts := new(mock_STSClient)
					mocksts.On("GetCallerIdentity", mock.Anything, mock.Anything, mock.Anything).Return(&sts.GetCallerIdentityOutput{
						Account: aws.String("test"),
					}, nil)
					return mocksts
				}(),
				stratCfg: StrategyConfig{Region: "test"},
				cache: &elasticachetypes.NodeGroupMember{
					CacheClusterId:            aws.String("test"),
					CacheNodeId:               aws.String("test"),
					PreferredAvailabilityZone: aws.String("test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("failed to add tags to aws elasticache snapshot"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				CacheSvc:          tt.fields.ElastiCacheClient,
			}
			got, err := p.TagElasticacheNode(tt.args.ctx, tt.args.elastiCacheClient, tt.args.stsClient, tt.args.r, *tt.args.cache)
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

func Test_buildElasticacheUpdateStrategy(t *testing.T) {
	type args struct {
		ctx                      context.Context
		ec2Client                EC2API
		elasticacheConfig        *elasticache.CreateReplicationGroupInput
		foundConfig              *elasticachetypes.ReplicationGroup
		replicationGroupClusters []elasticachetypes.CacheCluster
		logger                   *logrus.Entry
		redis                    *v1alpha1.Redis
	}
	tests := []struct {
		name    string
		args    args
		want    *elasticache.ModifyReplicationGroupInput
		wantErr string
	}{
		{
			name: "test no modification required",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int32(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("3.2.6"),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int32(30),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("3.2.6"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
					},
				},
				logger: testLogger,
			},
			want: nil,
		},
		{
			name: "test no modification required when current engine version higher than desired",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int32(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("3.2.6"),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int32(30),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("5.0.0"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
					},
				},
				logger: testLogger,
			},
			want: nil,
		},
		{
			name: "test error when invalid desired engine version",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int32(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("some invalid value"),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int32(30),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("5.0.0"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
					},
				},
				logger: testLogger,
			},
			want:    nil,
			wantErr: "invalid redis version: failed to parse desired version: Malformed version: some invalid value",
		},
		{
			name: "test error when invalid current engine version",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int32(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("some invalid value"),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int32(30),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("some invalid value"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
					},
				},
				logger: testLogger,
			},
			want:    nil,
			wantErr: "invalid redis version: failed to parse current version: Malformed version: some invalid value",
		},
		{
			name: "test when modification is required",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String("test"),
							},
						},
					}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.newValue"),
					SnapshotRetentionLimit:     aws.Int32(50),
					PreferredMaintenanceWindow: aws.String("newValue"),
					SnapshotWindow:             aws.String("newValue"),
					EngineVersion:              aws.String(defaultEngineVersion),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int32(30),
					ReplicationGroupId:     aws.String("test-id"),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("3.2.6"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
						PreferredAvailabilityZone:  aws.String("test"),
					},
				},
				logger: testLogger,
				redis:  &v1alpha1.Redis{},
			},
			want: &elasticache.ModifyReplicationGroupInput{
				CacheNodeType:              aws.String("cache.newValue"),
				SnapshotRetentionLimit:     aws.Int32(50),
				PreferredMaintenanceWindow: aws.String("newValue"),
				SnapshotWindow:             aws.String("newValue"),
				ReplicationGroupId:         aws.String("test-id"),
				EngineVersion:              aws.String(defaultEngineVersion),
				ApplyImmediately:           aws.Bool(true),
			},
		},
		{
			name: "test failed aws instance type offering list results in error",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeInstanceTypeOfferingsOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType: aws.String("cache.test"),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					CacheNodeType:          aws.String("cache.test2"),
					ReplicationGroupId:     aws.String("test-id"),
					SnapshotRetentionLimit: aws.Int32(50),
					SnapshotWindow:         aws.String("newValue"),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{},
				logger:                   testLogger,
			},
			want:    nil,
			wantErr: "failed to get instance type offerings for type cache.test2: test",
		},
		{
			name: "test unsupported instance types changes are not added to proposed modification",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String("current-cache-type"),
							},
						},
					}, nil)
					return mockEc2
				}(),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.unsupported-cache-type"),
					SnapshotRetentionLimit:     aws.Int32(50),
					PreferredMaintenanceWindow: aws.String("newValue"),
					SnapshotWindow:             aws.String("newValue"),
					EngineVersion:              aws.String(defaultEngineVersion),
				},
				foundConfig: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.current-cache-type"),
					SnapshotRetentionLimit: aws.Int32(30),
				},
				replicationGroupClusters: []elasticachetypes.CacheCluster{
					{
						EngineVersion:              aws.String("3.2.6"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
						PreferredAvailabilityZone:  aws.String("test2"),
					},
				},
				logger: testLogger,
			},
			want: &elasticache.ModifyReplicationGroupInput{
				SnapshotRetentionLimit:     aws.Int32(50),
				PreferredMaintenanceWindow: aws.String("newValue"),
				SnapshotWindow:             aws.String("newValue"),
				ReplicationGroupId:         aws.String("test-id"),
				EngineVersion:              aws.String(defaultEngineVersion),
				ApplyImmediately:           aws.Bool(true),
			},
		},
		{
			name: "test nil parameters returned in aws objects",
			args: args{
				ctx: context.TODO(),
				ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				elasticacheConfig:        &elasticache.CreateReplicationGroupInput{},
				foundConfig:              &elasticachetypes.ReplicationGroup{},
				replicationGroupClusters: []elasticachetypes.CacheCluster{},
				logger:                   testLogger,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildElasticacheUpdateStrategy(tt.args.ctx, tt.args.ec2Client, tt.args.elasticacheConfig, tt.args.foundConfig, tt.args.replicationGroupClusters, tt.args.logger, tt.args.redis)
			if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Errorf("createElasticacheCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildElasticacheUpdateStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// specified update that is critical security - have it be applied
// non critical/non security update be scheduled for maintenance window
// if it's completed - make sure we don't get errors or call applybatch action.
// ignore that calls are made in certain states - e.g in progress
// apply critical security update return if it wants to apply it immediately - if true call the apply immediately logic next in the provider
// having a return value.

func TestRedisProvider_applySpecifiedSecurityUpdates(t *testing.T) {
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		ElasticacheClient ElastiCacheAPI
		TCPPinger         resources.ConnectionTester
	}
	type args struct {
		ctx               context.Context
		elasticacheClient ElastiCacheAPI
		replicationGroup  *elasticachetypes.ReplicationGroup
		specifiedUpdates  *ServiceUpdate
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "if a specified update that is critical security update it should be applied immediately",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
		},
		{
			name: "expect specified update that is not critical security update to be batch applied but not modified",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityImportant,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
		},
		{
			name: "expect batchupdate to be called but not modify for a specified update that is critical but not security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateType("othertype"),
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityImportant,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
		},
		{
			name: "expect modify and batchapply not to be called if a non specified update that is critical and is security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{}},
			},
		},
		{
			name: "expect modify and batchapply not to be called if specified critical security update is already complete",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusComplete,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{}},
			},
		},
		{
			name: "expect modify to not be called if there is an unprocessed update action returned by batchapplyupdate for update that is critical security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{
						UnprocessedUpdateActions: []elasticachetypes.UnprocessedUpdateAction{
							{
								CacheClusterId: aws.String("test-replication-group"),
								ErrorMessage:   aws.String("The update action is not in a valid status"),
								ErrorType:      aws.String("InvalidParameterValue"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			wantErr: true,
		},
		{
			name: "expect modify to not be called if there is an error returned by batchapplyupdate for update that is critical security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, genericAWSError)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			wantErr: true,
		},
		{
			name: "expect an error if ModifyReplicationGroup return error",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				ElasticacheClient: nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				ctx: context.TODO(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker: nil,
						UpdateActions: []elasticachetypes.UpdateAction{
							{
								ServiceUpdateName:     aws.String("test-service-update"),
								ServiceUpdateType:     elasticachetypes.ServiceUpdateTypeSecurityUpdate,
								ServiceUpdateSeverity: elasticachetypes.ServiceUpdateSeverityCritical,
								UpdateActionStatus:    elasticachetypes.UpdateActionStatusScheduling,
								ServiceUpdateStatus:   elasticachetypes.ServiceUpdateStatusAvailable,
							},
						},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, errors.New("modify error"))
					mockElasticache.On("BatchApplyUpdateAction", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.BatchApplyUpdateActionOutput{}, nil)
					return mockElasticache
				}(),
				replicationGroup: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				CacheSvc:          tt.fields.ElasticacheClient,
				TCPPinger:         tt.fields.TCPPinger,
			}
			err := p.applySpecifiedSecurityUpdates(tt.args.ctx, tt.args.elasticacheClient, tt.args.replicationGroup, tt.args.specifiedUpdates)
			if (err != nil) != tt.wantErr {
				t.Errorf("applylSpecifiedSecurityUpdates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestNewAWSRedisProvider(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	type args struct {
		client func() client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name    string
		args    args
		want    *RedisProvider
		wantErr bool
	}{
		{
			name: "successfully create new redis provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: false,
		},
		{
			name: "fail to create new redis provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return errors.New("generic error")
					}
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAWSRedisProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSRedisProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSRedisProvider() got = %v, want non-nil result", got)
			}
		})
	}
}

func TestRedisProvider_getElasticacheConfig(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		ctx context.Context
		r   *v1alpha1.Redis
	}

	infra := &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.InfrastructureSpec{},
		Status: configv1.InfrastructureStatus{
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
				AWS: &configv1.AWSPlatformStatus{
					Region: "testRegion",
				},
			},
		},
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *elasticache.CreateReplicationGroupInput
		want1   *elasticache.DeleteReplicationGroupInput
		want2   *ServiceUpdate
		wantErr bool
	}{
		{
			name: "test node size from create strategy is returned if size is not set in spec",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      DefaultConfigMapName,
							Namespace: "test",
						},
						Data: map[string]string{
							"redis": "{\"development\": { \"region\": \"\", \"createStrategy\": {\"cacheNodeType\": \"cache.t3.small\"}, \"deleteStrategy\": {}, \"serviceUpdates\": [\"elasticache-20210615-002\"]  }}",
						},
					},
					infra,
				),
				Logger: testLogger,
			},
			args: args{
				r: &v1alpha1.Redis{Spec: croType.ResourceTypeSpec{
					Tier: "development",
				}},
			},
			want: &elasticache.CreateReplicationGroupInput{
				CacheNodeType: aws.String("cache.t3.small"),
			},
			want1: &elasticache.DeleteReplicationGroupInput{},
			want2: &ServiceUpdate{
				updates: []string{"elasticache-20210615-002"},
			},
		},
		{
			name: "test node size from spec is returned when set in spec",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      DefaultConfigMapName,
							Namespace: "test",
						},
						Data: map[string]string{
							"redis": "{\"development\": { \"region\": \"\", \"createStrategy\": {}, \"deleteStrategy\": {}, \"serviceUpdates\": [\"elasticache-20210615-002\"]  }}",
						},
					},
					infra,
				),
				Logger: testLogger,
			},
			args: args{
				r: &v1alpha1.Redis{Spec: croType.ResourceTypeSpec{
					Tier: "development",
					Size: "cache.m5.large",
				}},
			},
			want: &elasticache.CreateReplicationGroupInput{
				CacheNodeType: aws.String("cache.m5.large"),
			},
			want1: &elasticache.DeleteReplicationGroupInput{},
			want2: &ServiceUpdate{
				updates: []string{"elasticache-20210615-002"},
			},
		},
		{
			name: "test node size from spec takes precedence even if node type is specified in strategy config map",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      DefaultConfigMapName,
							Namespace: "test",
						},
						Data: map[string]string{
							"redis": "{\"development\": { \"region\": \"\", \"createStrategy\": {\"cacheNodeType\": \"cache.t3.small\"}, \"deleteStrategy\": {}, \"serviceUpdates\": [\"elasticache-20210615-002\"]  }}",
						},
					},
					infra,
				),
				Logger: testLogger,
			},
			args: args{
				r: &v1alpha1.Redis{Spec: croType.ResourceTypeSpec{
					Tier: "development",
					Size: "cache.m5.large",
				}},
			},
			want: &elasticache.CreateReplicationGroupInput{
				CacheNodeType: aws.String("cache.m5.large"),
			},
			want1: &elasticache.DeleteReplicationGroupInput{},
			want2: &ServiceUpdate{
				updates: []string{"elasticache-20210615-002"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: NewConfigMapConfigManager(DefaultConfigMapName, "test", tt.fields.Client),
			}
			got, got1, got2, _, err := p.getElasticacheConfig(tt.args.ctx, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("getElasticacheConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getElasticacheConfig() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("getElasticacheConfig() got1 = %v, want %v", got1, tt.want1)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("getElasticacheConfig() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func TestRedisProvider_createElasticacheConnectionMetric(t *testing.T) {
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client    client.Client
		Logger    *logrus.Entry
		TCPPinger resources.ConnectionTester
	}
	type args struct {
		ctx        context.Context
		r          *v1alpha1.Redis
		foundCache *elasticachetypes.ReplicationGroup
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "test healthy cluster with node groups",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestInfra()),
				Logger:    testLogger,
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				foundCache: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-redis"),
					Status:             aws.String("available"),
					NodeGroups: []elasticachetypes.NodeGroup{
						{
							PrimaryEndpoint: &elasticachetypes.Endpoint{
								Address: testAddress,
								Port:    testPort,
							},
						},
					},
				},
			},
		},
		{
			name: "test cluster with node groups but nil endpoints (should still be healthy)",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestInfra()),
				Logger:    testLogger,
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				foundCache: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-redis"),
					Status:             aws.String("create-failed"),
					NodeGroups: []elasticachetypes.NodeGroup{
						{
							PrimaryEndpoint: nil,
						},
					},
				},
			},
		},
		{
			name: "test cluster with empty NodeGroups array (unhealthy)",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestInfra()),
				Logger:    testLogger,
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				foundCache: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-redis"),
					Status:             aws.String("available"),
					NodeGroups:         []elasticachetypes.NodeGroup{},
				},
			},
		},
		{
			name: "test cluster with nil NodeGroups (unhealthy)",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestInfra()),
				Logger:    testLogger,
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				foundCache: &elasticachetypes.ReplicationGroup{
					ReplicationGroupId: aws.String("test-redis"),
					Status:             aws.String("available"),
					NodeGroups:         nil,
				},
			},
		},
		{
			name: "test nil cache (unhealthy)",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestInfra()),
				Logger:    testLogger,
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx:        context.TODO(),
				r:          buildTestRedisCR(),
				foundCache: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:    tt.fields.Client,
				Logger:    tt.fields.Logger,
				TCPPinger: tt.fields.TCPPinger,
			}
			p.createElasticacheConnectionMetric(tt.args.ctx, tt.args.r, tt.args.foundCache)
		})
	}
}
