package aws

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/sirupsen/logrus"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
		ReplicationGroups: []*elasticache.ReplicationGroup{
			{
				Status: aws.String(status),
				NodeGroups: []*elasticache.NodeGroup{
					{
						NodeGroupMembers: []*elasticache.NodeGroupMember{
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build()

	// testIdentifier, err := BuildInfraNameFromObject(context.TODO(), fakeClient, buildTestRedisSnapshotCR().ObjectMeta, DefaultAwsIdentifierLength)
	testTimestampedIdentifier, err := BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestRedisSnapshotCR().ObjectMeta, DefaultAwsIdentifierLength)

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
		ctx        context.Context
		snapshotCr *v1alpha1.RedisSnapshot
		redisCr    *v1alpha1.Redis
		cacheSvc   *mockElasticacheClient
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		wantSnapshot *providers.RedisSnapshotInstance
		wantMsg      croType.StatusMessage
		wantErr      string
		wantFn       func(mock *mockElasticacheClient) error
	}{
		{
			name: "test elasticache CreateSnapshot is called",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{}, nil
					}
					mock.describeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil
					}
					mock.createSnapshotFn = func(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
						return &elasticache.CreateSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantSnapshot: nil,
			wantMsg:      "snapshot started",
			wantFn: func(mock *mockElasticacheClient) error {
				if len(mock.calls.CreateSnapshot) != 1 {
					return errors.New("CreateSnapshot was not called")
				}
				wantSnapshotInput := &elasticache.CreateSnapshotInput{
					CacheClusterId: aws.String(testPrimaryCacheNodeId),
					SnapshotName:   aws.String(testTimestampedIdentifier),
				}
				gotSnapshotInput := mock.calls.CreateSnapshot[0].In1
				if !reflect.DeepEqual(gotSnapshotInput, wantSnapshotInput) {
					return errors.New(fmt.Sprintf("wrong CreateSnapshotInput got = %+v, want = %+v", gotSnapshotInput, wantSnapshotInput))
				}
				return nil
			},
		},
		{
			name: "test SnapshotInstance is returned when DescribeSnapshots returns snapshot with status available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{
								{
									SnapshotName:   &testTimestampedIdentifier,
									SnapshotStatus: aws.String("available"),
								},
							},
						}, nil
					}
					mock.describeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil
					}
					mock.createSnapshotFn = func(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
						return &elasticache.CreateSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build(),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{
								{
									SnapshotName:   &testTimestampedIdentifier,
									SnapshotStatus: aws.String("creating"),
								},
							},
						}, nil
					}
					mock.describeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil
					}
					mock.createSnapshotFn = func(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
						return &elasticache.CreateSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build(),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(in *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{}, errors.New("")
					}
				}),
			},
			fields: fields{
				Client:            fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build(),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{}, nil
					}
					mock.describeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return buildDescribeReplicationGroupsOutput(testReplicationGroupStatusAvailable), nil
					}
					mock.createSnapshotFn = func(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
						return &elasticache.CreateSnapshotOutput{}, errors.New("")
					}
				}),
			},
			fields: fields{
				Client:            fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build(),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{}, nil
					}
					mock.describeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return buildDescribeReplicationGroupsOutput(testReplicationGroupStatusNotAvailable), nil
					}
					mock.createSnapshotFn = func(input *elasticache.CreateSnapshotInput) (*elasticache.CreateSnapshotOutput, error) {
						return &elasticache.CreateSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build(),
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
			gotSnapshot, gotMsg, err := p.createRedisSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.redisCr, tt.args.cacheSvc)
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
			if tt.wantFn != nil {
				if err := tt.wantFn(tt.args.cacheSvc); err != nil {
					t.Errorf("createPostgresSnapshot() err = %v", err)
				}
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build()

	testTimestampedIdentifier, err := BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestRedisSnapshotCR().ObjectMeta, DefaultAwsIdentifierLength)

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
		ctx        context.Context
		snapshotCr *v1alpha1.RedisSnapshot
		redisCr    *v1alpha1.Redis
		cacheSvc   *mockElasticacheClient
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr string
		wantFn  func(mock *mockElasticacheClient) error
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{
								{
									SnapshotName:   &testTimestampedIdentifier,
									SnapshotStatus: aws.String("available"),
								},
							},
						}, nil
					}
					mock.deleteSnapshotFn = func(input *elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error) {
						return &elasticache.DeleteSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: "snapshot deletion started",
			wantFn: func(mock *mockElasticacheClient) error {
				if len(mock.calls.DeleteSnapshot) != 1 {
					return errors.New("DeleteSnapshot was not called")
				}
				wantDeleteSnapshotInput := &elasticache.DeleteSnapshotInput{
					SnapshotName: aws.String(testTimestampedIdentifier),
				}
				gotDeleteSnapshotInput := mock.calls.DeleteSnapshot[0].In1
				if !reflect.DeepEqual(gotDeleteSnapshotInput, wantDeleteSnapshotInput) {
					return errors.New(fmt.Sprintf("wrong DeleteSnapshotInput got = %+v, want = %+v", gotDeleteSnapshotInput, wantDeleteSnapshotInput))
				}
				return nil
			},
		},
		{
			name: "test returns snapshot deleted when snapshot instance is not found",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestRedisSnapshotCR(),
				redisCr:    buildTestRedisCR(),
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{},
						}, nil
					}
					mock.deleteSnapshotFn = func(input *elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error) {
						return &elasticache.DeleteSnapshotOutput{}, nil
					}
				}),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{},
						}, errors.New("")
					}
				}),
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
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{
								{
									SnapshotName:   &testTimestampedIdentifier,
									SnapshotStatus: aws.String("available"),
								},
							},
						}, nil
					}
					mock.deleteSnapshotFn = func(input *elasticache.DeleteSnapshotInput) (*elasticache.DeleteSnapshotOutput, error) {
						return &elasticache.DeleteSnapshotOutput{}, errors.New("")
					}
				}),
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
			got, err := p.deleteRedisSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.redisCr, tt.args.cacheSvc)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("deletePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deletePostgresSnapshot() got = %+v, want %v", got, tt.want)
			}
			if tt.wantFn != nil {
				if err := tt.wantFn(tt.args.cacheSvc); err != nil {
					t.Errorf("deletePostgresSnapshot() err = %v", err)
				}
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildTestRedisCR(), buildTestRedisSnapshotCR(), builtTestCredSecret(), buildTestInfra()).Build()
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
		cacheSvc     *mockElasticacheClient
		snapshotName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *elasticache.Snapshot
		wantErr string
	}{
		{
			name: "test findSnapshotInstance returns the snapshotInstance",
			args: args{
				snapshotName: testIdentifier,
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{
								{
									SnapshotName:   aws.String(testIdentifier),
									SnapshotStatus: aws.String("available"),
								},
							},
						}, nil
					}
				}),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: &elasticache.Snapshot{
				SnapshotName:   aws.String(testIdentifier),
				SnapshotStatus: aws.String("available"),
			},
		},
		{
			name: "test returns nil when no snapshots are found",
			args: args{
				snapshotName: testIdentifier,
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{},
						}, nil
					}
				}),
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
				snapshotName: testIdentifier,
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{},
						}, errors.New("error msg")
					}
				}),
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
				snapshotName: testIdentifier,
				cacheSvc: buildMockElasticacheClient(func(mock *mockElasticacheClient) {
					mock.describeSnapshotsFn = func(input *elasticache.DescribeSnapshotsInput) (*elasticache.DescribeSnapshotsOutput, error) {
						errorMsg := ""
						return &elasticache.DescribeSnapshotsOutput{
							Snapshots: []*elasticache.Snapshot{},
						}, awserr.New("SnapshotNotFound", errorMsg, errors.New(errorMsg))
					}
				}),
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
			got, err := p.findSnapshotInstance(tt.args.cacheSvc, tt.args.snapshotName)
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
