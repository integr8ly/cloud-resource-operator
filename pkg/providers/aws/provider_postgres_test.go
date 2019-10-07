package aws

import (
	"context"
	"reflect"

	v12 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/aws"

	v1 "k8s.io/api/core/v1"

	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"

	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
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
		StringData: map[string]string{
			"user":     "postgres",
			"password": "test",
		},
	}
}

func TestAWSPostgresProvider_createPostgresInstance(t *testing.T) {
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
						DBInstanceIdentifier:  aws.String(defaultAwsDBInstanceIdentifier),
						DBInstanceStatus:      aws.String("available"),
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
					DBInstanceIdentifier: aws.String(defaultAwsDBInstanceIdentifier),
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
				Password: defaultAwsPostgresPassword,
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
						DBInstanceIdentifier:  aws.String(defaultAwsDBInstanceIdentifier),
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
					DBInstanceIdentifier: aws.String(defaultAwsDBInstanceIdentifier),
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
			got, _, err := p.createPostgresInstance(tt.args.ctx, tt.args.cr, tt.args.rdsSvc, tt.args.postgresCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("createPostgresInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createPostgresInstance() got = %+v, want %v", got.DeploymentDetails, tt.want)
			}
		})
	}
}
