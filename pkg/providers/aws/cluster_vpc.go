package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	errorUtil "github.com/pkg/errors"
)

const (
	defaultSubnetPostfix        = "subnet-group"
	defaultSecurityGroupPostfix = "security-group"
)

// GetVPCSubnets returns a list of subnets associated with cluster VPC
func GetVPCSubnets(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) ([]*ec2.Subnet, error) {
	logrus.Info("gathering cluster vpc and subnet information")

	// poll subnets to ensure credentials have reconciled
	subs, err := getSubnets(ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting subnets")
	}

	foundVPC, err := getVpc(ctx, c, ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpcs")
	}

	// check if found cluster vpc
	if foundVPC == nil {
		return nil, errorUtil.New("error, unable to find a vpc")
	}

	// find associated subnets
	var associatedSubs []*ec2.Subnet
	for _, sub := range subs {
		if *sub.VpcId == *foundVPC.VpcId {
			associatedSubs = append(associatedSubs, sub)
		}
	}

	// check if found subnets associated with cluster vpc
	if associatedSubs == nil {
		return nil, errorUtil.New("error, unable to find subnets associated with cluster vpc")
	}
	return associatedSubs, nil

}

// GetSubnetIDS returns a list of subnet ids associated with cluster vpc
func GetAllSubnetIDS(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) ([]*string, error) {
	logrus.Info("gathering all vpc subnets")
	subs, err := GetVPCSubnets(ctx, c, ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpc subnets")
	}

	// build list of subnet ids
	var subIDs []*string
	for _, sub := range subs {
		subIDs = append(subIDs, sub.SubnetId)
	}

	if subIDs == nil {
		return nil, errorUtil.New("failed to get list of subnet ids")
	}
	return subIDs, nil
}

// GetSubnetIDS returns a list of subnet ids associated with cluster vpc
func GetPrivateSubnetIDS(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) ([]*string, error) {
	logrus.Info("gathering private vpc subnets")
	subs, err := GetVPCSubnets(ctx, c, ec2Svc)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting vpc subnets")
	}

	regexpStr := "\\b(\\w*private\\w*)\\b"
	anReg, err := regexp.Compile(regexpStr)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to compile regexp %s", regexpStr)
	}

	var privateSubs []*ec2.Subnet
	for _, sub := range subs {
		for _, tags := range sub.Tags {
			if anReg.MatchString(*tags.Value) {
				privateSubs = append(privateSubs, sub)
			}
		}
	}

	// build list of subnet ids
	var subIDs []*string
	for _, sub := range privateSubs {
		subIDs = append(subIDs, sub.SubnetId)
	}

	if subIDs == nil {
		return nil, errorUtil.New("failed to get list of private subnet ids")
	}
	return subIDs, nil
}

// returns vpc id and cidr block for found vpc
func GetCidr(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) (string, string, error) {
	logrus.Info("gathering cidr block for cluster")
	foundVPC, err := getVpc(ctx, c, ec2Svc)
	if err != nil {
		return "", "", errorUtil.Wrap(err, "error getting vpcs")
	}

	// check if found cluster vpc
	if foundVPC == nil {
		return "", "", errorUtil.New("error, unable to find a vpc")
	}

	return *foundVPC.VpcId, *foundVPC.CidrBlock, nil
}

// function to get subnets, used to check/wait on AWS credentials
func getSubnets(ec2Svc ec2iface.EC2API) ([]*ec2.Subnet, error) {
	var subs []*ec2.Subnet
	err := wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		listOutput, err := ec2Svc.DescribeSubnets(&ec2.DescribeSubnetsInput{})
		if err != nil {
			return false, nil
		}
		subs = listOutput.Subnets
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// function to get vpc of a cluster
func getVpc(ctx context.Context, c client.Client, ec2Svc ec2iface.EC2API) (*ec2.Vpc, error) {
	logrus.Info("finding cluster vpc")
	// get vpcs
	vpcs, err := ec2Svc.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting subnets")
	}

	// get cluster id
	clusterID, err := resources.GetClusterID(ctx, c)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting clusterID")
	}

	// find associated vpc to cluster
	var foundVPC *ec2.Vpc
	for _, vpc := range vpcs.Vpcs {
		for _, tag := range vpc.Tags {
			if *tag.Value == fmt.Sprintf("%s-vpc", clusterID) {
				foundVPC = vpc
			}
		}
	}

	if foundVPC == nil {
		return nil, errorUtil.New("error, no vpc found")
	}

	return foundVPC, nil
}

func getSecurityGroup(ec2Svc ec2iface.EC2API, secName string) (*ec2.SecurityGroup, error) {
	// get security groups
	secGroups, err := ec2Svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to return information about security groups")
	}

	// check if security group exists
	var foundSecGroup *ec2.SecurityGroup
	for _, sec := range secGroups.SecurityGroups {
		if *sec.GroupName == secName {
			foundSecGroup = sec
			break
		}
	}
	return foundSecGroup, nil
}
