package aws

import (
	"context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	v12 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockRdsClient struct {
	rdsiface.RDSAPI
	wantErrList   bool
	wantErrCreate bool
	wantErrDelete bool
	wantEmpty     bool
	dbInstances   []*rds.DBInstance
}

func (m *mockRdsClient) DescribeDBInstances(*rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
	if m.wantEmpty {
		return &rds.DescribeDBInstancesOutput{}, nil
	}
	return &rds.DescribeDBInstancesOutput{
		DBInstances: m.dbInstances,
	}, nil
}

func (m *mockRdsClient) CreateDBInstance(*rds.CreateDBInstanceInput) (*rds.CreateDBInstanceOutput, error) {
	return &rds.CreateDBInstanceOutput{}, nil
}

func (m *mockRdsClient) ModifyDBInstance(*rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error) {
	return &rds.ModifyDBInstanceOutput{}, nil
}

func (m *mockRdsClient) DeleteDBInstance(*rds.DeleteDBInstanceInput) (*rds.DeleteDBInstanceOutput, error) {
	return &rds.DeleteDBInstanceOutput{}, nil
}

func (m *mockRdsClient) AddTagsToResource(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
	return &rds.AddTagsToResourceOutput{}, nil
}

func (m *mockRdsClient) DescribeDBSnapshots(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
	return &rds.DescribeDBSnapshotsOutput{}, nil
}

func buildTestPostgresCR() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
	}
}

func buildTestInfra() *v12.Infrastructure {
	return &v12.Infrastructure{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Status: v12.InfrastructureStatus{
			InfrastructureName: "test",
		},
	}
}

func builtTestCredSecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test-aws-rds-credentials",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"user":     []byte("postgres"),
			"password": []byte("test"),
		},
	}
}

func buildDbInstanceGroupPending() []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier: aws.String("test-id"),
			AvailabilityZone:     aws.String("test-availabilityZone"),
			DBInstanceStatus:     aws.String("pending"),
		},
	}
}

func buildDbInstanceGroupAvailable() []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier: aws.String("test-id"),
			DBInstanceStatus:     aws.String("available"),
			AvailabilityZone:     aws.String("test-availabilityZone"),
			DeletionProtection:   aws.Bool(false),
		},
	}
}

func buildDbInstanceDeletionProtection() []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier: aws.String("test-id"),
			DBInstanceStatus:     aws.String("available"),
			AvailabilityZone:     aws.String("test-availabilityZone"),
			DeletionProtection:   aws.Bool(true),
		},
	}
}

func TestAWSPostgresProvider_createPostgresInstance(t *testing.T) {
	scheme, err := buildTestScheme()
	testIdentifier := "test-identifier"
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx         context.Context
		cr          *v1alpha1.Postgres
		rdsSvc      rdsiface.RDSAPI
		postgresCfg *rds.CreateDBInstanceInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.PostgresInstance
		wantErr bool
	}{
		{
			name: "test rds is created",
			args: args{
				rdsSvc:      &mockRdsClient{dbInstances: []*rds.DBInstance{}},
				ctx:         context.TODO(),
				cr:          buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds is exists and is available",
			args: args{
				rdsSvc: &mockRdsClient{dbInstances: []*rds.DBInstance{
					{
						DBInstanceIdentifier:  aws.String(testIdentifier),
						DBInstanceStatus:      aws.String("available"),
						AvailabilityZone:      aws.String("test-availabilityZone"),
						DBInstanceArn:         aws.String("arn-test"),
						DeletionProtection:    aws.Bool(defaultAwsPostgresDeletionProtection),
						MasterUsername:        aws.String(defaultAwsPostgresUser),
						DBName:                aws.String(defaultAwsPostgresDatabase),
						BackupRetentionPeriod: aws.Int64(defaultAwsBackupRetentionPeriod),
						DBInstanceClass:       aws.String(defaultAwsDBInstanceClass),
						PubliclyAccessible:    aws.Bool(defaultAwsPubliclyAccessible),
						AllocatedStorage:      aws.Int64(defaultAwsAllocatedStorage),
						EngineVersion:         aws.String(defaultAwsEngineVersion),
						Engine:                aws.String(defaultAwsEngine),
						Endpoint: &rds.Endpoint{
							Address:      aws.String("blob"),
							HostedZoneId: aws.String("blog"),
							Port:         aws.Int64(defaultAwsPostgresPort),
						},
					},
				}},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want: &providers.PostgresInstance{DeploymentDetails: &providers.PostgresDeploymentDetails{
				Username: defaultAwsPostgresUser,
				Password: "test",
				Host:     "blob",
				Database: defaultAwsEngine,
				Port:     defaultAwsPostgresPort,
			}},
			wantErr: false,
		},
		{
			name: "test rds needs to be modified",
			args: args{
				rdsSvc: &mockRdsClient{dbInstances: []*rds.DBInstance{
					{
						DBInstanceIdentifier:  aws.String(testIdentifier),
						DBInstanceStatus:      aws.String("available"),
						DeletionProtection:    aws.Bool(defaultAwsPostgresDeletionProtection),
						MasterUsername:        aws.String("newmasteruser"),
						DBName:                aws.String(defaultAwsPostgresDatabase),
						BackupRetentionPeriod: aws.Int64(defaultAwsBackupRetentionPeriod),
						DBInstanceClass:       aws.String(defaultAwsDBInstanceClass),
						PubliclyAccessible:    aws.Bool(defaultAwsPubliclyAccessible),
						AllocatedStorage:      aws.Int64(defaultAwsAllocatedStorage),
						EngineVersion:         aws.String("9.6"),
						Engine:                aws.String(defaultAwsEngine),
						Endpoint: &rds.Endpoint{
							Address:      aws.String("blob"),
							HostedZoneId: aws.String("blog"),
							Port:         aws.Int64(defaultAwsPostgresPort),
						},
					},
				}},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSPostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, _, err := p.createRDSInstance(tt.args.ctx, tt.args.cr, tt.args.rdsSvc, tt.args.postgresCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRDSInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			fmt.Println(got)
			fmt.Println(tt.want)
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createRDSInstance() got = %+v, want %v", got.DeploymentDetails, tt.want)
			}
		})
	}
}

func TestAWSPostgresProvider_deletePostgresInstance(t *testing.T) {
	scheme, err := buildTestScheme()
	testIdentifier := "test-id"
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx                  context.Context
		pg                   *v1alpha1.Postgres
		instanceSvc          rdsiface.RDSAPI
		postgresCreateConfig *rds.CreateDBInstanceInput
		postgresDeleteConfig *rds.DeleteDBInstanceInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "test successful delete with no postgres",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{},
				postgresCreateConfig: &rds.CreateDBInstanceInput{},
				pg:                   buildTestPostgresCR(),
				instanceSvc:          &mockRdsClient{dbInstances: []*rds.DBInstance{}},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage(""),
			wantErr: false,
		}, {
			name: "test successful delete with existing unavailable postgres",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				postgresCreateConfig: &rds.CreateDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				pg:                   buildTestPostgresCR(),
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceGroupPending()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage("delete detected, deleteDBInstance() in progress, current aws rds status is pending"),
			wantErr: false,
		}, {
			name: "test successful delete with existing available postgres",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				postgresCreateConfig: &rds.CreateDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				pg:                   buildTestPostgresCR(),
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceGroupAvailable()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage("delete detected, deleteDBInstance() started"),
			wantErr: false,
		}, {
			name: "test successful delete with existing available postgres and deletion protection",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				postgresCreateConfig: &rds.CreateDBInstanceInput{DBInstanceIdentifier: aws.String(testIdentifier)},
				pg:                   buildTestPostgresCR(),
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceDeletionProtection()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage("deletion protection detected, modifyDBInstance() in progress, current aws rds status is available"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSPostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.deleteRDSInstance(tt.args.ctx, tt.args.pg, tt.args.instanceSvc, tt.args.postgresCreateConfig, tt.args.postgresDeleteConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteRDSInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("deleteRDSInstance() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSPostgresProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "test short reconcile when the cr is not complete",
			args: args{
				p: &v1alpha1.Postgres{
					Status: v1alpha1.PostgresStatus{
						Phase: croType.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				p: &v1alpha1.Postgres{
					Status: v1alpha1.PostgresStatus{
						Phase: croType.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSPostgresProvider{}
			if got := p.GetReconcileTime(tt.args.p); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSPostgresProvider_TagRDSPostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	testIdentifier := "test-id"
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx           context.Context
		cr            *v1alpha1.Postgres
		rdsSvc        rdsiface.RDSAPI
		foundInstance *rds.DBInstance
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "test tagging is successful",
			args: args{
				ctx:    context.TODO(),
				cr:     buildTestPostgresCR(),
				rdsSvc: &mockRdsClient{dbInstances: []*rds.DBInstance{}},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
					AvailabilityZone:     aws.String("test-availabilityZone"),
					DBInstanceArn:        aws.String("arn:test"),
				},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AWSPostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.TagRDSPostgres(tt.args.ctx, tt.args.cr, tt.args.rdsSvc, tt.args.foundInstance)
			if (err != nil) != tt.wantErr {
				t.Errorf("TagRDSPostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TagRDSPostgres() got = %v, want %v", got, tt.want)
			}
		})
	}
}
