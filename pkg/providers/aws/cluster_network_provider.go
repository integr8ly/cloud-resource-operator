package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	errorUtil "github.com/pkg/errors"
)

//todo we need to ensure we test that if `IsEnabled` is true, it remains true for consecutive reconciles

type NetworkManager interface {
	CreateNetwork() error
	DeleteNetwork() error
	IsEnabled(context.Context, client.Client, ec2iface.EC2API) (bool, error)
}

var _ NetworkManager = (*NetworkProvider)(nil)

type NetworkProvider struct {
	logger *logrus.Entry
}

func NewNetworkManager(logger *logrus.Entry) *NetworkProvider {
	return &NetworkProvider{
		logger: logger.WithField("provider", "standalone_network_provider"),
	}
}

func (n *NetworkProvider) CreateNetwork() error {
	/*
		todo CreateNetwork is called we need to ensure the following
		we expect _network to exist with a valid cidr
		the absence of either _network or valid cidr we should return with an informative message
		re-reconcile until we have a valid _network strat and valid cidr
	*/
	fmt.Println("CreateNetwork stub")
	return nil
}

func (n *NetworkProvider) DeleteNetwork() error {
	fmt.Println("DeleteNetwork stub")
	return nil
}

/*
IsEnabled checks for valid rhmi subnets within the cluster VPC
a valid rhmi subnet will contain a tag with the `organizationTag` value

when rhmi subnets are present in a cluster vpc it indicates that the vpc configuration
was created in a cluster with a cluster version <= 4.4.5

when rhmi subnets are absent in a cluster vpc it indicates that the vpc configuration has not been created
and we should create a new vpc for all resources to be deployed in and we should peer the
resource vpc and cluster vpc
*/
func (n *NetworkProvider) IsEnabled(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) (bool, error) {
	logger := n.logger.WithField("action", "isEnabled")

	// returning subnets from cluster vpc
	logger.Info("getting cluster vpc subnets")
	vpcSubnets, err := GetVPCSubnets(ctx, c, ec2Svc)
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
