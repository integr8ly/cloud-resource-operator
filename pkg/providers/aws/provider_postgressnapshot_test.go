package aws

import (
	"context"
	"errors"
	"fmt"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/stretchr/testify/mock"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
	"github.com/sirupsen/logrus"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type rdsClientMock struct {
	mock.Mock
	rds.Client
	DescribeDBSnapshotsFunc func(ctx context.Context, input *rds.DescribeDBSnapshotsInput, opts ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error)
	CreateDBSnapshotFunc    func(ctx context.Context, input *rds.CreateDBSnapshotInput, opts ...func(*rds.Options)) (*rds.CreateDBSnapshotOutput, error)
	DeleteDBSnapshotFunc    func(ctx context.Context, in1 *rds.DeleteDBSnapshotInput, opts ...func(*rds.Options)) (*rds.DeleteDBSnapshotOutput, error)
	calls                   struct {
		DescribeDBSnapshots []struct {
			Ctx   context.Context
			Input *rds.DescribeDBSnapshotsInput
		}
		CreateDBSnapshot []struct {
			Ctx   context.Context
			Input *rds.CreateDBSnapshotInput
		}
		DeleteDBSnapshot []struct {
			Ctx   context.Context
			Input *rds.DeleteDBSnapshotInput
		}
	}
}

func (mock *rdsClientMock) CreateDBSnapshot(ctx context.Context, in1 *rds.CreateDBSnapshotInput, opts ...func(*rds.Options)) (*rds.CreateDBSnapshotOutput, error) {
	if mock.CreateDBSnapshotFunc == nil {
		panic("rdsClientMock.CreateDBSnapshot: method is nil but rdsClient.CreateDBSnapshots was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Input *rds.CreateDBSnapshotInput
	}{
		Ctx:   ctx,
		Input: in1,
	}
	mock.calls.CreateDBSnapshot = append(mock.calls.CreateDBSnapshot, callInfo)
	return mock.CreateDBSnapshotFunc(ctx, in1, opts...)
}

func (mock *rdsClientMock) DeleteDBSnapshot(ctx context.Context, in1 *rds.DeleteDBSnapshotInput, opts ...func(*rds.Options)) (*rds.DeleteDBSnapshotOutput, error) {
	if mock.DeleteDBSnapshotFunc == nil {
		panic("rdsClientMock.DeleteDBSnapshot: method is nil but rdsClient.DeleteDBSnapshot was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Input *rds.DeleteDBSnapshotInput
	}{
		Ctx:   ctx,
		Input: in1,
	}
	mock.calls.DeleteDBSnapshot = append(mock.calls.DeleteDBSnapshot, callInfo)
	return mock.DeleteDBSnapshotFunc(ctx, in1, opts...)
}

func (mock *rdsClientMock) DescribeDBSnapshots(ctx context.Context, in1 *rds.DescribeDBSnapshotsInput, opts ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error) {
	if mock.DescribeDBSnapshotsFunc == nil {
		panic("rdsClientMock.DescribeDBSnapshotsFunc: method is nil but rdsClient.DescribeDBSnapshots was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Input *rds.DescribeDBSnapshotsInput
	}{
		Ctx:   ctx,
		Input: in1,
	}
	mock.calls.DescribeDBSnapshots = append(mock.calls.DescribeDBSnapshots, callInfo)
	return mock.DescribeDBSnapshotsFunc(ctx, in1, opts...)
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

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())
	testIdentifier, err := resources.BuildInfraNameFromObject(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build test identifier", err)
	}
	testTimestampedIdentifier, err := resources.BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build timestamped identifier", err)
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
		rdsClient  *rds.Client
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
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
				fakeTags := []rdstypes.Tag{
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
				gotSnapshotInput := mock.calls.CreateDBSnapshot[0].Input
				if !reflect.DeepEqual(gotSnapshotInput, wantSnapshotInput) {
					return fmt.Errorf("wrong CreateDBSnapshotInput got = %+v, want = %+v", gotSnapshotInput, wantSnapshotInput)
				}
				return nil
			},
		},
		{
			name: "test DBSnapshotInstance is returned when DescribeDBSnapshots returns snapshot with status available",
			args: args{
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotIdentifier: &testTimestampedIdentifier,
								Status:               aws.String("available"),
							},
						},
					}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotIdentifier: &testTimestampedIdentifier,
								Status:               aws.String("creating"),
							},
						},
					}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "current snapshot status : creating",
		},
		{
			name: "test an error occurs when describe db snapshots fails",
			args: args{
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{}, errors.New(""))
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, errors.New(""))
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ctx:        context.TODO(),
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
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
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantMsg: "waiting for postgres instance to be available",
		},
		{
			name: "test error occurs when Postgres CR status is PhaseDeleteInProgress",
			args: args{
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{}, nil)
					mockRds.On("CreateDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
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
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra()),
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
			gotSnapshot, gotMsg, err := p.createPostgresSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.postgresCr, *tt.args.rdsClient)
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
				//TODO check whether this works for wantFn
				mockRds := (*rdsClientMock)(unsafe.Pointer(tt.args.rdsClient))
				if err := tt.wantFn(mockRds); err != nil {
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

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())

	testTimestampedIdentifier, err := resources.BuildTimestampedInfraNameFromObjectCreation(context.TODO(), fakeClient, buildTestPostgresSnapshotCr().ObjectMeta, defaultAwsIdentifierLength)

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
		rdsClient  *rds.Client
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotIdentifier: &testTimestampedIdentifier,
								Status:               aws.String("available"),
							},
						},
					}, nil)
					mockRds.On("DeleteDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
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
				gotDeleteSnapshotInput := mock.calls.DeleteDBSnapshot[0].Input
				if !reflect.DeepEqual(gotDeleteSnapshotInput, wantDeleteSnapshotInput) {
					return fmt.Errorf("wrong DeleteDBSnapshotInput got = %+v, want = %+v", gotDeleteSnapshotInput, wantDeleteSnapshotInput)
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{},
					}, nil)
					mockRds.On("DeleteDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSnapshotOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
				snapshotCr: buildTestPostgresSnapshotCr(),
				postgresCr: buildTestPostgresCR(),
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{},
					}, errors.New(""))
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotIdentifier: &testTimestampedIdentifier,
								Status:               aws.String("available"),
							},
						},
					}, nil)
					mockRds.On("DeleteDBSnapshot", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSnapshotOutput{}, errors.New(""))
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
			p := &PostgresSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.deletePostgresSnapshot(tt.args.ctx, tt.args.snapshotCr, tt.args.postgresCr, *tt.args.rdsClient)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("deletePostgresSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deletePostgresSnapshot() got = %+v, want %v", got, tt.want)
			}
			if tt.wantFn != nil {
				//TODO check to see if this works for wantFn
				mockRds := (*rdsClientMock)(unsafe.Pointer(tt.args.rdsClient))
				if err := tt.wantFn(mockRds); err != nil {
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

	fakeClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresSnapshotCr(), builtTestCredSecret(), buildTestInfra())
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
		ctx          context.Context
		rdsClient    *rds.Client
		snapshotName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *rdstypes.DBSnapshot
		wantErr string
	}{
		{
			name: "test findSnapshotInstance returns the snapshotInstance",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotIdentifier: aws.String(testIdentifier),
								Status:               aws.String("available"),
							},
						},
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
			},
			fields: fields{
				Client:            fakeClient,
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: &rdstypes.DBSnapshot{
				DBSnapshotIdentifier: aws.String(testIdentifier),
				Status:               aws.String("available"),
			},
		},
		{
			name: "test returns nil when no snapshots are found",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{},
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
			name: "test an error is returned when DescribeDBSnapshots fails",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{},
					}, errors.New("error msg"))
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
			name: "test an error is not returned when DescribeDBSnapshots fails with a DBSnapshotNotFound error",
			args: args{
				ctx:          context.TODO(),
				snapshotName: testIdentifier,
				rdsClient: func() *rds.Client {
					mockRds := new(rdsClientMock)
					errorMsg := ""
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{},
					}, &rdstypes.DBSnapshotNotFoundFault{
						Message: aws.String(errorMsg),
					})
					return (*rds.Client)(unsafe.Pointer(mockRds))
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
			p := &PostgresSnapshotProvider{
				client:            tt.fields.Client,
				logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.findSnapshotInstance(tt.args.ctx, *tt.args.rdsClient, tt.args.snapshotName)
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

func TestNewAWSPostgresSnapshotProvider(t *testing.T) {
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
		want    *PostgresSnapshotProvider
		wantErr bool
	}{
		{
			name: "successfully create new postgres snapshot provider",
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
			name: "fail to create new postgres snapshot provider",
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
			got, err := NewAWSPostgresSnapshotProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSPostgresSnapshotProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSPostgresSnapshotProvider() got = %v, want non-nil result", got)
			}
		})
	}
}
