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

		- Check if a VPC exists with the integreatly.org/clusterID tag
		- If VPC doesn't exist, create a VPC with CIDR block and tag it
	*/
	logger := n.logger.WithField("action", "CreateNetwork")
	logger.Debug("CreateNetwork stub")
	return errorUtil.New("CreateNetwork stub")
}

func (n *NetworkProvider) DeleteNetwork() error {
	logger := n.logger.WithField("action", "DeleteNetwork")
	logger.Debug("DeleteNetwork stub")
	return nil
}

/*
IsEnabled returns true when no subnets created by the cloud resource operator exist in the openshift cluster vpc.

subnets created by the cloud resource operator are identified by having a tag with the name `<organizationTag>/clusterID`.
By default, `integreatly.org/clusterID`.

this check allows us to maintain backwards compatibility with openshift clusters that used the cloud resource operator before this standalone vpc provider was added.
If this function returns false, we should continue using the backwards compatible approach of bundling resources in with the openshift cluster vpc.

*/
func (n *NetworkProvider) IsEnabled(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) (bool, error) {
	logger := n.logger.WithField("action", "isEnabled")

	// returning subnets from cluster vpc
	logger.Info("getting cluster vpc subnets")
	vpcSubnets, err := GetVPCSubnets(ctx, c, ec2Svc, logger)
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
