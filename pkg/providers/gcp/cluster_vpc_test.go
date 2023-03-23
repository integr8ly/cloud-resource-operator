package gcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_getClusterVpc(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx           context.Context
		client        client.Client
		networkClient gcpiface.NetworksAPI
	}
	tests := []struct {
		name    string
		args    args
		want    *computepb.Network
		wantErr bool
	}{
		{
			name: "successfully get cluster vpc",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				networkClient: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildValidGcpListNetworks
				}),
			},
			want:    buildValidGcpNetwork(gcpTestClusterName),
			wantErr: false,
		},
		{
			name: "error getting cluster id",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error getting cluster vpc",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				networkClient: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = func(lnr *computepb.ListNetworksRequest) ([]*computepb.Network, error) {
						return nil, errors.New("failed to list networks")
					}
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error getting cluster vpc, no networks listed",
			args: args{
				client:        moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				networkClient: gcpiface.GetMockNetworksClient(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error getting cluster vpc, multiple networks with cluster ID",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				networkClient: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildInvalidGcpListNetworksMultiple
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error getting cluster vpc, insufficient subnets",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				networkClient: gcpiface.GetMockNetworksClient(func(networksClient *gcpiface.MockNetworksClient) {
					networksClient.ListFn = buildInvalidGcpListNetworksOneSubnet
				}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getClusterVpc(tt.args.ctx, tt.args.client, tt.args.networkClient, gcpTestProjectId, logrus.NewEntry(logrus.StandardLogger()))
			if (err != nil) != tt.wantErr {
				t.Errorf("getClusterVpc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getClusterVpc() = %v, want %v", got, tt.want)
			}
		})
	}
}
