package gcp

import (
	redis "cloud.google.com/go/redis/apiv1"
	"context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/annotations"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	redis2 "google.golang.org/genproto/googleapis/cloud/redis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	redisProviderName       = "gcp-memory-store"
	defaultRegion           = "us-central1"
	defaultEngineVersion    = "5.0.6"
	defaultNumCacheClusters = 2
)

var _ providers.RedisProvider = (*RedisProvider)(nil)

type RedisProvider struct {
	Client client.Client
	Logger *logrus.Entry
}

func NewGCPRedisProvider(client client.Client, logger *logrus.Entry) *RedisProvider {
	return &RedisProvider{
		Client: client,
		Logger: logger.WithFields(logrus.Fields{"provider": redisProviderName}),
	}
}

func (p RedisProvider) GetName() string {
	return redisProviderName
}

func (p RedisProvider) SupportsStrategy(s string) bool {
	return s == providers.GCPDeploymentStrategy
}

func (p RedisProvider) GetReconcileTime(r *v1alpha1.Redis) time.Duration {
	if r.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (p RedisProvider) CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster,
	croType.StatusMessage, error) {

	logger := p.Logger.WithField("action", "CreateRedis")
	logger.Infof("reconciling redes %s", r.Name)

	if err := resources.CreateFinalizer(ctx, p.Client, r, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	backgroundContext := context.Background()
	redisClient, err := redis.NewCloudRedisClient(backgroundContext)
	if err != nil {
		return nil, croType.StatusMessage("Could not create redis client"), err
	}

	createInstanceRequest, _ := p.getMemoryStoreConfig(backgroundContext, r)

	return p.createMemoryStoreCluster(backgroundContext, r, redisClient, &createInstanceRequest)
}

func (p RedisProvider) createMemoryStoreCluster(ctx context.Context, r *v1alpha1.Redis,
	redisClient *redis.CloudRedisClient,
	createInstanceRequest *redis2.CreateInstanceRequest) (*providers.RedisCluster,
	croType.StatusMessage, error) {

	logger := p.Logger.WithField("action", "CreateMemoryStoreCLuster")
	logger.Infof("Retreiving all redis instances %s", r.Name)
	redisInstances, err := getRedisInstances(ctx, redisClient)
	if err != nil {
		return nil, croType.StatusMessage("Could not get redis instances"), err
	}

	var foundInstance *redis2.Instance
	for _, i := range redisInstances {
		if i.Name == fmt.Sprintf("%v/instances/%v", createInstanceRequest.Parent, createInstanceRequest.Instance.Name) {
			foundInstance = i
			break
		}
	}

	if foundInstance == nil {
		logger.Infof("Could not find instnace on gcp %s", r.Name)
		if annotations.Has(r, ResourceIdentifierAnnotation) {
			errMsg := fmt.Sprintf("Redis CR %s in %s namespace has %s annotation with value %s, "+
				"but no corresponding Memorystore cluster was found",
				r.Name, r.Namespace, ResourceIdentifierAnnotation, r.ObjectMeta.Annotations[ResourceIdentifierAnnotation])
			return nil, croType.StatusMessage(errMsg), fmt.Errorf(errMsg)
		}

		if _, err := redisClient.CreateInstance(ctx, createInstanceRequest); err != nil {
			errMsg := fmt.Sprintf("error creating memorystore cluster %s", err)
			return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}

		annotations.Add(r, ResourceIdentifierAnnotation, createInstanceRequest.InstanceId)
		if err := p.Client.Update(ctx, r); err != nil {
			return nil, croType.StatusMessage("failed to add annotation"), err
		}
		return nil, "started memorystore provision", nil
	}

	if foundInstance.State != redis2.Instance_READY {
		return nil, croType.StatusMessage(fmt.Sprintf("createReplicationGroup() in progress, "+
			"current gcp memorystore status is %s", foundInstance.State.String())), nil
	}

	rdd := &providers.RedisDeploymentDetails{
		URI:  foundInstance.Host,
		Port: int64(foundInstance.Port),
	}

	return &providers.RedisCluster{DeploymentDetails: rdd},
		croType.StatusMessage(fmt.Sprintf("successfully created, gcp memorystore status is %s",
			foundInstance.State)),
		nil
}

func getRedisInstances(ctx context.Context, redisClient *redis.CloudRedisClient) ([]*redis2.Instance, error) {
	request := redis2.ListInstancesRequest{
		Parent: fmt.Sprintf("projects/%v/locations/%v", projectID, defaultRegion),
	}
	var instances []*redis2.Instance

	it := redisClient.ListInstances(ctx, &request)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		_ = resp
		instances = append(instances, resp)
	}

	return instances, nil
}

func (p RedisProvider) getMemoryStoreConfig(ctx context.Context, r *v1alpha1.Redis) (redis2.CreateInstanceRequest,
	redis2.DeleteInstanceRequest) {

	return redis2.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%v/locations/%v", projectID, defaultRegion),
		InstanceId: r.Name,
		Instance: &redis2.Instance{
			Name:         r.Name,
			MemorySizeGb: 10,
			Tier:         redis2.Instance_BASIC,
		},
	},
	redis2.DeleteInstanceRequest{
		Name:     fmt.Sprintf("projects/%v/locations/%v/instances/%v", projectID, defaultRegion, r.Name),
	}

}

func (p RedisProvider) DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (croType.StatusMessage, error) {

	// --> TODO: look for nstance matching delete config
	// --> TODO: if found and availbible then delete
	// --> TODO: else return delete in progress

	backgroundContext := context.Background()
	redisClient, err := redis.NewCloudRedisClient(backgroundContext)
	if err != nil {
		return croType.StatusMessage("Could not create redis client"), err
	}

	_, deleteInstanceRequest := p.getMemoryStoreConfig(backgroundContext, r)

	return p.deleteMemoryStoreCluster(backgroundContext, r, redisClient, &deleteInstanceRequest)

}

func(p RedisProvider) deleteMemoryStoreCluster(ctx context.Context, r *v1alpha1.Redis,
	redisClient *redis.CloudRedisClient,
	deleteInstanceRequest *redis2.DeleteInstanceRequest) (croType.StatusMessage, error) {

	logger := p.Logger.WithField("action", "DeleteMemoryStoreCluster")
	logger.Infof("Retreiving all redis instances %s", r.Name)
	redisInstances, err := getRedisInstances(ctx, redisClient)
	if err != nil {
		return croType.StatusMessage("Could not get redis instances"), err
	}

	var foundInstance *redis2.Instance
	for _, i := range redisInstances {
		if i.Name == deleteInstanceRequest.Name {
			foundInstance = i
			break
		}
	}

	if foundInstance != nil {
		if foundInstance.State != redis2.Instance_READY {
			return croType.StatusMessage("Instance is already deleting"), nil
		}

		_, err = redisClient.DeleteInstance(ctx, deleteInstanceRequest)
		if err != nil {
			return croType.StatusMessage("Error while calling to delete the instance"), nil
		}

		return "delete detected, deleteRedisInstance started", nil
	}

	resources.RemoveFinalizer(&r.ObjectMeta, DefaultFinalizer)
	if err := p.Client.Update(ctx, r); err != nil {
		errMsg := "failed to update instance as part of finalizer reconcile"
		return croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}
	return croType.StatusEmpty, nil
}
