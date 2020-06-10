// utility to manage a dedicated vpc for the resources created by the cloud resource operator.
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

package aws

import (
	"context"
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	errorUtil "github.com/pkg/errors"
)

const (
	DefaultRHMIVpcNameTagKey   = "Name"
	DefaultRHMIVpcNameTagValue = "RHMI Cloud Resource VPC"
)

// wrapper for ec2 vpcs, to allow for extensibility
type Network struct {
	Vpc *ec2.Vpc
}

type NetworkManager interface {
	CreateNetwork(context.Context, *net.IPNet) (*Network, error)
	DeleteNetwork(context.Context) error
	IsEnabled(context.Context) (bool, error)
}

var _ NetworkManager = (*NetworkProvider)(nil)

type NetworkProvider struct {
	Client client.Client
	Ec2Svc ec2iface.EC2API
	Logger *logrus.Entry
}

func NewNetworkManager(client client.Client, ec2Svc ec2iface.EC2API, logger *logrus.Entry) *NetworkProvider {
	return &NetworkProvider{
		Client: client,
		Ec2Svc: ec2Svc,
		Logger: logger.WithField("provider", "standalone_network_provider"),
	}
}

/*
	- Check if a VPC exists with the integreatly.org/clusterID tag
	- If VPC doesn't exist, create a VPC with CIDR block and tag it
*/
func (n *NetworkProvider) CreateNetwork(ctx context.Context, CIDR *net.IPNet) (*Network, error) {
	logger := n.Logger.WithField("action", "CreateNetwork")
	logger.Debug("CreateNetwork called")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	// check if there is cluster specific vpc already created.
	foundVpc, err := getStandaloneVpc(n.Ec2Svc, logger, clusterID, organizationTag)
	if err != nil {
		return nil, errorUtil.Wrap(err, "unable to get vpc")
	}

	// if vpc does not exist create it
	// we do not want to update the vpc configuration (eg. cidr block) after it has been created
	// to avoid unwanted and unexpected behaviour
	if foundVpc == nil {
		// expected valid CIDR block between /16 and /26
		if !isValidCIDR(CIDR) {
			return nil, errorUtil.New(fmt.Sprintf("%s is out of range, block sizes must be between `/16` and `/26`, please update `_network` strategy", CIDR.String()))
		}
		logger.Infof("cidr %s is valid 👍", CIDR.String())

		// create vpc using cidr string from _network
		createVpcOutput, err := n.Ec2Svc.CreateVpc(&ec2.CreateVpcInput{
			CidrBlock: aws.String(CIDR.String()),
		})
		ec2Err, isAwsErr := err.(awserr.Error)
		if err != nil && isAwsErr && ec2Err.Code() == "InvalidVpc.Range" {
			return nil, errorUtil.New(fmt.Sprintf("%s is out of range, block sizes must be between `/16` and `/26`, please update `_network` strategy", CIDR.String()))
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
			Vpc: createVpcOutput.Vpc,
		}, nil
	}

	/* if vpc is found reconcile on vpc networking
	* subnets (2 private)
	* subnet groups -> rds and elasticache
	* security groups
	* route table configuration
	 */
	var privateSubnets []*ec2.Subnet
	if err = n.reconcileVPCAssociatedPrivateSubnets(ctx, n.Logger, foundVpc, privateSubnets); err != nil {
		return nil, errorUtil.Wrap(err, "unexpected error creating vpc subnets")
	}

	return &Network{
		Vpc: foundVpc,
	}, nil
}

/*
DeleteNetwork deletes standalone cro networking
	* all vpc associated subnets
	* vpc
*/
func (n *NetworkProvider) DeleteNetwork(ctx context.Context) error {
	logger := n.Logger.WithField("action", "DeleteNetwork")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	//check if there is a standalone vpc already created.
	foundVpc, err := getStandaloneVpc(n.Ec2Svc, logger, clusterID, organizationTag)
	if err != nil {
		return errorUtil.Wrap(err, "unable to get vpc")
	}
	if foundVpc == nil {
		logger.Infof("no vpc found for clusterID: %s", clusterID)
		return nil
	}

	vpcSubs, err := getVPCAssociatedSubnets(n.Ec2Svc, logger, foundVpc)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get standalone vpc subnets")
	}

	for _, subnet := range vpcSubs {
		fmt.Printf("attempting to delete subnet with id: %s", *subnet.SubnetId)
		_, err = n.Ec2Svc.DeleteSubnet(&ec2.DeleteSubnetInput{
			SubnetId: aws.String(*subnet.SubnetId),
		})
		if err != nil {
			return errorUtil.Wrapf(err, "failed to delete subnet with id: %s", *subnet.SubnetId)
		}
	}

	logger.Infof("attempting to delete vpc id: %s for clusterID: %s", *foundVpc.VpcId, clusterID)
	_, err = n.Ec2Svc.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: aws.String(*foundVpc.VpcId),
	})
	if err != nil {
		return errorUtil.Wrap(err, "unable to delete vpc")
	}
	return nil
}

/*
IsEnabled returns true when no subnets created by the cloud resource operator exist in the openshift cluster vpc.

subnets created by the cloud resource operator are identified by having a tag with the name `<organizationTag>/clusterID`.
By default, `integreatly.org/clusterID`.

this check allows us to maintain backwards compatibility with openshift clusters that used the cloud resource operator before this standalone vpc provider was added.
If this function returns false, we should continue using the backwards compatible approach of bundling resources in with the openshift cluster vpc.

*/
func (n *NetworkProvider) IsEnabled(ctx context.Context) (bool, error) {
	logger := n.Logger.WithField("action", "isEnabled")

	//check if there is a standalone vpc already created.
	foundVpc, err := getClusterVpc(ctx, n.Client, n.Ec2Svc, logger)
	if err != nil {
		return false, errorUtil.Wrap(err, "unable to get vpc")
	}

	// returning subnets from cluster vpc
	logger.Info("getting cluster vpc subnets")
	vpcSubnets, err := GetVPCSubnets(n.Ec2Svc, logger, foundVpc)
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

/*

 */
func (n *NetworkProvider) reconcileVPCAssociatedPrivateSubnets(ctx context.Context, logger *logrus.Entry, vpc *ec2.Vpc, privateSubnets []*ec2.Subnet) error {
	logger.Info("gathering all private subnets in cluster vpc")

	// get a list of availability zones
	azs, err := getAZs(n.Ec2Svc)
	if err != nil {
		return errorUtil.Wrap(err, "error getting availability zones")
	}

	// filter based on a tag key attached to private subnets
	// get subnets in vpc
	subs, err := getVPCAssociatedSubnets(n.Ec2Svc, logger, vpc)
	if err != nil {
		return errorUtil.Wrap(err, "error getting vpc subnets")
	}

	for _, sub := range subs {
		for _, tags := range sub.Tags {
			if *tags.Key == defaultAWSPrivateSubnetTagKey {
				logger.Infof("found existing private subnet: %s, in vpc: %s ", *sub.SubnetId, *sub.VpcId)
				privateSubnets = append(privateSubnets, sub)
			}
		}
	}

	// for every az check there is a private subnet, if none create one
	for countAzs, az := range azs {
		logger.Infof("checking if private subnet exists in zone %s", *az.ZoneName)
		if !privateSubnetExists(privateSubnets, az) {
			logger.Infof("no private subnet found in %s", *az.ZoneName)
			subnet, err := createPrivateSubnet(ctx, n.Client, n.Ec2Svc, vpc, logger, *az.ZoneName)
			if err != nil {
				return errorUtil.Wrap(err, "failed to created private subnet")
			}
			privateSubnets = append(privateSubnets, subnet)
		}
		// only looking at the first 2 azs so breaking out after the 1th index
		if countAzs == 1 {
			logger.Infof("created subnet in 2 azs successfully")
			break
		}
	}
	return nil
}

/*
 */
func (n *NetworkProvider) reconcileVPCTags(vpc *ec2.Vpc, clusterIDTag *ec2.Tag) error {
	logger := n.Logger.WithField("action", "reconcileVPCTags")

	vpcTags := ec2TagsToGeneric(vpc.Tags)
	if !tagsContains(vpcTags, DefaultRHMIVpcNameTagKey, DefaultRHMIVpcNameTagValue) ||
		!tagsContains(vpcTags, *clusterIDTag.Key, *clusterIDTag.Value) {

		_, err := n.Ec2Svc.CreateTags(&ec2.CreateTagsInput{
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

func getStandaloneVpc(ec2Svc ec2iface.EC2API, logger *logrus.Entry, clusterID, organizationTag string) (*ec2.Vpc, error) {
	// get vpcs
	vpcs, err := ec2Svc.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpcs")
	}

	//there will be a tags associated with the seperate vpc
	//Name='RHMI Cloud Resource VPC' and integreatly.org/clusterID=<infrastructure-id>
	//use these in order to pick up the correct one.
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

func getVPCAssociatedSubnets(ec2Svc ec2iface.EC2API, logger *logrus.Entry, vpc *ec2.Vpc) ([]*ec2.Subnet, error) {
	logger.Info("gathering cluster vpc and subnet information")
	// poll subnets to ensure credentials have reconciled
	subs, err := getSubnets(ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting subnets")
	}

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

func isValidCIDR(CIDR *net.IPNet) bool {
	mask, _ := CIDR.Mask.Size()
	return mask > 15 && mask < 27

}
