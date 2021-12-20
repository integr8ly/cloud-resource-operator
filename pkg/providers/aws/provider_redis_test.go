package aws

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	croApis "github.com/integr8ly/cloud-resource-operator/apis"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	cloudCredentialApis "github.com/openshift/cloud-credential-operator/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinery "k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"

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
	replicationGroups []*elasticache.ReplicationGroup

	// new approach for manually defined mocks
	// to allow for simple overrides in test table declarations
	modifyCacheSubnetGroupFn    func(*elasticache.ModifyCacheSubnetGroupInput) (*elasticache.ModifyCacheSubnetGroupOutput, error)
	deleteCacheSubnetGroupFn    func(*elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error)
	describeCacheSubnetGroupsFn func(*elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error)
	describeCacheClustersFn     func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error)
	describeReplicationGroupsFn func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error)
	describeSnapshotsFn         func(*elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error)
	createSnapshotFn            func(*elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error)
	deleteSnapshotFn            func(*elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error)
	describeUpdateActionsFn     func(*elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error)
	modifyReplicationGroupFn    func(*elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error)
	batchApplyUpdateActionFn    func(*elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error)

	calls struct {
		DescribeSnapshots []struct {
			In1 *elasticache.DescribeSnapshotsInput
		}
		DescribeReplicationGroups []struct {
			In1 *elasticache.DescribeReplicationGroupsInput
		}
		CreateSnapshot []struct {
			In1 *elasticache.CreateSnapshotInput
		}
		DeleteSnapshot []struct {
			In1 *elasticache.DeleteSnapshotInput
		}
		DescribeUpdateActions []struct {
			In1 *elasticache.DescribeUpdateActionsInput
		}
		ModifyReplicationGroup []struct {
			In1 *elasticache.ModifyReplicationGroupInput
		}
		BatchApplyUpdateAction []struct {
			In1 *elasticache.BatchApplyUpdateActionInput
		}
	}
}

func buildMockElasticacheClient(modifyFn func(*mockElasticacheClient)) *mockElasticacheClient {
	mock := &mockElasticacheClient{
		describeCacheSubnetGroupsFn: func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
			return &elasticache.DescribeCacheSubnetGroupsOutput{}, nil
		},
		describeUpdateActionsFn: func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
			return &elasticache.DescribeUpdateActionsOutput{
				Marker:        nil,
				UpdateActions: []*elasticache.UpdateAction{},
			}, nil
		},
		modifyReplicationGroupFn: func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
			return &elasticache.ModifyReplicationGroupOutput{}, nil
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

type mockStsClient struct {
	stsiface.STSAPI
}

func buildCacheClusterList(modifyFn func([]*elasticache.CacheCluster)) []*elasticache.CacheCluster {
	mock := []*elasticache.CacheCluster{
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
	scheme := apimachinery.NewScheme()
	err := croApis.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	err = cloudCredentialApis.AddToScheme(scheme)
	err = monitoringv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

// mock elasticache DescribeReplicationGroups output
func (m *mockElasticacheClient) DescribeReplicationGroups(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	callInfo := struct {
		In1 *elasticache.DescribeReplicationGroupsInput
	}{
		In1: input,
	}
	m.calls.DescribeReplicationGroups = append(m.calls.DescribeReplicationGroups, callInfo)
	return m.describeReplicationGroupsFn(input)

}

func (m *mockElasticacheClient) DescribeUpdateActions(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
	callInfo := struct {
		In1 *elasticache.DescribeUpdateActionsInput
	}{
		In1: input,
	}
	m.calls.DescribeUpdateActions = append(m.calls.DescribeUpdateActions, callInfo)
	return m.describeUpdateActionsFn(input)

}

func (m *mockElasticacheClient) ModifyReplicationGroup(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
	callInfo := struct {
		In1 *elasticache.ModifyReplicationGroupInput
	}{
		In1: input,
	}
	m.calls.ModifyReplicationGroup = append(m.calls.ModifyReplicationGroup, callInfo)
	return m.modifyReplicationGroupFn(input)
}

// mock elasticache CreateReplicationGroup output
func (m *mockElasticacheClient) CreateReplicationGroup(*elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
	return &elasticache.CreateReplicationGroupOutput{}, nil
}

// mock elasticache DeleteReplicationGroup output
func (m *mockElasticacheClient) DeleteReplicationGroup(*elasticache.DeleteReplicationGroupInput) (*elasticache.DeleteReplicationGroupOutput, error) {
	return &elasticache.DeleteReplicationGroupOutput{}, nil
}

// mock elasticache AddTagsToResource output
func (m *mockElasticacheClient) AddTagsToResource(*elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
	return &elasticache.TagListMessage{}, nil
}

// mock elasticache DescribeSnapshots
func (m *mockElasticacheClient) DescribeSnapshots(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
	if m.describeSnapshotsFn == nil {
		panic("describeSnapshotsFn: method is nil but elasticacheClient.DescribeSnapshots was just called")
	}
	callInfo := struct {
		In1 *elasticache.DescribeSnapshotsInput
	}{
		In1: input,
	}
	m.calls.DescribeSnapshots = append(m.calls.DescribeSnapshots, callInfo)
	return m.describeSnapshotsFn(input)
}

func (m *mockElasticacheClient) CreateSnapshot(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
	if m.createSnapshotFn == nil {
		panic("createSnapshotFn: method is nil but elasticacheClient.CreateSnapshot was just called")
	}
	callInfo := struct {
		In1 *elasticache.CreateSnapshotInput
	}{
		In1: input,
	}
	m.calls.CreateSnapshot = append(m.calls.CreateSnapshot, callInfo)
	return m.createSnapshotFn(input)
}

func (m *mockElasticacheClient) DeleteSnapshot(input *elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error) {
	if m.deleteSnapshotFn == nil {
		panic("deleteSnapshotFn: method is nil but elasticacheClient.DeleteSnapshot was just called")
	}
	callInfo := struct {
		In1 *elasticache.DeleteSnapshotInput
	}{
		In1: input,
	}
	m.calls.DeleteSnapshot = append(m.calls.DeleteSnapshot, callInfo)
	return m.deleteSnapshotFn(input)
}

func (m *mockElasticacheClient) DescribeCacheClusters(input *elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
	if m.describeCacheClustersFn == nil {
		panic("describeCacheClustersFn: method is nil but elasticacheClient.DescribeCacheClusters was just called")
	}
	return m.describeCacheClustersFn(input)
}

func (m *mockElasticacheClient) BatchApplyUpdateAction(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
	if m.batchApplyUpdateActionFn == nil {
		panic("batchApplyUpdateActionFn: method is nil but elasticacheClient.batchApplyUpdateActionFn was just called")
	}
	callInfo := struct {
		In1 *elasticache.BatchApplyUpdateActionInput
	}{
		In1: input,
	}
	m.calls.BatchApplyUpdateAction = append(m.calls.BatchApplyUpdateAction, callInfo)
	return m.batchApplyUpdateActionFn(input)
}

func (m *mockElasticacheClient) DescribeCacheSubnetGroups(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
	return m.describeCacheSubnetGroupsFn(input)
}

func (m *mockElasticacheClient) CreateCacheSubnetGroup(*elasticache.CreateCacheSubnetGroupInput) (*elasticache.CreateCacheSubnetGroupOutput, error) {
	return &elasticache.CreateCacheSubnetGroupOutput{}, nil
}

func (m *mockElasticacheClient) DeleteCacheSubnetGroup(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
	return m.deleteCacheSubnetGroupFn(input)
}

func (m *mockElasticacheClient) ModifyCacheSubnetGroup(input *elasticache.ModifyCacheSubnetGroupInput) (*elasticache.ModifyCacheSubnetGroupOutput, error) {
	return m.modifyCacheSubnetGroupFn(input)
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
			Name:      "test",
			Namespace: "test",
		},
	}
}

func buildReplicationGroup(modifyFn func(*elasticache.ReplicationGroup)) *elasticache.ReplicationGroup {
	mock := &elasticache.ReplicationGroup{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestRedisCluster() *providers.RedisCluster {
	return &providers.RedisCluster{DeploymentDetails: &providers.RedisDeploymentDetails{
		URI:  *testAddress,
		Port: *testPort,
	}}
}

func Test_createRedisCluster(t *testing.T) {
	scheme, err := buildTestSchemeRedis()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	secName, err := BuildInfraName(context.TODO(), fake.NewFakeClientWithScheme(scheme, buildTestInfra()), defaultSecurityGroupPostfix, DefaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build security name", err)
	}
	type args struct {
		ctx                     context.Context
		r                       *v1alpha1.Redis
		stsSvc                  stsiface.STSAPI
		cacheSvc                elasticacheiface.ElastiCacheAPI
		ec2Svc                  ec2iface.EC2API
		redisConfig             *elasticache.CreateReplicationGroupInput
		stratCfg                *StrategyConfig
		standaloneNetworkExists bool
		ServiceUpdate           *ServiceUpdate
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         ConnectionTester
	}
	tests := []struct {
		name    string
		args    args
		fields  fields
		want    *providers.RedisCluster
		wantErr bool
	}{
		{
			name: "test no error on cache clusters of type memcahced with no replicationgroupid",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(input *elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{
							CacheClusters: []*elasticache.CacheCluster{
								{
									CacheClusterId: aws.String("test-id"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache buildReplicationGroupPending is called (valid cluster rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName)},
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
				redisConfig:             &elasticache.CreateReplicationGroupInput{},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is available (valid cluster rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is not available (valid cluster rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("pending")
								}),
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and needs to be modified (valid cluster rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache exists and status is available and does not need to be modified (valid cluster rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				redisConfig: &elasticache.CreateReplicationGroupInput{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("test"),
					SnapshotRetentionLimit: aws.Int64(20),
				},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: false,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
		{
			name: "test elasticache already exists and status is available (valid standalone rhmi subnets)",
			args: args{
				ctx: context.TODO(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.secGroups = buildSecurityGroups(secName)
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
				redisConfig:             &elasticache.CreateReplicationGroupInput{ReplicationGroupId: aws.String("test-id")},
				stratCfg:                &StrategyConfig{Region: "test"},
				standaloneNetworkExists: true,
			},
			fields: fields{
				ConfigManager:     nil,
				CredentialManager: nil,
				Logger:            testLogger,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			want:    buildTestRedisCluster(),
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
				TCPPinger:         tt.fields.TCPPinger,
			}
			got, _, err := p.createElasticacheCluster(tt.args.ctx, tt.args.r, tt.args.cacheSvc, tt.args.stsSvc, tt.args.ec2Svc, tt.args.redisConfig, tt.args.stratCfg, tt.args.ServiceUpdate, tt.args.standaloneNetworkExists)
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
		CacheSvc          elasticacheiface.ElastiCacheAPI
	}
	type args struct {
		cacheSvc                elasticacheiface.ElastiCacheAPI
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("pending")
								}),
							},
						}, nil
					}
				}),
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: []*elasticache.ReplicationGroup{
								buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
									group.ReplicationGroupId = aws.String("test-id")
									group.Status = aws.String("available")
									group.CacheNodeType = aws.String("test")
									group.SnapshotRetentionLimit = aws.Int64(20)
									group.NodeGroups = []*elasticache.NodeGroup{
										{
											NodeGroupId:      aws.String("primary-node"),
											NodeGroupMembers: nil,
											PrimaryEndpoint: &elasticache.Endpoint{
												Address: testAddress,
												Port:    testPort,
											},
											Status: aws.String("available"),
										},
									}
								},
								)},
						}, nil
					}
				}),
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
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
				CacheSvc:          tt.fields.CacheSvc,
			}
			if _, err := p.deleteElasticacheCluster(tt.args.ctx, tt.args.networkManager, tt.fields.CacheSvc, tt.args.redisCreateConfig, tt.args.redisDeleteConfig, tt.args.redis, tt.args.standaloneNetworkExists, tt.args.isLastResource); (err != nil) != tt.wantErr {
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
					Status: croType.ResourceTypeStatus{
						Phase: types.PhaseComplete,
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
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeCacheClustersFn = func(input *elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{
							CacheClusters: buildCacheClusterList(nil),
						}, nil
					}
					elasticacheClient.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{}, nil
					}
				}),
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
			want:    types.StatusMessage("successfully created and tagged"),
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
				CacheSvc:          tt.fields.CacheSvc,
			}
			got, err := p.TagElasticacheNode(tt.args.ctx, tt.args.cacheSvc, tt.args.stsSvc, tt.args.r, tt.args.cache)
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
		ec2Client                ec2iface.EC2API
		elasticacheConfig        *elasticache.CreateReplicationGroupInput
		foundConfig              *elasticache.ReplicationGroup
		replicationGroupClusters []elasticache.CacheCluster
		logger                   *logrus.Entry
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
				ec2Client: buildMockEc2Client(nil),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int64(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("3.2.6"),
				},
				foundConfig: &elasticache.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int64(30),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
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
				ec2Client: buildMockEc2Client(nil),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int64(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("3.2.6"),
				},
				foundConfig: &elasticache.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int64(30),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
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
				ec2Client: buildMockEc2Client(nil),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int64(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("some invalid value"),
				},
				foundConfig: &elasticache.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int64(30),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
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
				ec2Client: buildMockEc2Client(nil),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.test"),
					SnapshotRetentionLimit:     aws.Int64(30),
					PreferredMaintenanceWindow: aws.String("test"),
					SnapshotWindow:             aws.String("test"),
					EngineVersion:              aws.String("some invalid value"),
				},
				foundConfig: &elasticache.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int64(30),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
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
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return &ec2.DescribeInstanceTypeOfferingsOutput{
							InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
								{
									Location: aws.String("test"),
								},
							},
						}, nil
					}
				}),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.newValue"),
					SnapshotRetentionLimit:     aws.Int64(50),
					PreferredMaintenanceWindow: aws.String("newValue"),
					SnapshotWindow:             aws.String("newValue"),
					EngineVersion:              aws.String(defaultEngineVersion),
				},
				foundConfig: &elasticache.ReplicationGroup{
					CacheNodeType:          aws.String("cache.test"),
					SnapshotRetentionLimit: aws.Int64(30),
					ReplicationGroupId:     aws.String("test-id"),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
					{
						EngineVersion:              aws.String("3.2.6"),
						PreferredMaintenanceWindow: aws.String("test"),
						SnapshotWindow:             aws.String("test"),
						PreferredAvailabilityZone:  aws.String("test"),
					},
				},
				logger: testLogger,
			},
			want: &elasticache.ModifyReplicationGroupInput{
				CacheNodeType:              aws.String("cache.newValue"),
				SnapshotRetentionLimit:     aws.Int64(50),
				PreferredMaintenanceWindow: aws.String("newValue"),
				SnapshotWindow:             aws.String("newValue"),
				ReplicationGroupId:         aws.String("test-id"),
				EngineVersion:              aws.String(defaultEngineVersion),
			},
		},
		{
			name: "test failed aws instance type offering list results in error",
			args: args{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return nil, errors.New("test")
					}
				}),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType: aws.String("cache.test"),
				},
				foundConfig: &elasticache.ReplicationGroup{
					CacheNodeType:          aws.String("cache.test2"),
					ReplicationGroupId:     aws.String("test-id"),
					SnapshotRetentionLimit: aws.Int64(50),
					SnapshotWindow:         aws.String("newValue"),
				},
				replicationGroupClusters: []elasticache.CacheCluster{},
				logger:                   testLogger,
			},
			want:    nil,
			wantErr: "failed to get instance type offerings for type cache.test2: test",
		},
		{
			name: "test unsupported instance types changes are not added to proposed modification",
			args: args{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return &ec2.DescribeInstanceTypeOfferingsOutput{
							InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
								{
									Location: aws.String("current-cache-type"),
								},
							},
						}, nil
					}
				}),
				elasticacheConfig: &elasticache.CreateReplicationGroupInput{
					CacheNodeType:              aws.String("cache.unsupported-cache-type"),
					SnapshotRetentionLimit:     aws.Int64(50),
					PreferredMaintenanceWindow: aws.String("newValue"),
					SnapshotWindow:             aws.String("newValue"),
					EngineVersion:              aws.String(defaultEngineVersion),
				},
				foundConfig: &elasticache.ReplicationGroup{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("cache.current-cache-type"),
					SnapshotRetentionLimit: aws.Int64(30),
				},
				replicationGroupClusters: []elasticache.CacheCluster{
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
				SnapshotRetentionLimit:     aws.Int64(50),
				PreferredMaintenanceWindow: aws.String("newValue"),
				SnapshotWindow:             aws.String("newValue"),
				ReplicationGroupId:         aws.String("test-id"),
				EngineVersion:              aws.String(defaultEngineVersion),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildElasticacheUpdateStrategy(tt.args.ec2Client, tt.args.elasticacheConfig, tt.args.foundConfig, tt.args.replicationGroupClusters, tt.args.logger)
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

func TestRedisProvider_checkSpecifiedSecurityUpdates(t *testing.T) {
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
		CacheSvc          elasticacheiface.ElastiCacheAPI
		TCPPinger         ConnectionTester
	}
	type args struct {
		cacheSvc         *mockElasticacheClient
		replicationGroup *elasticache.ReplicationGroup
		specifiedUpdates *ServiceUpdate
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      bool
		wantErr   bool
		checkfunc func(t *testing.T, cacheSvc *mockElasticacheClient)
	}{
		{
			name: "if a specified update that is critical security update it should be applied immediately",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.modifyReplicationGroupFn = func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
						return &elasticache.ModifyReplicationGroupOutput{}, nil
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 1 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
		},
		{
			name: "expect specified update that is not critical security update to be batch applied but not modified",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityImportant),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
		},
		{
			name: "expect batchupdate to be called but not modify for a specified update that is critical but not security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String("othertype"),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityImportant),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
		},
		{
			name: "expect modify and batchapply not to be called if a non specified update that is critical and is security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 0 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
		},
		{
			name: "expect modify and batchapply not to be called if specified critical security update is already complete",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusComplete),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 0 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
		},
		{
			name: "expect modify to not be called if there is an unprocessed update action returned by batchapplyupdate for update that is critical security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.modifyReplicationGroupFn = func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
						return &elasticache.ModifyReplicationGroupOutput{}, nil
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{
							UnprocessedUpdateActions: []*elasticache.UnprocessedUpdateAction{
								{
									CacheClusterId: aws.String("test-replication-group"),
									ErrorMessage:   aws.String("The update action is not in a valid status"),
									ErrorType:      aws.String(elasticache.ErrCodeInvalidParameterValueException),
								},
							},
						}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
			wantErr: true,
		},
		{
			name: "expect modify to not be called if there is an error returned by batchapplyupdate for update that is critical security update",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.modifyReplicationGroupFn = func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
						return &elasticache.ModifyReplicationGroupOutput{}, nil
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{}, errors.New("Random error")
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 0 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
			},
			wantErr: true,
		},
		{
			name: "expect an error if ModifyReplicationGroup return error",
			fields: fields{
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				CacheSvc:          nil,
				TCPPinger:         buildMockConnectionTester(),
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
			},
			args: args{
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker: nil,
							UpdateActions: []*elasticache.UpdateAction{
								{
									ServiceUpdateName:     aws.String("test-service-update"),
									ServiceUpdateType:     aws.String(elasticache.ServiceUpdateTypeSecurityUpdate),
									ServiceUpdateSeverity: aws.String(elasticache.ServiceUpdateSeverityCritical),
									UpdateActionStatus:    aws.String(elasticache.UpdateActionStatusScheduling),
									ServiceUpdateStatus:   aws.String(elasticache.ServiceUpdateStatusAvailable),
								},
							},
						}, nil
					}
					elasticacheClient.modifyReplicationGroupFn = func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
						return &elasticache.ModifyReplicationGroupOutput{}, errors.New("Modify error")
					}
					elasticacheClient.batchApplyUpdateActionFn = func(input *elasticache.BatchApplyUpdateActionInput) (*elasticache.BatchApplyUpdateActionOutput, error) {
						return &elasticache.BatchApplyUpdateActionOutput{}, nil
					}
				}),
				replicationGroup: &elasticache.ReplicationGroup{
					ReplicationGroupId: aws.String("test-replication-group"),
				},
				specifiedUpdates: &ServiceUpdate{updates: []string{"test-service-update"}},
			},
			checkfunc: func(t *testing.T, cacheSvc *mockElasticacheClient) {
				if len(cacheSvc.calls.ModifyReplicationGroup) != 1 {
					t.Errorf("expected ModifyReplicationGroup Function to be called 0 time but was called '%d' times", len(cacheSvc.calls.ModifyReplicationGroup))
				}
				if len(cacheSvc.calls.BatchApplyUpdateAction) != 1 {
					t.Errorf("expected BatchApplyUpdateAction Function to be called 1 time but was called '%d' times", len(cacheSvc.calls.BatchApplyUpdateAction))
				}
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
				CacheSvc:          tt.fields.CacheSvc,
				TCPPinger:         tt.fields.TCPPinger,
			}
			err := p.checkSpecifiedSecurityUpdates(tt.args.cacheSvc, tt.args.replicationGroup, tt.args.specifiedUpdates)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkSpecifiedSecurityUpdates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.checkfunc(t, tt.args.cacheSvc)
		})
	}
}
