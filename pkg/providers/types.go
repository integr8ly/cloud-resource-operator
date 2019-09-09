package providers

import (
	"context"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceType string

const (
	ManagedDeploymentType = "managed"

	AWSDeploymentStrategy       = "aws"
	OpenShiftDeploymentStrategy = "openshift"

	BlobStorageResourceType    ResourceType = "blobstorage"
	PostgresResourceType       ResourceType = "postgres"
	RedisResourceType          ResourceType = "redis"
	SMTPCredentialResourceType ResourceType = "smtpcredential"
)

type BlobStorageInstance struct {
	DeploymentDetails BlobStorageDeploymentDetails
}

type BlobStorageDeploymentDetails interface {
	Data() map[string][]byte
}

type BlobStorageProvider interface {
	GetName() string
	SupportsStrategy(s string) bool
	CreateStorage(ctx context.Context, client client.Client, bs *v1alpha1.BlobStorage) (*BlobStorageInstance, error)
	DeleteStorage(ctx context.Context, client client.Client, bs *v1alpha1.BlobStorage) error
}

type RedisProvider interface {
	GetName() string
	SupportsStrategy(s string) bool
	CreateRedis(ctx context.Context) error
	DeleteRedis(ctx context.Context) error
}
