package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/apis"
	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"google.golang.org/api/googleapi"
	servicenetworking "google.golang.org/api/servicenetworking/v1"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utils "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gcpTestNetworkName string = "gcp-test-cluster-network"
	gcpTestIpRangeName string = "gcptestclusteriprange"
	gcpTestProjectId   string = "gcp-test-project"
)

func buildMockNetworkManager() *NetworkManagerMock {
	return &NetworkManagerMock{
		DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
			return nil
		},
		DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
			return nil
		},
		DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
			return nil
		},
		ComponentsExistFunc: func(contextMoqParam context.Context) (bool, error) {
			return false, nil
		},
	}
}

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := multierr.Append(
		corev1.AddToScheme(scheme),
		apis.AddToScheme(scheme))
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestGcpInfrastructure() *v1.Infrastructure {
	return &v1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: v1.InfrastructureStatus{
			InfrastructureName: "gcp-test-cluster",
			Platform:           v1.GCPPlatformType,
			PlatformStatus: &v1.PlatformStatus{
				Type: v1.GCPPlatformType,
				GCP: &v1.GCPPlatformStatus{
					ProjectID: gcpTestProjectId,
					Region:    "eu-west2",
				},
			},
		},
	}
}

func buildTestInvalidInfrastructure() *v1.Infrastructure {
	return &v1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: v1.InfrastructureStatus{
			InfrastructureName: "gcp-test-cluster",
			Platform:           v1.GCPPlatformType,
			PlatformStatus: &v1.PlatformStatus{
				Type: v1.AWSPlatformType,
			},
		},
	}
}

func buildValidGcpListNetworks(req *computepb.ListNetworksRequest) ([]*computepb.Network, error) {
	return testBuildNetwork(req, buildValidGcpNetwork)
}

func buildValidEmptyGcpListNetworksPeering(req *computepb.ListNetworksRequest) ([]*computepb.Network, error) {
	return testBuildNetwork(req, buildEmptyGcpNetworkPeering)
}

func buildValidGcpListNetworksPeering(req *computepb.ListNetworksRequest) ([]*computepb.Network, error) {
	return testBuildNetwork(req, buildValidGcpNetworkPeering)
}

func testBuildNetwork(req *computepb.ListNetworksRequest, buildNetwork func(string) *computepb.Network) ([]*computepb.Network, error) {
	clusterID := retrieveTestClusterId(resources.SafeStringDereference(req.Filter))
	return []*computepb.Network{
		buildNetwork(clusterID),
	}, nil
}

func retrieveTestClusterId(filter string) string {
	return strings.TrimSuffix(strings.TrimPrefix(filter, "name = \""), "-*\"")
}

func buildValidGcpNetwork(clusterID string) *computepb.Network {
	return &computepb.Network{
		Name: utils.String(fmt.Sprintf("%s-network", clusterID)),
		Subnetworks: []string{
			fmt.Sprintf("%s-master-subnet", clusterID),
			fmt.Sprintf("%s-worker-subnet", clusterID),
		},
	}
}

func buildEmptyGcpNetworkPeering(clusterID string) *computepb.Network {
	net := buildValidGcpNetwork(clusterID)
	net.Peerings = []*computepb.NetworkPeering{}
	return net
}

func buildValidGcpNetworkPeering(clusterID string) *computepb.Network {
	net := buildEmptyGcpNetworkPeering(clusterID)
	net.Peerings = append(net.Peerings, &computepb.NetworkPeering{
		Name: utils.String(defaultServiceConnectionName),
	})
	return net
}

func buildValidGcpAddressRange(name string) *computepb.Address {
	return buildValidGcpAddressRangeStatus(name, computepb.Address_RESERVED.String())
}

func buildValidGcpAddressRangeStatus(name string, status string) *computepb.Address {
	return &computepb.Address{
		Name:    utils.String(name),
		Purpose: utils.String(computepb.Address_VPC_PEERING.String()),
		Status:  utils.String(status),
	}
}

func buildValidListConnectionsResponse(name string, projectID string, parent string) *servicenetworking.ListConnectionsResponse {
	return &servicenetworking.ListConnectionsResponse{
		Connections: []*servicenetworking.Connection{
			buildValidConnection(name, projectID, parent),
		},
	}
}

func buildValidConnection(name string, projectID string, parent string) *servicenetworking.Connection {
	return &servicenetworking.Connection{
		Network: fmt.Sprintf("projects/%s/global/networks/%s", projectID, name),
		Peering: defaultServiceConnectionName,
		Service: parent,
		ReservedPeeringRanges: []string{
			gcpTestIpRangeName,
		},
	}
}

func TestNetworkProvider_CreateNetworkIpRange(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client     client.Client
		NetworkApi gcpiface.NetworksAPI
		AddressApi gcpiface.AddressAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *computepb.Address
		wantErr bool
	}{
		{
			name: "create ip range created",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFnTwo = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
			},
			want:    buildValidGcpAddressRange(gcpTestIpRangeName),
			wantErr: false,
		},
		{
			name: "create ip range in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(nil),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "create ip range exists",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
			},
			want:    buildValidGcpAddressRange(gcpTestIpRangeName),
			wantErr: false,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "googleapi error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, &googleapi.Error{
							Code: http.StatusBadGateway,
						}
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "unknown error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, errors.New("failed to get")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error creating ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.InsertFn = func(*computepb.InsertGlobalAddressRequest) error {
						return errors.New("failed to insert")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "googleapi error retrieving ip address range - post creation",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFnTwo = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, &googleapi.Error{
							Code: http.StatusBadGateway,
						}
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "unknown error retrieving ip address range - post creation",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFnTwo = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, errors.New("failed to get")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:     logrus.NewEntry(logrus.StandardLogger()),
				Client:     tt.fields.Client,
				NetworkApi: tt.fields.NetworkApi,
				AddressApi: tt.fields.AddressApi,
			}
			got, err := n.CreateNetworkIpRange(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNetworkIpRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateNetworkIpRange() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_CreateNetworkService(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client      client.Client
		NetworkApi  gcpiface.NetworksAPI
		AddressApi  gcpiface.AddressAPI
		ServicesApi gcpiface.ServicesAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *servicenetworking.Connection
		wantErr bool
	}{
		{
			name: "create service network created",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(req *computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFnTwo = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
				}),
			},
			want:    buildValidConnection(gcpTestNetworkName, gcpTestProjectId, fmt.Sprintf("services/%s", defaultServiceConnectionURI)),
			wantErr: false,
		},
		{
			name: "create service network in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(req *computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "create service network exists",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(nil),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
				}),
			},
			want:    buildValidConnection(gcpTestNetworkName, gcpTestProjectId, fmt.Sprintf("services/%s", defaultServiceConnectionURI)),
			wantErr: false,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error retrieving service connections",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(*computepb.Network, string, string) (*servicenetworking.ListConnectionsResponse, error) {
						return nil, errors.New("failed to list")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "googleapi error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, &googleapi.Error{
							Code: http.StatusBadGateway,
						}
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "unknown error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, errors.New("failed to get")
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error ip address range does not exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi:  gcpiface.GetMockAddressClient(nil),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error ip address range creation in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(req *computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRangeStatus(gcpTestIpRangeName, computepb.Address_RESERVING.String()), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error retrieving service connections - post creation",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(req *computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFnTwo = func(*computepb.Network, string, string) (*servicenetworking.ListConnectionsResponse, error) {
						return nil, errors.New("failed to list")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:      logrus.NewEntry(logrus.StandardLogger()),
				Client:      tt.fields.Client,
				NetworkApi:  tt.fields.NetworkApi,
				AddressApi:  tt.fields.AddressApi,
				ServicesApi: tt.fields.ServicesApi,
			}
			got, err := n.CreateNetworkService(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNetworkService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateNetworkService() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkProvider_DeleteNetworkPeering(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client     client.Client
		NetworkApi gcpiface.NetworksAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "delete peering",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworksPeering
				}),
			},
			wantErr: false,
		},
		{
			name: "delete peering does not exist - nil",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
			},
			wantErr: false,
		},
		{
			name: "delete peering does not exist - empty",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidEmptyGcpListNetworksPeering
				}),
			},
			wantErr: false,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			wantErr: true,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			wantErr: true,
		},
		{
			name: "error deleting peering",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworksPeering
					networksClient.RemovePeeringFn = func(req *computepb.RemovePeeringNetworkRequest) error {
						return errors.New("failed remove peering")
					}
				}),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:     logrus.NewEntry(logrus.StandardLogger()),
				Client:     tt.fields.Client,
				NetworkApi: tt.fields.NetworkApi,
			}
			err := n.DeleteNetworkPeering(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetworkPeering() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestNetworkProvider_DeleteNetworkService(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client      client.Client
		NetworkApi  gcpiface.NetworksAPI
		ServicesApi gcpiface.ServicesAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "delete service connection",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
				}),
			},
			wantErr: false,
		},
		{
			name: "delete service connection does not exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			wantErr: false,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			wantErr: true,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			wantErr: true,
		},
		{
			name: "error retrieving service connections",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(*computepb.Network, string, string) (*servicenetworking.ListConnectionsResponse, error) {
						return nil, errors.New("failed to list")
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error deleting service connections",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
					servicesClient.ConnectionsDeleteFn = func(string, *servicenetworking.DeleteConnectionRequest) (*servicenetworking.Operation, error) {
						return nil, errors.New("failed to delete")
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error deleting service connection, response in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
					servicesClient.Done = false
				}),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:      logrus.NewEntry(logrus.StandardLogger()),
				Client:      tt.fields.Client,
				NetworkApi:  tt.fields.NetworkApi,
				ServicesApi: tt.fields.ServicesApi,
			}
			err := n.DeleteNetworkService(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetworkService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestNetworkProvider_DeleteNetworkIpRange(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client      client.Client
		NetworkApi  gcpiface.NetworksAPI
		ServicesApi gcpiface.ServicesAPI
		AddressApi  gcpiface.AddressAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "delete ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			wantErr: false,
		},
		{
			name: "delete ip address range does not exist",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi:  gcpiface.GetMockAddressClient(nil),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			wantErr: false,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			wantErr: true,
		},
		{
			name: "googleapi error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, &googleapi.Error{
							Code: http.StatusBadGateway,
						}
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "unknown error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, errors.New("failed to get")
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error retrieving service connections",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(*computepb.Network, string, string) (*servicenetworking.ListConnectionsResponse, error) {
						return nil, errors.New("failed to list")
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error service connection still present",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID string, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "error deleting ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
					addressClient.DeleteFn = func(*computepb.DeleteGlobalAddressRequest) error {
						return errors.New("failed to delete")
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:      logrus.NewEntry(logrus.StandardLogger()),
				Client:      tt.fields.Client,
				NetworkApi:  tt.fields.NetworkApi,
				AddressApi:  tt.fields.AddressApi,
				ServicesApi: tt.fields.ServicesApi,
			}
			err := n.DeleteNetworkIpRange(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetworkIpRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestNetworkProvider_ComponentsExist(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client      client.Client
		NetworkApi  gcpiface.NetworksAPI
		ServicesApi gcpiface.ServicesAPI
		AddressApi  gcpiface.AddressAPI
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "ip address range exists",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),

				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(req *computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return buildValidGcpAddressRange(gcpTestIpRangeName), nil
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "service connection exists",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(nil),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(clusterVpc *computepb.Network, projectID, parent string) (*servicenetworking.ListConnectionsResponse, error) {
						return buildValidListConnectionsResponse(resources.SafeStringDereference(clusterVpc.Name), projectID, parent), nil
					}
				}),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "error no cluster vpc present",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "error infrastructure does not contain gcp project",
			fields: fields{
				Client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInvalidInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(nil),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "error retrieving service connections",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(nil),
				ServicesApi: gcpiface.GetMockServicesClient(func(servicesClient *gcpiface.MockServicesClient) {
					servicesClient.ConnectionsListFn = func(*computepb.Network, string, string) (*servicenetworking.ListConnectionsResponse, error) {
						return nil, errors.New("failed to list")
					}
				}),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "googleapi error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, &googleapi.Error{
							Code: http.StatusBadGateway,
						}
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "unknown error retrieving ip address range",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure()),
				NetworkApi: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
				AddressApi: gcpiface.GetMockAddressClient(func(addressClient *gcpiface.MockAddressClient) {
					addressClient.GetFn = func(*computepb.GetGlobalAddressRequest) (*computepb.Address, error) {
						return nil, errors.New("failed to get")
					}
				}),
				ServicesApi: gcpiface.GetMockServicesClient(nil),
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				Logger:      logrus.NewEntry(logrus.StandardLogger()),
				Client:      tt.fields.Client,
				NetworkApi:  tt.fields.NetworkApi,
				AddressApi:  tt.fields.AddressApi,
				ServicesApi: tt.fields.ServicesApi,
			}
			got, err := n.ComponentsExist(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComponentsExist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ComponentsExist() got = %v, want %v", got, tt.want)
			}
		})
	}
}
