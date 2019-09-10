package providers

import (
	"context"
	"github.com/aws/aws-sdk-go/service/elasticache"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
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
	CreateStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (*BlobStorageInstance, error)
	DeleteStorage(ctx context.Context, bs *v1alpha1.BlobStorage) error
}

type RedisCluster struct {
	DeploymentDetails RedisDeploymentDetails
}

type RedisDeploymentDetails interface {
	Data() *elasticache.Endpoint
}

type RedisProvider interface {
	GetName() string
	SupportsStrategy(s string) bool
	CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*RedisCluster, error)
	DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (*RedisCluster, error)
}
