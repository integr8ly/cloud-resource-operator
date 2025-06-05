package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	configv1 "github.com/openshift/api/config/v1"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"net"
	"reflect"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const (
	defaultAzIdOne                = "test-zone-1"
	defaultAzIdTwo                = "test-zone-2"
	defaultNonOverlappingCidr     = "192.0.0.0/20"
	defaultSecurityGroupId        = "testSecurityGroupId"
	defaultSecurityGroupName      = "testsecuritygroup"
	defaultStandaloneRouteTableId = "testRouteTableId"
	defaultStandaloneVpcId        = "standaloneID"
	defaultSubnetIdOne            = "test-id-1"
	defaultSubnetIdTwo            = "test-id-2"
	defaultSubnetTag              = "integreatly.org/clusterID"
	defaultValidSubnetMaskOneA    = "10.0.0.0/27"
	defaultValidSubnetMaskOneB    = "10.0.0.32/27"
	defaultValidSubnetMaskTwoA    = "10.0.50.0/24"
	defaultValidSubnetMaskTwoB    = "10.0.51.0/24"
	mockNetworkVpcId              = "test"
	validCIDREighteen             = "10.0.0.0/18"
	validCIDRFifteen              = "10.0.0.0/15"
	validCIDRSixteen              = "10.0.0.0/16"
	validCIDRTwentySeven          = "10.0.0.0/27"
	validCIDRTwentySix            = "10.0.0.0/26"
	validCIDRTwentyThree          = "10.0.50.0/23"
)

var (
	genericAWSError = &smithy.GenericAPIError{
		Code:    "666",
		Message: "generic aws error",
		Fault:   smithy.FaultUnknown,
	}
)

func buildMockNetwork(modifyFn func(n *Network)) *Network {
	mock := &Network{Vpc: &ec2types.Vpc{VpcId: aws.String(mockNetworkVpcId)}}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockNetworkConnection(modifyFn func(n *NetworkConnection)) *NetworkConnection {
	mock := &NetworkConnection{
		StandaloneSecurityGroup: &ec2types.SecurityGroup{
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

func buildMockVpcPeeringConnection(modifyFn func(*ec2types.VpcPeeringConnection)) *ec2types.VpcPeeringConnection {
	mock := &ec2types.VpcPeeringConnection{
		VpcPeeringConnectionId: aws.String(mockVpcPeeringConnectionID),
		Status: &ec2types.VpcPeeringConnectionStateReason{
			Code: ec2types.VpcPeeringConnectionStateReasonCodeActive,
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

func buildMockVpc(modifyFn func(*ec2types.Vpc)) *ec2types.Vpc {
	mock := &ec2types.Vpc{
		VpcId:     aws.String(defaultVpcId),
		CidrBlock: aws.String(defaultNonOverlappingCidr),
		Tags: []ec2types.Tag{
			buildMockEc2Tag(func(e *ec2types.Tag) {
				e.Key = aws.String("test-vpc")
				e.Value = aws.String("test-vpc")
			}),
		},
		State: ec2types.VpcStateAvailable,
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2Tag(modifyFn func(*ec2types.Tag)) ec2types.Tag {
	mock := ec2types.Tag{
		Key:   aws.String(defaultSubnetTag),
		Value: aws.String(defaultInfraName),
	}
	if modifyFn != nil {
		modifyFn(&mock)
	}
	return mock
}

func buildMockEc2SecurityGroup(modifyFn func(*ec2types.SecurityGroup)) *ec2types.SecurityGroup {
	mock := &ec2types.SecurityGroup{
		GroupName: aws.String(defaultSecurityGroupName),
		GroupId:   aws.String(defaultSecurityGroupId),
		VpcId:     aws.String(defaultStandaloneVpcId),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2IpPermission(modifyFn func(*ec2types.IpPermission)) *ec2types.IpPermission {
	mock := &ec2types.IpPermission{
		IpProtocol: aws.String("-1"),
		IpRanges: []ec2types.IpRange{
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

func buildMockEc2RouteTable(modifyFn func(*ec2types.RouteTable)) *ec2types.RouteTable {
	mock := &ec2types.RouteTable{
		RouteTableId: aws.String(defaultStandaloneRouteTableId),
		VpcId:        aws.String(defaultStandaloneVpcId),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockEc2Route(modifyFn func(*ec2types.Route)) *ec2types.Route {
	mock := &ec2types.Route{
		DestinationCidrBlock:   aws.String(validCIDRTwentySix),
		VpcPeeringConnectionId: aws.String(mockVpcPeeringConnectionID),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildSubnet(vpcID, subnetId, azId, cidrBlock string) *ec2types.Subnet {
	return &ec2types.Subnet{
		SubnetId:         aws.String(subnetId),
		VpcId:            aws.String(vpcID),
		AvailabilityZone: aws.String(azId),
		CidrBlock:        aws.String(cidrBlock),
		Tags: []ec2types.Tag{
			{
				Key:   aws.String(defaultAWSPrivateSubnetTagKey),
				Value: aws.String("1"),
			},
			{
				Key:   aws.String(defaultSubnetTag),
				Value: aws.String("test"),
			},
			{
				Key:   aws.String(resources.TagDisplayName),
				Value: aws.String(defaultSubnetNameTagValue),
			},
			*genericToEc2Tag(resources.BuildManagedTag()),
		},
	}
}

func buildUntaggedSubnet(vpcID, subnetId, azId, cidrBlock string) *ec2types.Subnet {
	return &ec2types.Subnet{
		SubnetId:         aws.String(subnetId),
		VpcId:            aws.String(vpcID),
		AvailabilityZone: aws.String(azId),
		CidrBlock:        aws.String(cidrBlock),
	}
}

func buildStandaloneSubnets() []ec2types.Subnet {
	return []ec2types.Subnet{
		*buildSubnet(defaultStandaloneVpcId, "test-id", "test", "test"),
	}
}

func buildValidBundleSubnets() []ec2types.Subnet {
	return []ec2types.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []ec2types.Tag{
				{
					Key:   aws.String(defaultSubnetTag),
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

func buildMultipleValidBundleSubnets() []ec2types.Subnet {
	return []ec2types.Subnet{
		{
			SubnetId:         aws.String("test-id"),
			VpcId:            aws.String(defaultVpcId),
			AvailabilityZone: aws.String("test"),
			Tags: []ec2types.Tag{
				{
					Key:   aws.String(defaultSubnetTag),
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
			Tags: []ec2types.Tag{
				{
					Key:   aws.String(defaultSubnetTag),
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

func buildValidClusterSubnet(modifyFn func(ec2types.Subnet)) ec2types.Subnet {
	mock := ec2types.Subnet{
		SubnetId:         aws.String("test-id-2"),
		VpcId:            aws.String(defaultVpcId),
		AvailabilityZone: aws.String("test"),
		CidrBlock:        aws.String("10.0.0.0/24"),
		Tags: []ec2types.Tag{
			buildMockEc2Tag(func(e *ec2types.Tag) {
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

func buildStandaloneVPCAssociatedSubnets(subnetOne, subnetTwo string) []ec2types.Subnet {
	return []ec2types.Subnet{
		*buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, subnetOne),
		*buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, subnetTwo),
	}
}

func buildValidClusterVPC(cidrBlock string) []ec2types.Vpc {
	return []ec2types.Vpc{
		{
			VpcId:     aws.String(defaultVpcId),
			CidrBlock: aws.String(cidrBlock),
			Tags: []ec2types.Tag{
				{
					Key:   aws.String("test-vpc"),
					Value: aws.String("test-vpc"),
				},
				{
					Key:   aws.String(getOSDClusterTagKey(defaultInfraName)),
					Value: aws.String(clusterOwnedTagValue),
				},
				*genericToEc2Tag(resources.BuildManagedTag()),
			},
			State: ec2types.VpcStateAvailable,
		},
	}
}
func buildValidStandaloneVPCTags() []ec2types.Tag {
	return []ec2types.Tag{
		{
			Key:   aws.String(defaultSubnetTag),
			Value: aws.String(defaultInfraName),
		},
		*genericToEc2Tag(resources.BuildManagedTag()),
		{
			Key:   aws.String(resources.TagDisplayName),
			Value: aws.String(defaultVpcNameTagValue),
		},
	}
}

func buildValidStandaloneVPC(cidr string) *ec2types.Vpc {
	return &ec2types.Vpc{
		VpcId:     aws.String(defaultStandaloneVpcId),
		CidrBlock: aws.String(cidr),
		Tags:      buildValidStandaloneVPCTags(),
		State:     ec2types.VpcStateAvailable,
	}
}

func buildValidNonTaggedStandaloneVPC(cidr string) *ec2types.Vpc {
	return &ec2types.Vpc{
		VpcId:     aws.String(defaultVpcId),
		CidrBlock: aws.String(cidr),
		State:     ec2types.VpcStateAvailable,
	}
}

// the two below functions handle two cases inside CreateNetwork
// buildValidNetworkResponseVPCExists is used when we want to test case where the vpc
// already exists, i.e. go create subnets, subnet groups etc.
// buildValidNetworkResponseCreateVPC is used when we want to test case where no vpc exists
// i.e. create the vpc and return network response with vpc and all other resources are nil
func buildValidNetworkResponseVPCExists(cidr, vpcID, subnetOne, subnetTwo string) *Network {
	return &Network{
		Vpc: &ec2types.Vpc{
			CidrBlock: aws.String(cidr),
			VpcId:     aws.String(vpcID),
			Tags:      buildValidStandaloneVPCTags(),
			State:     ec2types.VpcStateAvailable,
		},
		Subnets: buildStandaloneVPCAssociatedSubnets(subnetOne, subnetTwo),
	}
}

func buildValidNetworkResponseCreateVPC(cidr, vpcID string) *Network {
	return &Network{
		Vpc: &ec2types.Vpc{
			CidrBlock: aws.String(cidr),
			VpcId:     aws.String(vpcID),
			Tags:      buildValidStandaloneVPCTags(),
			State:     ec2types.VpcStateAvailable,
		},
		Subnets: nil,
	}
}

func buildSortedStandaloneAZs() []ec2types.AvailabilityZone {
	return []ec2types.AvailabilityZone{
		{
			ZoneName: aws.String(defaultAzIdOne),
		},
		{
			ZoneName: aws.String(defaultAzIdTwo),
		},
	}
}
func buildMockInstanceTypeOffering() []ec2types.InstanceTypeOffering {
	return []ec2types.InstanceTypeOffering{
		ec2types.InstanceTypeOffering{
			InstanceType: ec2types.InstanceTypeT3Micro,
			Location:     aws.String(defaultAzIdOne),
			LocationType: ec2types.LocationTypeAvailabilityZone,
		},
		ec2types.InstanceTypeOffering{
			InstanceType: ec2types.InstanceTypeT3Micro,
			Location:     aws.String(defaultAzIdTwo),
			LocationType: ec2types.LocationTypeAvailabilityZone,
		},
	}
}

func buildValidCIDR(cidr string) *net.IPNet {
	_, ipnet, _ := net.ParseCIDR(cidr)
	return ipnet
}

func buildSubnetGroupID() string {
	return resources.ShortenString(fmt.Sprintf("%s-%s", defaultInfraName, "subnet-group"), defaultAwsIdentifierLength)
}

func buildSubnetGroupDescription() string {
	return fmt.Sprintf("%s-%s", defaultSubnetGroupDesc, "test")
}

func buildRDSSubnetGroup() []rdstypes.DBSubnetGroup {
	return []rdstypes.DBSubnetGroup{
		{
			DBSubnetGroupName: aws.String(buildSubnetGroupID()),
			VpcId:             aws.String(mockNetworkVpcId),
			DBSubnetGroupArn:  aws.String("subnetarn"),
		},
	}
}

func buildElasticacheSubnetGroup(modifyFn func(*elasticachetypes.CacheSubnetGroup)) *elasticachetypes.CacheSubnetGroup {
	mock := &elasticachetypes.CacheSubnetGroup{
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

// Mock client for EC2
type mockEc2Client struct {
	ec2.Client
	mock.Mock
	firstSubnet     *ec2types.Subnet
	secondSubnet    *ec2types.Subnet
	subnets         []ec2types.Subnet
	vpcs            []ec2types.Vpc
	vpc             *ec2types.Vpc
	secGroups       []ec2types.SecurityGroup
	azs             []ec2types.AvailabilityZone
	wantErrList     bool
	returnSecondSub bool

	createTagsFn                    func(ctx context.Context, input *ec2.CreateTagsInput, opts ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	describeVpcsFn                  func(ctx context.Context, input *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	describeSecurityGroupsFn        func(ctx context.Context, input *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	deleteSecurityGroupFn           func(ctx context.Context, input *ec2.DeleteSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
	describeVpcPeeringConnectionFn  func(ctx context.Context, input *ec2.DescribeVpcPeeringConnectionsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcPeeringConnectionsOutput, error)
	createVpcPeeringConnectionFn    func(ctx context.Context, input *ec2.CreateVpcPeeringConnectionInput, opts ...func(*ec2.Options)) (*ec2.CreateVpcPeeringConnectionOutput, error)
	acceptVpcPeeringConnectionFn    func(ctx context.Context, input *ec2.AcceptVpcPeeringConnectionInput, opts ...func(*ec2.Options)) (*ec2.AcceptVpcPeeringConnectionOutput, error)
	deleteVpcPeeringConnectionFn    func(ctx context.Context, input *ec2.DeleteVpcPeeringConnectionInput, opts ...func(*ec2.Options)) (*ec2.DeleteVpcPeeringConnectionOutput, error)
	describeRouteTablesFn           func(ctx context.Context, input *ec2.DescribeRouteTablesInput, opts ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	createRouteFn                   func(ctx context.Context, input *ec2.CreateRouteInput, opts ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)
	deleteRouteFn                   func(ctx context.Context, input *ec2.DeleteRouteInput, opts ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error)
	createVpcFn                     func(ctx context.Context, input *ec2.CreateVpcInput, opts ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
	deleteVpcFn                     func(ctx context.Context, input *ec2.DeleteVpcInput, opts ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error)
	WaitUntilVpcExistsFn            func(ctx context.Context, input *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) error
	createSubnetFn                  func(ctx context.Context, input *ec2.CreateSubnetInput, opts ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
	describeInstanceTypeOfferingsFn func(ctx context.Context, input *ec2.DescribeInstanceTypeOfferingsInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
	describeSubnetsFn               func(ctx context.Context, input *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	describeAvailabilityZonesFn     func(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	createSecurityGroupFn           func(ctx context.Context, input *ec2.CreateSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)

	// Optional call tracking if needed
	calls struct {
		DescribeRouteTables       []ec2.DescribeRouteTablesInput
		DescribeSecurityGroups    []ec2.DescribeSecurityGroupsInput
		DescribeAvailabilityZones []ec2.DescribeAvailabilityZonesInput
		DescribeSubnets           []struct {
			Input *ec2.DescribeSubnetsInput
		}
		DescribeVpcs []struct {
			Input *ec2.DescribeVpcsInput
		}
		CreateRoute []ec2.CreateRouteInput
	}
}

type mockRdsClient struct {
	mock.Mock
	rds.Client
	// Define function fields to mock specific method calls
	modifyDBSubnetGroupFn               func(ctx context.Context, input *rds.ModifyDBSubnetGroupInput, opts ...func(*rds.Options)) (*rds.ModifyDBSubnetGroupOutput, error)
	listTagsForResourceFn               func(ctx context.Context, input *rds.ListTagsForResourceInput, opts ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error)
	removeTagsFromResourceFn            func(ctx context.Context, input *rds.RemoveTagsFromResourceInput, opts ...func(*rds.Options)) (*rds.RemoveTagsFromResourceOutput, error)
	deleteDBSubnetGroupFn               func(ctx context.Context, input *rds.DeleteDBSubnetGroupInput, opts ...func(*rds.Options)) (*rds.DeleteDBSubnetGroupOutput, error)
	addTagsToResourceFn                 func(ctx context.Context, input *rds.AddTagsToResourceInput, opts ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error)
	describeDBSnapshotsFn               func(ctx context.Context, input *rds.DescribeDBSnapshotsInput, opts ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error)
	describeDBInstancesFn               func(ctx context.Context, input *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	describeDBSubnetGroupsFn            func(ctx context.Context, input *rds.DescribeDBSubnetGroupsInput, opts ...func(*rds.Options)) (*rds.DescribeDBSubnetGroupsOutput, error)
	describePendingMaintenanceActionsFn func(ctx context.Context, input *rds.DescribePendingMaintenanceActionsInput, opts ...func(*rds.Options)) (*rds.DescribePendingMaintenanceActionsOutput, error)
	applyPendingMaintenanceActionFn     func(ctx context.Context, input *rds.ApplyPendingMaintenanceActionInput, opts ...func(*rds.Options)) (*rds.ApplyPendingMaintenanceActionOutput, error)
	modifyDBInstanceFn                  func(ctx context.Context, input *rds.ModifyDBInstanceInput, opts ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error)
}

type mockElasticacheClient struct {
	mock.Mock
	elasticache.Client
	// Define function fields to mock specific method calls
	modifyCacheSubnetGroupFn    func(ctx context.Context, input *elasticache.ModifyCacheSubnetGroupInput, opts ...func(*elasticache.Options)) (*elasticache.ModifyCacheSubnetGroupOutput, error)
	deleteCacheSubnetGroupFn    func(ctx context.Context, input *elasticache.DeleteCacheSubnetGroupInput, opts ...func(*elasticache.Options)) (*elasticache.DeleteCacheSubnetGroupOutput, error)
	describeCacheSubnetGroupsFn func(ctx context.Context, input *elasticache.DescribeCacheSubnetGroupsInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeCacheSubnetGroupsOutput, error)
	describeCacheClustersFn     func(ctx context.Context, input *elasticache.DescribeCacheClustersInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
	describeReplicationGroupsFn func(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
	describeSnapshotsFn         func(ctx context.Context, input *elasticache.DescribeSnapshotsInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeSnapshotsOutput, error)
	createSnapshotFn            func(ctx context.Context, input *elasticache.CreateSnapshotInput, opts ...func(*elasticache.Options)) (*elasticache.CreateSnapshotOutput, error)
	deleteSnapshotFn            func(ctx context.Context, input *elasticache.DeleteSnapshotInput, opts ...func(*elasticache.Options)) (*elasticache.DeleteSnapshotOutput, error)
	describeUpdateActionsFn     func(ctx context.Context, input *elasticache.DescribeUpdateActionsInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeUpdateActionsOutput, error)
	modifyReplicationGroupFn    func(ctx context.Context, input *elasticache.ModifyReplicationGroupInput, opts ...func(*elasticache.Options)) (*elasticache.ModifyReplicationGroupOutput, error)
	batchApplyUpdateActionFn    func(ctx context.Context, input *elasticache.BatchApplyUpdateActionInput, opts ...func(*elasticache.Options)) (*elasticache.BatchApplyUpdateActionOutput, error)
	addTagsToResourceFn         func(ctx context.Context, input *elasticache.AddTagsToResourceInput, opts ...func(*elasticache.Options)) (*elasticache.AddTagsToResourceOutput, error)
	createReplicationGroupFn    func(ctx context.Context, input *elasticache.CreateReplicationGroupInput, opts ...func(*elasticache.Options)) (*elasticache.CreateReplicationGroupOutput, error)
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

func TestNetworkProvider_IsEnabled(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Logger    *logrus.Entry
		Client    client.Client
		Ec2Client EC2API
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
			//if no subnets exist in the cluster vpc then isEnabled will return true
			name: "verify isEnabled is true, no bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
			},
			want:    true,
			wantErr: false,
		},
		{
			// we expect isEnable to return false if a single subnet is found in cluster vpc
			name: "verify isEnabled is false, a single bundle subnet is found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					return mockEc2
				}(),
			},
			want:    false,
			wantErr: false,
		},
		{
			// we expect isEnable to return false if more than one subnet is found in cluster vpc
			name: "verify isEnabled is false, multiple bundle subnets found in cluster vpc",
			args: args{
				ctx: context.TODO(),
			},
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildMultipleValidBundleSubnets(),
					}, nil)
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{},
					}, nil)
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{},
					}, nil)
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
					}, nil)
					return mockEc2
				}(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:       tt.fields.Logger,
				Client:       tt.fields.Client,
				Ec2Client:    tt.fields.Ec2Client,
				IsSTSCluster: false,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		VpcWaiter         VpcWaiter
		Logger            *logrus.Entry
		IsSTSCluster      bool
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(validCIDREighteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildStandaloneSubnets(),
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRSixteen, defaultStandaloneVpcId),
			wantErr: false,
		},
		{
			name: "successfully build standalone vpc network  - CIDR /16 (sts cluster)",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRSixteen),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger:       logrus.NewEntry(logrus.StandardLogger()),
				IsSTSCluster: true,
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeVpcsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{*buildValidStandaloneVPC(validCIDRTwentySix)},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							*buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
							*buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
						},
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: buildMockInstanceTypeOffering(),
						NextToken:             nil,
						ResultMetadata:        middleware.Metadata{},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
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
			name: "successfully reconcile on non tagged standalone vpc",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidNonTaggedStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidNonTaggedStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DeleteVpcOutput)(nil),
						errorUtil.New("can't delete VPC, it does not exists"),
					)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("timeout"))
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{
						DBSubnetGroups: buildRDSSubnetGroup(),
					}, nil)
					mockRds.On("ModifyDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.ModifyDBSubnetGroupOutput{}, nil)
					mockRds.On("ListTagsForResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.ListTagsForResourceOutput{
						TagList: []rdstypes.Tag{
							{
								Key:   aws.String("something"),
								Value: aws.String("something value"),
							},
						},
					}, nil)
					mockRds.On("RemoveTagsFromResource", mock.Anything, mock.Anything, mock.Anything).Return((*rds.RemoveTagsFromResourceOutput)(nil), nil)
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.AddTagsToResourceOutput{}, nil)

					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil).Maybe()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneA && aws.ToString(input.AvailabilityZone) == defaultAzIdOne
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneB && aws.ToString(input.AvailabilityZone) == defaultAzIdTwo
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: buildMockInstanceTypeOffering(),
						NextToken:             nil,
						ResultMetadata:        middleware.Metadata{},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("ModifyCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							*buildElasticacheSubnetGroup(nil),
						},
					}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil).Maybe()
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: buildMockInstanceTypeOffering(),
						NextToken:             nil,
						ResultMetadata:        middleware.Metadata{},
					}, nil)
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneA && aws.ToString(input.AvailabilityZone) == defaultAzIdOne
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneB && aws.ToString(input.AvailabilityZone) == defaultAzIdTwo
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
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
			name: "successfully reconcile on standalone vpc - create subnets in large unsorted az zones list - zone one and two",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil).Maybe()
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil).Maybe()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneA && aws.ToString(input.AvailabilityZone) == defaultAzIdOne
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneB && aws.ToString(input.AvailabilityZone) == defaultAzIdTwo
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
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
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
			name: "successfully reconcile on standalone vpc - create correct subnets for vpc cidr block 10.0.50.0/23",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{*buildValidStandaloneVPC(validCIDRTwentyThree)},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentyThree),
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: buildMockInstanceTypeOffering(),
						NextToken:             nil,
						ResultMetadata:        middleware.Metadata{},
					}, nil)
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							fmt.Printf("Attempting to match Subnet 1: CidrBlock=%s, AZ=%s\n", aws.ToString(input.CidrBlock), aws.ToString(input.AvailabilityZone))
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskTwoA && aws.ToString(input.AvailabilityZone) == defaultAzIdOne
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskTwoA),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskTwoB && aws.ToString(input.AvailabilityZone) == defaultAzIdTwo
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskTwoB),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentyThree),
						},
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(defaultNonOverlappingCidr),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(
						awserr.New("VpcLimitExceeded", "The maximum number of VPCs has been reached.", nil))
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(defaultNonOverlappingCidr),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(
						&smithy.GenericAPIError{
							Code:    "InvalidVpcRange",
							Message: "The specified CIDR block range is not valid. The block range must be between a /28 netmask and /16 netmask",
						},
					)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			wantErr: true,
		},
		{
			name: "successfully error if vpc route table does not exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
						},
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
		},
		{
			name: "fail while reconciling vpc tags",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &configv1.Infrastructure{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						InfrastructureName: defaultInfraName,
						PlatformStatus: &configv1.PlatformStatus{
							Type: configv1.AWSPlatformType,
							AWS: &configv1.AWSPlatformStatus{
								Region: "eu-west-1",
							},
						},
					},
				}),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							{
								VpcId:     aws.String(defaultStandaloneVpcId),
								CidrBlock: aws.String(validCIDRTwentySix),
								Tags: []ec2types.Tag{
									{
										Key:   aws.String("integreatly.org/clusterID"),
										Value: aws.String("test"),
									},
								},
								State: ec2types.VpcStateAvailable,
							},
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = []ec2types.Subnet{}
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							*mockEc2.firstSubnet,
							*mockEc2.secondSubnet,
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								tags, _ := getDefaultTagSpec(context.TODO(), moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()), &resources.Tag{Key: resources.TagDisplayName, Value: defaultRouteTableNameTagValue}, string(ec2types.ResourceTypeRouteTable))
								table.Tags = tags[0].Tags
							}),
						},
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateTagsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRTwentySix),
			},
			wantErr: true,
		},
		{
			name: "verify untagged vpc is provided with correct tags",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &configv1.Infrastructure{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						InfrastructureName: defaultInfraName,
					},
				}),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything,
						mock.MatchedBy(func(input *ec2.DescribeVpcsInput) bool {
							return input != nil &&
								len(input.VpcIds) == 1 &&
								input.VpcIds[0] == defaultVpcId
						}),
						mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC(defaultNonOverlappingCidr),
					}, nil).Once()
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							ec2types.RouteTable{
								Associations:    []ec2types.RouteTableAssociation{},
								OwnerId:         nil,
								PropagatingVgws: nil,
								RouteTableId:    aws.String("test"),
								Routes:          nil,
								Tags:            buildValidStandaloneVPCTags(),
								VpcId:           aws.String(defaultVpcId),
							},
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					// runs when sts disabled
					vpc := buildValidNonTaggedStandaloneVPC(validCIDRSixteen)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							input := args.Get(1).(*ec2.CreateTagsInput)
							vpc.Tags = append(vpc.Tags, input.Tags...)
						}).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							input := args.Get(1).(*ec2.CreateVpcInput)
							if len(input.TagSpecifications) == 1 {
								vpc.Tags = append(vpc.Tags, input.TagSpecifications[0].Tags...)
							}
						}).Return(&ec2.CreateVpcOutput{Vpc: vpc}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger:       logrus.NewEntry(logrus.StandardLogger()),
				IsSTSCluster: false,
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			want:    buildValidNetworkResponseCreateVPC(validCIDRSixteen, defaultVpcId),
			wantErr: false,
		},
		{
			name: "verify untagged subnets are provided with correct tags",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &configv1.Infrastructure{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						InfrastructureName: defaultInfraName,
					},
				}),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("CreateDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.CreateDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.firstSubnet = buildUntaggedSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildUntaggedSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					// runs when sts disabled
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							input, ok := args.Get(1).(*ec2.CreateTagsInput)
							if !ok || input == nil {
								return
							}
							for _, resourceID := range input.Resources {
								if resourceID == aws.ToString(mockEc2.firstSubnet.SubnetId) {
									mockEc2.firstSubnet.Tags = append(mockEc2.firstSubnet.Tags, input.Tags...)
								} else if resourceID == aws.ToString(mockEc2.secondSubnet.SubnetId) {
									mockEc2.secondSubnet.Tags = append(mockEc2.secondSubnet.Tags, input.Tags...)
								}
							}
						}).
						Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneA && aws.ToString(input.AvailabilityZone) == defaultAzIdOne
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("CreateSubnet",
						mock.Anything,
						mock.MatchedBy(func(input *ec2.CreateSubnetInput) bool {
							return aws.ToString(input.CidrBlock) == defaultValidSubnetMaskOneB && aws.ToString(input.AvailabilityZone) == defaultAzIdTwo
						}),
						mock.Anything,
					).Return(&ec2.CreateSubnetOutput{
						Subnet:         buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB),
						ResultMetadata: middleware.Metadata{},
					}, nil).Once()
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					mockEc2.On("DescribeInstanceTypeOfferings", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{
								Location: aws.String(defaultAzIdOne),
							},
							{
								Location: aws.String(defaultAzIdTwo),
							},
						},
					}, nil)
					return mockEc2
				}(),
				VpcWaiter: func() VpcWaiter {
					mockVpcWaiter := new(MockVpcWaiter)
					mockVpcWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
					return mockVpcWaiter
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("CreateCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.CreateCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:  context.TODO(),
				CIDR: buildValidCIDR(validCIDRSixteen),
			},
			want:    buildValidNetworkResponseVPCExists(validCIDRTwentySix, defaultStandaloneVpcId, defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				VpcWaiter:         tt.fields.VpcWaiter,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      tt.fields.IsSTSCluster,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc and associated subnets",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen)},
					}, nil)
					mockEc2.subnets = buildStandaloneSubnets()
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{}, nil)
					mockElasticache.On("DescribeUpdateActions", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeUpdateActionsOutput{
						Marker:        nil,
						UpdateActions: []elasticachetypes.UpdateAction{},
					}, nil)
					mockElasticache.On("ModifyReplicationGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.ModifyReplicationGroupOutput{}, nil)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "verify deletion - of standalone vpc and associated subnets and subnet groups",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen)},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DeleteVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteVpcOutput{}, nil)
					mockEc2.subnets = buildStandaloneSubnets()
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					mockElasticache.On("DescribeCacheSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DescribeCacheSubnetGroupsOutput{
						CacheSubnetGroups: []elasticachetypes.CacheSubnetGroup{
							*buildElasticacheSubnetGroup(nil),
						},
					}, nil)
					return mockElasticache
				}(),
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
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {}),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {}),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {}),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *configv1.Network) {})),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.CidrBlock = aws.String("10.4.0.0/16")
							}),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *configv1.Network) {
					network.Spec.ClusterNetwork = []configv1.ClusterNetworkEntry{
						{
							CIDR: "",
						},
					}
					network.Spec.ServiceNetwork = []string{
						"",
					}
				})),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.CidrBlock = aws.String("")
							}),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), buildTestNetwork(func(network *configv1.Network) {
					network.Spec.ClusterNetwork = []configv1.ClusterNetworkEntry{
						{
							CIDR: "10.0.0.0/8",
						},
					}
					network.Spec.ServiceNetwork = []string{
						"172.0.0.0/8",
					}
				})),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {}),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
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
		Ec2Client  EC2API
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							ec2types.Subnet{
								SubnetId:         aws.String("test-id-2"),
								VpcId:            aws.String(defaultVpcId),
								AvailabilityZone: aws.String("test"),
								CidrBlock:        aws.String("10.0.0.0/24"),
								Tags: []ec2types.Tag{
									ec2types.Tag{},
								},
							},
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							{
								VpcId: aws.String(mockNetworkVpcId),
							},
						},
					}, nil)

					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeVpcPeeringConnectionsOutput)(nil), errors.New("test"))
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{},
					}, nil)
					mockEc2.On("CreateVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateVpcPeeringConnectionOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{},
					}, nil)
					mockEc2.On("CreateVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcPeeringConnectionOutput{
						VpcPeeringConnection: buildMockVpcPeeringConnection(nil),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateTagsOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{},
					}, nil)
					mockEc2.On("CreateVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcPeeringConnectionOutput{
						VpcPeeringConnection: buildMockVpcPeeringConnection(func(mock *ec2types.VpcPeeringConnection) {
							mock.Status.Code = ec2types.VpcPeeringConnectionStateReasonCodePendingAcceptance
						},
						),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateTagsOutput)(nil), nil)
					mockEc2.On("AcceptVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.AcceptVpcPeeringConnectionOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{},
					}, nil)
					mockEc2.On("CreateVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcPeeringConnectionOutput{
						VpcPeeringConnection: buildMockVpcPeeringConnection(func(mock *ec2types.VpcPeeringConnection) {
							mock.Status.Code = ec2types.VpcPeeringConnectionStateReasonCodeExpired
							mock.Status.Message = aws.String("")
						}),
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.CreateTagsOutput)(nil), nil)
					mockEc2.On("AcceptVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.AcceptVpcPeeringConnectionOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
				kubeClient: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
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
				Ec2Client:    tt.fields.Ec2Client,
				Client:       tt.fields.kubeClient,
				Logger:       tt.fields.logger,
				IsSTSCluster: false,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, genericAWSError)
					return mockEc2
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: "failed to get standalone vpc",
		},
		{
			name: "fails when cannot get vpc peering connection",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDREighteen),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
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
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
			}
			got, err := n.GetClusterNetworkPeering(tt.args.ctx)
			if err != nil && !errorContains(err, tt.wantErr) {
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
	}
	type args struct {
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("test"))
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{}, nil)
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DeleteVpcPeeringConnectionOutput)(nil), errors.New("test"))
					return mockEc2
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(func(connection *ec2types.VpcPeeringConnection) {
								connection.Status.Code = ec2types.VpcPeeringConnectionStateReasonCodeDeleting
							}),
						},
					}, nil)
					mockEc2.On("DeleteVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
					return mockEc2
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				peering: &NetworkPeering{PeeringConnection: buildMockVpcPeeringConnection(nil)},
			},
		},
		{
			name: "success when vpc deletion succeeds",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("DeleteVpcPeeringConnection", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DeleteVpcPeeringConnectionOutput)(nil), nil)
					return mockEc2
				}(),
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
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
			}
			if err := n.DeleteNetworkPeering(context.TODO(), tt.args.peering); err != nil && err.Error() != tt.wantErr {
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					// call describe security groups twice to hit both conditionals
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
						},
					}, nil).Once()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.GroupName = aws.String("not test security group id")
							}),
						},
					}, nil).Once()
					// call describe route table twice to hit both conditionals
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String("kubernetes.io/cluster/test")
										tag.Value = aws.String("owned")
									}),
								}
							}),
						},
					}, nil).Once()
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String(defaultSubnetTag)
										tag.Value = aws.String("test")
									}),
								}
							}),
						},
					}, nil).Once()
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateRouteOutput{}, nil)
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateSecurityGroupOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want:    buildMockNetworkConnection(nil),
			wantErr: false,
		},
		{
			name: "test successful security group creation with Firewall and Private Link Route Tables",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					// call describe security groups twice to hit both conditionals
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
						},
					}, nil).Once()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.GroupName = aws.String("not test security group id")
							}),
						},
					}, nil).Once()
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							calls := mockEc2.CreateRouteCalls()
							if len(calls) == 1 {
								args[0] = nil
								args[1] = errors.New("RouteNotSupported: Route table contains routes that do not target a network interface")
								return
							}
							args[0] = &ec2.CreateRouteOutput{}
							args[1] = nil
						}).
						Return(&ec2.CreateRouteOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String(defaultSubnetTag)
										tag.Value = aws.String("test")
									}),
								}
							}),
						},
					}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want:    buildMockNetworkConnection(nil),
			wantErr: false,
		},
		{
			name: "test error on route table route creation error",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					// call describe security groups twice to hit both conditionals
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
						},
					}, nil).Once()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.GroupName = aws.String("not test security group id")
							}),
						},
					}, nil).Once()
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							calls := mockEc2.DescribeRouteTablesCalls()
							var output *ec2.DescribeRouteTablesOutput
							if len(calls) == 1 {
								output = &ec2.DescribeRouteTablesOutput{
									RouteTables: []ec2types.RouteTable{
										*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
											table.Tags = []ec2types.Tag{
												buildMockEc2Tag(func(e *ec2types.Tag) {
													e.Key = aws.String("kubernetes.io/cluster/test")
													e.Value = aws.String("owned")
												}),
											}
										}),
									},
								}
							} else {
								output = &ec2.DescribeRouteTablesOutput{
									RouteTables: []ec2types.RouteTable{
										*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
											table.Tags = []ec2types.Tag{
												buildMockEc2Tag(func(e *ec2types.Tag) {
													e.Key = aws.String(defaultSubnetTag)
													e.Value = aws.String("test")
												}),
											}
										}),
									},
								}
							}

							args[0] = output
							args[1] = nil
						}).
						Return(&ec2.DescribeRouteTablesOutput{}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).
						Return(errors.New("OtherError: Route table contains routes that do not target a network interface"), nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: true,
		},
		{
			name: "error creating security group",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					// call describe security groups twice to hit both conditionals
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
						},
					}, nil).Once()
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.GroupName = aws.String("not test security group id")
							}),
						},
					}, nil).Once()
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			wantErr: true,
		},
		{
			name: "test security group exists with no tags",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String(defaultSubnetTag)
										tag.Value = aws.String("test")
									}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateRouteOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {}),
			},
			wantErr: false,
		},
		{
			name: "test security group exists with tags and invalid permissions",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String(defaultSubnetTag)
										tag.Value = aws.String("test")
									}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateRouteOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
					group.Tags = []ec2types.Tag{
						buildMockEc2Tag(func(e *ec2types.Tag) {}),
						buildMockEc2Tag(func(e *ec2types.Tag) {
							e.Key = aws.String(resources.TagDisplayName)
							e.Value = aws.String(defaultVpcNameTagValue)
						}),
					}
				}),
			},
			wantErr: false,
		},
		{
			name: "test security group exists with tags and valid permissions",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
								}
								group.IpPermissions = []ec2types.IpPermission{
									*buildMockEc2IpPermission(func(permission *ec2types.IpPermission) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
								table.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(tag *ec2types.Tag) {
										tag.Key = aws.String(defaultSubnetTag)
										tag.Value = aws.String("test")
									}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcPeeringConnections", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcPeeringConnectionsOutput{
						VpcPeeringConnections: []ec2types.VpcPeeringConnection{
							*buildMockVpcPeeringConnection(nil),
						},
					}, nil)
					mockEc2.On("CreateRoute", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateRouteOutput{}, nil)
					mockEc2.On("CreateTags", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateTagsOutput{}, nil)
					mockEc2.On("AuthorizeSecurityGroupIngress", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.AuthorizeSecurityGroupIngressOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx:     context.TODO(),
				network: buildMockNetwork(nil),
			},
			want: &NetworkConnection{
				StandaloneSecurityGroup: buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
					group.Tags = []ec2types.Tag{
						buildMockEc2Tag(func(e *ec2types.Tag) {}),
						buildMockEc2Tag(func(e *ec2types.Tag) {
							e.Key = aws.String(resources.TagDisplayName)
							e.Value = aws.String(defaultVpcNameTagValue)
						}),
					}
					group.IpPermissions = []ec2types.IpPermission{
						*buildMockEc2IpPermission(func(permission *ec2types.IpPermission) {}),
					}
				}),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).
						Return(&ec2.DescribeRouteTablesOutput{
							RouteTables: []ec2types.RouteTable{
								*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
									table.Tags = []ec2types.Tag{
										buildMockEc2Tag(func(e *ec2types.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2types.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete security group is called if security group is not nil and is a security group provisioned by cro",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
								}
								group.IpPermissions = []ec2types.IpPermission{
									*buildMockEc2IpPermission(func(permission *ec2types.IpPermission) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).
						Return(&ec2.DescribeRouteTablesOutput{
							RouteTables: []ec2types.RouteTable{
								*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
									table.Tags = []ec2types.Tag{
										buildMockEc2Tag(func(e *ec2types.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2types.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete security group is not called if security groups are found but not a cro provisioned security group",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.GroupName = aws.String("not a cro security group")
							}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).
						Return(&ec2.DescribeRouteTablesOutput{
							RouteTables: []ec2types.RouteTable{
								*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
									table.Tags = []ec2types.Tag{
										buildMockEc2Tag(func(e *ec2types.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2types.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
		{
			name: "ensure ec2 delete routes is called",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildMockEc2SecurityGroup(func(group *ec2types.SecurityGroup) {
								group.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
								}
								group.IpPermissions = []ec2types.IpPermission{
									*buildMockEc2IpPermission(func(permission *ec2types.IpPermission) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).
						Return(&ec2.DescribeRouteTablesOutput{
							RouteTables: []ec2types.RouteTable{
								*buildMockEc2RouteTable(func(table *ec2types.RouteTable) {
									table.Tags = []ec2types.Tag{
										buildMockEc2Tag(func(e *ec2types.Tag) {
											e.Key = aws.String("kubernetes.io/cluster/test")
											e.Value = aws.String("owned")
										}),
									}
								}),
							},
						}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildMockVpc(func(vpc *ec2types.Vpc) {
								vpc.VpcId = aws.String(defaultStandaloneVpcId)
								vpc.CidrBlock = aws.String(validCIDRTwentySix)
								vpc.Tags = []ec2types.Tag{
									buildMockEc2Tag(func(e *ec2types.Tag) {
										e.Key = aws.String(resources.TagDisplayName)
										e.Value = aws.String(defaultVpcNameTagValue)
									}),
									buildMockEc2Tag(func(e *ec2types.Tag) {}),
								}
							}),
						},
					}, nil)
					mockEc2.On("DeleteRoute", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteRouteOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					return mockElasticache
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				networkPeering: &NetworkPeering{
					PeeringConnection: buildMockVpcPeeringConnection(func(connection *ec2types.VpcPeeringConnection) {

					}),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
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
		Client            client.Client
		RdsClient         RDSAPI
		Ec2Client         EC2API
		ElasticacheClient ElastiCacheAPI
		Logger            *logrus.Entry
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildSecurityGroup(func(mock *ec2types.SecurityGroup) {
								mock.GroupName = aws.String("testsecuritygroup")
								mock.VpcId = aws.String("testID")
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC("10.0.0.0/23"),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: false,
		},
		{
			name: "error building bundle subnet group resource name on deletion",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
		},
		{
			name: "error getting ec2 security group",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return((*ec2.DescribeSecurityGroupsOutput)(nil), genericAWSError)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
		},
		{
			name: "retrieved ec2 security group that is nil",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildSecurityGroup(func(mock *ec2types.SecurityGroup) {
								mock.GroupName = aws.String("testsecuritygroup")
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(
						&elasticache.DeleteCacheSubnetGroupOutput{}, errors.New("error: Cache subnet group not found"))
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).
						Return(&rds.DeleteDBSubnetGroupOutput{}, errors.New("error: Cache subnet group not found"))
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).
						Return(&elasticache.DeleteCacheSubnetGroupOutput{}, genericAWSError)
					return mockElasticache
				}(),
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).
						Return(&rds.DeleteDBSubnetGroupOutput{}, genericAWSError)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
			},
			args: args{
				ctx: context.TODO(),
			},
			wantErr: true,
		},
		{
			name: "return error when aws error returned on deletesecuritygroup",
			fields: fields{
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				RdsClient: func() RDSAPI {
					mockRds := new(mock_RdsClient)
					mockRds.On("DeleteDBSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DeleteDBSubnetGroupOutput{}, nil)
					return mockRds
				}(),
				Ec2Client: func() EC2API {
					mockEc2 := new(mock_Ec2Client)
					mockEc2.On("DeleteSecurityGroup", mock.Anything, mock.Anything, mock.Anything).
						Return((*ec2.DeleteSecurityGroupOutput)(nil), genericAWSError)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []ec2types.SecurityGroup{
							*buildSecurityGroup(func(mock *ec2types.SecurityGroup) {
								mock.GroupName = aws.String("testsecuritygroup")
								mock.VpcId = aws.String("testID")
							}),
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildValidClusterVPC("10.0.0.0/23"),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							buildValidClusterSubnet(nil),
						},
					}, nil)
					return mockEc2
				}(),
				ElasticacheClient: func() ElastiCacheAPI {
					mockElasticache := new(mock_ElasticacheClient)
					mockElasticache.On("DeleteCacheSubnetGroup", mock.Anything, mock.Anything, mock.Anything).Return(&elasticache.DeleteCacheSubnetGroupOutput{}, nil)
					return mockElasticache
				}(),
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
				Client:            tt.fields.Client,
				RdsClient:         tt.fields.RdsClient,
				Ec2Client:         tt.fields.Ec2Client,
				ElasticacheClient: tt.fields.ElasticacheClient,
				Logger:            tt.fields.Logger,
				IsSTSCluster:      false,
			}
			n.DeleteBundledCloudResources(tt.args.ctx)
			if err := n.DeleteBundledCloudResources(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("NetworkProvider.DeleteBundledCloudResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
