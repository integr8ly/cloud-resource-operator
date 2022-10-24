package gcp

import (
	"context"
	"fmt"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const redisProviderName = "gcp-memorystore"

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

func (rp RedisProvider) GetName() string {
	return redisProviderName
}

func (rp RedisProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (rp RedisProvider) GetReconcileTime(r *v1alpha1.Redis) time.Duration {
	if r.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

var _ providers.RedisProvider = (*RedisProvider)(nil)

func (p *RedisProvider) CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster, croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "CreateRedis")
	logger.Infof("reconciling redis %s", r.Name)
	// handle provider-specific finalizer
	if err := resources.CreateFinalizer(ctx, p.Client, r, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	networkManager, err := NewNetworkManager(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), p.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	address, err := networkManager.CreateNetworkIpRange(ctx)
	if err != nil {
		msg := "failed to create network service"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	if address == nil || address.GetStatus() == computepb.Address_RESERVING.String() {
		return nil, croType.StatusMessage("network ip address range creation in progress"), nil
	}
	logger.Infof("created ip address range %s: %s/%d", address.GetName(), address.GetAddress(), address.GetPrefixLength())

	logger.Infof("creating network service connection")
	service, err := networkManager.CreateNetworkService(ctx)
	if err != nil {
		msg := "failed to create network service"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	if service == nil {
		return nil, croType.StatusMessage("network service connection creation in progress"), nil
	}
	logger.Infof("created network service connection %s", service.Service)

	// TODO implement me
	return &providers.RedisCluster{
		DeploymentDetails: &providers.RedisDeploymentDetails{
			URI:  "test",
			Port: 1234,
		},
	}, "", nil
}

func (p *RedisProvider) DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "DeleteRedis")
	logger.Infof("reconciling delete redis %s", r.Name)

	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	// TODO implement me
	isLastResource, err := resources.IsLastResource(ctx, p.Client)
	if err != nil {
		errMsg := "failed to check if this cr is the last cr of type postgres and redis"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	networkManager, err := NewNetworkManager(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), p.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	return p.deleteRedisCluster(ctx, networkManager, r, isLastResource)
}

// TODO
func (p *RedisProvider) deleteRedisCluster(ctx context.Context, networkManager NetworkManager, r *v1alpha1.Redis, isLastResource bool) (croType.StatusMessage, error) {

	// TODO delete GCP redis cluster

	// remove networking components
	if isLastResource {
		if err := networkManager.DeleteNetworkPeering(ctx); err != nil {
			msg := "failed to delete cluster network peering"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if err := networkManager.DeleteNetworkService(ctx); err != nil {
			msg := "failed to delete cluster network peering"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if err := networkManager.DeleteNetworkIpRange(ctx); err != nil {
			msg := "failed to delete aws networking"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if exist, err := networkManager.ComponentsExist(ctx); err != nil || exist {
			if exist {
				return croType.StatusMessage("network component deletion in progress"), nil
			}
			msg := "failed to check if components exist"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
	}

	// remove the finalizer added by the provider
	resources.RemoveFinalizer(&r.ObjectMeta, DefaultFinalizer)
	if err := p.Client.Update(ctx, r); err != nil {
		errMsg := "failed to update instance as part of finalizer reconcile"
		return croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}
	return croType.StatusEmpty, nil
}
