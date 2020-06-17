package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"net"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	defaultRHMISubnetTag       = "integreatly.org/clusterID"
	defaultStandaloneVPCID     = "standaloneID"
	validCIDRFifteen           = "10.0.0.0/15"
	validCIDRSixteen           = "10.0.0.0/16"
	validCIDREighteen          = "10.0.0.0/18"
	validCIDRTwentySix         = "10.0.0.0/26"
	validCIDRTwentySeven       = "10.0.0.0/27"
	validCIDRTwentyThree       = "10.0.50.0/23"
	defaultValidSubnetMaskTwoA = "10.0.50.0/24"
	defaultValidSubnetMaskTwoB = "10.0.51.0/24"
	defaultSubnetIdOne         = "test-id-1"
	defaultSubnetIdTwo         = "test-id-2"
	defaultAzIdOne             = "test-zone-1"
	defaultAzIdTwo             = "test-zone-2"
	defaultValidSubnetMaskOneA = "10.0.0.0/27"
	defaultValidSubnetMaskOneB = "10.0.0.32/27"
)

type mockNetworkManager struct {
	NetworkManager
}

var _ NetworkManager = (*mockNetworkManager)(nil)

func (m mockNetworkManager) CreateNetwork(context.Context, *net.IPNet) (*Network, error) {
	return &Network{}, nil
}

func (m mockNetworkManager) DeleteNetwork(context.Context) error {
	return nil
}

func (m mockNetworkManager) IsEnabled(context.Context) (bool, error) {
	return false, nil
}

func buildSubnet(vpcID, subnetId, azId, cidrBlock string) *ec2.Subnet {
	return &ec2.Subnet{
		SubnetId:         aws.String(subnetId),
		VpcId:            aws.String(vpcID),
		AvailabilityZone: aws.String(azId),
		CidrBlock:        aws.String(cidrBlock),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String(defaultAWSPrivateSubnetTagKey),
				Value: aws.String("1"),
			},
		},
	}
}

func buildStandaloneSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultStandaloneVPCID, "test-id", "test", "test"),
	}
}

func buildBundledSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultVPCID, "test-id", "test", "test"),
	}
}

func buildClusterVpc(cidrBlock string) []*ec2.Vpc {
	return []*ec2.Vpc{
		{
			VpcId:     aws.String(defaultVPCID),
			CidrBlock: aws.String(cidrBlock),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String("test-vpc"),
					Value: aws.String("test-vpc"),
				},
			},
		},
	}
}

func buildValidBundleSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVPCID),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildMultipleValidBundleSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVPCID),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
			},
		},
		{
			SubnetId:         aws.String("test-id-2"),
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildStandaloneVPCAssociatedSubnets(subnetOne, subnetTwo string) []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, subnetOne),
		buildSubnet(defaultStandaloneVPCID, defaultSubnetIdTwo, defaultAzIdTwo, subnetTwo),
	}
}

func buildValidClusterVPC(cidrBlock string) []*ec2.Vpc {
	return []*ec2.Vpc{
		{
			VpcId:     aws.String(defaultVPCID),
			CidrBlock: aws.String(cidrBlock),
		},
	}
}
func buildValidStandaloneVPCTags() []*ec2.Tag {
	return []*ec2.Tag{

		{
			Key:   aws.String(DefaultRHMIVpcNameTagKey),
			Value: aws.String(DefaultRHMIVpcNameTagValue),
		},
		{
			Key:   aws.String("integreatly.org/clusterID"),
			Value: aws.String(dafaultInfraName),
		},
	}
}

func buildValidStandaloneVPC(cidr string) *ec2.Vpc {
	return &ec2.Vpc{
		VpcId:     aws.String(defaultStandaloneVPCID),
		CidrBlock: aws.String(cidr),
		Tags:      buildValidStandaloneVPCTags(),
	}
}

func buildValidNonTaggedStandaloneVPC(cidr string) *ec2.Vpc {
	return &ec2.Vpc{
		VpcId:     aws.String(defaultVPCID),
		CidrBlock: aws.String(cidr),
	}
}

// the two below functions handle two cases inside CreateNetwork
// buildValidNetworkResponseVPCExists is used when we want to test case where the vpc
// already exists, i.e. go create subnets, subnet groups etc.
// buildValidNetworkResponseCreateVPC is used when we want to test case where no vpc exists
// i.e. create the vpc and return network response with vpc and all other resources are nil
func buildValidNetworkResponseVPCExists(cidr, vpcID, subnetOne, subnetTwo string) *Network {
	return &Network{
		Vpc: &ec2.Vpc{
			CidrBlock: aws.String(cidr),
			VpcId:     aws.String(vpcID),
			Tags:      buildValidStandaloneVPCTags(),
		},
		Subnets: buildStandaloneVPCAssociatedSubnets(subnetOne, subnetTwo),
	}
}

func buildValidNetworkResponseCreateVPC(cidr, vpcID string) *Network {
	return &Network{
		Vpc: &ec2.Vpc{
			CidrBlock: aws.String(cidr),
			VpcId:     aws.String(vpcID),
			Tags:      buildValidStandaloneVPCTags(),
		},
		Subnets: nil,
	}
}

func buildSortedStandaloneAZs() []*ec2.AvailabilityZone {
	return []*ec2.AvailabilityZone{
		{
			ZoneName: aws.String(defaultAzIdOne),
		},
		{
			ZoneName: aws.String(defaultAzIdTwo),
		},
	}
}

func buildUnsortedStandaloneAZs() []*ec2.AvailabilityZone {
	return []*ec2.AvailabilityZone{
		{
			ZoneName: aws.String(defaultAzIdTwo),
		},
		{
			ZoneName: aws.String(defaultAzIdOne),
		},
	}
}

func buildLargeUnsortedStandaloneAZs() []*ec2.AvailabilityZone {
	return []*ec2.AvailabilityZone{
		{
			ZoneName: aws.String("test-zone-3"),
		},
		{
			ZoneName: aws.String("test-zone-4"),
		},
		{
			ZoneName: aws.String(defaultAzIdTwo),
		},
		{
			ZoneName: aws.String(defaultAzIdOne),
		},
	}
}

func buildValidCIDR(cidr string) *net.IPNet {
	_, ipnet, _ := net.ParseCIDR(cidr)
	return ipnet
}

func buildSubnetGroupID() string {
	return resources.ShortenString(fmt.Sprintf("%s-%s", dafaultInfraName, "subnet-group"), DefaultAwsIdentifierLength)
}

func buildRDSSubnetGroup() []*rds.DBSubnetGroup {
	return []*rds.DBSubnetGroup{
		{
			DBSubnetGroupName: aws.String(buildSubnetGroupID()),
			VpcId:             aws.String("test"),
		},
	}
}

func buildElasticacheSubnetGroup() []*elasticache.CacheSubnetGroup {
	return []*elasticache.CacheSubnetGroup{
		{
			CacheSubnetGroupName: aws.String(buildSubnetGroupID()),
			VpcId:                aws.String("test"),
		},
	}
}

func TestNetworkProvider_IsEnabled(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Logger *logrus.Entry
		Client client.Client
		Ec2Svc ec2iface.EC2API
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			// we expect if no rhmi subnets exist in the cluster vpc isEnabled will return true
			name: "verify isEnabled is true, no bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{vpcs: buildClusterVpc(validCIDRSixteen), subnets: buildBundledSubnets()},
			},
			want:    true,
			wantErr: false,
		},
		{

			//we expect if a single rhmi subnet is found in cluster vpc isEnabled will return true
			name: "verify isEnabled is false, a single bundle subnet is found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidBundleSubnets()},
			},
			want:    false,
			wantErr: false,
		},
		{
			// we expect isEnable to return true if more then one rhmi subnet is found in cluster vpc
			name: "verify isEnabled is true, multiple bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildMultipleValidBundleSubnets()},
			},
			want:    false,
			wantErr: false,
		},
		{
			// we always expect subnets to exist in the cluster vpc, this ensures we get an error if none exist
			name: "verify error, if no subnets are found",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{vpcs: buildVpcs()},
			},
			wantErr: true,
		},
		{
			// we always expect a cluster vpc, this ensures we get an error is none exist
			name: "verify error, if no cluster vpc is found",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{},
			},
			wantErr: true,
		},
		{
			// we always expect subnets to exist in the cluster vpc,
			// this test ensures an error if subnets exist in the cluster vpc but not associated with the vpc
			name: "verify error, if no subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger: tt.fields.Logger,
				Client: tt.fields.Client,
				Ec2Api: tt.fields.Ec2Svc,
			}
			got, err := n.IsEnabled(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsEnabled() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsEnabled() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_CreateNetwork(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client         client.Client
		RdsApi         rdsiface.RDSAPI
		Ec2Api         ec2iface.EC2API
		ElasticacheApi elasticacheiface.ElastiCacheAPI
		Logger         *logrus.Entry
	}
	type args struct {
		ctx  context.Context
		CIDR *net.IPNet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *Network
		wantErr bool
	}{
		{
			name: "successfully build standalone vpc network - CIDR /15",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: buildValidClusterVPC(validCIDREighteen)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRFifteen),
			},
			wantErr: true,
		},
		{
			name: "successfully build standalone vpc network  - CIDR /16",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: buildValidClusterVPC(validCIDREighteen), vpc: buildValidStandaloneVPC(validCIDRSixteen)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRSixteen, defaultStandaloneVPCID),
			wantErr: false,
		},
		{
			name: "successfully build standalone vpc network - CIDR /26",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: buildValidClusterVPC(validCIDREighteen), vpc: buildValidStandaloneVPC(validCIDRTwentySix)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRTwentySix, defaultStandaloneVPCID),
			wantErr: false,
		},
		{
			name: "successfully build standalone vpc network - CIDR /27",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: buildValidClusterVPC(validCIDREighteen)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySeven),
			},
			wantErr: true,
		},
		{
			name: "fail if unable to get cluster id",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
		},
		{
			name: "unable to get vpc",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{wantErrList: true},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
		},
		{
			name: "successfully reconcile on standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs:        []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)},
					vpc:         buildValidStandaloneVPC(validCIDRTwentySix),
					subnets:     buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
					azs:         buildSortedStandaloneAZs(),
					firstSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVPCID, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on non tagged standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs:        buildVpcs(),
					vpc:         buildValidNonTaggedStandaloneVPC(validCIDRTwentySix),
					subnets:     buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
					azs:         buildSortedStandaloneAZs(),
					firstSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want: &Network{
				Vpc: buildValidNonTaggedStandaloneVPC(validCIDRTwentySix),
			},
		},
		{
			name: "successfully reconcile on already created rds subnet groups for standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{subnetGroups: buildRDSSubnetGroup()},
				Ec2Api: &mockEc2Client{
					vpcs:        []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)},
					vpc:         buildValidStandaloneVPC(validCIDRTwentySix),
					subnets:     buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
					azs:         buildSortedStandaloneAZs(),
					firstSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)},
				ElasticacheApi: &mockElasticacheClient{cacheSubnetGroup: buildElasticacheSubnetGroup()},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVPCID, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create subnets in correct azs",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs:         []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)},
					vpc:          buildValidStandaloneVPC(validCIDRTwentySix),
					subnets:      []*ec2.Subnet{},
					azs:          buildUnsortedStandaloneAZs(),
					firstSubnet:  buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
					secondSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
				},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVPCID, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create subnets in large unsorted az zones list - zone one and two",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs:         []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)},
					vpc:          buildValidStandaloneVPC(validCIDRTwentySix),
					subnets:      []*ec2.Subnet{},
					azs:          buildLargeUnsortedStandaloneAZs(),
					firstSubnet:  buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
					secondSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
				},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVPCID, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create correct subnets for vpc cidr block 10.0.50.0/23",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs:         []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentyThree)},
					vpc:          buildValidStandaloneVPC(validCIDRTwentyThree),
					subnets:      []*ec2.Subnet{},
					azs:          buildSortedStandaloneAZs(),
					firstSubnet:  buildSubnet(defaultStandaloneVPCID, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskTwoA),
					secondSubnet: buildSubnet(defaultStandaloneVPCID, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskTwoB),
				},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentyThree),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentyThree, defaultStandaloneVPCID, defaultValidSubnetMaskTwoA, defaultValidSubnetMaskTwoB),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Client:         tt.fields.Client,
				RdsApi:         tt.fields.RdsApi,
				Ec2Api:         tt.fields.Ec2Api,
				ElasticacheApi: tt.fields.ElasticacheApi,
				Logger:         tt.fields.Logger,
			}
			got, err := n.CreateNetwork(tt.args.ctx, tt.args.CIDR)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNetwork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateNetwork() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_DeleteNetwork(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client         client.Client
		RdsApi         rdsiface.RDSAPI
		Ec2Api         ec2iface.EC2API
		ElasticacheApi elasticacheiface.ElastiCacheAPI
		Logger         *logrus.Entry
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "verify deletion - no vpc found",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc and associated subnets",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				Ec2Api:         &mockEc2Client{vpcs: []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}, subnets: buildStandaloneSubnets()},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc and associated subnets and subnet groups",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{subnetGroups: buildRDSSubnetGroup()},
				Ec2Api:         &mockEc2Client{vpcs: []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}, subnets: buildStandaloneSubnets()},
				ElasticacheApi: &mockElasticacheClient{cacheSubnetGroup: buildElasticacheSubnetGroup()},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Client:         tt.fields.Client,
				RdsApi:         tt.fields.RdsApi,
				Ec2Api:         tt.fields.Ec2Api,
				ElasticacheApi: tt.fields.ElasticacheApi,
				Logger:         tt.fields.Logger,
			}
			if err := n.DeleteNetwork(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetwork() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
