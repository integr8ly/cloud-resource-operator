package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
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
	defaultServicesFormat          = "services/%s"
	defaultServiceConnectionFormat = defaultServicesFormat + "/connections/%s"
	defaultNetworksFormat          = "projects/%s/global/networks/%s"
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
	Client      client.Client
	NetworkApi  gcpiface.NetworksAPI
	ServicesApi gcpiface.ServicesAPI
	AddressApi  gcpiface.AddressAPI
	Logger      *logrus.Entry
	ProjectID   string
}

// initialises the three required clients
func NewNetworkManager(ctx context.Context, projectID string, opt option.ClientOption, client client.Client, logger *logrus.Entry) (*NetworkProvider, error) {
	networksApi, err := gcpiface.NewNetworksAPI(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise network client")
	}
	servicesApi, err := gcpiface.NewServicesAPI(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise servicenetworking client")
	}
	addressApi, err := gcpiface.NewAddressAPI(ctx, opt)
	if err != nil {
		return nil, errorUtil.Wrap(err, "Failed to initialise addresses client")
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &NetworkProvider{
		Client:      client,
		NetworkApi:  networksApi,
		ServicesApi: servicesApi,
		AddressApi:  addressApi,
		Logger:      logger.WithField("provider", "gcp_network_provider"),
		ProjectID:   projectID,
	}, nil
}

func (n *NetworkProvider) CreateNetworkIpRange(ctx context.Context) (*computepb.Address, error) {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	address, err := n.getAddressRange(ctx, ipRange)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	// if it does not exist, create it
	if address == nil {
		err := n.createAddressRange(ctx, clusterVpc, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to create ip address range")
		}
		// check if address exists
		address, err = n.getAddressRange(ctx, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
		}
	}
	return address, nil
}

// Creates the network service connection and will return the service if it has been created successfully
// This automatically creates a peering connection to the clusterVpc named after the service connection
func (n *NetworkProvider) CreateNetworkService(ctx context.Context) (*servicenetworking.Connection, error) {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	service, err := n.getServiceConnection(clusterVpc)
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
		address, err := n.getAddressRange(ctx, ipRange)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to retrieve ip address range")
		}
		// if the ip range is present, and is ready for use
		// possible states for address are RESERVING, RESERVED, IN_USE
		if address == nil || address.GetStatus() == computepb.Address_RESERVING.String() {
			return nil, errors.New("ip address range does not exist or is pending creation")
		}
		if address != nil && address.GetStatus() == computepb.Address_RESERVED.String() {
			err = n.createServiceConnection(clusterVpc, ipRange)
			if err != nil {
				return nil, errorUtil.Wrap(err, "failed to create service connection")
			}
			// check if service exists
			service, err = n.getServiceConnection(clusterVpc)
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
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	peering := n.getPeeringConnection(ctx, clusterVpc)
	// if it exists, delete it
	if peering != nil {
		err = n.deletePeeringConnection(ctx, clusterVpc)
		if err != nil {
			return errorUtil.Wrap(err, "failed to delete peering connection")
		}
	}
	return nil
}

// This deletes the network service connection, but can get stuck if peering
// has not been removed
func (n *NetworkProvider) DeleteNetworkService(ctx context.Context) error {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
	if err != nil {
		return errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	service, err := n.getServiceConnection(clusterVpc)
	if err != nil {
		return err
	}
	// if the service exists, delete it
	if service != nil {
		err := n.deleteServiceConnection(service)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *NetworkProvider) DeleteNetworkIpRange(ctx context.Context) error {
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	address, err := n.getAddressRange(ctx, ipRange)
	if err != nil {
		return errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	// if the address exists, delete it
	if address != nil {
		clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
		if err != nil {
			return errorUtil.Wrap(err, "failed to get cluster vpc")
		}
		service, err := n.getServiceConnection(clusterVpc)
		if err != nil {
			return err
		}
		if service != nil && service.ReservedPeeringRanges[0] == address.GetName() {
			return errors.New("failed to delete ip address range, service connection still present")
		}
		err = n.deleteAddressRange(ctx, ipRange)
		if err != nil {
			return errorUtil.Wrap(err, "failed to delete ip address range")
		}
	}
	return nil
}

func (n *NetworkProvider) ComponentsExist(ctx context.Context) (bool, error) {
	clusterVpc, err := getClusterVpc(ctx, n.Client, n.NetworkApi, n.ProjectID, n.Logger)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to get cluster vpc")
	}
	// build ip address range name
	ipRange, err := resources.BuildInfraName(ctx, n.Client, defaultIpRangePostfix, defaultGcpIdentifierLength)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to build ip address range infra name")
	}
	address, err := n.getAddressRange(ctx, ipRange)
	if err != nil {
		return false, errorUtil.Wrap(err, "failed to retrieve ip address range")
	}
	if address != nil {
		n.Logger.Infof("ip address range %s deletion in progress", address.GetName())
		return true, nil
	}
	service, err := n.getServiceConnection(clusterVpc)
	if err != nil {
		return false, err
	}
	if service != nil {
		n.Logger.Infof("service connection %s deletion in progress", service.Service)
		return true, nil
	}
	return false, nil
}

func (n *NetworkProvider) getServiceConnection(clusterVpc *computepb.Network) (*servicenetworking.Connection, error) {
	resp, err := n.ServicesApi.ConnectionsList(clusterVpc, n.ProjectID, fmt.Sprintf(defaultServicesFormat, defaultServiceConnectionURI))
	if err != nil {
		return nil, err
	}
	if len(resp.Connections) == 0 {
		return nil, nil
	}
	return resp.Connections[0], nil
}

func (n *NetworkProvider) createServiceConnection(clusterVpc *computepb.Network, ipRange string) error {
	n.Logger.Infof("creating service connection %s", defaultServiceConnectionName)
	_, err := n.ServicesApi.ConnectionsCreate(
		fmt.Sprintf(defaultServicesFormat, defaultServiceConnectionURI),
		&servicenetworking.Connection{
			Network: fmt.Sprintf(defaultNetworksFormat, n.ProjectID, clusterVpc.GetName()),
			ReservedPeeringRanges: []string{
				ipRange,
			},
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func (n *NetworkProvider) deleteServiceConnection(service *servicenetworking.Connection) error {
	n.Logger.Infof("deleting service connection %s", service.Service)
	resp, err := n.ServicesApi.ConnectionsDelete(
		fmt.Sprintf(defaultServiceConnectionFormat, defaultServiceConnectionURI, defaultServiceConnectionName),
		&servicenetworking.DeleteConnectionRequest{
			ConsumerNetwork: service.Network,
		})
	if err != nil {
		return err
	}
	if !resp.Done {
		return errors.New("service connection deletion in progress")
	}
	return nil
}

func (n *NetworkProvider) getAddressRange(ctx context.Context, ipRange string) (*computepb.Address, error) {
	address, err := n.AddressApi.Get(ctx, &computepb.GetGlobalAddressRequest{
		Address: ipRange,
		Project: n.ProjectID,
	})
	if err != nil {
		var gerr *googleapi.Error
		if !errors.As(err, &gerr) {
			return nil, errorUtil.Wrap(err, "unknown error getting addresses from gcp")
		}
		if gerr.Code != http.StatusNotFound {
			return nil, errorUtil.Wrap(err, fmt.Sprintf("unexpected error %d getting addresses from gcp", gerr.Code))
		}
	}
	return address, nil
}

func (n *NetworkProvider) createAddressRange(ctx context.Context, clusterVpc *computepb.Network, ipRange string) error {
	n.Logger.Infof("creating address %s", ipRange)
	return n.AddressApi.Insert(ctx, &computepb.InsertGlobalAddressRequest{
		Project: n.ProjectID,
		AddressResource: &computepb.Address{
			AddressType:  utils.String(computepb.Address_INTERNAL.String()),
			IpVersion:    utils.String(computepb.Address_IPV4.String()),
			Name:         &ipRange,
			Network:      clusterVpc.SelfLink,
			PrefixLength: utils.Int32(defaultIpRangeCIDRMask),
			Purpose:      utils.String(computepb.Address_VPC_PEERING.String()),
		},
	})
}

func (n *NetworkProvider) deleteAddressRange(ctx context.Context, ipRange string) error {
	n.Logger.Infof("deleting address %s", ipRange)
	return n.AddressApi.Delete(ctx, &computepb.DeleteGlobalAddressRequest{
		Project: n.ProjectID,
		Address: ipRange,
	})
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

func (n *NetworkProvider) deletePeeringConnection(ctx context.Context, clusterVpc *computepb.Network) error {
	n.Logger.Infof("deleting peering %s", defaultServiceConnectionName)
	return n.NetworkApi.RemovePeering(ctx, &computepb.RemovePeeringNetworkRequest{
		Project: n.ProjectID,
		Network: clusterVpc.GetName(),
		NetworksRemovePeeringRequestResource: &computepb.NetworksRemovePeeringRequest{
			Name: utils.String(defaultServiceConnectionName),
		},
	})
}
