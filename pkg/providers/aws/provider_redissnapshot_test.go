package aws

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/mock"
	"os"
	"reflect"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
	"github.com/sirupsen/logrus"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	testPrimaryCacheNodeId                 = "test-primary"
	testReplicationGroupStatusAvailable    = "available"
	testReplicationGroupStatusNotAvailable = "not available"
	fakeResourceVersion                    = "1000"
)

func buildTestRedisSnapshotCR() *v1alpha1.RedisSnapshot {
	return &v1alpha1.RedisSnapshot{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:            "test",
			Namespace:       "test",
			ResourceVersion: fakeResourceVersion,
		},
		Status: croType.ResourceTypeSnapshotStatus{
			SnapshotID: "test-identifier",
		},
	}
}

func buildDescribeReplicationGroupsOutput(status string) *elasticache.DescribeReplicationGroupsOutput {
	return &elasticache.DescribeReplicationGroupsOutput{
		ReplicationGroups: []elasticachetypes.ReplicationGroup{
			{
				Status: aws.String(status),
				NodeGroups: []elasticachetypes.NodeGroup{
					{
						NodeGroupMembers: []elasticachetypes.NodeGroupMember{
							{
								CacheClusterId: aws.String(testPrimaryCacheNodeId),
								CurrentRole:    aws.String("primary"),
							},
						},
					},
				},
			},
		},
	}
}

// todo tests should be extended when createNetwork is implemented, we should ensure creation of both vpc implementations
func TestAWSRedisSnapshotProvider_createRedisSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra())

	testTimestampedIdentifier, err := resources.BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestRedisSnapshotCR().ObjectMeta, defaultAwsIdentifierLength)

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build test identifier", err)
	}
	defaultOrgTag := resources.GetOrganizationTag()
	expectedFakeTags := []elasticachetypes.Tag{ // Renamed to avoid conflict later
		{
			Key:   aws.String("test-key"),
			Value: aws.String("test-value"),
		},
		{
			Key:   aws.String(defaultOrgTag + "clusterID"),
			Value: aws.String("test"),
		},
		{
			Key:   aws.String(defaultOrgTag + "resource-type"),
			Value: aws.String(""),
		},
		{
			Key:   aws.String(defaultOrgTag + "resource-name"),
			Value: aws.String("testtesttest000101010000000000UTC"),
		},
		{
			Key:   aws.String(resources.TagManagedKey),
			Value: aws.String("true"),
		},
	}
	expectedWantSnapshotInput := &elasticache.CreateSnapshotInput{ // Renamed
		CacheClusterId: aws.String(testPrimaryCacheNodeId), // Assuming testPrimaryCacheNodeId is defined
		SnapshotName:   aws.String(testTimestampedIdentifier),
		Tags:           expectedFakeTags,
	}

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx               context.Context
		snapshotCr        *v1alpha1.RedisSnapshot
		redisCr           *v1alpha1.Redis
		elasticacheClient ElastiCacheAPI
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		wantSnapshot *providers.RedisSnapshotInstance
		wantMsg      croType.StatusMessage
		wantErr      string
	}{
		{
			name: "test elasticache CreateSnapshot is called",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   &testTimestampedIdentifier,
								SnapshotStatus: aws.String("creating"),
							},
						},
					}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(
						buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil)
					mockElasticache.On("CreateSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateSnapshotOutput{
						Snapshot: &elasticachetypes.Snapshot{
							SnapshotName:   &testTimestampedIdentifier,
							SnapshotStatus: aws.String("creating"),
						},
					}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantSnapshot: nil,
			wantMsg:      "current snapshot status : creating",
		},
		{
			name: "test SnapshotInstance is returned when DescribeSnapshots returns snapshot with status available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   &testTimestampedIdentifier,
								SnapshotStatus: aws.String("available"),
							},
						},
					}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil)
					mockElasticache.On("CreateSnapshot",
						mock.AnythingOfType("context.Context"),
						mock.MatchedBy(func(input *elasticache.CreateSnapshotInput) bool {
							isEqual := reflect.DeepEqual(input, expectedWantSnapshotInput)
							if !isEqual {
								t.Errorf("CreateSnapshot input mismatch: got = %+v, want = %+v", input, expectedWantSnapshotInput)
							}
							return isEqual
						}),
						mock.Anything,
					).Return(&elasticache.CreateSnapshotOutput{}, nil).Once()
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantSnapshot: &providers.RedisSnapshotInstance{
				Name: testTimestampedIdentifier,
			},
			wantMsg: "snapshot created",
		},
		{
			name: "test snapshot instance not returned when status is not available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   &testTimestampedIdentifier,
								SnapshotStatus: aws.String("creating"),
							},
						},
					}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil)
					mockElasticache.On("CreateSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateSnapshotOutput{}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "current snapshot status : creating",
		},
		{
			name: "test an error occurs when describe cache snapshots fails",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{}, errors.New(""))
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "failed to describe snaphots in AWS",
			wantErr: "failed to describe snaphots in AWS: ",
		},
		{
			name: "test an error occurs when CreateSnapshot fails",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil)
					mockElasticache.On("CreateSnapshot", mock.Anything, mock.Anything, mock.Anything).Return((*elasticache.CreateSnapshotOutput)(nil), errors.New(""))
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "error creating elasticache snapshot",
			wantErr: "error creating elasticache snapshot: ",
		},
		{
			name: "test skips creation when replication group status not available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr: &v1alpha1.Redis{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseInProgress,
					},
				},
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{}, nil)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything, mock.Anything).Return(buildDescribeReplicationGroupsOutput(testReplicationGroupStatusNotAvailable), nil)
					mockElasticache.On("CreateSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateSnapshotOutput{}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: croType.StatusMessage(fmt.Sprintf("current replication group status is %s", testReplicationGroupStatusNotAvailable)),
			wantErr: fmt.Sprintf("current replication group status is %s: ", testReplicationGroupStatusNotAvailable),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			gotSnapshot, gotMsg, err := p.createRedisSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.redisCr, tt.args.elasticacheClient)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("createPostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMsg, tt.wantMsg) {
				t.Errorf("createPostgresSnapshot() got = %v, want %v", gotMsg, tt.wantMsg)
			}
			if tt.wantSnapshot != nil && !reflect.DeepEqual(tt.wantSnapshot, gotSnapshot) {
				t.Errorf("createPostgresSnapshot() got = %+v, want %+v", gotSnapshot, tt.wantSnapshot)
			}
		})
	}
}

func TestAWSRedisSnapshotProvider_deleteRedisSnapshot(t *testing.T) {
	scheme, err := buildTestScheme()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra())

	testTimestampedIdentifier, err := resources.BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestRedisSnapshotCR().ObjectMeta, defaultAwsIdentifierLength)

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build test identifier", err)
	}

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx               context.Context
		snapshotCr        *v1alpha1.RedisSnapshot
		redisCr           *v1alpha1.Redis
		elasticacheClient ElastiCacheAPI
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr string
	}{
		{
			name: "test elasticache DeleteSnapshot is called",
			args: args{
				ctx: context.TODO(),
				snapshotCr: &v1alpha1.RedisSnapshot{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeSnapshotStatus{
						SnapshotID: testTimestampedIdentifier,
					},
				},
				redisCr: buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   &testTimestampedIdentifier,
								SnapshotStatus: aws.String("available"),
							},
						},
					}, nil)
					wantDeleteSnapshotInput := &elasticache.DeleteSnapshotInput{
						SnapshotName: aws.String(testTimestampedIdentifier),
					}
					mockElasticache.On("DeleteSnapshot", mock.Anything,
						mock.MatchedBy(func(input *elasticache.DeleteSnapshotInput) bool {
							isEqual := reflect.DeepEqual(input, wantDeleteSnapshotInput)
							if !isEqual {
								t.Errorf("DeleteSnapshot input mismatch: got = %+v, want = %+v", input, wantDeleteSnapshotInput)
							}
							return isEqual
						}),
						mock.Anything,
					).Return(&elasticache.DeleteSnapshotOutput{}, nil).Once()
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: "snapshot deletion started",
		},
		{
			name: "test returns snapshot deleted when snapshot instance is not found",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{},
					}, nil)
					mockElasticache.On("DeleteSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteSnapshotOutput{}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: "snapshot deleted",
		},
		{
			name: "test returns error when describing snapshots fails",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{},
					}, errors.New(""))
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want:    "failed to describe snaphots in AWS",
			wantErr: "failed to describe snaphots in AWS: ",
		},
		{
			name: "test an error is returned when DeleteSnapshot fails",
			args: args{
				ctx: context.TODO(),
				snapshotCr: &v1alpha1.RedisSnapshot{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeSnapshotStatus{
						SnapshotID: testTimestampedIdentifier,
					},
				},
				redisCr: buildTestRedisCR(),
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   &testTimestampedIdentifier,
								SnapshotStatus: aws.String("available"),
							},
						},
					}, nil)
					mockElasticache.On("DeleteSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteSnapshotOutput{}, errors.New(""))
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want:    croType.StatusMessage(fmt.Sprintf("failed to delete snapshot %s in aws", testTimestampedIdentifier)),
			wantErr: fmt.Sprintf("failed to delete snapshot %s in aws: ", testTimestampedIdentifier),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.deleteRedisSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.redisCr, tt.args.elasticacheClient)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("deletePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deletePostgresSnapshot() got = %+v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSRedisSnapshotProvider_findSnapshotInstance(t *testing.T) {
	scheme, err := buildTestScheme()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra())
	testIdentifier := "test-identifier"
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build test identifier", err)
	}

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx               context.Context
		elasticacheClient ElastiCacheAPI
		snapshotName      string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *elasticachetypes.Snapshot
		wantErr string
	}{
		{
			name: "test findSnapshotInstance returns the snapshotInstance",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{
							{
								SnapshotName:   aws.String(testIdentifier),
								SnapshotStatus: aws.String("available"),
							},
						},
					}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: &elasticachetypes.Snapshot{
				SnapshotName:   aws.String(testIdentifier),
				SnapshotStatus: aws.String("available"),
			},
		},
		{
			name: "test returns nil when no snapshots are found",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{},
					}, nil)
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: nil,
		},
		{
			name: "test an error is returned when DescribeSnapshots fails",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{},
					}, errors.New("error msg"))
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want:    nil,
			wantErr: "error msg",
		},
		{
			name: "test an error is not returned when DescribeSnapshots fails with a SnapshotNotFound error",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				elasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeSnapshotsOutput{
						Snapshots: []elasticachetypes.Snapshot{},
					}, &elasticachetypes.SnapshotNotFoundFault{
						Message: aws.String(""),
					})
					return mockElasticache
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.findSnapshotInstance(tt.args.ctx, tt.args.elasticacheClient, tt.args.snapshotName)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("findSnapshotInstance() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findSnapshotInstance() got = %+v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAWSRedisSnapshotProvider(t *testing.T) {
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
		want    *RedisSnapshotProvider
		wantErr bool
	}{
		{
			name: "successfully create new redis snapshot provider",
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
			name: "fail to create new redis snapshot provider",
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
			got, err := NewAWSRedisSnapshotProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSRedisSnapshotProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSRedisSnapshotProvider() got = %v, want non-nil result", got)
			}
		})
	}
}
