// utility to manage a standalone vpc for the resources created by the cloud resource operator.
//
// this has been added to allow the operator to work with clusters provisioned with openshift tooling >= 4.4.6,
// as they will not allow multi-az cloud resources to be created in single-az openshift clusters due to the single-az
// cluster subnets taking up all networking addresses in the cluster vpc.
//
// any openshift clusters that have used the cloud resource operator before this utility was added will be using the old approach,
// which is bundling cloud resources in with the cluster vpc. backwards compatibility for this approach must be maintained.
//
// see [1] for more details.
//
// [1] https://docs.google.com/document/d/1UWfon-tBNfiDS5pJRAUqPXoJuUUqO1P4B6TTR8SMqSc/edit?usp=sharing
//
// terminology
// bundled: refers to networking resources installed using the old approach
// standalone: refers to networking resources installed using the new approach

package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"

	errorUtil "github.com/pkg/errors"
)

const (
	DefaultRHMIVpcNameTagKey       = "Name"
	DefaultRHMIVpcNameTagValue     = "RHMI Cloud Resource VPC"
	DefaultRHMISubnetNameTagValue  = "RHMI Cloud Resource Subnet"
	defaultNumberOfExpectedSubnets = 2
)

// wrapper for ec2 vpcs, to allow for extensibility
type Network struct {
	Vpc     *ec2.Vpc
	Subnets []*ec2.Subnet
}

// used to map expected ip addresses to availability zones
type NetworkAZSubnet struct {
	IP net.IPNet
	AZ *ec2.AvailabilityZone
}

type NetworkManager interface {
	CreateNetwork(context.Context, *net.IPNet) (*Network, error)
	DeleteNetwork(context.Context) error
	IsEnabled(context.Context) (bool, error)
}

var _ NetworkManager = (*NetworkProvider)(nil)

type NetworkProvider struct {
	Client         client.Client
	RdsApi         rdsiface.RDSAPI
	Ec2Api         ec2iface.EC2API
	ElasticacheApi elasticacheiface.ElastiCacheAPI
	Logger         *logrus.Entry
}

func NewNetworkManager(session *session.Session, client client.Client, logger *logrus.Entry) *NetworkProvider {
	return &NetworkProvider{
		Client:         client,
		RdsApi:         rds.New(session),
		Ec2Api:         ec2.New(session),
		ElasticacheApi: elasticache.New(session),
		Logger:         logger.WithField("provider", "standalone_network_provider"),
	}
}

//CreateNetwork returns a Network type or error
//
//VPC's created by the cloud resource operator are identified by having a tag with the name `<organizationTag>/clusterID`.
//By default, `integreatly.org/clusterID`.
//
//CreateNetwork does:
//    * create a VPC with CIDR block and tag it, if a VPC does not exist,
//	* reconcile on subnets and subnet groups
//
//CreateNetwork does not:
//	* reconcile the vpc if the VPC already exist (this is to avoid potential changes to the CIDR range and unwanted/unexpected behaviour)
func (n *NetworkProvider) CreateNetwork(ctx context.Context, vpcCidrBlock *net.IPNet) (*Network, error) {
	logger := n.Logger.WithField("action", "CreateNetwork")
	logger.Debug("CreateNetwork called")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	// check if there is cluster specific vpc already created.
	foundVpc, err := getStandaloneVpc(n.Ec2Api, logger, clusterID, organizationTag)
	if err != nil {
		return nil, errorUtil.Wrap(err, "unable to get vpc")
	}
	if foundVpc == nil {
		//VPCs must be created with a valid CIDR Block range, between \16 and \26
		//if an invalid range is passed, the function returns an error
		//
		//VPCs are tagged with the name `<organizationTag>/clusterID`.
		//By default, `integreatly.org/clusterID`.
		//
		//NOTE - Once a VPC is created we do not want to update it. To avoid changing cidr block
		if !isValidCIDRRange(vpcCidrBlock) {
			return nil, errorUtil.New(fmt.Sprintf("%s is out of range, block sizes must be between `/16` and `/26`, please update `_network` strategy", vpcCidrBlock.String()))
		}

		logger.Infof("cidr %s is valid 👍", vpcCidrBlock.String())

		// create vpc using cidr string from _network
		createVpcOutput, err := n.Ec2Api.CreateVpc(&ec2.CreateVpcInput{
			CidrBlock: aws.String(vpcCidrBlock.String()),
		})
		ec2Err, isAwsErr := err.(awserr.Error)
		if err != nil && isAwsErr && ec2Err.Code() == "InvalidVpc.Range" {
			return nil, errorUtil.New(fmt.Sprintf("%s is out of range, block sizes must be between `/16` and `/26`, please update `_network` strategy", vpcCidrBlock.String()))
		}
		if err != nil {
			return nil, errorUtil.Wrap(err, "unexpected error creating vpc")
		}
		logger.Infof("creating vpc: %s for clusterID: %s", *createVpcOutput.Vpc.VpcId, clusterID)

		// ensure standalone vpc has correct tags
		clusterIDTag := &ec2.Tag{
			Key:   aws.String(fmt.Sprintf("%sclusterID", organizationTag)),
			Value: aws.String(clusterID),
		}
		if err = n.reconcileVPCTags(createVpcOutput.Vpc, clusterIDTag); err != nil {
			return nil, errorUtil.Wrapf(err, "unexpected error while reconciling vpc tags")
		}

		return &Network{
			Vpc:     createVpcOutput.Vpc,
			Subnets: nil,
		}, nil
	}

	// reconciling on vpc networking, ensuring the following are present :
	//     * subnets (2 private)
	//     * subnet groups -> rds and elasticache
	//     * TODO security groups - https://issues.redhat.com/browse/INTLY-8105
	//     * TODO route table configuration - https://issues.redhat.com/browse/INTLY-8106
	//
	privateSubnets, err := n.reconcileStandaloneVPCSubnets(ctx, n.Logger, foundVpc, clusterID, organizationTag)
	if err != nil {
		return nil, errorUtil.Wrap(err, "unexpected error creating vpc subnets")
	}

	// create rds subnet group
	if err = n.reconcileRDSVPCConfiguration(ctx, privateSubnets, clusterID); err != nil {
		return nil, errorUtil.Wrap(err, "unexpected error reconciling standalone rds vpc networking")
	}

	// create elasticache subnet groups
	if err = n.reconcileElasticacheVPCConfiguration(ctx, privateSubnets); err != nil {
		return nil, errorUtil.Wrap(err, "unexpected error reconciling standalone elasticache vpc networking")
	}

	return &Network{
		Vpc:     foundVpc,
		Subnets: privateSubnets,
	}, nil
}

//DeleteNetwork returns an error
//
//VPCs are tagged with with the name `<organizationTag>/clusterID`.
//By default, `integreatly.org/clusterID`.
//
//This tag is used to find a standalone VPC
//If found DeleteNetwork will attempt to remove:
//	* all vpc associated subnets
//	* both subnet groups (rds and elasticache)
//	* the vpc
func (n *NetworkProvider) DeleteNetwork(ctx context.Context) error {
	logger := n.Logger.WithField("action", "DeleteNetwork")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	//check if there is a standalone vpc already created.
	foundVpc, err := getStandaloneVpc(n.Ec2Api, logger, clusterID, organizationTag)
	if err != nil {
		return errorUtil.Wrap(err, "unable to get vpc")
	}
	if foundVpc == nil {
		logger.Infof("no vpc found for clusterID: %s", clusterID)
		return nil
	}

	vpcSubs, err := getVPCAssociatedSubnets(n.Ec2Api, logger, foundVpc)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get standalone vpc subnets")
	}

	for _, subnet := range vpcSubs {
		logger.Infof("attempting to delete subnet with id: %s", *subnet.SubnetId)
		_, err = n.Ec2Api.DeleteSubnet(&ec2.DeleteSubnetInput{
			SubnetId: aws.String(*subnet.SubnetId),
		})
		if err != nil {
			return errorUtil.Wrapf(err, "failed to delete subnet with id: %s", *subnet.SubnetId)
		}
	}

	subnetGroupName, err := BuildInfraName(ctx, n.Client, defaultSubnetPostfix, DefaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrap(err, "error building subnet group name")
	}

	rdsSubnetGroup, err := getRDSSubnetByGroup(n.RdsApi, subnetGroupName)
	if err != nil {
		return errorUtil.Wrap(err, "error getting subnet group on delete")
	}
	if rdsSubnetGroup != nil {
		logger.Infof("attempting to delete subnetgroup name: %s for clusterID: %s", *rdsSubnetGroup.DBSubnetGroupName, *rdsSubnetGroup.VpcId)
		_, err := n.RdsApi.DeleteDBSubnetGroup(&rds.DeleteDBSubnetGroupInput{
			DBSubnetGroupName: rdsSubnetGroup.DBSubnetGroupName,
		})
		if err != nil {
			return errorUtil.Wrap(err, "error deleting subnet group")
		}
	}

	elasticacheSubnetGroup, err := getElasticacheSubnetByGroup(n.ElasticacheApi, subnetGroupName)
	if err != nil {
		return errorUtil.Wrap(err, "error getting subnet group on delete")
	}
	if elasticacheSubnetGroup != nil {
		_, err := n.ElasticacheApi.DeleteCacheSubnetGroup(&elasticache.DeleteCacheSubnetGroupInput{
			CacheSubnetGroupName: aws.String(*elasticacheSubnetGroup.CacheSubnetGroupName),
		})
		if err != nil {
			return errorUtil.Wrap(err, "error deleting subnet group")
		}
	}

	logger.Infof("attempting to delete vpc id: %s for clusterID: %s", *foundVpc.VpcId, clusterID)
	_, err = n.Ec2Api.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: aws.String(*foundVpc.VpcId),
	})
	if err != nil {
		return errorUtil.Wrap(err, "unable to delete vpc")
	}
	logger.Infof("vpc %s deleted successfully", *foundVpc.VpcId)
	return nil
}

//IsEnabled returns true when no bundled subnets are found in the openshift cluster vpc.
//
//All subnets created by the cloud resource operator are identified by having a tag with the name `<organizationTag>/clusterID`.
//By default, `integreatly.org/clusterID`.
//
//this check allows us to maintain backwards compatibility with openshift clusters that used the cloud resource operator before this standalone vpc provider was added.
//If this function returns false, we should continue using the backwards compatible approach of bundling resources in with the openshift cluster vpc.
func (n *NetworkProvider) IsEnabled(ctx context.Context) (bool, error) {
	logger := n.Logger.WithField("action", "isEnabled")

	//check if there is a standalone vpc already created.
	foundVpc, err := getClusterVpc(ctx, n.Client, n.Ec2Api, logger)
	if err != nil {
		return false, errorUtil.Wrap(err, "unable to get vpc")
	}

	// returning subnets from cluster vpc
	logger.Info("getting cluster vpc subnets")
	vpcSubnets, err := GetVPCSubnets(n.Ec2Api, logger, foundVpc)
	if err != nil {
		return false, errorUtil.Wrap(err, "error happened while returning vpc subnets")
	}

	// iterate all cluster vpc's checking for valid bundled vpc subnets
	organizationTag := resources.GetOrganizationTag()
	var validBundledVPCSubnets []*ec2.Subnet
	for _, subnet := range vpcSubnets {
		for _, tag := range subnet.Tags {
			if aws.StringValue(tag.Key) == fmt.Sprintf("%sclusterID", organizationTag) {
				validBundledVPCSubnets = append(validBundledVPCSubnets, subnet)
				logger.Infof("found bundled vpc subnet %s in cluster vpc %s", *subnet.SubnetId, *subnet.VpcId)
			}
		}
	}
	logger.Infof("found %d bundled vpc subnets in cluster vpc", len(validBundledVPCSubnets))
	return len(validBundledVPCSubnets) == 0, nil
}

//reconcileStandaloneVPCSubnets returns an array list of private subnets associated with a vpc or an error
//
//each standalone vpc cidr block is split in half, to create two private subnets.
//these subnets are located in different az's
//the az is determined by the cro strategy, either provided by override config map or provided by the infrastructure CR
func (n *NetworkProvider) reconcileStandaloneVPCSubnets(ctx context.Context, logger *logrus.Entry, vpc *ec2.Vpc, clusterID, organizationTag string) ([]*ec2.Subnet, error) {
	logger.Info("gathering all private subnets in cluster vpc")

	// build our subnets, so we know if the vpc /26 then we /27
	if *vpc.CidrBlock == "" {
		return nil, errorUtil.New("standalone vpc cidr block can't be empty")
	}

	// AWS stores it's CIDR block as a string, convert it
	_, awsCIDR, err := net.ParseCIDR(*vpc.CidrBlock)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to parse vpc cidr block %s", *vpc.CidrBlock)
	}
	// Get the cluster VPC mask size
	// e.g. If the cluster VPC CIDR block is 10.0.0.0/8, the size is 8 (8 bits)
	maskSize, _ := awsCIDR.Mask.Size()

	// If the VPC CIDR mask size is greater or equal to the size that CRO requires
	// - If equal, CRO will not be able to subdivide the VPC CIDR into sub-networks
	// - If greater, there will be fewer host addresses available in the sub-networks than CRO needs
	// Note: The larger the mask size, the less hosts the network can support
	if maskSize >= defaultSubnetMask {
		return nil, errorUtil.New(fmt.Sprintf("vpc cidr block %s cannot contain generated subnet mask /%d", *vpc.CidrBlock, defaultSubnetMask))
	}

	// Split vpc cidr mask by increasing mask by 1
	halfMaskStr := fmt.Sprintf("%s/%d", awsCIDR.IP.String(), maskSize+1)
	_, halfMaskCidr, err := net.ParseCIDR(halfMaskStr)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to parse half mask cidr block %s", halfMaskStr)
	}

	// Generate 2 valid sub-networks that can be used in the cluster VPC CIDR range
	validSubnets := generateAvailableSubnets(awsCIDR, halfMaskCidr)
	if len(validSubnets) != defaultNumberOfExpectedSubnets {
		return nil, errorUtil.New(fmt.Sprintf("expected at least two subnet ranges, found %s", validSubnets))
	}

	// get a list of valid availability zones
	var validAzs []*ec2.AvailabilityZone
	azs, err := n.Ec2Api.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting availability zones")
	}

	// sort the azs first
	sort.Sort(azByZoneName(azs.AvailabilityZones))

	for index, az := range azs.AvailabilityZones {
		validAzs = append(validAzs, az)
		if index == 1 {
			break
		}
	}
	if len(validAzs) != defaultNumberOfExpectedSubnets {
		return nil, errorUtil.New(fmt.Sprintf("expected 2 availability zones, found %s", validAzs))
	}

	// validSubnets and validAzs contain the same index (2 items)
	// to mitigate the chance of a nil pointer during subnet creation,
	// both azs and subnets are mapped to type `NetworkAZSubnet`
	var expectedAZSubnets []*NetworkAZSubnet
	for subnetIndex, subnet := range validSubnets {
		for azIndex, az := range validAzs {
			if azIndex == subnetIndex {
				azSubnet := &NetworkAZSubnet{
					IP: subnet,
					AZ: az,
				}
				expectedAZSubnets = append(expectedAZSubnets, azSubnet)
			}
		}
	}

	// check expected subnets exist in expect az
	// filter based on a tag key attached to private subnets
	// get subnets in vpc
	subs, err := getVPCAssociatedSubnets(n.Ec2Api, logger, vpc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpc subnets")
	}
	// for create a subnet for every expected subnet to exist
	for _, expectedAZSubnet := range expectedAZSubnets {
		if !subnetExists(subs, expectedAZSubnet.IP.String()) {
			zoneName := expectedAZSubnet.AZ.ZoneName
			logger.Infof("attempting to create subnet with cidr block %s for vpc %s in zone %s", expectedAZSubnet.IP.String(), *vpc.VpcId, *zoneName)
			createOutput, err := n.Ec2Api.CreateSubnet(&ec2.CreateSubnetInput{
				AvailabilityZone: aws.String(*zoneName),
				CidrBlock:        aws.String(expectedAZSubnet.IP.String()),
				VpcId:            aws.String(*vpc.VpcId),
			})
			ec2err, isAwsErr := err.(awserr.Error)
			if err != nil && isAwsErr && ec2err.Code() == "InvalidSubnet.Conflict" {
				// if two or more crs are created at the same time the network provider may run in parallel
				// in this case it's expected that there will be a conflict, as they will each be reconciling the required subnets
				// one will get in first and the following ones will see the expected conflict as the subnet is already created
				logger.Debugf("%s conflicts with a current subnet", expectedAZSubnet.IP.String())
			}
			if err != nil {
				return nil, errorUtil.Wrap(err, "error creating new subnet")
			}
			if newErr := tagPrivateSubnet(ctx, n.Client, n.Ec2Api, createOutput.Subnet, logger); newErr != nil {
				return nil, newErr
			}

			subs = append(subs, createOutput.Subnet)
			logger.Infof("created new subnet %s in %s", expectedAZSubnet.IP.String(), *vpc.VpcId)
		}
	}

	for _, sub := range subs {
		logger.Infof("validating subnet %s", *sub.SubnetId)
		if !tagsContains(ec2TagsToGeneric(sub.Tags), defaultAWSPrivateSubnetTagKey, "1") ||
			!tagsContains(ec2TagsToGeneric(sub.Tags), fmt.Sprintf("%sclusterID", organizationTag), clusterID) ||
			!tagsContains(ec2TagsToGeneric(sub.Tags), "Name", DefaultRHMISubnetNameTagValue) {
			if err := tagPrivateSubnet(ctx, n.Client, n.Ec2Api, sub, logger); err != nil {
				return nil, errorUtil.Wrap(err, "failed to tag subnet")
			}
		}
	}

	return subs, nil
}

//reconcileVPCTags will tag a VPC or return an error
//
//VPCs are tagged with with the name `<organizationTag>/clusterID`.
//By default, `integreatly.org/clusterID`.
func (n *NetworkProvider) reconcileVPCTags(vpc *ec2.Vpc, clusterIDTag *ec2.Tag) error {
	logger := n.Logger.WithField("action", "reconcileVPCTags")

	vpcTags := ec2TagsToGeneric(vpc.Tags)
	if !tagsContains(vpcTags, DefaultRHMIVpcNameTagKey, DefaultRHMIVpcNameTagValue) ||
		!tagsContains(vpcTags, *clusterIDTag.Key, *clusterIDTag.Value) {

		_, err := n.Ec2Api.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{
				aws.String(*vpc.VpcId),
			},
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(DefaultRHMIVpcNameTagKey),
					Value: aws.String(DefaultRHMIVpcNameTagValue),
				}, clusterIDTag,
			},
		})
		if err != nil {
			return errorUtil.Wrap(err, "unable to tag vpc")
		}
		logger.Infof("successfully tagged vpc: %s for clusterID: %s", *vpc.VpcId, *clusterIDTag.Value)
	}
	return nil
}

//an rds subnet group is required to be in place when provisioning rds resources
//
//reconcileRDSVPCConfiguration ensures that an rds subnet group is created with 2 private subnets
func (n *NetworkProvider) reconcileRDSVPCConfiguration(ctx context.Context, privateVPCSubnets []*ec2.Subnet, clusterID string) error {
	logger := n.Logger.WithField("action", "reconcileRDSVPCConfiguration")
	logger.Info("ensuring rds subnet groups in vpc are as expected")
	// get subnet group id
	subnetGroupName, err := BuildInfraName(ctx, n.Client, defaultSubnetPostfix, DefaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrap(err, "error building subnet group name")
	}

	foundSubnet, err := getRDSSubnetByGroup(n.RdsApi, subnetGroupName)
	if foundSubnet != nil {
		logger.Infof("subnet group %s found", *foundSubnet.DBSubnetGroupName)
		return nil
	}

	// in the case of no private subnets being found, we return a less verbose error message compared to obscure aws error message
	if len(privateVPCSubnets) == 0 {
		return errorUtil.New("no private subnets found, can not create subnet group for rds")
	}

	// build array list of all vpc private subnets
	var subnetIDs []*string
	for _, subnet := range privateVPCSubnets {
		subnetIDs = append(subnetIDs, subnet.SubnetId)
	}

	// build subnet group input
	subnetGroupInput := &rds.CreateDBSubnetGroupInput{
		DBSubnetGroupDescription: aws.String(defaultSubnetGroupDesc),
		DBSubnetGroupName:        aws.String(subnetGroupName),
		SubnetIds:                subnetIDs,
		Tags: []*rds.Tag{
			{
				Key:   aws.String("cluster"),
				Value: aws.String(clusterID),
			},
		},
	}

	// create db subnet group
	logger.Infof("creating resource subnet group %s", *subnetGroupInput.DBSubnetGroupName)
	if _, err := n.RdsApi.CreateDBSubnetGroup(subnetGroupInput); err != nil {
		return errorUtil.Wrap(err, "unable to create db subnet group")
	}
	return nil
}

//It is required to have an elasticache subnet group in place when provisioning elasticache resources
//
//reconcileElasticacheVPCConfiguration ensures that an elasticache subnet group is created with 2 private subnets
func (n *NetworkProvider) reconcileElasticacheVPCConfiguration(ctx context.Context, privateVPCSubnets []*ec2.Subnet) error {
	logger := n.Logger.WithField("action", "reconcileElasticacheVPCConfiguration")
	logger.Info("ensuring elasticache subnet groups in vpc are as expected")
	// get subnet group id
	subnetGroupName, err := BuildInfraName(ctx, n.Client, defaultSubnetPostfix, DefaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrap(err, "error building subnet group name")
	}

	// check if group exists
	subnetGroup, err := getElasticacheSubnetByGroup(n.ElasticacheApi, subnetGroupName)
	if err != nil {
		return errorUtil.Wrap(err, "error getting elasticache subnet group on reconcile")
	}
	if subnetGroup != nil {
		logger.Infof("subnet group %s found", *subnetGroup.CacheSubnetGroupName)
		return nil
	}

	// in the case of no private subnets found, a less verbose error message compared to obscure aws error message is returned
	if len(privateVPCSubnets) == 0 {
		return errorUtil.New("no private subnets found, can not create subnet group for rds")
	}

	// build array list of all vpc private subnets
	var subnetIDs []*string
	for _, subnet := range privateVPCSubnets {
		subnetIDs = append(subnetIDs, subnet.SubnetId)
	}

	// build subnet group input
	subnetGroupInput := &elasticache.CreateCacheSubnetGroupInput{
		CacheSubnetGroupDescription: aws.String("subnet group created by the cloud resource operator"),
		CacheSubnetGroupName:        aws.String(subnetGroupName),
		SubnetIds:                   subnetIDs,
	}

	logger.Infof("creating resource subnet group %s", subnetGroupName)
	if _, err := n.ElasticacheApi.CreateCacheSubnetGroup(subnetGroupInput); err != nil {
		return errorUtil.Wrap(err, "unable to create cache subnet group")
	}
	return nil
}

//getStandaloneVpc will return a vpc type or error
//
//Standalone VPCs are tagged with with the name `<organizationTag>/clusterID`.
//By default, `integreatly.org/clusterID`.
//
//This tag is used to identify a standalone vpc
func getStandaloneVpc(ec2Svc ec2iface.EC2API, logger *logrus.Entry, clusterID, organizationTag string) (*ec2.Vpc, error) {
	// get all vpcs
	vpcs, err := ec2Svc.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpcs")
	}

	// find associated vpc to tag
	var foundVPC *ec2.Vpc
	for _, vpc := range vpcs.Vpcs {
		for _, tag := range vpc.Tags {
			if *tag.Key == fmt.Sprintf("%sclusterID", organizationTag) && *tag.Value == clusterID {
				logger.Infof("found vpc: %s", *vpc.VpcId)
				foundVPC = vpc
			}
		}
	}
	return foundVPC, nil
}

/*
getVPCAssociatedSubnets will return a list of subnets or an error

this is used twice, to find all subnets associated with a vpc in order to remove all subnets on deletion
it is also used as a helper function when we filter private associated subnets
*/
func getVPCAssociatedSubnets(ec2Svc ec2iface.EC2API, logger *logrus.Entry, vpc *ec2.Vpc) ([]*ec2.Subnet, error) {
	logger.Info("gathering cluster vpc and subnet information")
	// poll subnets to ensure credentials have reconciled
	subs, err := getSubnets(ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting subnets")
	}

	// in the rare chance no vpc is found we should return an error to avoid an unexpected nil pointer
	if vpc == nil {
		return nil, errorUtil.Wrap(err, "vpc is nil, need vpc to find associated subnets")
	}

	// find associated subnets
	var associatedSubs []*ec2.Subnet
	for _, sub := range subs {
		if *sub.VpcId == *vpc.VpcId {
			logger.Infof("found subnet: %s in vpc %s", *sub.SubnetId, *sub.VpcId)
			associatedSubs = append(associatedSubs, sub)
		}
	}
	return associatedSubs, nil
}

// getRDSSubnetByGroup returns rds db subnet group by the group name or an error
func getRDSSubnetByGroup(rdsApi rdsiface.RDSAPI, subnetGroupName string) (*rds.DBSubnetGroup, error) {
	// check if group exists
	groups, err := rdsApi.DescribeDBSubnetGroups(&rds.DescribeDBSubnetGroupsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error describing subnet groups")
	}
	for _, sub := range groups.DBSubnetGroups {
		if *sub.DBSubnetGroupName == subnetGroupName {
			return sub, nil
		}
	}
	return nil, nil
}

// getElasticacheSubnetByGroup returns elasticache subnet group by the group name or an error
func getElasticacheSubnetByGroup(elasticacheApi elasticacheiface.ElastiCacheAPI, subnetGroupName string) (*elasticache.CacheSubnetGroup, error) {
	// check if group exists
	groups, err := elasticacheApi.DescribeCacheSubnetGroups(&elasticache.DescribeCacheSubnetGroupsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error describing subnet groups")
	}
	for _, sub := range groups.CacheSubnetGroups {
		if *sub.CacheSubnetGroupName == subnetGroupName {
			return sub, nil
		}
	}
	return nil, nil
}

//subnetExists is a helper function for checking if a subnet exists with a specific cidr block
func subnetExists(subnets []*ec2.Subnet, cidr string) bool {
	for _, subnet := range subnets {
		if *subnet.CidrBlock == cidr {
			return true
		}
	}
	return false
}

//isValidCIDRRange returns a bool denoting if a cidr mask is valid
//
//we accept cidr mask ranges from \16 to \26
func isValidCIDRRange(CIDR *net.IPNet) bool {
	mask, _ := CIDR.Mask.Size()
	return mask > 15 && mask < 27
}
