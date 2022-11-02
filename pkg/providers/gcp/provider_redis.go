package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/annotations"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	redispb "google.golang.org/genproto/googleapis/cloud/redis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	redisInstanceNameFormat = "projects/%s/locations/%s/instances/%s"
	redisProviderName       = "gcp-memorystore"
)

type RedisProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewGCPRedisProvider(client client.Client, logger *logrus.Entry) *RedisProvider {
	return &RedisProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": redisProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (p *RedisProvider) GetName() string {
	return redisProviderName
}

func (p *RedisProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (p *RedisProvider) GetReconcileTime(r *v1alpha1.Redis) time.Duration {
	if r.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

var _ providers.RedisProvider = (*RedisProvider)(nil)

func (p *RedisProvider) CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster, croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "CreateRedis")
	logger.Infof("reconciling redis %s", r.Name)

	_, _, strategyConfig, err := p.getRedisConfig(ctx, r)
	if err != nil {
		statusMessage := "failed to retrieve redis strategy config"
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	if err := resources.CreateFinalizer(ctx, p.Client, r, DefaultFinalizer); err != nil {
		statusMessage := "failed to set finalizer"
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		statusMessage := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	clientOption := option.WithCredentialsJSON(creds.ServiceAccountJson)
	networkManager, err := NewNetworkManager(ctx, strategyConfig.ProjectID, clientOption, p.Client, logger)
	if err != nil {
		statusMessage := "failed to initialise network manager"
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	// get cidr block from _network strat map, based on tier from postgres cr
	ipRangeCidr, err := networkManager.ReconcileNetworkProviderConfig(ctx, p.ConfigManager, r.Spec.Tier, logger)
	if err != nil {
		errMsg := "failed to reconcile network provider config"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	address, err := networkManager.CreateNetworkIpRange(ctx, ipRangeCidr)
	if err != nil {
		statusMessage := "failed to create network service"
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	if address == nil || address.GetStatus() == computepb.Address_RESERVING.String() {
		statusMessage := "network ip address range creation in progress"
		return nil, croType.StatusMessage(statusMessage), nil
	}
	logger.Infof("created ip address range %s: %s/%d", address.GetName(), address.GetAddress(), address.GetPrefixLength())
	logger.Infof("creating network service connection")
	service, err := networkManager.CreateNetworkService(ctx)
	if err != nil {
		statusMessage := "failed to create network service"
		return nil, croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	if service == nil {
		statusMessage := "network service connection creation in progress"
		return nil, croType.StatusMessage(statusMessage), nil
	}
	logger.Infof("created network service connection %s", service.Service)

	//TODO implement me
	return p.createRedisCluster(ctx, r)
}

func (p *RedisProvider) createRedisCluster(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster, croType.StatusMessage, error) {
	// TODO implement me
	statusMessage := "successfully created gcp redis"
	return nil, croType.StatusMessage(statusMessage), nil
}

func (p *RedisProvider) DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "DeleteRedis")
	logger.Infof("reconciling delete redis %s", r.Name)

	_, deleteInstanceRequest, strategyConfig, err := p.getRedisConfig(ctx, r)
	if err != nil {
		statusMessage := "failed to retrieve redis strategy config"
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		statusMessage := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	isLastResource, err := resources.IsLastResource(ctx, p.Client)
	if err != nil {
		statusMessage := "failed to check if this cr is the last cr of type postgres and redis"
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	clientOption := option.WithCredentialsJSON(creds.ServiceAccountJson)
	networkManager, err := NewNetworkManager(ctx, strategyConfig.ProjectID, clientOption, p.Client, logger)
	if err != nil {
		statusMessage := "failed to initialise network manager"
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	redisClient, err := gcpiface.NewRedisAPI(ctx, clientOption)
	if err != nil {
		statusMessage := "could not initialise redis client"
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	return p.deleteRedisCluster(ctx, networkManager, redisClient, deleteInstanceRequest, strategyConfig, r, isLastResource)
}

func (p *RedisProvider) deleteRedisCluster(ctx context.Context, networkManager NetworkManager, redisClient gcpiface.RedisAPI, deleteInstanceRequest *redispb.DeleteInstanceRequest, strategyConfig *StrategyConfig, r *v1alpha1.Redis, isLastResource bool) (croType.StatusMessage, error) {
	redisInstances, err := p.getRedisInstances(ctx, redisClient, strategyConfig.ProjectID, strategyConfig.Region)
	if err != nil {
		statusMessage := "failed to retrieve redis instances"
		return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
	}
	var foundInstance *redispb.Instance
	for _, instance := range redisInstances {
		if instance.Name == deleteInstanceRequest.Name {
			foundInstance = instance
			break
		}
	}
	if foundInstance != nil {
		if foundInstance.State == redispb.Instance_DELETING {
			statusMessage := fmt.Sprintf("deletion in progress for redis instance %s", r.Name)
			return croType.StatusMessage(statusMessage), nil
		}
		_, err = redisClient.DeleteInstance(ctx, deleteInstanceRequest)
		if err != nil {
			statusMessage := fmt.Sprintf("failed to delete redis instance %s", r.Name)
			return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
		}
		statusMessage := fmt.Sprintf("delete detected, redis instance %s started", r.Name)
		return croType.StatusMessage(statusMessage), nil
	}

	// remove networking components
	if isLastResource {
		if err = networkManager.DeleteNetworkPeering(ctx); err != nil {
			statusMessage := "failed to delete cluster network peering"
			return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
		}
		if err = networkManager.DeleteNetworkService(ctx); err != nil {
			statusMessage := "failed to delete network service"
			return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
		}
		if err = networkManager.DeleteNetworkIpRange(ctx); err != nil {
			statusMessage := "failed to delete network ip range"
			return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
		}
		if exist, err := networkManager.ComponentsExist(ctx); err != nil || exist {
			if exist {
				statusMessage := "network component deletion in progress"
				return croType.StatusMessage(statusMessage), nil
			}
			statusMessage := "failed to check if components exist"
			return croType.StatusMessage(statusMessage), errorUtil.Wrap(err, statusMessage)
		}
	}

	// remove the finalizer added by the provider
	resources.RemoveFinalizer(&r.ObjectMeta, DefaultFinalizer)
	if err = p.Client.Update(ctx, r); err != nil {
		statusMessage := fmt.Sprintf("failed to update instance %s as part of finalizer reconcile", r.Name)
		return croType.StatusMessage(statusMessage), errorUtil.Wrapf(err, statusMessage)
	}
	statusMessage := fmt.Sprintf("successfully deleted redis instance %s", r.Name)
	return croType.StatusMessage(statusMessage), nil
}

func (p *RedisProvider) getRedisInstances(ctx context.Context, redisClient gcpiface.RedisAPI, projectID, region string) ([]*redispb.Instance, error) {
	request := redispb.ListInstancesRequest{
		Parent: fmt.Sprintf("projects/%v/locations/%v", projectID, region),
	}
	instances, err := redisClient.ListInstances(ctx, &request)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func (p *RedisProvider) getRedisConfig(ctx context.Context, r *v1alpha1.Redis) (*redispb.CreateInstanceRequest, *redispb.DeleteInstanceRequest, *StrategyConfig, error) {
	strategyConfig, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.RedisResourceType, r.Spec.Tier)
	if err != nil {
		errMsg := "failed to read gcp strategy config"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}
	defaultProject, err := GetProjectFromStrategyOrDefault(ctx, p.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp project"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}
	if strategyConfig.ProjectID == "" {
		p.Logger.Debugf("project not set in deployment strategy configuration, using default project %s", defaultProject)
		strategyConfig.ProjectID = defaultProject
	}
	defaultRegion, err := GetRegionFromStrategyOrDefault(ctx, p.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp region"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}
	if strategyConfig.Region == "" {
		p.Logger.Debugf("region not set in deployment strategy configuration, using default region %s", defaultRegion)
		strategyConfig.Region = defaultRegion
	}
	createInstanceRequest, deleteInstanceRequest, err := p.buildRedisConfig(r, strategyConfig)
	if err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to build redis config")
	}
	return createInstanceRequest, deleteInstanceRequest, strategyConfig, nil
}

func (p *RedisProvider) buildRedisConfig(r *v1alpha1.Redis, strategyConfig *StrategyConfig) (*redispb.CreateInstanceRequest, *redispb.DeleteInstanceRequest, error) {
	// TODO (MGDAPI-4667): create a request with all required fields
	createInstanceRequest := &redispb.CreateInstanceRequest{}

	deleteInstanceRequest := &redispb.DeleteInstanceRequest{}
	if err := json.Unmarshal(strategyConfig.DeleteStrategy, deleteInstanceRequest); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal gcp redis delete strategy")
	}
	if deleteInstanceRequest.Name == "" {
		resourceID := annotations.Get(r, ResourceIdentifierAnnotation)
		if resourceID == "" {
			errMsg := "failed to find redis instance name from annotations"
			return nil, nil, fmt.Errorf(errMsg)
		}
		deleteInstanceRequest.Name = fmt.Sprintf(redisInstanceNameFormat, strategyConfig.ProjectID, strategyConfig.Region, resourceID)
	}
	return createInstanceRequest, deleteInstanceRequest, nil
}
