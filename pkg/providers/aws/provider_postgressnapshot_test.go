package aws

import (
	"context"
	"errors"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"reflect"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/sirupsen/logrus"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type rdsClientMock struct {
	rdsiface.RDSAPI
	DescribeDBSnapshotsFunc func(in1 *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error)
	CreateDBSnapshotFunc    func(in1 *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error)
	DeleteDBSnapshotFunc    func(in1 *rds.DeleteDBSnapshotInput) (*rds.DeleteDBSnapshotOutput, error)
	calls                   struct {
		DescribeDBSnapshots []struct{ In1 *rds.DescribeDBSnapshotsInput }
		CreateDBSnapshot    []struct{ In1 *rds.CreateDBSnapshotInput }
		DeleteDBSnapshot    []struct{ In1 *rds.DeleteDBSnapshotInput }
	}
}

func (mock *rdsClientMock) CreateDBSnapshot(in1 *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
	if mock.CreateDBSnapshotFunc == nil {
		panic("rdsClientMock.CreateDBSnapshot: method is nil but rdsClient.CreateDBSnapshots was just called")
	}
	callInfo := struct {
		In1 *rds.CreateDBSnapshotInput
	}{
		In1: in1,
	}
	mock.calls.CreateDBSnapshot = append(mock.calls.CreateDBSnapshot, callInfo)
	return mock.CreateDBSnapshotFunc(in1)
}

func (mock *rdsClientMock) DeleteDBSnapshot(in1 *rds.DeleteDBSnapshotInput) (*rds.DeleteDBSnapshotOutput, error) {
	if mock.DeleteDBSnapshotFunc == nil {
		panic("rdsClientMock.DeleteDBSnapshot: method is nil but rdsClient.DeleteDBSnapshot was just called")
	}
	callInfo := struct {
		In1 *rds.DeleteDBSnapshotInput
	}{
		In1: in1,
	}
	mock.calls.DeleteDBSnapshot = append(mock.calls.DeleteDBSnapshot, callInfo)
	return mock.DeleteDBSnapshotFunc(in1)
}

func (mock *rdsClientMock) DescribeDBSnapshots(in1 *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
	if mock.DescribeDBSnapshotsFunc == nil {
		panic("rdsClientMock.DescribeDBSnapshotsFunc: method is nil but rdsClient.DescribeDBSnapshots was just called")
	}
	callInfo := struct {
		In1 *rds.DescribeDBSnapshotsInput
	}{
		In1: in1,
	}
	mock.calls.DescribeDBSnapshots = append(mock.calls.DescribeDBSnapshots, callInfo)
	return mock.DescribeDBSnapshotsFunc(in1)
}

func buildRdsClientMock(modifyFn func(*rdsClientMock)) *rdsClientMock {
	mock := &rdsClientMock{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestPostgresSnapshotCr() *v1alpha1.PostgresSnapshot {
	return &v1alpha1.PostgresSnapshot{
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

// todo tests should be extended when createNetwork is implemented, we should ensure creation of both vpc implementations
func TestAWSPostgresSnapshotProvider_createPostgresSnapshot(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())
	testIdentifier, err := BuildInfraNameFromObject(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)
	testTimestampedIdentifier, err := BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)

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
		snapshotCr *v1alpha1.PostgresSnapshot
		postgresCr *v1alpha1.Postgres
		rdsSvc     *rdsClientMock
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		wantSnapshot *providers.PostgresSnapshotInstance
		wantMsg      croType.StatusMessage
		wantErr      string
		wantFn       func(mock *rdsClientMock) error
	}{
		{
			name: "test rds CreateDBSnapshot is called",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, nil
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
			wantFn: func(mock *rdsClientMock) error {
				if len(mock.calls.CreateDBSnapshot) != 1 {
					return errors.New("CreateDBSnapshot was not called")
				}
				defaultOrgTag := resources.GetOrganizationTag()
				fakeTags := []*rds.Tag{
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
						Key:   aws.String(tagManagedKey),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String(defaultOrgTag + "product-name"),
						Value: aws.String("test_product"),
					},
				}
				wantSnapshotInput := &rds.CreateDBSnapshotInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
					DBSnapshotIdentifier: aws.String(testTimestampedIdentifier),
					Tags:                 fakeTags,
				}
				gotSnapshotInput := mock.calls.CreateDBSnapshot[0].In1
				if !reflect.DeepEqual(gotSnapshotInput, wantSnapshotInput) {
					return errors.New(fmt.Sprintf("wrong CreateDBSnapshotInput got = %+v, want = %+v", gotSnapshotInput, wantSnapshotInput))
				}
				return nil
			},
		},
		{
			name: "test DBSnapshotInstance is returned when DescribeDBSnapshots returns snapshot with status available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotIdentifier: &testTimestampedIdentifier,
									Status:               aws.String("available"),
								},
							},
						}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantSnapshot: &providers.PostgresSnapshotInstance{
				Name: testTimestampedIdentifier,
			},
			wantMsg: "snapshot created",
		},
		{
			name: "test snapshot instance not returned when status is not available",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotIdentifier: &testTimestampedIdentifier,
									Status:               aws.String("creating"),
								},
							},
						}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "current snapshot status : creating",
		},
		{
			name: "test an error occurs when describe db snapshots fails",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{}, errors.New("")
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "failed to describe snaphots in AWS",
			wantErr: "failed to describe snaphots in AWS: ",
		},
		{
			name: "test an error occurs when CreateDbSnapshot fails",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, errors.New("")
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "error creating rds snapshot",
			wantErr: "error creating rds snapshot: ",
		},
		{
			name: "test skips creation when Postgres CR status is InProgress",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: &v1alpha1.Postgres{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseInProgress,
					},
				},
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "waiting for postgres instance to be available",
		},
		{
			name: "test error occurs when Postgres CR status is PhaseDeleteInProgress",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: &v1alpha1.Postgres{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseDeleteInProgress,
					},
				},
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{}, nil
					}
					mock.CreateDBSnapshotFunc = func(in *rds.CreateDBSnapshotInput) (*rds.CreateDBSnapshotOutput, error) {
						return &rds.CreateDBSnapshotOutput{}, nil
					}
				}),
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "cannot create snapshot when instance deletion is in progress",
			wantErr: "cannot create snapshot when instance deletion is in progress",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			gotSnapshot, gotMsg, err := p.createPostgresSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.postgresCr, tt.args.rdsSvc)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("createPostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMsg, tt.wantMsg) {
				t.Errorf("createPostgresSnapshot() got = %+v, want %v", gotMsg, tt.wantMsg)
			}
			if tt.wantSnapshot != nil && !reflect.DeepEqual(tt.wantSnapshot, gotSnapshot) {
				t.Errorf("createPostgresSnapshot() got = %+v, want %v", gotSnapshot, tt.wantSnapshot)
			}
			if tt.wantFn != nil {
				if err := tt.wantFn(tt.args.rdsSvc); err != nil {
					t.Errorf("createPostgresSnapshot() err = %v", err)
				}
			}
		})
	}
}

func TestAWSPostgresSnapshotProvider_deletePostgresSnapshot(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())

	testTimestampedIdentifier, err := BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)

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
		snapshotCr *v1alpha1.PostgresSnapshot
		postgresCr *v1alpha1.Postgres
		rdsSvc     *rdsClientMock
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr string
		wantFn  func(mock *rdsClientMock) error
	}{
		{
			name: "test rds DeleteDBSnapshot is called",
			args: args{
				ctx: context.TODO(),
				snapshotCr: &v1alpha1.PostgresSnapshot{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeSnapshotStatus{
						SnapshotID: testTimestampedIdentifier,
					},
				},
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotIdentifier: &testTimestampedIdentifier,
									Status:               aws.String("available"),
								},
							},
						}, nil
					}
					mock.DeleteDBSnapshotFunc = func(in *rds.DeleteDBSnapshotInput) (*rds.DeleteDBSnapshotOutput, error) {
						return &rds.DeleteDBSnapshotOutput{}, nil
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
			wantFn: func(mock *rdsClientMock) error {
				if len(mock.calls.DeleteDBSnapshot) != 1 {
					return errors.New("DeleteDBSnapshot was not called")
				}
				wantDeleteSnapshotInput := &rds.DeleteDBSnapshotInput{
					DBSnapshotIdentifier: aws.String(testTimestampedIdentifier),
				}
				gotDeleteSnapshotInput := mock.calls.DeleteDBSnapshot[0].In1
				if !reflect.DeepEqual(gotDeleteSnapshotInput, wantDeleteSnapshotInput) {
					return errors.New(fmt.Sprintf("wrong DeleteDBSnapshotInput got = %+v, want = %+v", gotDeleteSnapshotInput, wantDeleteSnapshotInput))
				}
				return nil
			},
		},
		{
			name: "test returns snapshot deleted when snapshot instance is not found",
			args: args{
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{},
						}, nil
					}
					mock.DeleteDBSnapshotFunc = func(in *rds.DeleteDBSnapshotInput) (*rds.DeleteDBSnapshotOutput, error) {
						return &rds.DeleteDBSnapshotOutput{}, nil
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
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{},
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
			name: "test an error is returned when DeleteDBSnapshot fails",
			args: args{
				ctx: context.TODO(),
				snapshotCr: &v1alpha1.PostgresSnapshot{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Status: croType.ResourceTypeSnapshotStatus{
						SnapshotID: testTimestampedIdentifier,
					},
				},
				postgresCr: buildTestPostgresCR(),
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotIdentifier: &testTimestampedIdentifier,
									Status:               aws.String("available"),
								},
							},
						}, nil
					}
					mock.DeleteDBSnapshotFunc = func(in *rds.DeleteDBSnapshotInput) (*rds.DeleteDBSnapshotOutput, error) {
						return &rds.DeleteDBSnapshotOutput{}, errors.New("")
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
			p := &PostgresSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.deletePostgresSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.postgresCr, tt.args.rdsSvc)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("deletePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deletePostgresSnapshot() got = %+v, want %v", got, tt.want)
			}
			if tt.wantFn != nil {
				if err := tt.wantFn(tt.args.rdsSvc); err != nil {
					t.Errorf("deletePostgresSnapshot() err = %v", err)
				}
			}
		})
	}
}

func TestAWSPostgresSnapshotProvider_findSnapshotInstance(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()

	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())
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
		rdsSvc       *rdsClientMock
		snapshotName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *rds.DBSnapshot
		wantErr string
	}{
		{
			name: "test findSnapshotInstance returns the snapshotInstance",
			args: args{
				snapshotName: testIdentifier,
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotIdentifier: aws.String(testIdentifier),
									Status:               aws.String("available"),
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
			want: &rds.DBSnapshot{
				DBSnapshotIdentifier: aws.String(testIdentifier),
				Status:               aws.String("available"),
			},
		},
		{
			name: "test returns nil when no snapshots are found",
			args: args{
				snapshotName: testIdentifier,
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{},
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
			name: "test an error is returned when DescribeDBSnapshots fails",
			args: args{
				snapshotName: testIdentifier,
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{},
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
			name: "test an error is not returned when DescribeDBSnapshots fails with a DBSnapshotNotFound error",
			args: args{
				snapshotName: testIdentifier,
				rdsSvc: buildRdsClientMock(func(mock *rdsClientMock) {
					mock.DescribeDBSnapshotsFunc = func(in *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						errorMsg := ""
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{},
						}, awserr.New("DBSnapshotNotFound", errorMsg, errors.New(errorMsg))
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
			p := &PostgresSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.findSnapshotInstance(tt.args.rdsSvc, tt.args.snapshotName)
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
