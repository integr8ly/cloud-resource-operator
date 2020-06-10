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

//todo we need to ensure we test that if `IsEnabled` is true, it remains true for consecutive reconciles

type NetworkManager interface {
	CreateNetwork(context.Context, *net.IPNet) (*ec2.Vpc, error)
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

func (n *NetworkProvider) CreateNetwork(ctx context.Context, CIDR *net.IPNet) (*ec2.Vpc, error) {
	/*
		todo CreateNetwork is called we need to ensure the following
		we expect _network to exist with a valid cidr
		the absence of either _network or valid cidr we should return with an informative message
		re-reconcile until we have a valid _network strat and valid cidr

		- Check if a VPC exists with the integreatly.org/clusterID tag
		- If VPC doesn't exist, create a VPC with CIDR block and tag it

		verify there is a _network strategy
		note we need to handle the tier differently to other products
	*/
	logger := n.Logger.WithField("action", "CreateNetwork")
	logger.Debug("CreateNetwork called")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	//check if there is an rhmi specific vpc already created.
	foundVpc, err := getStandaloneVpc(ctx, n.Client, n.Ec2Svc, logger, clusterID, organizationTag)
	if err != nil {
		return nil, errorUtil.Wrap(err, "unable to get vpc")
	}
	if foundVpc != nil {
		return foundVpc, nil
	}

	mask, _ := CIDR.Mask.Size()
	if mask < 16 || mask > 26 {
		return nil, errorUtil.New(fmt.Sprintf("%s is out of range, block sizes must be between `/16` and `/26`, please update `_network` strategy", CIDR.String()))
	}

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
	if err != nil {
		return nil, errorUtil.Wrap(err, "unable to create vpc")
	}
	logger.Infof("creating vpc: %s for clusterID: %s", *createVpcOutput.Vpc.VpcId, clusterID)
	_, err = n.Ec2Svc.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{
			aws.String(*createVpcOutput.Vpc.VpcId),
		},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String(DefaultRHMIVpcNameTagKey),
				Value: aws.String(DefaultRHMIVpcNameTagValue),
			}, {
				Key:   aws.String(fmt.Sprintf("%sclusterID", organizationTag)),
				Value: aws.String(clusterID),
			},
		},
	})
	if err != nil {
		return nil, errorUtil.Wrap(err, "unable to tag vpc")
	}
	logger.Infof("successfully tagged vpc: %s for clusterID: %s", *createVpcOutput.Vpc.VpcId, clusterID)

	return createVpcOutput.Vpc, nil
}

func (n *NetworkProvider) DeleteNetwork(ctx context.Context) error {
	logger := n.Logger.WithField("action", "DeleteNetwork")
	logger.Debug("DeleteNetwork stub")

	clusterID, err := resources.GetClusterID(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting clusterID")
	}
	organizationTag := resources.GetOrganizationTag()

	//check if there is an rhmi specific vpc already created.
	foundVpc, err := getStandaloneVpc(ctx, n.Client, n.Ec2Svc, logger, clusterID, organizationTag)
	if err != nil {
		return errorUtil.Wrap(err, "unable to get vpc")
	}
	if foundVpc == nil {
		logger.Infof("no vpc found for clusterID: %s", clusterID)
		return nil
	}

	vpcSubs, err := getVPCAssociatedSubnets(n.Ec2Svc, logger, foundVpc)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get standalone vpc subnetes")
	}

	for _, subnet := range vpcSubs {
		_, err = n.Ec2Svc.DeleteSubnet(&ec2.DeleteSubnetInput{
			SubnetId: aws.String(*subnet.SubnetId),
		})
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

	//check if there is an rhmi specific vpc already created.
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

	// iterate all cluster vpc's checking for valid rhmi subnets
	organizationTag := resources.GetOrganizationTag()
	var validRHMISubnets []*ec2.Subnet
	for _, subnet := range vpcSubnets {
		for _, tag := range subnet.Tags {
			if aws.StringValue(tag.Key) == fmt.Sprintf("%sclusterID", organizationTag) {
				validRHMISubnets = append(validRHMISubnets, subnet)
			}
		}
	}
	logger.Infof("found %d rhmi subnets in cluster vpc", len(validRHMISubnets))
	return len(validRHMISubnets) == 0, nil
}

func getStandaloneVpc(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API, logger *logrus.Entry, clusterID, organizationTag string) (*ec2.Vpc, error) {
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
			associatedSubs = append(associatedSubs, sub)
		}
	}

	return associatedSubs, nil
}
