package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	croApis "github.com/integr8ly/cloud-resource-operator/apis"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	cloudCredentialApis "github.com/openshift/cloud-credential-operator/pkg/apis"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	testLogger   = logrus.WithFields(logrus.Fields{"testing": "true"})
	testAddress  = aws.String("redis")
	testPort     = aws.Int64(6397)
	snapshotName = "test-snapshot"
)

type mockElasticacheClient struct {
	elasticacheiface.ElastiCacheAPI
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
	addTagsToResourceFn         func(*elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error)
	createReplicationGroupFn    func(*elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error)
	calls                       struct {
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
		CreateReplicationGroup []struct {
			In1 *elasticache.CreateReplicationGroupInput
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
	scheme := runtime.NewScheme()
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
func (m *mockElasticacheClient) CreateReplicationGroup(input *elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
	if m.createReplicationGroupFn == nil {
		panic("createReplicationGroupFn: method is nil but elasticacheClient.CreateReplicationGroup was just called")
	}
	callInfo := struct {
		In1 *elasticache.CreateReplicationGroupInput
	}{
		In1: input,
	}
	m.calls.CreateReplicationGroup = append(m.calls.CreateReplicationGroup, callInfo)
	return m.createReplicationGroupFn(input)
}

// mock elasticache DeleteReplicationGroup output
func (m *mockElasticacheClient) DeleteReplicationGroup(*elasticache.DeleteReplicationGroupInput) (*elasticache.DeleteReplicationGroupOutput, error) {
	return &elasticache.DeleteReplicationGroupOutput{}, nil
}

// mock elasticache AddTagsToResource output
func (m *mockElasticacheClient) AddTagsToResource(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
	if resources.SafeStringDereference(input.ResourceName) == "arn:aws:elasticache:tes:test:cluster:test" {
		return &elasticache.TagListMessage{}, nil
	} else {
		return m.addTagsToResourceFn(input)
	}
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
			Name:            "test",
			Namespace:       "test",
			ResourceVersion: fakeResourceVersion,
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
	secName, err := resources.BuildInfraName(context.TODO(), moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()), defaultSecurityGroupPostfix, defaultAwsIdentifierLength)
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
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
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return nil, genericAWSError
					}
				}),
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
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return nil, genericAWSError
					}
					elasticacheClient.createReplicationGroupFn = func(input *elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRSixteen),
								buildValidStandaloneVPC(validCIDRSixteen),
							},
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidNonTaggedStandaloneVPC(validCIDRSixteen),
							},
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("nonexistentcachesubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidNonTaggedStandaloneVPC(validCIDRSixteen),
							},
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									State:    aws.String("available"),
									ZoneName: aws.String("new-zone"),
								},
							},
						}, nil
					}
					ec2Client.createSubnetFn = func(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("testsubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("testsubnetgroup"),
								},
							},
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.createSecurityGroupFn = func(input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								{
									CacheSubnetGroupName: aws.String("testsubnetgroup"),
								},
							},
						}, nil
					}
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return &elasticache.DescribeUpdateActionsOutput{
							Marker:        nil,
							UpdateActions: []*elasticache.UpdateAction{},
						}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(input *elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return nil, genericAWSError
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						sgs := buildSecurityGroups(secName)
						sgs[0].IpPermissions = []*ec2.IpPermission{
							{
								IpProtocol: aws.String("-1"),
								IpRanges: []*ec2.IpRange{
									{
										CidrIp: aws.String("10.0.0.0/16"),
									},
								},
							},
						}
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: sgs,
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
				}),
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
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
					elasticacheClient.createReplicationGroupFn = func(input *elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
						return &elasticache.CreateReplicationGroupOutput{}, nil
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									ZoneName: aws.String("test"),
									State:    aws.String("available"),
								},
							},
						}, nil
					},
				},
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									ZoneName: aws.String("test"),
									State:    aws.String("available"),
								},
							},
						}, nil
					}
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
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
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									ZoneName: aws.String("test"),
									State:    aws.String("available"),
								},
							},
						}, nil
					}
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									ZoneName: aws.String("test"),
									State:    aws.String("available"),
								},
							},
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return nil, genericAWSError
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
					elasticacheClient.modifyReplicationGroupFn = func(input *elasticache.ModifyReplicationGroupInput) (*elasticache.ModifyReplicationGroupOutput, error) {
						return nil, genericAWSError
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
					elasticacheClient.describeUpdateActionsFn = func(input *elasticache.DescribeUpdateActionsInput) (*elasticache.DescribeUpdateActionsOutput, error) {
						return nil, genericAWSError
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				r:      buildTestRedisCR(),
				stsSvc: &mockStsClient{},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.subnets = buildValidBundleSubnets()
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2.AvailabilityZone{
								{
									ZoneName: aws.String("test"),
									State:    aws.String("available"),
								},
							},
						}, nil
					}
				}),
				redisConfig: &elasticache.CreateReplicationGroupInput{
					ReplicationGroupId:     aws.String("test-id"),
					CacheNodeType:          aws.String("test"),
					SnapshotRetentionLimit: aws.Int64(20),
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
											NodeGroupId: aws.String("primary-node"),
											NodeGroupMembers: []*elasticache.NodeGroupMember{
												{
													PreferredAvailabilityZone: aws.String("eu-west-3"),
												},
											},
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
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
					elasticacheClient.describeCacheClustersFn = func(*elasticache.DescribeCacheClustersInput) (*elasticache.DescribeCacheClustersOutput, error) {
						return &elasticache.DescribeCacheClustersOutput{}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					}
				}),
				r:                       buildTestRedisCR(),
				stsSvc:                  &mockStsClient{},
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
			got, _, err := p.createElasticacheCluster(tt.args.ctx, tt.args.r, tt.args.cacheSvc, tt.args.stsSvc, tt.args.ec2Svc, tt.args.redisConfig, tt.args.stratCfg, tt.args.ServiceUpdate, tt.args.standaloneNetworkExists, tt.args.maintenanceWindow)
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
		Ec2Svc            ec2iface.EC2API
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				CacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.describeReplicationGroupsFn = func(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
				Ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRSixteen),
							},
						}, nil
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
			if _, err := p.deleteElasticacheCluster(tt.args.ctx, tt.args.networkManager, tt.fields.CacheSvc, tt.fields.Ec2Svc, tt.args.redisCreateConfig, tt.args.redisDeleteConfig, tt.args.redis, tt.args.standaloneNetworkExists, tt.args.isLastResource); (err != nil) != tt.wantErr {
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
		CacheSvc          elasticacheiface.ElastiCacheAPI
	}
	type args struct {
		ctx              context.Context
		cacheSvc         elasticacheiface.ElastiCacheAPI
		stsSvc           stsiface.STSAPI
		r                *v1alpha1.Redis
		stratCfg         StrategyConfig
		replicationGroup *elasticache.ReplicationGroup
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "test tags reconcile fails with invalid arn",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return nil, fmt.Errorf("%v", "invalid arn")
					}
				}),
				stsSvc:   &mockStsClient{},
				stratCfg: StrategyConfig{Region: "test"},
				replicationGroup: buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
					group.ReplicationGroupId = aws.String("test-id")
					group.Status = aws.String("available")
					group.CacheNodeType = aws.String("test")
					group.NodeGroups = []*elasticache.NodeGroup{
						{
							NodeGroupId: aws.String("primary-node"),
							NodeGroupMembers: []*elasticache.NodeGroupMember{
								{
									PreferredAvailabilityZone: aws.String("1"),
								},
							},
							PrimaryEndpoint: &elasticache.Endpoint{
								Address: testAddress,
								Port:    testPort,
							},
							Status: aws.String("available"),
						},
					}
				},
				),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("failed to add tags to AWS ElastiCache replication group arn:aws:elasticache::test:replicationgroup:test-id: invalid arn"),
			wantErr: true,
		},
		{
			name: "test tags reconcile completes successfully",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return &elasticache.TagListMessage{}, nil
					}
				}),
				stsSvc:   &mockStsClient{},
				stratCfg: StrategyConfig{Region: "test"},
				replicationGroup: buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
					group.ReplicationGroupId = aws.String("test-id")
					group.Status = aws.String("available")
					group.CacheNodeType = aws.String("test")
					group.NodeGroups = []*elasticache.NodeGroup{
						{
							NodeGroupId: aws.String("primary-node"),
							NodeGroupMembers: []*elasticache.NodeGroupMember{
								{
									PreferredAvailabilityZone: aws.String("eu-west-3"),
								},
							},
							PrimaryEndpoint: &elasticache.Endpoint{
								Address: testAddress,
								Port:    testPort,
							},
							Status: aws.String("available"),
						},
					}
				},
				),
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
			name: "test tags already exist",
			args: args{
				ctx: context.TODO(),
				r:   buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.addTagsToResourceFn = func(input *elasticache.AddTagsToResourceInput) (*elasticache.TagListMessage, error) {
						return nil, awserr.New(elasticache.ErrCodeSnapshotAlreadyExistsFault, elasticache.ErrCodeSnapshotAlreadyExistsFault, fmt.Errorf("%v", elasticache.ErrCodeSnapshotAlreadyExistsFault))
					}
				}),
				stsSvc:   &mockStsClient{},
				stratCfg: StrategyConfig{Region: "test"},
				replicationGroup: buildReplicationGroup(func(group *elasticache.ReplicationGroup) {
					group.ReplicationGroupId = aws.String("test-id")
					group.Status = aws.String("available")
					group.CacheNodeType = aws.String("test")
					group.NodeGroups = []*elasticache.NodeGroup{
						{
							NodeGroupId: aws.String("primary-node"),
							NodeGroupMembers: []*elasticache.NodeGroupMember{
								{
									PreferredAvailabilityZone: aws.String("eu-west-3"),
								},
							},
							PrimaryEndpoint: &elasticache.Endpoint{
								Address: testAddress,
								Port:    testPort,
							},
							Status: aws.String("available"),
						},
					}
				},
				),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra()),
				ConfigManager:     &ConfigManagerMock{},
				CredentialManager: &CredentialManagerMock{},
			},
			want:    croType.StatusMessage("Tags already added to AWS ElastiCache replication group"),
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
			got, err := p.TagElasticacheReplicationGroup(tt.args.ctx, tt.args.cacheSvc, tt.args.stsSvc, tt.args.r, tt.args.replicationGroup)
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
				redis:  &v1alpha1.Redis{},
			},
			want: &elasticache.ModifyReplicationGroupInput{
				CacheNodeType:              aws.String("cache.newValue"),
				SnapshotRetentionLimit:     aws.Int64(50),
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
				ApplyImmediately:           aws.Bool(true),
			},
		},
		{
			name: "test nil parameters returned in aws objects",
			args: args{
				ec2Client:                buildMockEc2Client(nil),
				elasticacheConfig:        &elasticache.CreateReplicationGroupInput{},
				foundConfig:              &elasticache.ReplicationGroup{},
				replicationGroupClusters: []elasticache.CacheCluster{},
				logger:                   testLogger,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildElasticacheUpdateStrategy(tt.args.ec2Client, tt.args.elasticacheConfig, tt.args.foundConfig, tt.args.replicationGroupClusters, tt.args.logger, tt.args.redis)
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
		CacheSvc          elasticacheiface.ElastiCacheAPI
		TCPPinger         resources.ConnectionTester
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
						return &elasticache.BatchApplyUpdateActionOutput{}, errors.New("random error")
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
				TCPPinger:         resources.BuildMockConnectionTester(),
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), builtTestCredSecret(), buildTestInfra(), buildTestPrometheusRule()),
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
						return &elasticache.ModifyReplicationGroupOutput{}, errors.New("modify error")
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
			err := p.applySpecifiedSecurityUpdates(tt.args.cacheSvc, tt.args.replicationGroup, tt.args.specifiedUpdates)
			if (err != nil) != tt.wantErr {
				t.Errorf("applylSpecifiedSecurityUpdates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.checkfunc(t, tt.args.cacheSvc)
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
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
