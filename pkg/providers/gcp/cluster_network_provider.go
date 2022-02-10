package gcp

import (
	compute "cloud.google.com/go/compute/apiv1"
	"context"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

func CreateNetwork() (croType.StatusMessage, error) {
	ctx := context.Background()

	networkClient, err := compute.NewNetworksRESTClient(ctx)
	if err != nil {
		return "Could not create network client", err
	}

	op, err := networkClient.Insert(ctx, computepb.InsertNetworkRequest{})

	if err != nil {
		return "Could not create network", err
	}
}
