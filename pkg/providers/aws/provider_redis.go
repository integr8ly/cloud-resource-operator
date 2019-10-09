package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	errorUtil "github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	redisProviderName = "aws-elasticache"
	redisNameLen      = 40

	defaultCacheNodeType      = "cache.t2.micro"
	defaultEngineVersion      = "3.2.10"
	defaultDescription        = "A Redis replication group"
	defaultNumCacheClusters   = 2
	defaultSnapshotRetention  = 30
	NoFinalSnapshotIdentifier = ""
)

// AWS Redis Provider implementation for AWS Elasticache
type AWSRedisProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
	CacheSvc          elasticacheiface.ElastiCacheAPI
}

func NewAWSRedisProvider(client client.Client, logger *logrus.Entry) *AWSRedisProvider {
	return &AWSRedisProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": redisProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigMapConfigManager(client),
	}
}

func (p *AWSRedisProvider) GetName() string {
	return redisProviderName
}

func (p *AWSRedisProvider) SupportsStrategy(d string) bool {
	return d == providers.AWSDeploymentStrategy
}

// CreateRedis Create an Elasticache Replication Group from strategy config
func (p *AWSRedisProvider) CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster, v1alpha1.StatusMessage, error) {
	// handle provider-specific finalizer
	if err := resources.CreateFinalizer(ctx, p.Client, r, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	// info about the redis cluster to be created
	redisCreateConfig, _, stratCfg, err := p.getRedisConfig(ctx, r)
	if err != nil {
		errMsg := fmt.Sprintf("failed to retrieve aws elasticache cluster config %s", r.Name)
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// create the credentials to be used by the aws resource providers, not to be used by end-user
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		msg := "failed to reconcile elasticache credentials"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// setup aws redis cluster sdk session
	cacheSvc := createElasticacheService(stratCfg, providerCreds)

	// create the aws redis cluster
	return p.createElasticacheCluster(ctx, r, cacheSvc, redisConfig)
}

func createElasticacheService(stratCfg *StrategyConfig, providerCreds *AWSCredentials) elasticacheiface.ElastiCacheAPI {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	}))
	return elasticache.New(sess)
}

func (p *AWSRedisProvider) createElasticacheCluster(ctx context.Context, r *v1alpha1.Redis, cacheSvc elasticacheiface.ElastiCacheAPI, elasticacheConfig *elasticache.CreateReplicationGroupInput) (*providers.RedisCluster, v1alpha1.StatusMessage, error) {
	// the aws access key can sometimes still not be registered in aws on first try, so loop
	rgs, err := getReplicationGroups(cacheSvc)
	if err != nil {
		// return nil error so this function can be requeueed
		errMsg := "error getting replication groups"
		logrus.Info(errMsg, err)
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// verify and build elasticache create config
	if err := p.buildElasticacheCreateStrategy(ctx, r, elasticacheConfig); err != nil {
		errMsg := "failed to build and verify aws elasticache create strategy"
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// check if the cluster has already been created
	var foundCache *elasticache.ReplicationGroup
	for _, c := range rgs {
		if *c.ReplicationGroupId == *elasticacheConfig.ReplicationGroupId {
			foundCache = c
			break
		}
	}

	// create elasticache cluster if it doesn't exist
	if foundCache == nil {
		logrus.Info("creating elasticache cluster")
		if _, err = cacheSvc.CreateReplicationGroup(elasticacheConfig); err != nil {
			errMsg := fmt.Sprintf("error creating elasticache cluster %s", err)
			return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}
		return nil, "started elasticache provision", nil
	}

	// check elasticache phase
	if *foundCache.Status != "available" {
		return nil, v1alpha1.StatusMessage(fmt.Sprintf("elasticache creation in progress, current status is %s", *foundCache.Status)), nil
	}

	// elasticache is available
	// check if found cluster and user strategy differs, and modify instance
	logrus.Info("found existing elasticache instance")
	ec := buildElasticacheUpdateStrategy(elasticacheConfig, foundCache)
	if ec != nil {
		if _, err = cacheSvc.ModifyReplicationGroup(ec); err != nil {
			errMsg := "failed to modify elasticache cluster"
			return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}
		return nil, "modify elasticache cluster in progress", nil
	}

	// return secret information
	primaryEndpoint := foundCache.NodeGroups[0].PrimaryEndpoint
	return &providers.RedisCluster{DeploymentDetails: &providers.RedisDeploymentDetails{
		URI:  *primaryEndpoint.Address,
		Port: *primaryEndpoint.Port,
	}}, "creation successful", nil
}

// DeleteStorage Delete elasticache replication group
func (p *AWSRedisProvider) DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (v1alpha1.StatusMessage, error) {
	// cluster infra info
	p.Logger.Info("getting cluster id from infrastructure for bucket naming")
	redisName, err := buildInfraNameFromObject(ctx, p.Client, r.ObjectMeta, defaultAwsCacheNameLength)
	if err != nil {
		return "failed to construct name for redis cluster from cluster infrastructure", errorUtil.Wrap(err, "failed to build redis cluster name")
	}

	// resolve redis information for redis created by provider
	redisCreateConfig, redisDeleteConfig, stratCfg, err := p.getRedisConfig(ctx, r)
	if err != nil {
		return "failed to retrieve aws redis config", errorUtil.Wrapf(err, "failed to retrieve aws redis config for instance %s", r.Name)
	}
	if redisCreateConfig.ReplicationGroupId == nil {
		redisCreateConfig.ReplicationGroupId = aws.String(redisName)
	}

	// get provider aws creds so the redis cluster can be deleted
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		msg := "failed to reconcile aws provider credentials"
		return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// setup aws redis cluster sdk session
	cacheSvc := createElasticacheService(stratCfg, providerCreds)

	// delete the redis cluster
	return p.deleteRedisCluster(cacheSvc, redisCreateConfig, redisDeleteConfig, ctx, r)
}

func (p *AWSRedisProvider) deleteRedisCluster(cacheSvc elasticacheiface.ElastiCacheAPI, redisCreateConfig *elasticache.CreateReplicationGroupInput, redisDeleteConfig *elasticache.DeleteReplicationGroupInput, ctx context.Context, r *v1alpha1.Redis) (v1alpha1.StatusMessage, error) {
	// the aws access key can sometimes still not be registered in aws on first try, so loop
	rgs, err := getReplicationGroups(cacheSvc)
	if err != nil {
		return "error getting replication groups", err
	}

	// check if the cluster has already been deleted
	var foundCache *elasticache.ReplicationGroup
	for _, c := range rgs {
		if *c.ReplicationGroupId == *redisCreateConfig.ReplicationGroupId {
			foundCache = c
			break
		}
	}
	// check if replication group does not exist and delete finalizer
	if foundCache == nil {
		// remove the finalizer added by the provider
		resources.RemoveFinalizer(&r.ObjectMeta, DefaultFinalizer)
		if err := p.Client.Update(ctx, r); err != nil {
			msg := "failed to update instance as part of finalizer reconcile"
			return v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
		}
		return "redis cache successfully deleted", nil
	}

	// check and verify delete config
	if err := p.buildRedisDeleteConfig(ctx, *r, redisCreateConfig, redisDeleteConfig); err != nil {
		msg := "failed to verify aws rds instance configuration"
		return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	// check if replication group exists and is available
	if *foundCache.Status == "available" {
		// delete the redis cluster that was created by the provider
		_, err = cacheSvc.DeleteReplicationGroup(redisDeleteConfig)
		redisErr, isAwsErr := err.(awserr.Error)
		if err != nil && !isAwsErr {
			return "failed to delete elasticache cluster", errorUtil.Wrapf(err, "failed to delete elasticache cluster %s", *redisDeleteConfig.ReplicationGroupId)
		}
		if err != nil && isAwsErr {
			if redisErr.Code() != elasticache.ErrCodeReplicationGroupNotFoundFault {
				return "failed to delete elasticache cluster", errorUtil.Wrapf(err, "failed to delete elasticache cluster %s, aws error", *redisDeleteConfig.ReplicationGroupId)
			}
		}
	}
	return "redis cache deletion in progress", nil
}

// poll for replication groups
func getReplicationGroups(cacheSvc elasticacheiface.ElastiCacheAPI) ([]*elasticache.ReplicationGroup, error) {
	var rgs []*elasticache.ReplicationGroup
	err := wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		listOutput, err := cacheSvc.DescribeReplicationGroups(&elasticache.DescribeReplicationGroupsInput{})
		if err != nil {
			return false, nil
		}
		rgs = listOutput.ReplicationGroups
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return rgs, nil
}

// getRedisConfig retrieves the redis config from the cloud-resources-aws-strategies configmap
func (p *AWSRedisProvider) getRedisConfig(ctx context.Context, r *v1alpha1.Redis) (*elasticache.CreateReplicationGroupInput, *elasticache.DeleteReplicationGroupInput, *StrategyConfig, error) {
	stratCfg, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.RedisResourceType, r.Spec.Tier)
	if err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to read aws strategy config")
	}
	if stratCfg.Region == "" {
		stratCfg.Region = DefaultRegion
	}

	// unmarshal the redis cluster config
	redisCreateConfig := &elasticache.CreateReplicationGroupInput{}
	if err := json.Unmarshal(stratCfg.CreateStrategy, redisCreateConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws redis cluster configuration")
	}

	redisDeleteConfig := &elasticache.DeleteReplicationGroupInput{}
	if err := json.Unmarshal(stratCfg.DeleteStrategy, redisDeleteConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws redis cluster configuration")
	}
	return redisCreateConfig, redisDeleteConfig, stratCfg, nil
}

func buildElasticacheUpdateStrategy(elasticacheConfig *elasticache.CreateReplicationGroupInput, foundConfig *elasticache.ReplicationGroup) *elasticache.ModifyReplicationGroupInput {
	updateFound := false

	ec := &elasticache.ModifyReplicationGroupInput{}
	ec.ReplicationGroupId = foundConfig.ReplicationGroupId

	if *elasticacheConfig.CacheNodeType != *foundConfig.CacheNodeType {
		ec.CacheNodeType = elasticacheConfig.CacheNodeType
		updateFound = true
	}
	if *elasticacheConfig.SnapshotRetentionLimit != *foundConfig.SnapshotRetentionLimit {
		ec.SnapshotRetentionLimit = elasticacheConfig.SnapshotRetentionLimit
		updateFound = true
	}
	if updateFound {
		return ec
	}
	return nil
}

// verifyRedisConfig checks redis config, if none exist sets values to default
func (p *AWSRedisProvider) buildElasticacheCreateStrategy(ctx context.Context, r *v1alpha1.Redis, elasticacheConfig *elasticache.CreateReplicationGroupInput) error {

	elasticacheConfig.AutomaticFailoverEnabled = aws.Bool(true)
	elasticacheConfig.Engine = aws.String("redis")

	if elasticacheConfig.CacheNodeType == nil {
		elasticacheConfig.CacheNodeType = aws.String(defaultCacheNodeType)
	}
	if elasticacheConfig.ReplicationGroupDescription == nil {
		elasticacheConfig.ReplicationGroupDescription = aws.String(defaultDescription)
	}
	if elasticacheConfig.EngineVersion == nil {
		elasticacheConfig.EngineVersion = aws.String(defaultEngineVersion)
	}
	if elasticacheConfig.NumCacheClusters == nil {
		elasticacheConfig.NumCacheClusters = aws.Int64(defaultNumCacheClusters)
	}
	if elasticacheConfig.SnapshotRetentionLimit == nil {
		elasticacheConfig.SnapshotRetentionLimit = aws.Int64(defaultSnapshotRetention)
	}
	cacheName, err := buildInfraNameFromObject(ctx, p.Client, r.ObjectMeta, defaultAwsCacheNameLength)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve elasticache config")
	}
	if elasticacheConfig.ReplicationGroupId == nil {
		elasticacheConfig.ReplicationGroupId = aws.String(cacheName)
	}
	return nil
}

func (p *AWSRedisProvider) buildRedisDeleteConfig(ctx context.Context, redis v1alpha1.Redis, redisCreateConfig *elasticache.CreateReplicationGroupInput, redisDeleteConfig *elasticache.DeleteReplicationGroupInput) error {
	instanceIdentifier, err := buildTimestampedInfraNameFromObject(ctx, p.Client, redis.ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve rds config")
	}
	if redisDeleteConfig.ReplicationGroupId == nil {
		if redisCreateConfig.ReplicationGroupId == nil {
			redisCreateConfig.ReplicationGroupId = aws.String(instanceIdentifier)
		}
		redisDeleteConfig.ReplicationGroupId = redisCreateConfig.ReplicationGroupId
	}

	if redisDeleteConfig.RetainPrimaryCluster == nil {
		redisDeleteConfig.RetainPrimaryCluster = aws.Bool(false)
	}
	// if no strategy is provided the default behavior is to take snapshot
	if redisDeleteConfig.FinalSnapshotIdentifier == nil {
		redisDeleteConfig.FinalSnapshotIdentifier = aws.String(instanceIdentifier)
	}
	//NoFinalSnapshotIdentifier, You can pass in an empty string for no snapshot to be taken (don't need to do this)
	if redisDeleteConfig.FinalSnapshotIdentifier == aws.String(NoFinalSnapshotIdentifier) {
		redisDeleteConfig.FinalSnapshotIdentifier = aws.String(NoFinalSnapshotIdentifier)
	}

	return nil
}
