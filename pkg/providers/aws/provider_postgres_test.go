package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go/aws/awserr"
	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	croApis "github.com/integr8ly/cloud-resource-operator/apis"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	cloudCredentialApis "github.com/openshift/cloud-credential-operator/pkg/apis"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultInfraName               = "test"
	defaultVpcId                   = "testID"
	testPreferredBackupWindow      = "02:40-03:10"
	testPreferredMaintenanceWindow = "mon:00:29-mon:00:59"
	testInvalidEngineVersion       = "xyz"
)

var (
	lockMockEc2ClientDescribeRouteTables       sync.RWMutex
	lockMockEc2ClientDescribeSecurityGroups    sync.RWMutex
	lockMockEc2ClientDescribeSubnets           sync.RWMutex
	lockMockEc2ClientDescribeAvailabilityZones sync.RWMutex
	lockMockEc2ClientDescribeVpcs              sync.RWMutex
	lockMockEc2ClientCreateRoute               sync.RWMutex
	snapshotARN                                = "test:arn"
	snapshotIdentifier                         = "testIdentifier"
)

type mockRdsClient struct {
	rdsiface.RDSAPI
	modifyDBSubnetGroupFn               func(*rds.ModifyDBSubnetGroupInput) (*rds.ModifyDBSubnetGroupOutput, error)
	listTagsForResourceFn               func(*rds.ListTagsForResourceInput) (*rds.ListTagsForResourceOutput, error)
	removeTagsFromResourceFn            func(*rds.RemoveTagsFromResourceInput) (*rds.RemoveTagsFromResourceOutput, error)
	deleteDBSubnetGroupFn               func(*rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error)
	addTagsToResourceFn                 func(*rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error)
	describeDBSnapshotsFn               func(*rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error)
	describeDBInstancesFn               func(*rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error)
	describeDBSubnetGroupsFn            func(*rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error)
	describePendingMaintenanceActionsFn func(*rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error)
	applyPendingMaintenanceActionFn     func(*rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error)
	modifyDBInstanceFn                  func(*rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error)
}

type mockEc2Client struct {
	ec2iface.EC2API
	firstSubnet     *ec2.Subnet
	secondSubnet    *ec2.Subnet
	subnets         []*ec2.Subnet
	vpcs            []*ec2.Vpc
	vpc             *ec2.Vpc
	secGroups       []*ec2.SecurityGroup
	azs             []*ec2.AvailabilityZone
	wantErrList     bool
	returnSecondSub bool
	// new approach for manually defined mocks
	// to allow for simple overrides in test table declarations
	createTagsFn                    func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
	describeVpcsFn                  func(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	describeSecurityGroupsFn        func(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	deleteSecurityGroupFn           func(*ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	describeVpcPeeringConnectionFn  func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error)
	createVpcPeeringConnectionFn    func(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error)
	acceptVpcPeeringConnectionFn    func(*ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error)
	deleteVpcPeeringConnectionFn    func(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error)
	describeRouteTablesFn           func(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
	createRouteFn                   func(*ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error)
	deleteRouteFn                   func(*ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error)
	createVpcFn                     func(*ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error)
	deleteVpcFn                     func(*ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error)
	createSubnetFn                  func(*ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error)
	describeInstanceTypeOfferingsFn func(*ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
	WaitUntilVpcExistsFn            func(*ec2.DescribeVpcsInput) error
	describeSubnetsFn               func(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	describeAvailabilityZonesFn     func(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error)
	createSecurityGroupFn           func(*ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	calls                           struct {
		DescribeRouteTables []struct {
			Tables *ec2.DescribeRouteTablesInput
		}
		DescribeSecurityGroups []struct {
			Groups *ec2.DescribeSecurityGroupsInput
		}
		DescribeSubnets []struct {
			Subnets *ec2.DescribeSubnetsInput
		}
		DescribeAvailabilityZones []struct {
			AvailabilityZones *ec2.DescribeAvailabilityZonesInput
		}
		DescribeVpcs []struct {
			Vpcs *ec2.DescribeVpcsInput
		}
		CreateRoute []struct {
			Route *ec2.CreateRouteInput
		}
	}
}

func buildMockEc2Client(modifyFn func(*mockEc2Client)) *mockEc2Client {
	mock := &mockEc2Client{}
	mock.WaitUntilVpcExistsFn = func(input *ec2.DescribeVpcsInput) error {
		return nil
	}
	mock.deleteVpcFn = func(*ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
		return &ec2.DeleteVpcOutput{}, nil
	}
	mock.createTagsFn = func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
		return &ec2.CreateTagsOutput{}, nil
	}
	mock.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
		return &ec2.DescribeInstanceTypeOfferingsOutput{
			InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
				{
					Location: aws.String(defaultAzIdOne),
				},
				{
					Location: aws.String(defaultAzIdTwo),
				},
			},
		}, nil
	}
	mock.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
		return &ec2.DescribeSubnetsOutput{}, nil
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildMockRdsClient(modifyFn func(*mockRdsClient)) *mockRdsClient {
	mock := &mockRdsClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildTestSchemePostgresql() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := croApis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = cloudCredentialApis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = monitoringv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func (m *mockRdsClient) DescribeDBInstances(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
	if m.describeDBInstancesFn == nil {
		panic("mockEc2Client.DescribeDBInstances: method is nil")
	}
	return m.describeDBInstancesFn(input)
}

func (m *mockRdsClient) CreateDBInstance(*rds.CreateDBInstanceInput) (*rds.CreateDBInstanceOutput, error) {
	return &rds.CreateDBInstanceOutput{}, nil
}

func (m *mockRdsClient) ModifyDBInstance(input *rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error) {
	if m.modifyDBInstanceFn != nil {
		return m.modifyDBInstanceFn(input)
	}
	return &rds.ModifyDBInstanceOutput{}, nil
}

func (m *mockRdsClient) DeleteDBInstance(*rds.DeleteDBInstanceInput) (*rds.DeleteDBInstanceOutput, error) {
	return &rds.DeleteDBInstanceOutput{}, nil
}

func (m *mockRdsClient) AddTagsToResource(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
	if resources.SafeStringDereference(input.ResourceName) == snapshotARN {
		return m.addTagsToResourceFn(input)
	} else {
		return &rds.AddTagsToResourceOutput{}, nil
	}
}

func (m *mockRdsClient) DescribeDBSnapshots(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
	return m.describeDBSnapshotsFn(input)
}

func (m *mockRdsClient) ApplyPendingMaintenanceAction(input *rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error) {
	if m.applyPendingMaintenanceActionFn == nil {
		panic("mockEc2Client.ApplyPendingMaintenanceAction: method is nil")
	}
	return m.applyPendingMaintenanceActionFn(input)
}

func (m *mockRdsClient) DescribePendingMaintenanceActions(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
	if m.describePendingMaintenanceActionsFn == nil {
		panic("mockEc2Client.DescribePendingMaintenanceActions: method is nil")
	}
	return m.describePendingMaintenanceActionsFn(input)
}

func (m *mockRdsClient) DescribeDBSubnetGroups(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
	if m.describeDBSubnetGroupsFn == nil {
		panic("mockEc2Client.DescribeDBSubnetGroups: method is nil")
	}
	return m.describeDBSubnetGroupsFn(input)
}

func (m *mockRdsClient) CreateDBSubnetGroup(*rds.CreateDBSubnetGroupInput) (*rds.CreateDBSubnetGroupOutput, error) {
	return &rds.CreateDBSubnetGroupOutput{}, nil
}

func (m *mockRdsClient) ModifyDBSubnetGroup(input *rds.ModifyDBSubnetGroupInput) (*rds.ModifyDBSubnetGroupOutput, error) {
	return m.modifyDBSubnetGroupFn(input)
}

func (m *mockRdsClient) DeleteDBSubnetGroup(input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
	return m.deleteDBSubnetGroupFn(input)
}

func (m *mockRdsClient) ListTagsForResource(input *rds.ListTagsForResourceInput) (*rds.ListTagsForResourceOutput, error) {
	return m.listTagsForResourceFn(input)
}

func (m *mockRdsClient) RemoveTagsFromResource(input *rds.RemoveTagsFromResourceInput) (*rds.RemoveTagsFromResourceOutput, error) {
	return m.removeTagsFromResourceFn(input)
}

func (m *mockEc2Client) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	if m.describeSubnetsFn == nil {
		panic("mockEc2Client.DescribeSubnets: method is nil")
	}
	callInfo := struct {
		Subnets *ec2.DescribeSubnetsInput
	}{
		Subnets: input,
	}

	lockMockEc2ClientDescribeSubnets.Lock()
	m.calls.DescribeSubnets = append(m.calls.DescribeSubnets, callInfo)
	lockMockEc2ClientDescribeSubnets.Unlock()

	return m.describeSubnetsFn(input)
}

func (m *mockEc2Client) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	if m.describeVpcsFn == nil {
		panic("mockEc2Client.DescribeVpcs: method is nil")
	}
	callInfo := struct {
		Vpcs *ec2.DescribeVpcsInput
	}{
		Vpcs: input,
	}

	lockMockEc2ClientDescribeVpcs.Lock()
	m.calls.DescribeVpcs = append(m.calls.DescribeVpcs, callInfo)
	lockMockEc2ClientDescribeVpcs.Unlock()

	return m.describeVpcsFn(input)
}

func (m *mockEc2Client) WaitUntilVpcExists(input *ec2.DescribeVpcsInput) error {
	return m.WaitUntilVpcExistsFn(input)
}

func (m *mockEc2Client) CreateVpc(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
	if m.createVpcFn == nil {
		panic("mockEc2Client.CreateVpc: method is nil")
	}
	return m.createVpcFn(input)
}

func (m *mockEc2Client) DeleteVpc(input *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
	return m.deleteVpcFn(input)
}

func (m *mockEc2Client) CreateSubnet(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
	if m.createSubnetFn != nil {
		return m.createSubnetFn(input)
	}
	if m.returnSecondSub {
		return &ec2.CreateSubnetOutput{
			Subnet: m.secondSubnet,
		}, nil
	}
	return m.returnFirstSubnet()
}

func (m *mockEc2Client) returnFirstSubnet() (*ec2.CreateSubnetOutput, error) {
	m.returnSecondSub = true
	return &ec2.CreateSubnetOutput{
		Subnet: m.firstSubnet,
	}, nil
}

func (m *mockEc2Client) DeleteSubnet(*ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error) {
	return &ec2.DeleteSubnetOutput{}, nil
}

func (m *mockEc2Client) CreateRouteCalls() []struct {
	Route *ec2.CreateRouteInput
} {
	var calls []struct {
		Route *ec2.CreateRouteInput
	}
	lockMockEc2ClientCreateRoute.RLock()
	calls = m.calls.CreateRoute
	lockMockEc2ClientCreateRoute.RUnlock()

	return calls
}

func (m *mockEc2Client) CreateRoute(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	if m.createRouteFn == nil {
		panic("mockEc2Client.DescribeRouteTables: method is nil")
	}
	callInfo := struct {
		Route *ec2.CreateRouteInput
	}{
		Route: input,
	}

	lockMockEc2ClientCreateRoute.Lock()
	m.calls.CreateRoute = append(m.calls.CreateRoute, callInfo)
	lockMockEc2ClientCreateRoute.Unlock()

	return m.createRouteFn(input)
}

func (m *mockEc2Client) DeleteRoute(input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	return m.deleteRouteFn(input)
}

func (m *mockEc2Client) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	if m.describeRouteTablesFn == nil {
		panic("mockEc2Client.DescribeRouteTables: method is nil")
	}
	callInfo := struct {
		Tables *ec2.DescribeRouteTablesInput
	}{
		Tables: input,
	}

	lockMockEc2ClientDescribeRouteTables.Lock()
	m.calls.DescribeRouteTables = append(m.calls.DescribeRouteTables, callInfo)
	lockMockEc2ClientDescribeRouteTables.Unlock()

	return m.describeRouteTablesFn(input)
}

func (m *mockEc2Client) DescribeRouteTablesCalls() []struct {
	Tables *ec2.DescribeRouteTablesInput
} {
	var calls []struct {
		Tables *ec2.DescribeRouteTablesInput
	}
	lockMockEc2ClientDescribeRouteTables.RLock()
	calls = m.calls.DescribeRouteTables
	lockMockEc2ClientDescribeRouteTables.RUnlock()

	return calls
}

func (m *mockEc2Client) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	if m.describeSecurityGroupsFn == nil {
		panic("mockEc2Client.DescribeSecurityGroups: method is nil")
	}
	callInfo := struct {
		Groups *ec2.DescribeSecurityGroupsInput
	}{
		Groups: input,
	}
	lockMockEc2ClientDescribeSecurityGroups.Lock()
	m.calls.DescribeSecurityGroups = append(m.calls.DescribeSecurityGroups, callInfo)
	lockMockEc2ClientDescribeSecurityGroups.Unlock()

	return m.describeSecurityGroupsFn(input)
}

func (m *mockEc2Client) DescribeSecurityGroupsCalls() []struct {
	Groups *ec2.DescribeSecurityGroupsInput
} {
	var calls []struct {
		Groups *ec2.DescribeSecurityGroupsInput
	}

	lockMockEc2ClientDescribeSecurityGroups.RLock()
	calls = m.calls.DescribeSecurityGroups
	lockMockEc2ClientDescribeSecurityGroups.RUnlock()

	return calls
}

func (m *mockEc2Client) CreateSecurityGroup(input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	if m.createSecurityGroupFn == nil {
		panic("mockEc2Client.CreateSecurityGroup: method is nil")
	}
	return m.createSecurityGroupFn(input)
}

func (m *mockEc2Client) DeleteSecurityGroup(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	return m.deleteSecurityGroupFn(input)
}

func (m *mockEc2Client) AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (m *mockEc2Client) DescribeAvailabilityZones(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if m.describeAvailabilityZonesFn == nil {
		panic("mockEc2Client.DescribeAvailabilityZones: method is nil")
	}
	callInfo := struct {
		AvailabilityZones *ec2.DescribeAvailabilityZonesInput
	}{
		AvailabilityZones: input,
	}
	lockMockEc2ClientDescribeAvailabilityZones.Lock()
	m.calls.DescribeAvailabilityZones = append(m.calls.DescribeAvailabilityZones, callInfo)
	lockMockEc2ClientDescribeAvailabilityZones.Unlock()

	return m.describeAvailabilityZonesFn(input)
}

func (m *mockEc2Client) DescribeVpcPeeringConnections(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	return m.describeVpcPeeringConnectionFn(input)
}

func (m *mockEc2Client) CreateVpcPeeringConnection(input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	return m.createVpcPeeringConnectionFn(input)
}

func (m *mockEc2Client) CreateTags(input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	return m.createTagsFn(input)
}

func (m *mockEc2Client) AcceptVpcPeeringConnection(input *ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	return m.acceptVpcPeeringConnectionFn(input)
}

func (m *mockEc2Client) DeleteVpcPeeringConnection(input *ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	return m.deleteVpcPeeringConnectionFn(input)
}

func (m *mockEc2Client) DescribeInstanceTypeOfferings(input *ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	return m.describeInstanceTypeOfferingsFn(input)
}

// the only place this is called is the exposePostgresMetrics func which is not being tested
// return empty result
func (m *mockEc2Client) DescribeInstanceTypes(*ec2.DescribeInstanceTypesInput) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{}, nil
}

func buildMockNetworkManager() *NetworkManagerMock {
	return &NetworkManagerMock{
		DeleteNetworkConnectionFunc: func(ctx context.Context, np *NetworkPeering) error {
			return nil
		},
		GetClusterNetworkPeeringFunc: func(ctx context.Context) (*NetworkPeering, error) {
			return &NetworkPeering{}, nil
		},
		DeleteNetworkPeeringFunc: func(np *NetworkPeering) error {
			return nil
		},
		DeleteNetworkFunc: func(ctx context.Context) error {
			return nil
		},
		DeleteBundledCloudResourcesFunc: func(ctx context.Context) error {
			return nil
		},
	}
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
			Labels: map[string]string{
				"productName": "test_product",
			},
			ResourceVersion: fakeResourceVersion,
		},
	}
}

func buildTestPostgresApplyImmediatelyCR() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels: map[string]string{
				"productName": "test_product",
			},
		},
		Spec: croType.ResourceTypeSpec{
			ApplyImmediately: true,
		},
	}
}

func buildTestInfra() *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName: defaultInfraName,
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
				AWS: &configv1.AWSPlatformStatus{
					Region: "eu-west-1",
					ResourceTags: []configv1.AWSResourceTag{
						{
							Key:   "test-key",
							Value: "test-value",
						},
					},
				},
			},
		},
	}
}

func buildTestNetwork(modifyFn func(network *configv1.Network)) *configv1.Network {

	mock := &configv1.Network{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.NetworkSpec{
			ClusterNetwork: []configv1.ClusterNetworkEntry{
				{
					CIDR:       "10.0.0.0/14",
					HostPrefix: 23,
				},
			},
			ServiceNetwork: []string{
				"10.5.0.0/16",
			},
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock

}

func builtTestCredSecret() *corev1.Secret {
	return &corev1.Secret{
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
			DBInstanceClass:      aws.String(defaultAwsDBInstanceClass),
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
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
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
			DBInstanceClass:      aws.String(defaultAwsDBInstanceClass),
		},
	}
}

func buildAvailableDBInstance(testID string) []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AutoMinorVersionUpgrade:    aws.Bool(false),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
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

func buildAvailableDBInstanceVersion(testID string, version string) []*rds.DBInstance {
	return []*rds.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AutoMinorVersionUpgrade:    aws.Bool(false),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
			EngineVersion:              aws.String(version),
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
			DBInstanceClass:      aws.String(defaultAwsDBInstanceClass),
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
		MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
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
		MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
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
		MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
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
			AutoMinorVersionUpgrade:    aws.Bool(false),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int64(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int64(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int64(defaultAwsMaxAllocatedStorage),
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
			VpcId:     aws.String(defaultVpcId),
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

func buildAZ() []*ec2.AvailabilityZone {
	return []*ec2.AvailabilityZone{
		{
			ZoneName: aws.String("test"),
			State:    aws.String("available"),
		},
	}
}
func buildSecurityGroup(modifyFn func(cluster *ec2.SecurityGroup)) *ec2.SecurityGroup {
	mock := &ec2.SecurityGroup{
		GroupName: aws.String("test"),
		GroupId:   aws.String("testID"),
	}

	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildSecurityGroups(groupName string) []*ec2.SecurityGroup {
	return []*ec2.SecurityGroup{
		buildSecurityGroup(func(mock *ec2.SecurityGroup) {
			mock.GroupName = aws.String(groupName)
		}),
	}
}

func TestAWSPostgresProvider_createPostgresInstance(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
	testIdentifier := "test-identifier"
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}
	secName, err := resources.BuildInfraName(context.TODO(), moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()), defaultSecurityGroupPostfix, defaultAwsIdentifierLength)
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build security name", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         resources.ConnectionTester
	}
	type args struct {
		ctx                     context.Context
		cr                      *v1alpha1.Postgres
		rdsSvc                  rdsiface.RDSAPI
		ec2Svc                  ec2iface.EC2API
		postgresCfg             *rds.CreateDBInstanceInput
		standaloneNetworkExists bool
		maintenanceWindow       bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.PostgresInstance
		wantErr bool
		mockFn  func()
	}{
		{
			name: "test rds CreateReplicationGroup is called (valid cluster bundle subnets)",
			args: args{
				rdsSvc: &mockRdsClient{
					describeDBSubnetGroupsFn: func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					},
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					},
				},
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             &rds.CreateDBInstanceInput{},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and is available (valid cluster bundle subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return &rds.AddTagsToResourceOutput{}, nil
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
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
			name: "test rds exists and is not available (valid cluster bundle subnets)",
			args: args{
				rdsSvc: &mockRdsClient{
					describeDBSubnetGroupsFn: func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					},
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildPendingDBInstance(testIdentifier),
						}, nil
					},
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					},
				},
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to be modified (valid cluster bundle subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return nil, awserr.New(rds.ErrCodeDBSnapshotNotFoundFault, rds.ErrCodeDBSnapshotNotFoundFault, fmt.Errorf("%v", rds.ErrCodeDBSnapshotNotFoundFault))
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
				maintenanceWindow:       true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds requires modification error creating update strategy (valid_standalone_subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstanceVersion(testIdentifier, testInvalidEngineVersion),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return nil, awserr.New(rds.ErrCodeDBSnapshotNotFoundFault, rds.ErrCodeDBSnapshotNotFoundFault, fmt.Errorf("%v", rds.ErrCodeDBSnapshotNotFoundFault))
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test error trying to modify available rds (valid_standalone_subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return nil, awserr.New(rds.ErrCodeDBSnapshotNotFoundFault, rds.ErrCodeDBSnapshotNotFoundFault, fmt.Errorf("%v", rds.ErrCodeDBSnapshotNotFoundFault))
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
					rdsClient.modifyDBInstanceFn = func(input *rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error) {
						return nil, genericAWSError
					}
				}),
				ec2Svc: &mockEc2Client{
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: true,
				maintenanceWindow:       true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test rds exists and status is available and does not need to be modified (valid cluster bundle subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return &rds.AddTagsToResourceOutput{}, nil
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildAvailableCreateInput(testIdentifier),
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to be modified but maintenance is pending (valid cluster bundle subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return nil, awserr.New(rds.ErrCodeDBSnapshotNotFoundFault, rds.ErrCodeDBSnapshotNotFoundFault, fmt.Errorf("%v", rds.ErrCodeDBSnapshotNotFoundFault))
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
				maintenanceWindow:       true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds exists and status is available and needs to update pending maintenance (valid cluster bundle subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{}, nil
					}
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return nil, awserr.New(rds.ErrCodeDBSnapshotNotFoundFault, rds.ErrCodeDBSnapshotNotFoundFault, fmt.Errorf("%v", rds.ErrCodeDBSnapshotNotFoundFault))
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeVpcsFn: func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					},
					subnets: buildValidBundleSubnets(),
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
					describeSubnetsFn: func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					},
					describeAvailabilityZonesFn: func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildAZ(),
						}, nil
					},
				},
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             buildNewRequiresModificationsCreateInput(testIdentifier),
				standaloneNetworkExists: false,
				maintenanceWindow:       true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test rds is exists and is available (valid cluster standalone subnets)",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					}
					rdsClient.addTagsToResourceFn = func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return &rds.AddTagsToResourceOutput{}, nil
					}
					rdsClient.describeDBSnapshotsFn = func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					}
					rdsClient.describePendingMaintenanceActionsFn = func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					}
				}),
				ec2Svc: &mockEc2Client{
					describeSecurityGroupsFn: func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: buildSecurityGroups(secName),
						}, nil
					},
				},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				postgresCfg: &rds.CreateDBInstanceInput{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
				standaloneNetworkExists: true,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         resources.BuildMockConnectionTester(),
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
			name: "error getting replication groups",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return nil, genericAWSError
					}
				}),
				ctx:                     context.TODO(),
				cr:                      nil,
				postgresCfg:             nil,
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         nil,
			},
			want:    nil,
			wantErr: true,
			mockFn: func() {
				timeOut = time.Millisecond * 10
			},
		},
		{
			name: "error setting up resource vpc",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					}
				}),
				ctx:                     context.TODO(),
				cr:                      nil,
				postgresCfg:             nil,
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error setting up security group",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					}
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{
							DBSubnetGroups: buildRDSSubnetGroup(),
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return nil, genericAWSError
					}
				}),
				ctx:                     context.TODO(),
				cr:                      nil,
				postgresCfg:             nil,
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failed to retrieve rds credential secret",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					}
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{
							DBSubnetGroups: buildRDSSubnetGroup(),
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.createSecurityGroupFn = func(input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
						return nil, nil
					}
				}),
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             nil,
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "unable to retrieve rds password",
			args: args{
				rdsSvc: buildMockRdsClient(func(rdsClient *mockRdsClient) {
					rdsClient.describeDBInstancesFn = func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					}
					rdsClient.describeDBSubnetGroupsFn = func(input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
						return &rds.DescribeDBSubnetGroupsOutput{
							DBSubnetGroups: buildRDSSubnetGroup(),
						}, nil
					}
				}),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []*ec2.Subnet{
								buildValidClusterSubnet(nil),
							},
						}, nil
					}
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: buildVpcs(),
						}, nil
					}
					ec2Client.describeSecurityGroupsFn = func(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					}
					ec2Client.createSecurityGroupFn = func(input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
						return nil, nil
					}
				}),
				ctx:                     context.TODO(),
				cr:                      buildTestPostgresCR(),
				postgresCfg:             nil,
				standaloneNetworkExists: false,
				maintenanceWindow:       false,
			},
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), &corev1.Secret{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name:      "test-aws-rds-credentials",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"user":     []byte("postgres"),
						"password": []byte(""),
					},
				}),
				Logger:            testLogger,
				CredentialManager: nil,
				ConfigManager:     nil,
				TCPPinger:         nil,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockFn != nil {
				tt.mockFn()
				defer func() {
					timeOut = time.Minute * 5
				}()
			}
			p := &PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         tt.fields.TCPPinger,
			}
			got, _, err := p.reconcileRDSInstance(tt.args.ctx, tt.args.cr, tt.args.rdsSvc, tt.args.ec2Svc, tt.args.postgresCfg, tt.args.standaloneNetworkExists, tt.args.maintenanceWindow)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileRDSInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reconcileRDSInstance() got = %v, want %v", got.DeploymentDetails, tt.want)
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
		ctx                     context.Context
		pg                      *v1alpha1.Postgres
		networkManager          NetworkManager
		instanceSvc             rdsiface.RDSAPI
		ec2Svc                  ec2iface.EC2API
		postgresCreateConfig    *rds.CreateDBInstanceInput
		postgresDeleteConfig    *rds.DeleteDBInstanceInput
		standaloneNetworkExists bool
		isLastResource          bool
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
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					},
				},
				ec2Svc:                  &mockEc2Client{},
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildDbInstanceGroupPending(),
						}, nil
					},
				},
				ec2Svc:                  &mockEc2Client{},
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildDbInstanceGroupAvailable(),
						}, nil
					},
				},
				ec2Svc:                  &mockEc2Client{},
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
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
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildDbInstanceDeletionProtection(),
						}, nil
					},
				},
				ec2Svc:                  &mockEc2Client{},
				standaloneNetworkExists: false,
				isLastResource:          false,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage("deletion protection detected, modifyDBInstance() in progress, current aws rds status is available"),
			wantErr: false,
		},
		{
			name: "test successful delete with no postgres and deletion of standalone network",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{},
				postgresCreateConfig: &rds.CreateDBInstanceInput{},
				pg:                   buildTestPostgresCR(),
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					},
				},
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRSixteen),
							},
						}, nil
					}
				}),
				standaloneNetworkExists: true,
				isLastResource:          true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage(""),
			wantErr: false,
		},
		{
			name: "test successful delete with no postgres and deletion of bundled network resources",
			args: args{
				postgresDeleteConfig: &rds.DeleteDBInstanceInput{},
				postgresCreateConfig: &rds.CreateDBInstanceInput{},
				pg:                   buildTestPostgresCR(),
				networkManager:       buildMockNetworkManager(),
				instanceSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					},
				},
				ec2Svc:                  &mockEc2Client{},
				standaloneNetworkExists: false,
				isLastResource:          true,
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:    croType.StatusMessage(""),
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
			got, err := p.deleteRDSInstance(tt.args.ctx, tt.args.pg, tt.args.networkManager, tt.args.instanceSvc, tt.args.ec2Svc, tt.args.postgresCreateConfig, tt.args.postgresDeleteConfig, tt.args.standaloneNetworkExists, tt.args.isLastResource)
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
					Status: croType.ResourceTypeStatus{
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
					Status: croType.ResourceTypeStatus{
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
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
				rdsSvc: &mockRdsClient{
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{}, nil
					},
					addTagsToResourceFn: func(input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
						return &rds.AddTagsToResourceOutput{}, nil
					},
					describeDBSnapshotsFn: func(input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
						return &rds.DescribeDBSnapshotsOutput{
							DBSnapshots: []*rds.DBSnapshot{
								{
									DBSnapshotArn:        &snapshotARN,
									DBSnapshotIdentifier: &snapshotIdentifier,
								},
							},
						}, nil
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
					AvailabilityZone:     aws.String("test-availabilityZone"),
					DBInstanceArn:        aws.String("arn:test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), builtTestCredSecret(), buildTestInfra()),
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
		cr          *v1alpha1.Postgres
	}
	tests := []struct {
		name    string
		args    args
		want    *rds.ModifyDBInstanceInput
		wantErr string
	}{
		{
			name: "test modification not required",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want: nil,
		},
		{
			name: "test when modification is required",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(false),
					BackupRetentionPeriod:      aws.Int64(0),
					DBInstanceClass:            aws.String("newValue"),
					PubliclyAccessible:         aws.Bool(false),
					MaxAllocatedStorage:        aws.Int64(0),
					EngineVersion:              aws.String("11.1"),
					MultiAZ:                    aws.Bool(false),
					PreferredBackupWindow:      aws.String("newValue"),
					PreferredMaintenanceWindow: aws.String("newValue"),
					Port:                       aws.Int64(0),
				},
				foundConfig: &rds.DBInstance{
					AutoMinorVersionUpgrade:    aws.Bool(true),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresApplyImmediatelyCR(),
			},
			want: &rds.ModifyDBInstanceInput{
				ApplyImmediately:           aws.Bool(true),
				AutoMinorVersionUpgrade:    aws.Bool(false),
				AllowMajorVersionUpgrade:   aws.Bool(true),
				DeletionProtection:         aws.Bool(false),
				BackupRetentionPeriod:      aws.Int64(0),
				DBInstanceClass:            aws.String("newValue"),
				PubliclyAccessible:         aws.Bool(false),
				EngineVersion:              aws.String("11.1"),
				MaxAllocatedStorage:        aws.Int64(0),
				MultiAZ:                    aws.Bool(false),
				PreferredBackupWindow:      aws.String("newValue"),
				PreferredMaintenanceWindow: aws.String("newValue"),
				DBPortNumber:               aws.Int64(0),
				DBInstanceIdentifier:       aws.String("test"),
			},
		},
		{
			name: "test modification not required when instance engine version is higher than configured",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					EngineVersion:              aws.String("10.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want: nil,
		},
		{
			name: "test modification not required when no engine version found in rdsConfig",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want: nil,
		},
		{
			name: "test invalid version number in rdsConfig causes an error",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					EngineVersion:              aws.String("broken version num"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want:    nil,
			wantErr: "invalid postgres version: failed to parse desired version: Malformed version: broken version num",
		},
		{
			name: "test invalid version number on foundConfig causes an error",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				foundConfig: &rds.DBInstance{
					EngineVersion:              aws.String("broken version num"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rds.Endpoint{
						Port: aws.Int64(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want:    nil,
			wantErr: "invalid postgres version: failed to parse current version: Malformed version: broken version num",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRDSUpdateStrategy(tt.args.rdsConfig, tt.args.foundConfig, tt.args.cr)

			if err != nil {
				if tt.wantErr == "" {
					t.Errorf("buildRDSUpdateStrategy() error: %v", err)
				} else if tt.wantErr != "" && err.Error() != tt.wantErr {
					t.Errorf("buildRDSUpdateStrategy() wanted error %v, got error %v", tt.wantErr, err.Error())
				}
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildRDSUpdateStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_rdsApplyServiceUpdates(t *testing.T) {
	testIdentifier := "test-identifier"
	scheme, err := buildTestSchemePostgresql()
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
		session        rdsiface.RDSAPI
		rdsCfg         *rds.CreateDBInstanceInput
		serviceUpdates *ServiceUpdate
		foundInstance  *rds.DBInstance
	}
	tests := []struct {
		name       string
		args       args
		fields     fields
		want       croType.StatusMessage
		wantErr    bool
		wantUpdate bool
	}{
		{
			name: "test empty update status",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					},
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					},
				},
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.15"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: nil,
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
					AvailabilityZone:     aws.String("test-availabilityZone"),
					DBInstanceArn:        aws.String("arn:test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       "completed check for service updates",
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "test populated update status",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					},
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					},
					applyPendingMaintenanceActionFn: func(input *rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error) {
						return &rds.ApplyPendingMaintenanceActionOutput{}, nil
					},
				},
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.18"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
					AvailabilityZone:     aws.String("test-availabilityZone"),
					DBInstanceArn:        aws.String("arn-test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       "completed check for service updates",
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "failed to get pending maintenance information",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return nil, genericAWSError
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       croType.StatusMessage("failed to get pending maintenance information for RDS with identifier : " + testIdentifier),
			wantErr:    true,
			wantUpdate: false,
		},
		{
			name: "error epoc timestamp requires string",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					},
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"invalid",
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       croType.StatusMessage("epoc timestamp requires string"),
			wantErr:    true,
			wantUpdate: false,
		},
		{
			name: "failed to apply service update",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return buildPendingMaintenanceActions()
					},
					applyPendingMaintenanceActionFn: func(input *rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error) {
						return nil, genericAWSError
					},
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       croType.StatusMessage("failed to apply service update"),
			wantErr:    true,
			wantUpdate: false,
		},
		{
			name: "test no autoapply date on pending maintenanance action",
			args: args{
				session: &mockRdsClient{
					describePendingMaintenanceActionsFn: func(input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
						return &rds.DescribePendingMaintenanceActionsOutput{PendingMaintenanceActions: []*rds.ResourcePendingMaintenanceActions{
							{
								ResourceIdentifier: aws.String("arn-test"),
								PendingMaintenanceActionDetails: []*rds.PendingMaintenanceAction{
									{
										Action:      aws.String("system-update"),
										Description: aws.String("New Operating System update is available"),
									},
								},
							},
						}}, nil
					},
					describeDBInstancesFn: func(input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
						return &rds.DescribeDBInstancesOutput{
							DBInstances: buildAvailableDBInstance(testIdentifier),
						}, nil
					},
					applyPendingMaintenanceActionFn: func(input *rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error) {
						return &rds.ApplyPendingMaintenanceActionOutput{}, nil
					},
				},
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int64(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int64(1),
					MaxAllocatedStorage:        aws.Int64(1),
					EngineVersion:              aws.String("10.18"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int64(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rds.DBInstance{
					DBInstanceIdentifier: aws.String(testIdentifier),
					AvailabilityZone:     aws.String("test-availabilityZone"),
					DBInstanceArn:        aws.String("arn-test"),
				},
			},
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(), buildTestInfra(), buildTestPostgresqlPrometheusRule()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			want:       "completed check for service updates",
			wantErr:    false,
			wantUpdate: false,
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
			update, got, err := p.rdsApplyServiceUpdates(tt.args.session, tt.args.serviceUpdates, tt.args.foundInstance)
			if (err != nil) != tt.wantErr {
				t.Errorf("rdsApplyStatusUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("rdsApplyStatusUpdate() got = %v, want %v", got, tt.want)
			}
			if update != tt.wantUpdate {
				t.Errorf("rdsApplyStatusUpdate() update = %v, wantUpdate %v", update, tt.wantUpdate)
			}
		})
	}
}

func buildPendingMaintenanceActions() (*rds.DescribePendingMaintenanceActionsOutput, error) {
	specifiedApplyAfterDate64, _ := strconv.ParseInt("1642032000", 10, 64)
	timeStamp := time.Unix(specifiedApplyAfterDate64, 0)
	return &rds.DescribePendingMaintenanceActionsOutput{
		Marker: nil,
		PendingMaintenanceActions: []*rds.ResourcePendingMaintenanceActions{
			{
				PendingMaintenanceActionDetails: []*rds.PendingMaintenanceAction{
					{
						Action:               aws.String("system-update"),
						AutoAppliedAfterDate: aws.Time(timeStamp),
						CurrentApplyDate:     aws.Time(timeStamp),
						Description:          aws.String("test maintenance"),
						ForcedApplyDate:      aws.Time(timeStamp),
						OptInStatus:          aws.String("immediate"),
					},
				},
				ResourceIdentifier: aws.String("arn-test"),
			},
		},
	}, nil
}

func TestReconcilePostgres(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
	if err != nil {
		t.Error("failed to build scheme", err)
		return
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         resources.ConnectionTester
	}
	type args struct {
		ctx context.Context
		pg  *v1alpha1.Postgres
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		want          *providers.PostgresInstance
		statusMessage croType.StatusMessage
		wantErr       bool
	}{
		{
			name: "failed to set finalizer",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				TCPPinger:         resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				pg:  buildTestPostgresCR(),
			},
			want:          nil,
			statusMessage: "failed to set finalizer",
			wantErr:       true,
		},
		{
			name: "failed to retrieve aws rds cluster config for instance",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR()),
				Logger:            testLogger,
				CredentialManager: &CredentialManagerMock{},
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"test\": \"test\" }"),
							DeleteStrategy: json.RawMessage("{ \"test\": \"test\" }"),
							ServiceUpdates: json.RawMessage(""),
						}, nil
					},
				},
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				pg:  buildTestPostgresCR(),
			},
			want:          nil,
			statusMessage: "failed to retrieve aws rds cluster config for instance",
			wantErr:       true,
		},
		{
			name: "failed to reconcile rds credentials",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), buildTestPostgresCR()),
				Logger: testLogger,
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return nil, genericAWSError
					},
				},
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"test\": \"test\" }"),
							DeleteStrategy: json.RawMessage("{ \"test\": \"test\" }"),
						}, nil
					},
				},
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				pg:  buildTestPostgresCR(),
			},
			want:          nil,
			statusMessage: "failed to reconcile rds credentials",
			wantErr:       true,
		},
		{
			name: "failed to check cluster vpc subnets",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra(), buildTestPostgresCR()),
				Logger: testLogger,
				CredentialManager: &CredentialManagerMock{
					ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (*Credentials, error) {
						return &Credentials{}, nil
					},
				},
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage("{ \"test\": \"test\" }"),
							DeleteStrategy: json.RawMessage("{ \"test\": \"test\" }"),
						}, nil
					},
				},
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				ctx: context.TODO(),
				pg:  buildTestPostgresCR(),
			},
			want:          nil,
			statusMessage: "failed to check cluster vpc subnets",
			wantErr:       true,
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
			got, statusMessage, err := p.ReconcilePostgres(tt.args.ctx, tt.args.pg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcilePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcilePostgres() got = %v, want %v", got, tt.want)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("ReconcilePostgres() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestNewAWSPostgresProvider(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	type args struct {
		client func() client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name    string
		args    args
		want    *PostgresProvider
		wantErr bool
	}{
		{
			name: "successfully create new postgres provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: false,
		},
		{
			name: "fail to create new postgres provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return errors.New("generic error")
					}
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAWSPostgresProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSPostgresProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSPostgresProvider() got = %v, want non-nil result", got)
			}
		})
	}
}

func TestAddAnnotation_ClientUpdate(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	type args struct {
		ctx    context.Context
		cr     *v1alpha1.Postgres
		client func() client.Client
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "success add annotation",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					return mockClient
				},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "fail add annotation",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return errors.New("failed to add annotation")
					}
					return mockClient
				},
				ctx: context.TODO(),
				cr:  buildTestPostgresCR(),
			},
			want:    "failed to add annotation",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := addAnnotation(tt.args.ctx, tt.args.client(), tt.args.cr, "test")
			if err != nil {
				if strings.Compare(string(msg), tt.want) != 0 {
					t.Errorf("addAnnotation() got = %v, want %v", string(msg), tt.want)
				}
				return
			}
		})
	}
}

func TestPostgresProvider_setPostgresDeletionTimestampMetric(t *testing.T) {
	type fields struct {
		Client client.Client
	}
	type args struct {
		cr *v1alpha1.Postgres
	}
	scheme, err := buildTestSchemePostgresql()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "success setting postgres deletion timestamp metric",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
			},
			args: args{
				cr: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
		},
		{
			name: "failure setting postgres deletion timestamp metric",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				cr: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresProvider{
				Client: tt.fields.Client,
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			}
			p.setPostgresDeletionTimestampMetric(context.TODO(), tt.args.cr)
		})
	}
}

func TestPostgresProvider_setPostgresMaxMemoryMetric(t *testing.T) {
	testSizeInMiB := int64(1)

	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		TCPPinger         resources.ConnectionTester
	}
	type args struct {
		response      *ec2.DescribeInstanceTypesOutput
		genericLabels map[string]string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "test no nil pointer if response is nil",
			args: args{
				response: nil,
			},
		},
		{
			name: "test metric is not set if instance type less than 1",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{},
			},
		},
		{
			name: "test no nil pointer if MemoryInfo is nil",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []*ec2.InstanceTypeInfo{
						{},
					},
				},
			},
		},
		{
			name: "test no nil pointer if SizeInMiB is nil",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []*ec2.InstanceTypeInfo{
						{
							MemoryInfo: &ec2.MemoryInfo{SizeInMiB: nil},
						},
					},
				},
			},
		},
		{
			name: "test metric is set",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []*ec2.InstanceTypeInfo{
						{
							MemoryInfo: &ec2.MemoryInfo{SizeInMiB: &testSizeInMiB},
						},
					},
				},
				genericLabels: nil,
			},
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
			p.setPostgresMaxMemoryMetric(tt.args.response, tt.args.genericLabels)
		})
	}
}
