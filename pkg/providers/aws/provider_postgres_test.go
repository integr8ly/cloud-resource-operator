package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/mock"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
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

//type mockRdsClient struct {
//	mock.Mock
//	modifyDBSubnetGroupFn               func(*rds.ModifyDBSubnetGroupInput) (*rds.ModifyDBSubnetGroupOutput, error)
//	listTagsForResourceFn               func(*rds.ListTagsForResourceInput) (*rds.ListTagsForResourceOutput, error)
//	removeTagsFromResourceFn            func(*rds.RemoveTagsFromResourceInput) (*rds.RemoveTagsFromResourceOutput, error)
//	deleteDBSubnetGroupFn               func(*rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error)
//	addTagsToResourceFn                 func(*rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error)
//	describeDBSnapshotsFn               func(*rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error)
//	describeDBInstancesFn               func(*rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error)
//	describeDBSubnetGroupsFn            func(*rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error)
//	describePendingMaintenanceActionsFn func(*rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error)
//	applyPendingMaintenanceActionFn     func(*rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error)
//	modifyDBInstanceFn                  func(*rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error)
//}

//type mockEc2Client struct {
//	ec2iface.EC2API
//	firstSubnet     *ec2.Subnet
//	secondSubnet    *ec2.Subnet
//	subnets         []*ec2.Subnet
//	vpcs            []*ec2.Vpc
//	vpc             *ec2.Vpc
//	secGroups       []*ec2.SecurityGroup
//	azs             []*ec2.AvailabilityZone
//	wantErrList     bool
//	returnSecondSub bool
//	// new approach for manually defined mocks
//	// to allow for simple overrides in test table declarations
//	createTagsFn                    func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
//	describeVpcsFn                  func(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
//	describeSecurityGroupsFn        func(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
//	deleteSecurityGroupFn           func(*ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
//	describeVpcPeeringConnectionFn  func(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error)
//	createVpcPeeringConnectionFn    func(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error)
//	acceptVpcPeeringConnectionFn    func(*ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error)
//	deleteVpcPeeringConnectionFn    func(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error)
//	describeRouteTablesFn           func(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
//	createRouteFn                   func(*ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error)
//	deleteRouteFn                   func(*ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error)
//	createVpcFn                     func(*ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error)
//	deleteVpcFn                     func(*ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error)
//	createSubnetFn                  func(*ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error)
//	describeInstanceTypeOfferingsFn func(*ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
//	WaitUntilVpcExistsFn            func(*ec2.DescribeVpcsInput) error
//	describeSubnetsFn               func(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
//	describeAvailabilityZonesFn     func(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error)
//	createSecurityGroupFn           func(*ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
//	calls                           struct {
//		DescribeRouteTables []struct {
//			Tables *ec2.DescribeRouteTablesInput
//		}
//		DescribeSecurityGroups []struct {
//			Groups *ec2.DescribeSecurityGroupsInput
//		}
//		DescribeSubnets []struct {
//			Subnets *ec2.DescribeSubnetsInput
//		}
//		DescribeAvailabilityZones []struct {
//			AvailabilityZones *ec2.DescribeAvailabilityZonesInput
//		}
//		DescribeVpcs []struct {
//			Vpcs *ec2.DescribeVpcsInput
//		}
//		CreateRoute []struct {
//			Route *ec2.CreateRouteInput
//		}
//	}
//}

//func buildMockEc2Client(modifyFn func(*mockEc2Client)) *mockEc2Client {
//	mock := &mockEc2Client{}
//	mock.WaitUntilVpcExistsFn = func(input *ec2.DescribeVpcsInput) error {
//		return nil
//	}
//	mock.deleteVpcFn = func(*ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
//		return &ec2.DeleteVpcOutput{}, nil
//	}
//	mock.createTagsFn = func(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
//		return &ec2.CreateTagsOutput{}, nil
//	}
//	mock.describeInstanceTypeOfferingsFn = func(input *ec2.DescribeInstanceTypeOfferingsInput) (output *ec2.DescribeInstanceTypeOfferingsOutput, e error) {
//		return &ec2.DescribeInstanceTypeOfferingsOutput{
//			InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
//				{
//					Location: aws.String(defaultAzIdOne),
//				},
//				{
//					Location: aws.String(defaultAzIdTwo),
//				},
//			},
//		}, nil
//	}
//	mock.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
//		return &ec2.DescribeSubnetsOutput{}, nil
//	}
//	if modifyFn != nil {
//		modifyFn(mock)
//	}
//	return mock
//}

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

func (m *mockRdsClient) DescribeDBInstances(ctx context.Context, input *rds.DescribeDBInstancesInput) (*rds.DescribeDBInstancesOutput, error) {
	if m.describeDBInstancesFn == nil {
		panic("mockEc2Client.DescribeDBInstances: method is nil")
	}
	return m.describeDBInstancesFn(ctx, input)
}

func (m *mockRdsClient) CreateDBInstance(*rds.CreateDBInstanceInput) (*rds.CreateDBInstanceOutput, error) {
	return &rds.CreateDBInstanceOutput{}, nil
}

func (m *mockRdsClient) ModifyDBInstance(ctx context.Context, input *rds.ModifyDBInstanceInput) (*rds.ModifyDBInstanceOutput, error) {
	if m.modifyDBInstanceFn != nil {
		return m.modifyDBInstanceFn(ctx, input)
	}
	return &rds.ModifyDBInstanceOutput{}, nil
}

func (m *mockRdsClient) DeleteDBInstance(*rds.DeleteDBInstanceInput) (*rds.DeleteDBInstanceOutput, error) {
	return &rds.DeleteDBInstanceOutput{}, nil
}

func (m *mockRdsClient) AddTagsToResource(ctx context.Context, input *rds.AddTagsToResourceInput) (*rds.AddTagsToResourceOutput, error) {
	if resources.SafeStringDereference(input.ResourceName) == snapshotARN {
		return m.addTagsToResourceFn(ctx, input)
	} else {
		return &rds.AddTagsToResourceOutput{}, nil
	}
}

func (m *mockRdsClient) DescribeDBSnapshots(ctx context.Context, input *rds.DescribeDBSnapshotsInput) (*rds.DescribeDBSnapshotsOutput, error) {
	return m.describeDBSnapshotsFn(ctx, input)
}

func (m *mockRdsClient) ApplyPendingMaintenanceAction(ctx context.Context, input *rds.ApplyPendingMaintenanceActionInput) (*rds.ApplyPendingMaintenanceActionOutput, error) {
	if m.applyPendingMaintenanceActionFn == nil {
		panic("mockEc2Client.ApplyPendingMaintenanceAction: method is nil")
	}
	return m.applyPendingMaintenanceActionFn(ctx, input)
}

func (m *mockRdsClient) DescribePendingMaintenanceActions(ctx context.Context, input *rds.DescribePendingMaintenanceActionsInput) (*rds.DescribePendingMaintenanceActionsOutput, error) {
	if m.describePendingMaintenanceActionsFn == nil {
		panic("mockEc2Client.DescribePendingMaintenanceActions: method is nil")
	}
	return m.describePendingMaintenanceActionsFn(ctx, input)
}

func (m *mockRdsClient) DescribeDBSubnetGroups(ctx context.Context, input *rds.DescribeDBSubnetGroupsInput) (*rds.DescribeDBSubnetGroupsOutput, error) {
	if m.describeDBSubnetGroupsFn == nil {
		panic("mockEc2Client.DescribeDBSubnetGroups: method is nil")
	}
	return m.describeDBSubnetGroupsFn(ctx, input)
}

func (m *mockRdsClient) CreateDBSubnetGroup(*rds.CreateDBSubnetGroupInput) (*rds.CreateDBSubnetGroupOutput, error) {
	return &rds.CreateDBSubnetGroupOutput{}, nil
}

func (m *mockRdsClient) ModifyDBSubnetGroup(ctx context.Context, input *rds.ModifyDBSubnetGroupInput) (*rds.ModifyDBSubnetGroupOutput, error) {
	return m.modifyDBSubnetGroupFn(ctx, input)
}

func (m *mockRdsClient) DeleteDBSubnetGroup(ctx context.Context, input *rds.DeleteDBSubnetGroupInput) (*rds.DeleteDBSubnetGroupOutput, error) {
	return m.deleteDBSubnetGroupFn(ctx, input)
}

func (m *mockRdsClient) ListTagsForResource(ctx context.Context, input *rds.ListTagsForResourceInput) (*rds.ListTagsForResourceOutput, error) {
	return m.listTagsForResourceFn(ctx, input)
}

func (m *mockRdsClient) RemoveTagsFromResource(ctx context.Context, input *rds.RemoveTagsFromResourceInput) (*rds.RemoveTagsFromResourceOutput, error) {
	return m.removeTagsFromResourceFn(ctx, input)
}

func (m *mockEc2Client) WaitUntilVpcExists(ctx context.Context, input *ec2.DescribeVpcsInput) error {
	return m.WaitUntilVpcExistsFn(ctx, input)
}

func (m *mockEc2Client) CreateVpc(ctx context.Context, input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
	if m.createVpcFn == nil {
		panic("mockEc2Client.CreateVpc: method is nil")
	}
	return m.createVpcFn(ctx, input)
}

func (m *mockEc2Client) DeleteVpc(ctx context.Context, input *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
	return m.deleteVpcFn(ctx, input)
}

func (m *mockEc2Client) CreateSubnet(ctx context.Context, input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
	if m.createSubnetFn != nil {
		return m.createSubnetFn(ctx, input)
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
	for _, routeInput := range m.calls.CreateRoute {
		calls = append(calls, struct {
			Route *ec2.CreateRouteInput
		}{
			Route: &routeInput,
		})
	}
	lockMockEc2ClientCreateRoute.RUnlock()

	return calls
}

func (m *mockEc2Client) CreateRoute(ctx context.Context, input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	if m.createRouteFn == nil {
		panic("mockEc2Client.DescribeRouteTables: method is nil")
	}

	lockMockEc2ClientCreateRoute.Lock()
	m.calls.CreateRoute = append(m.calls.CreateRoute, *input)
	lockMockEc2ClientCreateRoute.Unlock()

	return m.createRouteFn(ctx, input)
}

func (m *mockEc2Client) DeleteRoute(ctx context.Context, input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	return m.deleteRouteFn(ctx, input)
}

func (m *mockEc2Client) DescribeRouteTables(ctx context.Context, input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	if m.describeRouteTablesFn == nil {
		panic("mockEc2Client.DescribeRouteTables: method is nil")
	}

	lockMockEc2ClientDescribeRouteTables.Lock()
	m.calls.DescribeRouteTables = append(m.calls.DescribeRouteTables, *input)
	lockMockEc2ClientDescribeRouteTables.Unlock()

	return m.describeRouteTablesFn(ctx, input)
}

func (m *mockEc2Client) DescribeRouteTablesCalls() []struct {
	Tables *ec2.DescribeRouteTablesInput
} {
	var calls []struct {
		Tables *ec2.DescribeRouteTablesInput
	}
	lockMockEc2ClientDescribeRouteTables.RLock()
	for _, tableInput := range m.calls.DescribeRouteTables {
		calls = append(calls, struct {
			Tables *ec2.DescribeRouteTablesInput
		}{
			Tables: &tableInput,
		})
	}
	lockMockEc2ClientDescribeRouteTables.RUnlock()

	return calls
}

func (m *mockEc2Client) DescribeSecurityGroups(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	if m.describeSecurityGroupsFn == nil {
		panic("mockEc2Client.DescribeSecurityGroups: method is nil")
	}
	lockMockEc2ClientDescribeSecurityGroups.Lock()
	m.calls.DescribeSecurityGroups = append(m.calls.DescribeSecurityGroups, *input)
	lockMockEc2ClientDescribeSecurityGroups.Unlock()

	return m.describeSecurityGroupsFn(ctx, input)
}

func (m *mockEc2Client) DescribeSecurityGroupsCalls() []struct {
	Groups *ec2.DescribeSecurityGroupsInput
} {
	var calls []struct {
		Groups *ec2.DescribeSecurityGroupsInput
	}

	lockMockEc2ClientDescribeSecurityGroups.RLock()
	for _, groupInput := range m.calls.DescribeSecurityGroups {
		calls = append(calls, struct {
			Groups *ec2.DescribeSecurityGroupsInput
		}{
			Groups: &groupInput,
		})
	}
	lockMockEc2ClientDescribeSecurityGroups.RUnlock()

	return calls
}

func (m *mockEc2Client) CreateSecurityGroup(ctx context.Context, input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	if m.createSecurityGroupFn == nil {
		panic("mockEc2Client.CreateSecurityGroup: method is nil")
	}
	return m.createSecurityGroupFn(ctx, input)
}

func (m *mockEc2Client) DeleteSecurityGroup(ctx context.Context, input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	return m.deleteSecurityGroupFn(ctx, input)
}

func (m *mockEc2Client) AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (m *mockEc2Client) DescribeAvailabilityZones(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if m.describeAvailabilityZonesFn == nil {
		panic("mockEc2Client.DescribeAvailabilityZones: method is nil")
	}
	lockMockEc2ClientDescribeAvailabilityZones.Lock()
	m.calls.DescribeAvailabilityZones = append(m.calls.DescribeAvailabilityZones, *input)
	lockMockEc2ClientDescribeAvailabilityZones.Unlock()

	return m.describeAvailabilityZonesFn(ctx, input)
}

func (m *mockEc2Client) DescribeVpcPeeringConnections(ctx context.Context, input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	return m.describeVpcPeeringConnectionFn(ctx, input)
}

func (m *mockEc2Client) CreateVpcPeeringConnection(ctx context.Context, input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	return m.createVpcPeeringConnectionFn(ctx, input)
}

func (m *mockEc2Client) CreateTags(ctx context.Context, input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	return m.createTagsFn(ctx, input)
}

func (m *mockEc2Client) AcceptVpcPeeringConnection(ctx context.Context, input *ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	return m.acceptVpcPeeringConnectionFn(ctx, input)
}

func (m *mockEc2Client) DeleteVpcPeeringConnection(ctx context.Context, input *ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	return m.deleteVpcPeeringConnectionFn(ctx, input)
}

func (m *mockEc2Client) DescribeInstanceTypeOfferings(ctx context.Context, input *ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	return m.describeInstanceTypeOfferingsFn(ctx, input)
}

// the only place this is called is the exposePostgresMetrics func which is not being tested
// return empty result
func (m *mockEc2Client) DescribeInstanceTypes(*ec2.DescribeInstanceTypesInput) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{}, nil
}

// TODO need to remove all awserr from codebase to run moq with make code/gen
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

func buildDbInstanceGroupPending() []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
		{
			DBInstanceIdentifier: aws.String("test-id"),
			AvailabilityZone:     aws.String("test-availabilityZone"),
			DBInstanceStatus:     aws.String("pending"),
			DBInstanceClass:      aws.String(defaultAwsDBInstanceClass),
		},
	}
}

func buildDbInstanceGroupAvailable() []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
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

func buildDbInstanceDeletionProtection() []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
		{
			DBInstanceIdentifier: aws.String("test-id"),
			DBInstanceStatus:     aws.String("available"),
			AvailabilityZone:     aws.String("test-availabilityZone"),
			DeletionProtection:   aws.Bool(true),
			DBInstanceClass:      aws.String(defaultAwsDBInstanceClass),
		},
	}
}

func buildAvailableDBInstance(testID string) []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AutoMinorVersionUpgrade:    aws.Bool(false),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			CACertificateIdentifier:    aws.String(newCert),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int32(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
			EngineVersion:              aws.String(defaultAwsEngineVersion),
			Engine:                     aws.String(defaultAwsEngine),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			MultiAZ:                    aws.Bool(true),
			Endpoint: &rdstypes.Endpoint{
				Address:      aws.String("blob"),
				HostedZoneId: aws.String("blog"),
				Port:         aws.Int32(defaultAwsPostgresPort),
			},
		},
	}
}

func buildAvailableDBInstanceVersion(testID string, version string) []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AutoMinorVersionUpgrade:    aws.Bool(false),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			CACertificateIdentifier:    aws.String(newCert),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int32(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
			EngineVersion:              aws.String(version),
			Engine:                     aws.String(defaultAwsEngine),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			MultiAZ:                    aws.Bool(true),
			Endpoint: &rdstypes.Endpoint{
				Address:      aws.String("blob"),
				HostedZoneId: aws.String("blog"),
				Port:         aws.Int32(defaultAwsPostgresPort),
			},
		},
	}
}

func buildPendingDBInstance(testID string) []rdstypes.DBInstance {
	return []rdstypes.DBInstance{
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
		Port:                       aws.Int32(defaultAwsPostgresPort),
		BackupRetentionPeriod:      aws.Int32(defaultAwsBackupRetentionPeriod),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
		MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
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
		Port:                       aws.Int32(123),
		BackupRetentionPeriod:      aws.Int32(defaultAwsBackupRetentionPeriod),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
		MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
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
		Port:                       aws.Int32(123),
		BackupRetentionPeriod:      aws.Int32(123),
		DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
		PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
		AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
		MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
		EngineVersion:              aws.String(defaultAwsEngineVersion),
		PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
		PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
		MultiAZ:                    aws.Bool(true),
	}
}

func buildPendingModifiedDBInstance(testID string) []*rdstypes.DBInstance {
	return []*rdstypes.DBInstance{
		{
			DBInstanceIdentifier:       aws.String(testID),
			DBInstanceStatus:           aws.String("available"),
			AvailabilityZone:           aws.String("test-availabilityZone"),
			AutoMinorVersionUpgrade:    aws.Bool(false),
			DBInstanceArn:              aws.String("arn-test"),
			DeletionProtection:         aws.Bool(defaultAwsPostgresDeletionProtection),
			MasterUsername:             aws.String(defaultAwsPostgresUser),
			DBName:                     aws.String(defaultAwsPostgresDatabase),
			BackupRetentionPeriod:      aws.Int32(defaultAwsBackupRetentionPeriod),
			DBInstanceClass:            aws.String(defaultAwsDBInstanceClass),
			PubliclyAccessible:         aws.Bool(defaultAwsPubliclyAccessible),
			AllocatedStorage:           aws.Int32(defaultAwsAllocatedStorage),
			MaxAllocatedStorage:        aws.Int32(defaultAwsMaxAllocatedStorage),
			EngineVersion:              aws.String(defaultAwsEngineVersion),
			Engine:                     aws.String(defaultAwsEngine),
			PreferredMaintenanceWindow: aws.String(testPreferredMaintenanceWindow),
			PreferredBackupWindow:      aws.String(testPreferredBackupWindow),
			MultiAZ:                    aws.Bool(true),
			Endpoint: &rdstypes.Endpoint{
				Address:      aws.String("blob"),
				HostedZoneId: aws.String("blog"),
				Port:         aws.Int32(defaultAwsPostgresPort),
			},
			PendingModifiedValues: &rdstypes.PendingModifiedValues{
				Port: aws.Int32(123),
			},
		},
	}
}

func buildVpcs() []ec2types.Vpc {
	return []ec2types.Vpc{
		{
			VpcId:     aws.String(defaultVpcId),
			CidrBlock: aws.String("10.0.0.0/16"),
			Tags: []ec2types.Tag{
				{
					Key:   aws.String("test-vpc"),
					Value: aws.String("test-vpc"),
				},
			},
		},
	}
}

func buildAZ() []ec2types.AvailabilityZone {
	return []ec2types.AvailabilityZone{
		{
			ZoneName: aws.String("test"),
			State:    "available",
		},
	}
}
func buildSecurityGroup(modifyFn func(cluster *ec2types.SecurityGroup)) *ec2types.SecurityGroup {
	mock := &ec2types.SecurityGroup{
		GroupName: aws.String("test"),
		GroupId:   aws.String("testID"),
	}

	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func buildSecurityGroups(groupName string) []ec2types.SecurityGroup {
	return []ec2types.SecurityGroup{
		*buildSecurityGroup(func(mock *ec2types.SecurityGroup) {
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
		rdsClient               *rds.Client
		ec2Client               *ec2.Client
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.AddTagsToResourceOutput{}, nil)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildPendingDBInstance(testIdentifier),
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					//TODO Confirm this work as a replacement for awserr
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(
						nil,
						&smithy.OperationError{
							ServiceID:     "RDS",
							OperationName: "AddTagsToResource",
							Err: &smithy.GenericAPIError{
								Code:    "DBSnapshotNotFound",
								Message: "DB snapshot not found",
								Fault:   smithy.FaultClient,
							},
						},
					)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstanceVersion(testIdentifier, testInvalidEngineVersion),
					}, nil)
					//TODO Confirm this work as a replacement for awserr
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(
						nil,
						&smithy.OperationError{
							ServiceID:     "RDS",
							OperationName: "AddTagsToResource",
							Err: &smithy.GenericAPIError{
								Code:    "DBSnapshotNotFound",
								Message: "DB snapshot not found",
								Fault:   smithy.FaultClient,
							},
						},
					)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					//TODO Confirm this work as a replacement for awserr
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(
						nil,
						&smithy.OperationError{
							ServiceID:     "RDS",
							OperationName: "AddTagsToResource",
							Err: &smithy.GenericAPIError{
								Code:    "DBSnapshotNotFound",
								Message: "DB snapshot not found",
								Fault:   smithy.FaultClient,
							},
						},
					)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					mockRds.On("ModifyDBInstance", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.AddTagsToResourceOutput{}, nil)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					//TODO Confirm this works as a replacement for awserr
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(
						nil,
						&smithy.OperationError{
							ServiceID:     "RDS",
							OperationName: "AddTagsToResource",
							Err: &smithy.GenericAPIError{
								Code:    "DBSnapshotNotFound",
								Message: "DB snapshot not found",
								Fault:   smithy.FaultClient,
							},
						},
					)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					//TODO Confirm this works as a replacement for awserr
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(
						nil,
						&smithy.OperationError{
							ServiceID:     "RDS",
							OperationName: "AddTagsToResource",
							Err: &smithy.GenericAPIError{
								Code:    "DBSnapshotNotFound",
								Message: "DB snapshot not found",
								Fault:   smithy.FaultClient,
							},
						},
					)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					// subnets: buildValidBundleSubnets(), don't see this doing anything in the old moc , just adding a comment just in case
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildAZ(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.AddTagsToResourceOutput{}, nil)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(
						buildPendingMaintenanceActions(),
					)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: buildSecurityGroups(secName),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{
						DBSubnetGroups: buildRDSSubnetGroup(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{
						DBSubnetGroups: buildRDSSubnetGroup(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							*buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					mockRds.On("DescribeDBSubnetGroups", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSubnetGroupsOutput{
						DBSubnetGroups: buildRDSSubnetGroup(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: []ec2types.Subnet{
							*buildValidClusterSubnet(nil),
						},
					}, nil)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: buildVpcs(),
					}, nil)
					mockEc2.On("DescribeSecurityGroups", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{}, nil)
					mockEc2.On("CreateSecurityGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
			got, _, err := p.reconcileRDSInstance(tt.args.ctx, tt.args.cr, *tt.args.rdsClient, *tt.args.ec2Client, tt.args.postgresCfg, tt.args.standaloneNetworkExists, tt.args.maintenanceWindow)
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
		rdsClient               *rds.Client
		ec2Client               *ec2.Client
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildDbInstanceGroupPending(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildDbInstanceGroupAvailable(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildDbInstanceDeletionProtection(),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRSixteen),
						},
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
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
			got, err := p.deleteRDSInstance(tt.args.ctx, tt.args.pg, tt.args.networkManager, *tt.args.rdsClient, *tt.args.ec2Client, tt.args.postgresCreateConfig, tt.args.postgresDeleteConfig, tt.args.standaloneNetworkExists, tt.args.isLastResource)
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
		rdsClient     *rds.Client
		foundInstance *rdstypes.DBInstance
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
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{}, nil)
					mockRds.On("AddTagsToResource", mock.Anything, mock.Anything, mock.Anything).Return(&rds.AddTagsToResourceOutput{}, nil)
					mockRds.On("DescribeDBSnapshots", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBSnapshotsOutput{
						DBSnapshots: []rdstypes.DBSnapshot{
							{
								DBSnapshotArn:        &snapshotARN,
								DBSnapshotIdentifier: &snapshotIdentifier,
							},
						},
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				foundInstance: &rdstypes.DBInstance{
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
			got, err := p.TagRDSPostgres(tt.args.ctx, tt.args.cr, *tt.args.rdsClient, tt.args.foundInstance)
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
		foundConfig *rdstypes.DBInstance
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
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
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
					BackupRetentionPeriod:      aws.Int32(0),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("newValue"),
					PubliclyAccessible:         aws.Bool(false),
					MaxAllocatedStorage:        aws.Int32(0),
					EngineVersion:              aws.String("11.1"),
					MultiAZ:                    aws.Bool(false),
					PreferredBackupWindow:      aws.String("newValue"),
					PreferredMaintenanceWindow: aws.String("newValue"),
					Port:                       aws.Int32(0),
				},
				foundConfig: &rdstypes.DBInstance{
					AutoMinorVersionUpgrade:    aws.Bool(true),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
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
				BackupRetentionPeriod:      aws.Int32(0),
				DBInstanceClass:            aws.String("newValue"),
				PubliclyAccessible:         aws.Bool(false),
				EngineVersion:              aws.String("11.1"),
				MaxAllocatedStorage:        aws.Int32(0),
				MultiAZ:                    aws.Bool(false),
				PreferredBackupWindow:      aws.String("newValue"),
				PreferredMaintenanceWindow: aws.String("newValue"),
				DBPortNumber:               aws.Int32(0),
				DBInstanceIdentifier:       aws.String("test"),
			},
		},
		{
			name: "test modification not required when instance engine version is higher than configured",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					EngineVersion:              aws.String("10.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
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
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					CACertificateIdentifier:    aws.String(newCert),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
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
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					EngineVersion:              aws.String("11.1"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
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
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					EngineVersion:              aws.String("broken version num"),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					CACertificateIdentifier:    aws.String(newCert),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want:    nil,
			wantErr: "invalid postgres version: failed to parse current version: Malformed version: broken version num",
		},
		{
			name: "test CACertificate update expected",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					CACertificateIdentifier:    aws.String(existingCert),
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					CACertificateIdentifier:    aws.String(existingCert),
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want: &rds.ModifyDBInstanceInput{
				CACertificateIdentifier: aws.String(newCert),
				DBInstanceIdentifier:    aws.String("test"),
			},
		},
		{
			name: "test CACertificate no update expected",
			args: args{
				rdsConfig: &rds.CreateDBInstanceInput{
					CACertificateIdentifier:    aws.String(newCert),
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				foundConfig: &rdstypes.DBInstance{
					CACertificateIdentifier:    aws.String(newCert),
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.1"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Endpoint: &rdstypes.Endpoint{
						Port: aws.Int32(1),
					},
					DBInstanceIdentifier: aws.String("test"),
				},
				cr: buildTestPostgresCR(),
			},
			want: nil,
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
		ctx            context.Context
		rdsClient      *rds.Client
		rdsCfg         *rds.CreateDBInstanceInput
		serviceUpdates *ServiceUpdate
		foundInstance  *rdstypes.DBInstance
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(buildPendingMaintenanceActions())
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.15"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: nil,
				},
				foundInstance: &rdstypes.DBInstance{
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(buildPendingMaintenanceActions())
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					mockRds.On("ApplyPendingMaintenanceAction", mock.Anything, mock.Anything, mock.Anything).Return(&rds.ApplyPendingMaintenanceActionOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.18"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rdstypes.DBInstance{
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				foundInstance: &rdstypes.DBInstance{
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(buildPendingMaintenanceActions())
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"invalid",
					},
				},
				foundInstance: &rdstypes.DBInstance{
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(buildPendingMaintenanceActions())
					mockRds.On("ApplyPendingMaintenanceAction", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rdstypes.DBInstance{
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
				ctx: context.TODO(),
				rdsClient: func() *rds.Client {
					mockRds := new(mockRdsClient)
					mockRds.On("DescribePendingMaintenanceActions", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribePendingMaintenanceActionsOutput{
						PendingMaintenanceActions: []rdstypes.ResourcePendingMaintenanceActions{
							{
								ResourceIdentifier: aws.String("arn-test"),
								PendingMaintenanceActionDetails: []rdstypes.PendingMaintenanceAction{
									{
										Action:      aws.String("system-update"),
										Description: aws.String("New Operating System update is available"),
									},
								},
							},
						},
					}, nil)
					mockRds.On("DescribeDBInstances", mock.Anything, mock.Anything, mock.Anything).Return(&rds.DescribeDBInstancesOutput{
						DBInstances: buildAvailableDBInstance(testIdentifier),
					}, nil)
					mockRds.On("ApplyPendingMaintenanceAction", mock.Anything, mock.Anything, mock.Anything).Return(&rds.ApplyPendingMaintenanceActionOutput{}, nil)
					return (*rds.Client)(unsafe.Pointer(mockRds))
				}(),
				rdsCfg: &rds.CreateDBInstanceInput{
					AutoMinorVersionUpgrade:    aws.Bool(false),
					DeletionProtection:         aws.Bool(true),
					BackupRetentionPeriod:      aws.Int32(1),
					DBInstanceClass:            aws.String("test"),
					PubliclyAccessible:         aws.Bool(true),
					AllocatedStorage:           aws.Int32(1),
					MaxAllocatedStorage:        aws.Int32(1),
					EngineVersion:              aws.String("10.18"),
					MultiAZ:                    aws.Bool(true),
					PreferredBackupWindow:      aws.String("test"),
					PreferredMaintenanceWindow: aws.String("test"),
					Port:                       aws.Int32(1),
				},
				serviceUpdates: &ServiceUpdate{
					updates: []string{
						"1642032001",
					},
				},
				foundInstance: &rdstypes.DBInstance{
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
			update, got, err := p.rdsApplyServiceUpdates(tt.args.ctx, *tt.args.rdsClient, tt.args.serviceUpdates, tt.args.foundInstance)
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
		PendingMaintenanceActions: []rdstypes.ResourcePendingMaintenanceActions{
			{
				PendingMaintenanceActionDetails: []rdstypes.PendingMaintenanceAction{
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
					InstanceTypes: []ec2types.InstanceTypeInfo{
						{},
					},
				},
			},
		},
		{
			name: "test no nil pointer if SizeInMiB is nil",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []ec2types.InstanceTypeInfo{
						{
							MemoryInfo: &ec2types.MemoryInfo{SizeInMiB: nil},
						},
					},
				},
			},
		},
		{
			name: "test metric is set",
			args: args{
				response: &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []ec2types.InstanceTypeInfo{
						{
							MemoryInfo: &ec2types.MemoryInfo{SizeInMiB: &testSizeInMiB},
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
