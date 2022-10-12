package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	servicenetworking "google.golang.org/api/servicenetworking/v1"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	utils "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultServiceConnectionName   = "servicenetworking-googleapis-com"
	defaultServiceConnectionURI    = "servicenetworking.googleapis.com"
	defaultIpRangePostfix          = "ip-range"
	defaultIpRangeCIDRMask         = 22
	defaultNumberOfExpectedSubnets = 2
)

type NetworkManager interface {
	CreateNetworkIpRange(context.Context) (*computepb.Address, error)
	CreateNetworkService(context.Context) (*servicenetworking.Connection, error)
	DeleteNetworkPeering(context.Context) error
	DeleteNetworkService(context.Context) error
	DeleteNetworkIpRange(context.Context) error
	ComponentsExist(context.Context) (bool, error)
}

var _ NetworkManager = (*NetworkProvider)(nil)

type NetworkProvider struct {
	Client       client.Client
	NetworkApi   *compute.NetworksClient
	ServicesApi  *servicenetworking.APIService
	AddressApi   *compute.GlobalAddressesClient
	Logger       *logrus.Entry
	IsSTSCluster bool
}

// initialises the three required clients
func NewNetworkManager(ctx context.Context, opt option.ClientOption, client client.Client, logger *logrus.Entry) (*NetworkProvider, error) {
	networkApi, err := compute.NewNetworksRESTClient(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise network client")
	}
	servicesApi, err := servicenetworking.NewService(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise servicenetworking client")
	}
	addressApi, err := compute.NewGlobalAddressesRESTClient(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise address client")
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &NetworkProvider{
		Client:      client,
		NetworkApi:  networkApi,
		ServicesApi: servicesApi,
		AddressApi:  addressApi,
		Logger:      logger.WithField("provider", "gcp_network_provider"),
	}, nil
}

func (n *NetworkProvider) CreateNetworkIpRange(ctx context.Context) (*computepb.Address, error) {
	// get cluster vpc
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting project id")
	}
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	// retrieve ip address range
	address, err := n.getAddressRange(ctx, projectID, ipRange)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	// if it does not exist, create it
	if address == nil {
		err := n.createAddressRange(ctx, clusterVpc, projectID, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to create ip address range")
		}
		// check if address exists
		address, err = n.getAddressRange(ctx, projectID, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
		}
	}
	return address, nil
}

// Creates the network service connection and will return the service if it has been created successfully
// This automatically creates a peering connection to the clusterVpc named after the service connection
func (n *NetworkProvider) CreateNetworkService(ctx context.Context) (*servicenetworking.Connection, error) {
	// get cluster vpc
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return nil, errorUtil.Wrap(err, "error getting project name")
	}
	// get service connection
	service, err := n.getServiceConnection(clusterVpc, projectID)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to retrieve service connection")
	}
	// if it does not exist, create it
	if service == nil {
		// build ip address range name
		ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to build ip address range infra name")
		}
		// retrieve ip range
		address, err := n.getAddressRange(ctx, projectID, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
		}
		// if the ip range is present, and is ready for use
		// possible states for address are RESERVING, RESERVED, IN_USE
		if address != nil && address.GetStatus() == computepb.Address_RESERVED.String() {
			err = n.createServiceConnection(clusterVpc, projectID, ipRange)
			if err != nil {
				return nil, errorUtil.Wrap(err, "failed to create service connection")
			}
			// check if service exists
			service, err = n.getServiceConnection(clusterVpc, projectID)
			if err != nil {
				return nil, errorUtil.Wrap(err, "failed to retrieve service connection")
			}
		}
	}
	return service, nil
}

// Removes the peering connection from the cluster vpc
// The service connection removal can get stuck if this is not performed first
func (n *NetworkProvider) DeleteNetworkPeering(ctx context.Context) error {
	// get cluster vpc
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting project name")
	}
	// get peering connection
	peering := n.getPeeringConnection(ctx, clusterVpc)
	// if it exists, delete it
	if peering != nil {
		// delete peering connection
		err = n.deletePeeringConnection(ctx, clusterVpc, projectID)
		if err != nil {
			return errorUtil.Wrap(err, "failed to delete peering connection")
		}
	}
	return nil
}

// This deletes the network service connection, but can get stuck if peering
// has not been removed
func (n *NetworkProvider) DeleteNetworkService(ctx context.Context) error {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting project name")
	}
	// get service connection
	service, err := n.getServiceConnection(clusterVpc, projectID)
	if err != nil {
		return err
	}
	if service != nil {
		// delete service connection
		err := n.deleteServiceConnection(service)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *NetworkProvider) DeleteNetworkIpRange(ctx context.Context) error {
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return errorUtil.Wrap(err, "error getting project name")
	}
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	// get ip address range
	address, err := n.getAddressRange(ctx, projectID, ipRange)
	if err != nil {
		return errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	if address != nil {
		clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
		if err != nil {
			return errorUtil.Wrap(err, "failed to get cluster vpc")
		}
		// get service connection
		service, err := n.getServiceConnection(clusterVpc, projectID)
		if err != nil {
			return err
		}
		if service != nil && service.ReservedPeeringRanges[0] == address.GetName() {
			return errorUtil.Wrap(err, "failed to delete ip address range, service connection still present")
		}
		// delete ip address range
		err = n.deleteAddressRange(ctx, projectID, ipRange)
		if err != nil {
			return errorUtil.Wrap(err, "failed to delete ip address range")
		}
	}
	return nil
}

func (n *NetworkProvider) ComponentsExist(ctx context.Context) (bool, error) {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.Logger)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// get project ID
	projectID, err := resources.GetGCPProject(ctx, n.Client)
	if err != nil {
		return false, errorUtil.Wrap(err, "error getting project name")
	}
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	// get ip address range
	address, err := n.getAddressRange(ctx, projectID, ipRange)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	// get service connection
	service, err := n.getServiceConnection(clusterVpc, projectID)
	if err != nil {
		return false, err
	}
	if address == nil && service == nil {
		return false, nil
	}
	if address != nil {
		n.Logger.Infof("ip address range %s deletion in progress", address.GetName())
	}
	if service != nil {
		n.Logger.Infof("service connection %s deletion in progress", service.Service)
	}
	return true, nil
}

func (n *NetworkProvider) getServiceConnection(clusterVpc *computepb.Network, projectID string) (*servicenetworking.Connection, error) {
	call := n.ServicesApi.Services.Connections.List(fmt.Sprintf("services/%s", defaultServiceConnectionURI))
	call.Network(fmt.Sprintf("projects/%s/global/networks/%s", projectID, clusterVpc.GetName()))
	resp, err := call.Do()
	if err != nil {
		return nil, err
	}
	if len(resp.Connections) == 0 {
		return nil, nil
	}
	return resp.Connections[0], nil
}

func (n *NetworkProvider) createServiceConnection(clusterVpc *computepb.Network, projectID string, ipRange string) error {
	n.Logger.Infof("creating service connection %s", defaultServiceConnectionName)
	_, err := n.ServicesApi.Services.Connections.Create(
		fmt.Sprintf("services/%s", defaultServiceConnectionURI),
		&servicenetworking.Connection{
			Network: fmt.Sprintf("projects/%s/global/networks/%s", projectID, clusterVpc.GetName()),
			ReservedPeeringRanges: []string{
				ipRange,
			},
		},
	).Do()
	if err != nil {
		return err
	}
	return nil
}

func (n *NetworkProvider) deleteServiceConnection(service *servicenetworking.Connection) error {
	n.Logger.Infof("deleting service connection %s", service.Service)
	resp, err := n.ServicesApi.Services.Connections.DeleteConnection(
		fmt.Sprintf("services/%s/connections/%s", defaultServiceConnectionURI, defaultServiceConnectionName),
		&servicenetworking.DeleteConnectionRequest{
			ConsumerNetwork: service.Network,
		}).Do()
	if err != nil {
		return err
	}
	n.Logger.Infof("delete service resp: %v", resp)
	if !resp.Done {
		return errors.New("service connection deletion in progress")
	}
	return nil
}

func (n *NetworkProvider) getAddressRange(ctx context.Context, projectID string, ipRange string) (*computepb.Address, error) {
	address, err := n.AddressApi.Get(ctx, &computepb.GetGlobalAddressRequest{
		Address: ipRange,
		Project: projectID,
	})
	if err != nil {
		var gerr *googleapi.Error
		if !errors.As(err, &gerr) {
			return nil, errorUtil.Wrap(err, "unknown error getting addresses from gcp")
		}
		if gerr.Code != http.StatusNotFound {
			return nil, errorUtil.Wrap(err, "unknown error getting addresses from gcp")
		}
	}
	return address, nil
}

func (n *NetworkProvider) createAddressRange(ctx context.Context, clusterVpc *computepb.Network, projectID string, ipRange string) error {
	n.Logger.Infof("creating address %s", ipRange)
	op, err := n.AddressApi.Insert(ctx, &computepb.InsertGlobalAddressRequest{
		Project: projectID,
		AddressResource: &computepb.Address{
			AddressType:  utils.String(computepb.Address_INTERNAL.String()),
			IpVersion:    utils.String(computepb.Address_IPV4.String()),
			Name:         &ipRange,
			Network:      clusterVpc.SelfLink,
			PrefixLength: utils.Int32(defaultIpRangeCIDRMask),
			Purpose:      utils.String(computepb.Address_VPC_PEERING.String()),
		},
	})
	if err != nil {
		return err
	}
	if err = op.Wait(ctx); err != nil {
		return err
	}
	return nil
}

func (n *NetworkProvider) deleteAddressRange(ctx context.Context, projectID string, ipRange string) error {
	n.Logger.Infof("deleting address %s", ipRange)
	op, err := n.AddressApi.Delete(ctx, &computepb.DeleteGlobalAddressRequest{
		Project: projectID,
		Address: ipRange,
	})
	if err != nil {
		return err
	}
	if err = op.Wait(ctx); err != nil {
		return err
	}
	return nil
}

func (n *NetworkProvider) getPeeringConnection(ctx context.Context, clusterVpc *computepb.Network) *computepb.NetworkPeering {
	peerings := clusterVpc.GetPeerings()
	if peerings == nil {
		return nil
	}
	for _, p := range peerings {
		if p.GetName() == defaultServiceConnectionName {
			peering := p
			return peering
		}
	}
	return nil
}

func (n *NetworkProvider) deletePeeringConnection(ctx context.Context, clusterVpc *computepb.Network, projectID string) error {
	n.Logger.Infof("deleting peering %s", defaultServiceConnectionName)
	op, err := n.NetworkApi.RemovePeering(ctx, &computepb.RemovePeeringNetworkRequest{
		Project: projectID,
		Network: clusterVpc.GetName(),
		NetworksRemovePeeringRequestResource: &computepb.NetworksRemovePeeringRequest{
			Name: utils.String(defaultServiceConnectionName),
		},
	})
	if err != nil {
		return err
	}
	if err = op.Wait(ctx); err != nil {
		return err
	}
	return nil
}
