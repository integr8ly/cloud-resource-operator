package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	errorUtil "github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	v12 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	defaultRHMISubnetTag          = "integreatly.org/clusterID"
	defaultStandaloneVpcId        = "standaloneID"
	validCIDRFifteen              = "10.0.0.0/15"
	validCIDRSixteen              = "10.0.0.0/16"
	validCIDREighteen             = "10.0.0.0/18"
	validCIDRTwentySix            = "10.0.0.0/26"
	validCIDRTwentySeven          = "10.0.0.0/27"
	validCIDRTwentyThree          = "10.0.50.0/23"
	defaultValidSubnetMaskTwoA    = "10.0.50.0/24"
	defaultValidSubnetMaskTwoB    = "10.0.51.0/24"
	defaultNonOverlappingCidr     = "192.0.0.0/20"
	defaultSubnetIdOne            = "test-id-1"
	defaultSubnetIdTwo            = "test-id-2"
	defaultAzIdOne                = "test-zone-1"
	defaultAzIdTwo                = "test-zone-2"
	defaultValidSubnetMaskOneA    = "10.0.0.0/27"
	defaultValidSubnetMaskOneB    = "10.0.0.32/27"
	mockNetworkVpcId              = "test"
	defaultSecurityGroupName      = "testsecuritygroup"
	defaultSecurityGroupId        = "testSecurityGroupId"
	defaultStandaloneRouteTableId = "testRouteTableId"
	defaultClusterName            = "kubernetes.io/cluster/test"
)

func buildMockNetwork(modifyFn func(n *Network)) *Network {
	mock := &Network{Vpc: &ec2.Vpc{VpcId: aws.String(mockNetworkVpcId)}}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockNetworkConnection(modifyFn func(n *NetworkConnection)) *NetworkConnection {
	mock := &NetworkConnection{
		StandaloneSecurityGroup: &ec2.SecurityGroup{
			GroupId:   aws.String(defaultSecurityGroupId),
			GroupName: aws.String(defaultSecurityGroupName),
			VpcId:     aws.String(defaultStandaloneVpcId),
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

// Mock VPC Peering Connection
const (
	mockVpcPeeringConnectionID = "test"
)

func buildMockVpcPeeringConnection(modifyFn func(*ec2.VpcPeeringConnection)) *ec2.VpcPeeringConnection {
	mock := &ec2.VpcPeeringConnection{
		VpcPeeringConnectionId: aws.String(mockVpcPeeringConnectionID),
		Status: &ec2.VpcPeeringConnectionStateReason{
			Code: aws.String(ec2.VpcPeeringConnectionStateReasonCodeActive),
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestConfigManager(modifyFn func(m *ConfigManagerMock)) *ConfigManagerMock {
	mock := &ConfigManagerMock{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockVpc(modifyFn func(*ec2.Vpc)) *ec2.Vpc {
	mock := &ec2.Vpc{
		VpcId:     aws.String(defaultVpcId),
		CidrBlock: aws.String(defaultNonOverlappingCidr),
		Tags: []*ec2.Tag{
			buildMockEc2Tag(func(e *ec2.Tag) {
				e.Key = aws.String("test-vpc")
				e.Value = aws.String("test-vpc")
			}),
		},
		State: aws.String(ec2.VpcStateAvailable),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2Tag(modifyFn func(*ec2.Tag)) *ec2.Tag {
	mock := &ec2.Tag{
		Key:   aws.String(defaultRHMISubnetTag),
		Value: aws.String(defaultInfraName),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2SecurityGroup(modifyFn func(*ec2.SecurityGroup)) *ec2.SecurityGroup {
	mock := &ec2.SecurityGroup{
		GroupName: aws.String(defaultSecurityGroupName),
		GroupId:   aws.String(defaultSecurityGroupId),
		VpcId:     aws.String(defaultStandaloneVpcId),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2IpPermission(modifyFn func(*ec2.IpPermission)) *ec2.IpPermission {
	mock := &ec2.IpPermission{
		IpProtocol: aws.String("-1"),
		IpRanges: []*ec2.IpRange{
			{
				CidrIp: aws.String(defaultNonOverlappingCidr),
			},
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2RouteTable(modifyFn func(*ec2.RouteTable)) *ec2.RouteTable {
	mock := &ec2.RouteTable{
		RouteTableId: aws.String(defaultStandaloneRouteTableId),
		VpcId:        aws.String(defaultStandaloneVpcId),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2Route(modifyFn func(*ec2.Route)) *ec2.Route {
	mock := &ec2.Route{
		DestinationCidrBlock:   aws.String(validCIDRTwentySix),
		VpcPeeringConnectionId: aws.String(mockVpcPeeringConnectionID),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
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
			{
				Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
				Value: aws.String(clusterOwnedTagValue),
			},
		},
	}
}

func buildStandaloneSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultStandaloneVpcId, "test-id", "test", "test"),
	}
}

func buildBundledSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultVpcId, "test-id", "test", "test"),
	}
}

func buildValidRHMIBundleSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("test"),
				},
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildValidBundleSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildMultipleValidRHMIBundleSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("test"),
				},
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
			},
		},
		{
			SubnetId:         aws.String("test-id-2"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("test"),
				},
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
			},
		},
	}
}

func buildValidClusterSubnet(modifyFn func(*ec2.Subnet)) *ec2.Subnet {
	mock := &ec2.Subnet{
		SubnetId:         aws.String("test-id-2"),
		VpcId:            aws.String(defaultVpcId),
		AvailabilityZone: aws.String("test"),
		Tags: []*ec2.Tag{
			buildMockEc2Tag(func(e *ec2.Tag) {
				e.Key = aws.String(getOSDClusterTagKey(defaultInfraName))
				e.Value = aws.String(clusterOwnedTagValue)
			}),
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildStandaloneVPCAssociatedSubnets(subnetOne, subnetTwo string) []*ec2.Subnet {
	return []*ec2.Subnet{
		buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, subnetOne),
		buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, subnetTwo),
	}
}

func buildValidClusterVPC(cidrBlock string) []*ec2.Vpc {
	return []*ec2.Vpc{
		{
			VpcId:     aws.String(defaultVpcId),
			CidrBlock: aws.String(cidrBlock),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String("test-vpc"),
					Value: aws.String("test-vpc"),
				},
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
			},
			State: aws.String(ec2.VpcStateAvailable),
		},
	}
}
func buildValidStandaloneVPCTags() []*ec2.Tag {
	return []*ec2.Tag{

		{
			Key:   aws.String(tagDisplayName),
			Value: aws.String(DefaultRHMIVpcNameTagValue),
		},
		{
			Key:   aws.String(defaultRHMISubnetTag),
			Value: aws.String(defaultInfraName),
		},
	}
}

func buildValidStandaloneVPC(cidr string) *ec2.Vpc {
	return &ec2.Vpc{
		VpcId:     aws.String(defaultStandaloneVpcId),
		CidrBlock: aws.String(cidr),
		Tags:      buildValidStandaloneVPCTags(),
		State:     aws.String(ec2.VpcStateAvailable),
	}
}

func buildValidNonTaggedStandaloneVPC(cidr string) *ec2.Vpc {
	return &ec2.Vpc{
		VpcId:     aws.String(defaultVpcId),
		CidrBlock: aws.String(cidr),
		State:     aws.String(ec2.VpcStateAvailable),
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
			State:     aws.String(ec2.VpcStateAvailable),
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
			State:     aws.String(ec2.VpcStateAvailable),
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
	return resources.ShortenString(fmt.Sprintf("%s-%s", defaultInfraName, "subnet-group"), DefaultAwsIdentifierLength)
}

func buildSubnetGroupDescription() string {
	return fmt.Sprintf("%s-%s", defaultSubnetGroupDesc, "test")
}

func buildRDSSubnetGroup() []*rds.DBSubnetGroup {
	return []*rds.DBSubnetGroup{
		{
			DBSubnetGroupName: aws.String(buildSubnetGroupID()),
			VpcId:             aws.String(mockNetworkVpcId),
			DBSubnetGroupArn:  aws.String("subnetarn"),
		},
	}
}

func buildElasticacheSubnetGroup(modifyFn func(*elasticache.CacheSubnetGroup)) *elasticache.CacheSubnetGroup {
	mock := &elasticache.CacheSubnetGroup{
		CacheSubnetGroupName:        aws.String(buildSubnetGroupID()),
		VpcId:                       aws.String(mockNetworkVpcId),
		CacheSubnetGroupDescription: aws.String(buildSubnetGroupDescription()),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildValidIpNet(CIDR string) *net.IPNet {
	_, ip, _ := net.ParseCIDR(CIDR)
	return ip
}

func TestNetworkProvider_IsEnabled(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Logger *logrus.Entry
		Client client.Client
		Ec2Api ec2iface.EC2API
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
			//if no rhmi subnets exist in the cluster vpc then isEnabled will return true
			name: "verify isEnabled is true, no bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(validCIDRSixteen)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
			},
			want:    true,
			wantErr: false,
		},
		{
			// we expect isEnable to return false if a single rhmi subnet is found in cluster vpc
			name: "verify isEnabled is false, a single bundle subnet is found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(validCIDRSixteen)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidRHMIBundleSubnets(),
						}, nil
					}
				})},
			want:    false,
			wantErr: false,
		},
		{
			// we expect isEnable to return false if more than one rhmi subnet is found in cluster vpc
			name: "verify isEnabled is false, multiple bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(validCIDRSixteen)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildMultipleValidRHMIBundleSubnets(),
						}, nil
					}
				}),
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
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(validCIDRSixteen)
				}),
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
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{}
				}),
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
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
						}, nil
					}
				}),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger: tt.fields.Logger,
				Client: tt.fields.Client,
				Ec2Api: tt.fields.Ec2Api,
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
			name: "successfully error on invalid cidr params standalone vpc network - CIDR /15",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: &mockEc2Client{
					vpcs: buildValidClusterVPC(validCIDREighteen),
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					},
				},
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
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRSixteen)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRSixteen, defaultStandaloneVpcId),
			wantErr: false,
		},
		{
			name: "successfully build standalone vpc network - CIDR /26",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRTwentySix, defaultStandaloneVpcId),
			wantErr: false,
		},
		{
			name: "fail if trying to build standalone vpc network - CIDR /27",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
				}),
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
				Client: fake.NewFakeClientWithScheme(scheme),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
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
			name: "verify ec2 error when describing vpcs",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.wantErrList = true
					ec2Client.vpcs = []*ec2.Vpc{}
				}),
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
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
								buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVpcId, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on non tagged standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.vpc = buildValidNonTaggedStandaloneVPC(validCIDRTwentySix)
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
				}),
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
			name: "successfully timed out to check if VPC exists and failed the deletion",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.vpc = buildValidNonTaggedStandaloneVPC(validCIDRTwentySix)
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.WaitUntilVpcExistsFn = func(input *ec2.DescribeVpcsInput) error {
						return errorUtil.New("VPC does not exists")
					}
					ec2Client.deleteVpcFn = func(input *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
						return nil, errorUtil.New("can't delete VPC, it does not exists")
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    &Network{},
		},
		{
			name: "successfully reconcile on already created rds and elasticache subnet groups for standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.subnetGroups = buildRDSSubnetGroup()
					rdsClient.modifyDBSubnetGroupFn = func(input *rds.ModifyDBSubnetGroupInput) (*rds.ModifyDBSubnetGroupOutput, error) {
						return &rds.ModifyDBSubnetGroupOutput{}, nil
					}
					rdsClient.listTagsForResourceFn = func(input *rds.ListTagsForResourceInput) (*rds.ListTagsForResourceOutput, error) {
						return &rds.ListTagsForResourceOutput{
							TagList: []*rds.Tag{
								{
									Key:   aws.String("something"),
									Value: aws.String("something value"),
								},
							},
						}, nil
					}
					rdsClient.removeTagsFromResourceFn = func(input *rds.RemoveTagsFromResourceInput) (*rds.RemoveTagsFromResourceOutput, error) {
						return nil, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.modifyCacheSubnetGroupFn = func(input *elasticache.ModifyCacheSubnetGroupInput) (*elasticache.ModifyCacheSubnetGroupOutput, error) {
						return &elasticache.ModifyCacheSubnetGroupOutput{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								buildElasticacheSubnetGroup(nil),
							},
						}, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVpcId, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create subnets in correct azs",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.subnets = []*ec2.Subnet{}
					ec2Client.azs = buildUnsortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVpcId, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create subnets in large unsorted az zones list - zone one and two",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.subnets = []*ec2.Subnet{}
					ec2Client.azs = buildLargeUnsortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return &ec2.DescribeInstanceTypeOfferingsOutput{
							InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
								{
									Location: aws.String(defaultAzIdOne),
								},
								{
									Location: aws.String(defaultAzIdTwo),
								},
								{
									Location: aws.String("test-zone-3"),
								},
								{
									Location: aws.String("test-zone-4"),
								},
							},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVpcId, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
		},
		{
			name: "successfully reconcile on standalone vpc - create correct subnets for vpc cidr block 10.0.50.0/23",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentyThree)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentyThree)
					ec2Client.subnets = []*ec2.Subnet{}
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskTwoA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskTwoB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentyThree),
			},
			wantErr: false,
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentyThree, defaultStandaloneVpcId, defaultValidSubnetMaskTwoA, defaultValidSubnetMaskTwoB),
		},
		{
			name: "verify cluster vpc cidr block and standalone vpc cidr block overlaps return an error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: &mockEc2Client{
					vpcs:    []*ec2.Vpc{buildValidClusterVPC(validCIDRSixteen)[0]},
					subnets: []*ec2.Subnet{},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
				},
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySeven),
			},
			wantErr: true,
		},
		{
			name: "verify ec2 VpcLimitExceeded returns an error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{}, awserr.New("VpcLimitExceeded", "The maximum number of VPCs has been reached.", nil)
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			wantErr: true,
		},
		{
			name: "verify ec2 InvalidVpcRange returns an error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildValidClusterVPC(defaultNonOverlappingCidr)
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{}, awserr.New("InvalidVpcRange", "The specified CIDR block range is not valid. The block range must be between a /28 netmask and /16 netmask", nil)
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRFifteen),
			},
			wantErr: true,
		},
		{
			name: "successfully error if vpc route table does not exist",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.azs = buildSortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{RouteTables: []*ec2.RouteTable{}}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
		},
		{
			name: "fail when not enough availability zones support default node types",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(nil),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRTwentySix)}
					ec2Client.vpc = buildValidStandaloneVPC(validCIDRTwentySix)
					ec2Client.subnets = []*ec2.Subnet{}
					ec2Client.azs = buildLargeUnsortedStandaloneAZs()
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
						return &ec2.DescribeInstanceTypeOfferingsOutput{
							InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
								{
									Location: aws.String(defaultAzIdOne),
								},
							},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
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
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
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
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
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
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}
					ec2Client.subnets = buildStandaloneSubnets()
				}),
				ElasticacheApi: buildMockElasticacheClient(nil),
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
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDRSixteen)}
					ec2Client.subnets = buildStandaloneSubnets()
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, nil
					}
					elasticacheClient.describeCacheSubnetGroupsFn = func(input *elasticache.DescribeCacheSubnetGroupsInput) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
						return &elasticache.DescribeCacheSubnetGroupsOutput{
							CacheSubnetGroups: []*elasticache.CacheSubnetGroup{
								buildElasticacheSubnetGroup(nil),
							},
						}, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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

func TestNetworkProvider_ReconcileNetworkProviderConfig(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx           context.Context
		configManager ConfigManager
		logger        *logrus.Entry
		tier          string
	}
	type fields struct {
		Client         client.Client
		RdsApi         rdsiface.RDSAPI
		Ec2Api         ec2iface.EC2API
		ElasticacheApi elasticacheiface.ElastiCacheAPI
		Logger         *logrus.Entry
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *net.IPNet
		wantErr bool
	}{
		{
			name: "verify successful reconcile",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {}),
						}}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"CidrBlock\": \"10.0.0.0/16\" }"),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: false,
			want:    buildValidIpNet("10.0.0.0/16"),
		},
		{
			name: "verify invalid CIDR",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {}),
						}}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"CidrBlock\": \"malformed string\" }"),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: true,
		},
		{
			name: "verify unmarshal error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {}),
						}}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage(""),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: true,
		},
		{
			name: "verify default cidr block and no error on empty cidr block",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *v12.Network) {})),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.CidrBlock = aws.String("10.4.0.0/16")
							}),
						}}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildStandaloneSubnets(),
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{  }"),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: false,
			want:    buildValidIpNet("10.6.0.0/26"),
		},
		{
			name: "verify empty cidr blocks returns a error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *v12.Network) {
					network.Spec.ClusterNetwork = []v12.ClusterNetworkEntry{
						{
							CIDR: "",
						},
					}
					network.Spec.ServiceNetwork = []string{
						"",
					}
				})),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.CidrBlock = aws.String("")
							}),
						}}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{  }"),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: true,
		},
		{
			name: "verify no non overlapping available cidr blocks returns a error",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *v12.Network) {
					network.Spec.ClusterNetwork = []v12.ClusterNetworkEntry{
						{
							CIDR: "10.0.0.0/8",
						},
					}
					network.Spec.ServiceNetwork = []string{
						"172.0.0.0/8",
					}
				})),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {}),
						}}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				configManager: buildTestConfigManager(func(m *ConfigManagerMock) {
					m.ReadStorageStrategyFunc = func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{  }"),
						}, nil
					}
				}),
				logger: logrus.NewEntry(logrus.StandardLogger()),
				tier:   "test",
			},
			wantErr: true,
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
			got, err := n.ReconcileNetworkProviderConfig(tt.args.ctx, tt.args.configManager, tt.args.tier, tt.args.logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileNetworkProviderConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileNetworkProviderConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_CreateNetworkPeering(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		ec2Client  ec2iface.EC2API
		kubeClient client.Client
		logger     *logrus.Entry
	}
	type args struct {
		ctx     context.Context
		network *Network
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *NetworkPeering
		wantErr string
	}{
		{
			name: "fails when cluster vpc id cannot be found from associated subnets because subnets don't have the required tags",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{
						{
							VpcId: aws.String(mockNetworkVpcId),
						},
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(func(subnet *ec2.Subnet) {
									subnet.Tags = nil
								}),
							},
						}, nil
					}

				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "failed to get cluster vpc, no vpc found: error getting vpc id from associated subnets: failed to get cluster vpc id, no vpc found with osd cluster tag: could not find cluster associated subnets with clusterID test",
		},
		{
			name: "fails when peering connections cannot be listed",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return nil, errors.New("test")
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "failed to get peering connection: failed to describe peering connections: test",
		},
		{
			name: "fails when vpc peering cannot be created",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []*ec2.VpcPeeringConnection{}}, nil
					}
					ec2Client.createVpcPeeringConnectionFn = func(input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
						return nil, errors.New("test")
					}
				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "failed to create vpc peering connection: test",
		},
		{
			name: "fails when tags cannot be added to peering connection",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []*ec2.VpcPeeringConnection{}}, nil
					}
					ec2Client.createVpcPeeringConnectionFn = func(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
						return &ec2.CreateVpcPeeringConnectionOutput{VpcPeeringConnection: buildMockVpcPeeringConnection(nil)}, nil
					}
					ec2Client.createTagsFn = func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
						return nil, errors.New("test")
					}
				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "failed to tag peering connection: test",
		},
		{
			name: "fails when unable to accept peering connection",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []*ec2.VpcPeeringConnection{}}, nil
					}
					ec2Client.createVpcPeeringConnectionFn = func(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
						mockPeeringConnection := buildMockVpcPeeringConnection(func(mock *ec2.VpcPeeringConnection) {
							mock.Status.Code = aws.String(ec2.VpcPeeringConnectionStateReasonCodePendingAcceptance)
						})
						return &ec2.CreateVpcPeeringConnectionOutput{VpcPeeringConnection: mockPeeringConnection}, nil
					}
					ec2Client.createTagsFn = func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
						return nil, nil
					}
					ec2Client.acceptVpcPeeringConnectionFn = func(*ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
						return nil, errors.New("test")
					}
				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "failed to accept vpc peering connection: test",
		},
		{
			name: "fails when peering connection state is unknown",
			fields: fields{
				ec2Client: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = buildVpcs()
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []*ec2.VpcPeeringConnection{}}, nil
					}
					ec2Client.createVpcPeeringConnectionFn = func(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
						mockPeeringConnection := buildMockVpcPeeringConnection(func(mock *ec2.VpcPeeringConnection) {
							mock.Status.Code = aws.String(ec2.VpcPeeringConnectionStateReasonCodeExpired)
						})
						return &ec2.CreateVpcPeeringConnectionOutput{VpcPeeringConnection: mockPeeringConnection}, nil
					}
					ec2Client.createTagsFn = func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
						return nil, nil
					}
					ec2Client.acceptVpcPeeringConnectionFn = func(*ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
						return nil, errors.New("test")
					}
				}),
				kubeClient: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				logger:     logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: "vpc peering connection test is in an invalid state 'expired' with message ''",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Ec2Api: tt.fields.ec2Client,
				Client: tt.fields.kubeClient,
				Logger: tt.fields.logger,
			}
			got, err := n.CreateNetworkPeering(tt.args.ctx, tt.args.network)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("CreateNetworkPeering() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateNetworkPeering() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_GetClusterNetworkPeering(t *testing.T) {
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
		want    *NetworkPeering
		wantErr string
	}{
		{
			name: "fails when cannot get standalone vpc",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.wantErrList = true
					ec2Client.vpcs = []*ec2.Vpc{}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: "failed to get standalone vpc: error getting vpcs: ec2 get vpcs error",
		},
		{
			name: "fails when cannot get vpc peering connection",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: "failed to get network peering: failed to get cluster vpc: error, no vpc found",
		},
		{
			name: "success when network peering found",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.vpcs = []*ec2.Vpc{buildValidStandaloneVPC(validCIDREighteen)}
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			want: &NetworkPeering{
				PeeringConnection: buildMockVpcPeeringConnection(nil),
			},
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
			got, err := n.GetClusterNetworkPeering(tt.args.ctx)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("GetClusterNetworkPeering() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetClusterNetworkPeering() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_DeleteNetworkPeering(t *testing.T) {
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
		ctx     context.Context
		peering *NetworkPeering
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "fails when cannot describe peering connections",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return nil, errors.New("test")
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				peering: &NetworkPeering{PeeringConnection: buildMockVpcPeeringConnection(nil)},
			},
			wantErr: "failed to get vpc: test",
		},
		{
			name: "fails when cannot delete peering connections",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{buildMockVpcPeeringConnection(nil)},
						}, nil
					}
					ec2Client.deleteVpcPeeringConnectionFn = func(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
						return nil, errors.New("test")
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				peering: &NetworkPeering{PeeringConnection: buildMockVpcPeeringConnection(nil)},
			},
			wantErr: "failed to delete vpc peering connection: test",
		},
		{
			name: "success when status is deleting",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(func(connection *ec2.VpcPeeringConnection) {
									connection.Status.Code = aws.String(ec2.VpcPeeringConnectionStateReasonCodeDeleting)
								}),
							},
						}, nil
					}
					ec2Client.deleteVpcPeeringConnectionFn = func(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
						return nil, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				peering: &NetworkPeering{PeeringConnection: buildMockVpcPeeringConnection(nil)},
			},
		},
		{
			name: "success when vpc deletion succeeds",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{buildMockVpcPeeringConnection(nil)},
						}, nil
					}
					ec2Client.deleteVpcPeeringConnectionFn = func(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
						return nil, nil
					}
				}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				peering: &NetworkPeering{PeeringConnection: buildMockVpcPeeringConnection(nil)},
			},
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
			if err := n.DeleteNetworkPeering(tt.args.peering); err != nil && err.Error() != tt.wantErr {
				t.Errorf("DeleteNetworkPeering() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkProvider_CreateNetworkConnection(t *testing.T) {
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
		ctx     context.Context
		network *Network
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *NetworkConnection
		wantErr bool
	}{
		{
			name: "test successful security group creation",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						calls := ec2Client.DescribeSecurityGroupsCalls()
						if len(calls) == 1 {
							return &ec2.DescribeSecurityGroupsOutput{
								SecurityGroups: []*ec2.SecurityGroup{
									buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
										group.GroupName = aws.String("not test security group id")
									}),
								},
							}, nil
						}
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						calls := ec2Client.DescribeRouteTablesCalls()
						if len(calls) == 1 {
							return &ec2.DescribeRouteTablesOutput{
								RouteTables: []*ec2.RouteTable{
									buildMockEc2RouteTable(func(table *ec2.RouteTable) {
										table.Tags = []*ec2.Tag{
											buildMockEc2Tag(func(e *ec2.Tag) {
												e.Key = aws.String("kubernetes.io/cluster/test")
												e.Value = aws.String("owned")
											}),
										}
									}),
								},
							}, nil
						}
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(defaultRHMISubnetTag)
											e.Value = aws.String("test")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(nil),
							},
						}, nil
					}
					ec2Client.createRouteFn = func(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
						return &ec2.CreateRouteOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want:    buildMockNetworkConnection(nil),
			wantErr: false,
		},
		{
			name: "test security group exists with no tags",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						calls := ec2Client.DescribeRouteTablesCalls()
						if len(calls) == 1 {
							return &ec2.DescribeRouteTablesOutput{
								RouteTables: []*ec2.RouteTable{
									buildMockEc2RouteTable(func(table *ec2.RouteTable) {
										table.Tags = []*ec2.Tag{
											buildMockEc2Tag(func(e *ec2.Tag) {
												e.Key = aws.String("kubernetes.io/cluster/test")
												e.Value = aws.String("owned")
											}),
										}
									}),
								},
							}, nil
						}
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(defaultRHMISubnetTag)
											e.Value = aws.String("test")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(nil),
							},
						}, nil
					}
					ec2Client.createRouteFn = func(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
						return &ec2.CreateRouteOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {}),
			},
			wantErr: false,
		},
		{
			name: "test security group exists with tags and invalid permissions",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
									group.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {}),
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(tagDisplayName)
											e.Value = aws.String(DefaultRHMIVpcNameTagValue)
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						calls := ec2Client.DescribeRouteTablesCalls()
						if len(calls) == 1 {
							return &ec2.DescribeRouteTablesOutput{
								RouteTables: []*ec2.RouteTable{
									buildMockEc2RouteTable(func(table *ec2.RouteTable) {
										table.Tags = []*ec2.Tag{
											buildMockEc2Tag(func(e *ec2.Tag) {
												e.Key = aws.String("kubernetes.io/cluster/test")
												e.Value = aws.String("owned")
											}),
										}
									}),
								},
							}, nil
						}
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(defaultRHMISubnetTag)
											e.Value = aws.String("test")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(nil),
							},
						}, nil
					}
					ec2Client.createRouteFn = func(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
						return &ec2.CreateRouteOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
					group.Tags = []*ec2.Tag{
						buildMockEc2Tag(func(e *ec2.Tag) {}),
						buildMockEc2Tag(func(e *ec2.Tag) {
							e.Key = aws.String(tagDisplayName)
							e.Value = aws.String(DefaultRHMIVpcNameTagValue)
						}),
					}
				}),
			},
			wantErr: false,
		},
		{
			name: "test security group exists with tags and valid permissions",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: &mockRdsClient{},
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
									group.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {}),
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(tagDisplayName)
											e.Value = aws.String(DefaultRHMIVpcNameTagValue)
										}),
									}
									group.IpPermissions = []*ec2.IpPermission{
										buildMockEc2IpPermission(func(permission *ec2.IpPermission) {}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						calls := ec2Client.DescribeRouteTablesCalls()
						if len(calls) == 1 {
							return &ec2.DescribeRouteTablesOutput{
								RouteTables: []*ec2.RouteTable{
									buildMockEc2RouteTable(func(table *ec2.RouteTable) {
										table.Tags = []*ec2.Tag{
											buildMockEc2Tag(func(e *ec2.Tag) {
												e.Key = aws.String("kubernetes.io/cluster/test")
												e.Value = aws.String("owned")
											}),
										}
									}),
								},
							}, nil
						}
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(defaultRHMISubnetTag)
											e.Value = aws.String("test")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcPeeringConnectionFn = func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
						return &ec2.DescribeVpcPeeringConnectionsOutput{
							VpcPeeringConnections: []*ec2.VpcPeeringConnection{
								buildMockVpcPeeringConnection(nil),
							},
						}, nil
					}
					ec2Client.createRouteFn = func(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
						return &ec2.CreateRouteOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
					group.Tags = []*ec2.Tag{
						buildMockEc2Tag(func(e *ec2.Tag) {}),
						buildMockEc2Tag(func(e *ec2.Tag) {
							e.Key = aws.String(tagDisplayName)
							e.Value = aws.String(DefaultRHMIVpcNameTagValue)
						}),
					}
					group.IpPermissions = []*ec2.IpPermission{
						buildMockEc2IpPermission(func(permission *ec2.IpPermission) {}),
					}
				}),
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
			got, err := n.CreateNetworkConnection(tt.args.ctx, tt.args.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNetworkConnection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateNetworkConnection() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_DeleteNetworkConnection(t *testing.T) {
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
		ctx            context.Context
		networkPeering *NetworkPeering
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "ensure no error return if security group is nil",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete security group is called if security group is not nil and is a security group provisioned by cro",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
									group.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {}),
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(tagDisplayName)
											e.Value = aws.String(DefaultRHMIVpcNameTagValue)
										}),
									}
									group.IpPermissions = []*ec2.IpPermission{
										buildMockEc2IpPermission(func(permission *ec2.IpPermission) {}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete security group is not called if security groups are found but not a cro provisioned security group",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
									group.GroupName = aws.String("not a cro security group")
								}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete routes is called",
			fields: fields{
				Client:         fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi:         &mockRdsClient{},
				ElasticacheApi: &mockElasticacheClient{},
				Logger:         logrus.NewEntry(logrus.StandardLogger()),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildMockEc2SecurityGroup(func(group *ec2.SecurityGroup) {
									group.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String(tagDisplayName)
											e.Value = aws.String(DefaultRHMIVpcNameTagValue)
										}),
									}
									group.IpPermissions = []*ec2.IpPermission{
										buildMockEc2IpPermission(func(permission *ec2.IpPermission) {}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(func(table *ec2.RouteTable) {
									table.Routes = []*ec2.Route{
										buildMockEc2Route(nil),
									}
									table.Tags = []*ec2.Tag{
										buildMockEc2Tag(func(e *ec2.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{
							buildMockVpc(func(vpc *ec2.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []*ec2.Tag{
									buildMockEc2Tag(func(e *ec2.Tag) {
										e.Key = aws.String(tagDisplayName)
										e.Value = aws.String(DefaultRHMIVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2.Tag) {}),
								}
							}),
						}}, nil
					}
					ec2Client.deleteRouteFn = func(input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
						return &ec2.DeleteRouteOutput{}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2.VpcPeeringConnection) {

					}),
				},
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
			if err := n.DeleteNetworkConnection(tt.args.ctx, tt.args.networkPeering); (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetworkConnection() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkProvider_DeleteBundledCloudResources(t *testing.T) {
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
			name: "successfully delete subnet groups (rds and elasticache) and ec2 security group",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildSecurityGroup(func(mock *ec2.SecurityGroup) {
									mock.GroupName = aws.String("testsecuritygroup")
									mock.VpcId = aws.String("testID")
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildValidClusterVPC("10.0.0.0/23"),
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "return error when the cluster vpc is nil",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{
								buildSecurityGroup(func(mock *ec2.SecurityGroup) {
									mock.GroupName = aws.String("testsecuritygroup")
								}),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
		},
		{
			name: "ensure that no error is returned if elasticache.ErrCodeCacheSubnetGroupNotFoundFault is returned on delete request",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, awserr.New(elasticache.ErrCodeCacheSubnetGroupNotFoundFault, "", errors.New(elasticache.ErrCodeCacheSubnetGroupNotFoundFault))
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "ensure that no error is returned if rds.ErrCodeDBSubnetGroupNotFoundFault is returned on delete request",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, awserr.New(rds.ErrCodeDBSubnetGroupNotFoundFault, "", errors.New(rds.ErrCodeDBSubnetGroupNotFoundFault))
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "return error when aws error returned on deletecachesubnetgroup",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, nil
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, awserr.New(elasticache.ErrCodeAuthorizationNotFoundFault, "", errors.New(elasticache.ErrCodeAuthorizationNotFoundFault))
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
		},
		{
			name: "return error when aws error returned on deletedbsubnetgroup",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				RdsApi: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.deleteDBSubnetGroupFn = func(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
						return &rds.DeleteDBSubnetGroupOutput{}, awserr.New(rds.ErrCodeAuthorizationNotFoundFault, "", errors.New(rds.ErrCodeAuthorizationNotFoundFault))
					}
				}),
				Ec2Api: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.deleteSecurityGroupFn = func(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
						return &ec2.DeleteSecurityGroupOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []*ec2.SecurityGroup{},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{}, nil
					}
				}),
				ElasticacheApi: buildMockElasticacheClient(func(elasticacheClient *mockElasticacheClient) {
					elasticacheClient.deleteCacheSubnetGroupFn = func(input *elasticache.DeleteCacheSubnetGroupInput) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
						return &elasticache.DeleteCacheSubnetGroupOutput{}, nil
					}
				}),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
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
			if err := n.DeleteBundledCloudResources(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("NetworkProvider.DeleteBundledCloudResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
