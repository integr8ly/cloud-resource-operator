package aws

import (
	"context"
	"reflect"
	"testing"
	"time"

	v12 "github.com/integr8ly/cloud-resource-operator/pkg/apis/config/v1"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	croApis "github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	cloudCredentialApis "github.com/openshift/cloud-credential-operator/pkg/apis"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinery "k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testPreferredBackupWindow      = "02:40-03:10"
	testPreferredMaintenanceWindow = "mon:00:29-mon:00:59"
	defaultVPCID                   = "testID"
	dafaultInfraName               = "test"
)

type mockRdsClient struct {
	rdsiface.RDSAPI
	wantErrList   bool
	wantErrCreate bool
	wantErrDelete bool
	wantEmpty     bool
	dbInstances   []*rds.DBInstance
	subnetGroups  []*rds.DBSubnetGroup
}

type mockEc2Client struct {
	ec2iface.EC2API
	subnet      *ec2.Subnet
	subnets     []*ec2.Subnet
	vpcs        []*ec2.Vpc
	vpc         *ec2.Vpc
	secGroups   []*ec2.SecurityGroup
	azs         []*ec2.AvailabilityZone
	wantErrList bool
}

func buildTestSchemePostgresql() (*runtime.Scheme, error) {
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

func buildMockConnectionTester() *ConnectionTesterMock {
	mockTester := &ConnectionTesterMock{}
	mockTester.TCPConnectionFunc = func(host string, port int) bool {
		return true
	}
	return mockTester
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

func (m *mockRdsClient) DescribePendingMaintenanceActions(*rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
	return &rds.DescribePendingMaintenanceActionsOutput{}, nil
}

func (m *mockRdsClient) DescribeDBSubnetGroups(*rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
	return &rds.DescribeDBSubnetGroupsOutput{
		DBSubnetGroups: m.subnetGroups,
	}, nil
}

func (m *mockRdsClient) CreateDBSubnetGroup(*rds.CreateDBSubnetGroupInput) (*rds.CreateDBSubnetGroupOutput, error) {
	return &rds.CreateDBSubnetGroupOutput{}, nil
}

func (m *mockEc2Client) DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{
		Subnets: m.subnets,
	}, nil
}

func (m *mockEc2Client) DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	if m.wantErrList {
		return nil, errorUtil.New("ec2 get vpcs error")
	}
	return &ec2.DescribeVpcsOutput{
		Vpcs: m.vpcs,
	}, nil
}

func (m *mockEc2Client) CreateVpc(*ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
	return &ec2.CreateVpcOutput{
		Vpc: m.vpc,
	}, nil
}

func (m *mockEc2Client) DeleteVpc(*ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
	return &ec2.DeleteVpcOutput{}, nil
}

func (m *mockEc2Client) CreateTags(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	return &ec2.CreateTagsOutput{}, nil
}

func (m *mockEc2Client) CreateSubnet(*ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
	return &ec2.CreateSubnetOutput{
		Subnet: m.subnet,
	}, nil
}

func (m *mockEc2Client) DeleteSubnet(*ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error) {
	return &ec2.DeleteSubnetOutput{}, nil
}

func (m *mockEc2Client) DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	return &ec2.DescribeSecurityGroupsOutput{
		SecurityGroups: m.secGroups,
	}, nil
}

func (m *mockEc2Client) CreateSecurityGroup(*ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	return &ec2.CreateSecurityGroupOutput{}, nil
}

func (m *mockEc2Client) AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (m *mockEc2Client) DescribeAvailabilityZones(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: m.azs,
	}, nil
}

func buildTestPostgresqlPrometheusRule() *monitoringv1.PrometheusRule {
	return &monitoringv1.PrometheusRule{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "availability-rule-test",
			Namespace: "test",
		},
	}
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
			InfrastructureName: dafaultInfraName,
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
			DBInstanceIdentifier:       aws.String("test-id"),
			DBInstanceStatus:           aws.String("available"),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			DeletionProtection:         aws.Bool(false),
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

func buildAvailableDBInstance(testID string) []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
			EngineVersion:              aws.String(defaultAwsEngineVersion),
			Engine:                     aws.String(defaultAwsEngine),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			MultiAZ:                    aws.Bool(true),
			Endpoint: &rds.Endpoint{
				Address:      aws.String("blob"),
				HostedZoneId: aws.String("blog"),
				Port:         aws.Int64(defaultAwsPostgresPort),
			},
		},
	}
}

func buildPendingDBInstance(testID string) []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier: aws.String(testID),
			DBInstanceStatus:     aws.String("pending"),
		},
	}
}

func buildAvailableCreateInput(testID string) *rds.CreateDBInstanceInput {
	return &rds.CreateDBInstanceInput{
		DBInstanceIdentifier:       aws.String(testID),
		DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
		Port:                       aws.Int64(defaultAwsPostgresPort),
		BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
		EngineVersion:              aws.String(defaultAwsEngineVersion),
		PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
		PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
		MultiAZ:                    aws.Bool(true),
	}
}

func buildRequiresModificationsCreateInput(testID string) *rds.CreateDBInstanceInput {
	return &rds.CreateDBInstanceInput{
		DBInstanceIdentifier:       aws.String(testID),
		DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
		Port:                       aws.Int64(123),
		BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
		EngineVersion:              aws.String(defaultAwsEngineVersion),
		PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
		PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
		MultiAZ:                    aws.Bool(true),
	}
}

func buildNewRequiresModificationsCreateInput(testID string) *rds.CreateDBInstanceInput {
	return &rds.CreateDBInstanceInput{
		DBInstanceIdentifier:       aws.String(testID),
		DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
		Port:                       aws.Int64(123),
		BackupRetentionPeriod:      aws.Int64(123),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
		EngineVersion:              aws.String(defaultAwsEngineVersion),
		PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
		PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
		MultiAZ:                    aws.Bool(true),
	}
}

func buildPendingModifiedDBInstance(testID string) []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
			EngineVersion:              aws.String(defaultAwsEngineVersion),
			Engine:                     aws.String(defaultAwsEngine),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			MultiAZ:                    aws.Bool(true),
			Endpoint: &rds.Endpoint{
				Address:      aws.String("blob"),
				HostedZoneId: aws.String("blog"),
				Port:         aws.Int64(defaultAwsPostgresPort),
			},
			PendingModifiedValues: &rds.PendingModifiedValues{
				Port: aws.Int64(123),
			},
		},
	}
}

func buildVpcs() []*ec2.Vpc {
	return []*ec2.Vpc{
		{
			VpcId:     aws.String(defaultVPCID),
			CidrBlock: aws.String("10.0.0.0/16"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String("test-vpc"),
					Value: aws.String("test-vpc"),
				},
			},
		},
	}
}

func buildSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildAZ() []*ec2.AvailabilityZone {
	return []*ec2.AvailabilityZone{
		{
			ZoneName: aws.String("test"),
			State:    aws.String("available"),
		},
	}
}

func buildSecurityGroups(groupName string) []*ec2.SecurityGroup {
	return []*ec2.SecurityGroup{
		{
			GroupName: aws.String(groupName),
			GroupId:   aws.String("testID"),
		},
	}
}

// todo tests should be extended when createNetwork is implemented, we should ensure creation of both vpc implementations
func TestAWSPostgresProvider_createPostgresInstance(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
	testIdentifier := "test-identifier"
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	secName, err := BuildInfraName(context.TODO(), fake.NewFakeClientWithScheme(scheme, buildTestInfra()), defaultSecurityGroupPostfix, DefaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build security name", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         ConnectionTester
	}
	type args struct {
		ctx                     context.Context
		cr                      *v1alpha1.Postgres
		rdsSvc                  rdsiface.RDSAPI
		ec2Svc                  ec2iface.EC2API
		postgresCfg             *rds.CreateDBInstanceInput
		standaloneNetworkExists bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.PostgresInstance
		wantErr bool
	}{
		{
			name: "test rds CreateReplicationGroup is called (valid cluster bundle subnets)",
			args: args{
				rdsSvc:                  &mockRdsClient{dbInstances: []*rds.DBInstance{}},
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             &rds.CreateDBInstanceInput{},
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds is exists and is available (valid cluster bundle subnets)",
			args: args{
				rdsSvc: &mockRdsClient{dbInstances: buildAvailableDBInstance(testIdentifier)},
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:    context.TODO(),
				cr:     buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
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
			name: "test rds is exists and is not available (valid cluster bundle subnets)",
			args: args{
				rdsSvc: &mockRdsClient{dbInstances: buildPendingDBInstance(testIdentifier)},
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:    context.TODO(),
				cr:     buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to be modified (valid cluster bundle subnets)",
			args: args{
				rdsSvc:                  &mockRdsClient{dbInstances: buildAvailableDBInstance(testIdentifier)},
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and does not need to be modified (valid cluster bundle subnets)",
			args: args{
				rdsSvc:                  &mockRdsClient{dbInstances: buildAvailableDBInstance(testIdentifier)},
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildAvailableCreateInput(testIdentifier),
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to be modified but maintenance is pending (valid cluster bundle subnets)",
			args: args{
				rdsSvc:                  &mockRdsClient{dbInstances: buildPendingModifiedDBInstance(testIdentifier)},
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to update pending maintenance (valid cluster bundle subnets)",
			args: args{
				rdsSvc:                  &mockRdsClient{dbInstances: buildPendingModifiedDBInstance(testIdentifier)},
				ec2Svc:                  &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets(), secGroups: buildSecurityGroups(secName), azs: buildAZ()},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildNewRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         buildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         tt.fields.TCPPinger,
			}
			got, _, err := p.createRDSInstance(tt.args.ctx, tt.args.cr, tt.args.rdsSvc, tt.args.ec2Svc, tt.args.postgresCfg, tt.args.standaloneNetworkExists)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRDSInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createRDSInstance() got = %+v, want %v", got.DeploymentDetails, tt.want)
			}
		})
	}
}

func TestAWSPostgresProvider_deletePostgresInstance(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
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
		networkManager       NetworkManager
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
				networkManager:       &mockNetworkManager{},
				instanceSvc:          &mockRdsClient{dbInstances: []*rds.DBInstance{}},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       &mockNetworkManager{},
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceGroupPending()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       &mockNetworkManager{},
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceGroupAvailable()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       &mockNetworkManager{},
				instanceSvc:          &mockRdsClient{dbInstances: buildDbInstanceDeletionProtection()},
			},
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
			p := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.deleteRDSInstance(tt.args.ctx, tt.args.pg, tt.args.networkManager, tt.args.instanceSvc, tt.args.postgresCreateConfig, tt.args.postgresDeleteConfig)
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
			p := &PostgresProvider{}
			if got := p.GetReconcileTime(tt.args.p); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSPostgresProvider_TagRDSPostgres(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
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
			want:    croType.StatusMessage("successfully created and tagged"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresProvider{
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

func Test_buildRDSUpdateStrategy(t *testing.T) {
	type args struct {
		rdsConfig   *rds.CreateDBInstanceInput
		foundConfig *rds.DBInstance
	}
	tests := []struct {
		name string
		args args
		want *rds.ModifyDBInstanceInput
	}{
		{
			name: "test modification not required",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					EngineVersion:              aws.String("test"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					EngineVersion:              aws.String("test"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
			},
			want: nil,
		},
		{
			name: "test when modification is required",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					DeletionProtection:         aws.Bool(false),
					BackupRetentionPeriod:      aws.Int64(0),
					DBInstanceClass:            aws.String("newValue"),
					PubliclyAccessible:         aws.Bool(false),
					AllocatedStorage:           aws.Int64(0),
					EngineVersion:              aws.String("newValue"),
					MultiAZ:                    aws.Bool(false),
					PreferredBackupWindow:      aws.String("newValue"),
					PreferredMaintenanceWindow: aws.String("newValue"),
					Port:                       aws.Int64(0),
				},
				foundConfig: &rds.DBInstance{
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					EngineVersion:              aws.String("test"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
			},
			want: &rds.ModifyDBInstanceInput{
				DeletionProtection:         aws.Bool(false),
				BackupRetentionPeriod:      aws.Int64(0),
				DBInstanceClass:            aws.String("newValue"),
				PubliclyAccessible:         aws.Bool(false),
				AllocatedStorage:           aws.Int64(0),
				EngineVersion:              aws.String("newValue"),
				MultiAZ:                    aws.Bool(false),
				PreferredBackupWindow:      aws.String("newValue"),
				PreferredMaintenanceWindow: aws.String("newValue"),
				DBPortNumber:               aws.Int64(0),
				DBInstanceIdentifier:       aws.String("test"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildRDSUpdateStrategy(tt.args.rdsConfig, tt.args.foundConfig); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildRDSUpdateStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}
