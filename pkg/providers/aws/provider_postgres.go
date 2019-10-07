package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	errorUtil "github.com/pkg/errors"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	postgresProviderName       = "aws-rds"
	defaultAwsIdentifierLength = 40
	// default create options
	defaultAwsPostgresDeletionProtection = true
	defaultAwsPostgresPort               = 5432
	defaultAwsPostgresUser               = "postgres"
	defaultAwsAllocatedStorage           = 20
	defaultAwsPostgresDatabase           = "postgres"
	defaultAwsBackupRetentionPeriod      = 31
	defaultAwsDBInstanceClass            = "db.t2.small"
	defaultAwsEngine                     = "postgres"
	defaultAwsEngineVersion              = "10.6"
	defaultAwsPubliclyAccessible         = false
	// default delete options
	defaultAwsSkipFinalSnapshot      = true
	defaultAwsDeleteAutomatedBackups = true
	// defaults for DB user credentials
	defaultCredSecSuffix       = "-aws-rds-credentials"
	defaultPostgresUserKey     = "user"
	defaultPostgresPasswordKey = "password"
)

var (
	defaultSupportedEngineVersions = []string{"10.6", "9.6", "9.5"}
	defaultAwsPostgresPassword     = ""
)

type AWSPostgresProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewAWSPostgresProvider(client client.Client, logger *logrus.Entry) *AWSPostgresProvider {
	return &AWSPostgresProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": postgresProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigMapConfigManager(client),
	}
}

func (p *AWSPostgresProvider) GetName() string {
	return postgresProviderName
}

func (p *AWSPostgresProvider) SupportsStrategy(d string) bool {
	return d == providers.AWSDeploymentStrategy
}

// CreatePostgres creates an RDS Instance from strategy config
func (p *AWSPostgresProvider) CreatePostgres(ctx context.Context, r *v1alpha1.Postgres) (*providers.PostgresInstance, v1alpha1.StatusMessage, error) {
	// handle provider-specific finalizer
	if r.GetDeletionTimestamp() == nil {
		resources.AddFinalizer(&r.ObjectMeta, DefaultFinalizer)
		if err := p.Client.Update(ctx, r); err != nil {
			msg := "failed to add finalizer to instance"
			return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
		}
	}

	// info about the RDS instance to be created
	postgresCfg, _, stratCfg, err := p.getPostgresConfig(ctx, r)
	if err != nil {
		msg := "failed to retrieve aws RDS cluster config for instance"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
	}

	// create the credentials to be used by the aws resource providers, not to be used by end-user
	provoiderCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		msg := "failed to reconcile RDS credentials"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// create credentials secret
	if err := p.CreateSecret(ctx, buildDefaultPostgresSecret(r)); err != nil {
		msg := "failed to create or update postgres secret"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// setup aws RDS instance sdk session
	cacheSvc := createPostgresService(stratCfg, provoiderCreds)

	// create the aws RDS instance
	return p.createPostgresInstance(ctx, r, cacheSvc, postgresCfg)
}

func createPostgresService(stratCfg *StrategyConfig, providerCreds *AWSCredentials) rdsiface.RDSAPI {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	}))
	return rds.New(sess)
}

func (p *AWSPostgresProvider) createPostgresInstance(ctx context.Context, cr *v1alpha1.Postgres, rdsSvc rdsiface.RDSAPI, postgresCfg *rds.CreateDBInstanceInput) (*providers.PostgresInstance, v1alpha1.StatusMessage, error) {
	// the aws access key can sometimes still not be registered in aws on first try, so loop
	pi, err := getPostgresInstances(rdsSvc)
	if err != nil {
		// return nil error so this function can be requeued
		logrus.Info("error getting replication groups : ", err)
		return nil, "error getting replication groups", err
	}

	// verify postgresConfig
	if err := p.verifyPostgresCreateConfig(ctx, cr, postgresCfg); err != nil {
		msg := "failed to verify aws postgres cluster configuration"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	credSec := &v1.Secret{}
	if err := p.Client.Get(ctx, types.NamespacedName{Name: cr.Name + defaultCredSecSuffix, Namespace: cr.Namespace}, credSec); err != nil {
		msg := "failed to retrieve postgres credential secret"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	for k, p := range credSec.Data {
		if k == defaultPostgresPasswordKey {
			defaultAwsPostgresPassword = string(p)
		}
	}

	// check if the cluster has already been created
	var foundInstance *rds.DBInstance
	for _, i := range pi {
		if *i.DBInstanceIdentifier == *postgresCfg.DBInstanceIdentifier {
			foundInstance = i
			break
		}
	}
	if foundInstance != nil {
		if *foundInstance.DBInstanceStatus == "available" {
			logrus.Info("found existing rds instance")
			mi := verifyPostgresChange(postgresCfg, foundInstance)
			if mi != nil {
				_, err = rdsSvc.ModifyDBInstance(mi)
				if err != nil {
					return nil, "failed to modify instance", err
				}
				return nil, "modify instance in progress", nil
			}
			return &providers.PostgresInstance{DeploymentDetails: &providers.PostgresDeploymentDetails{
				Username: *foundInstance.MasterUsername,
				Password: defaultAwsPostgresPassword,
				Host:     *foundInstance.Endpoint.Address,
				Database: *foundInstance.DBName,
				Port:     int(*foundInstance.Endpoint.Port),
			}}, "creation successful", nil
		}
		return nil, "creation in progress", nil
	}

	logrus.Info("creating rds instance")
	_, err = rdsSvc.CreateDBInstance(postgresCfg)
	if err != nil {
		return nil, "error creating postgres instance", err
	}
	return nil, "", nil
}

func (p *AWSPostgresProvider) DeletePostgres(ctx context.Context, r *v1alpha1.Postgres) (v1alpha1.StatusMessage, error) {
	// resolve postgres information for postgres created by provider
	postgresCreateConfig, postgresDeleteConfig, stratCfg, err := p.getPostgresConfig(ctx, r)
	if err != nil {
		msg := "failed to retrieve aws postgres config"
		return v1alpha1.StatusMessage(msg), err
	}

	// get provider aws creds so the postgres instance can be deleted
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		msg := "failed to reconcile aws provider credentials"
		return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// setup aws postgres instance sdk session
	instanceSvc := createPostgresService(stratCfg, providerCreds)

	return p.deletePostgresInstance(ctx, r, instanceSvc, postgresCreateConfig, postgresDeleteConfig)
}

func (p *AWSPostgresProvider) deletePostgresInstance(ctx context.Context, pg *v1alpha1.Postgres, instanceSvc rdsiface.RDSAPI, postgresCreateConfig *rds.CreateDBInstanceInput, postgresDeleteConfig *rds.DeleteDBInstanceInput) (v1alpha1.StatusMessage, error) {
	// the aws access key can sometimes still not be registered in aws on first try, so loop
	pgs, err := getPostgresInstances(instanceSvc)
	if err != nil {
		return "error getting aws rds instances", err
	}

	// check and verify delete config
	if err := p.verifyPostgresDeleteConfig(ctx, pg, postgresCreateConfig, postgresDeleteConfig); err != nil {
		msg := "failed to verify aws postgres cluster configuration"
		return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// check if the instance has already been deleted
	var foundInstance *rds.DBInstance
	for _, i := range pgs {
		if *i.DBInstanceIdentifier == *postgresDeleteConfig.DBInstanceIdentifier {
			foundInstance = i
			break
		}
	}

	// check if instance does not exist, delete finalizer and credential secret
	if foundInstance == nil {
		// delete credential secret
		p.Logger.Info("Deleting postgres secret")
		sec := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pg.Name + defaultCredSecSuffix,
				Namespace: pg.Namespace,
			},
		}
		err = p.Client.Delete(ctx, sec)
		if err != nil && !k8serr.IsNotFound(err) {
			msg := "failed to deleted postgres secrets"
			return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}

		resources.RemoveFinalizer(&pg.ObjectMeta, DefaultFinalizer)
		if err := p.Client.Update(ctx, pg); err != nil {
			msg := "failed to update instance as part of finalizer reconcile"
			return v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
		}
		return "", nil
	}

	// check if instance db exists and is available
	if *foundInstance.DBInstanceStatus == "available" {
		if *foundInstance.DeletionProtection {
			_, err = instanceSvc.ModifyDBInstance(&rds.ModifyDBInstanceInput{
				DBInstanceIdentifier: postgresDeleteConfig.DBInstanceIdentifier,
				DeletionProtection:   aws.Bool(false),
			})
			if err != nil {
				msg := "waiting on AWS ModifyDBInstance permission"
				return v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
			}
			return "turning off deletion protection", nil
		}
		_, err = instanceSvc.DeleteDBInstance(postgresDeleteConfig)
		postgresErr, isAwsErr := err.(awserr.Error)
		if err != nil && !isAwsErr {
			msg := fmt.Sprintf("failed to delete rds instance : %s", err)
			return v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
		}
		if err != nil && isAwsErr {
			if postgresErr.Code() != rds.ErrCodeDBInstanceNotFoundFault {
				msg := fmt.Sprintf("failed to delete rds instance : %s", err)
				return v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
			}
		}
	}
	return "postgres instance deletion in progress", nil
}

// function to get rds instances, used to check/wait on AWS credentials
func getPostgresInstances(cacheSvc rdsiface.RDSAPI) ([]*rds.DBInstance, error) {
	var pi []*rds.DBInstance
	err := wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		listOutput, err := cacheSvc.DescribeDBInstances(&rds.DescribeDBInstancesInput{})
		if err != nil {
			return false, nil
		}
		pi = listOutput.DBInstances
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return pi, nil
}

func (p *AWSPostgresProvider) getPostgresConfig(ctx context.Context, r *v1alpha1.Postgres) (*rds.CreateDBInstanceInput, *rds.DeleteDBInstanceInput, *StrategyConfig, error) {
	stratCfg, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, r.Spec.Tier)
	if err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to read aws strategy config")
	}
	if stratCfg.Region == "" {
		stratCfg.Region = DefaultRegion
	}

	postgresCreateConfig := &rds.CreateDBInstanceInput{}
	if err := json.Unmarshal(stratCfg.RawStrategy, postgresCreateConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws postgres cluster configuration")
	}

	postgresDeleteConfig := &rds.DeleteDBInstanceInput{}
	if err := json.Unmarshal(stratCfg.DeleteStrategy, postgresDeleteConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws postgres cluster configuration")
	}
	return postgresCreateConfig, postgresDeleteConfig, stratCfg, nil
}

// verifies if there is a change between a found instance and the configuration from the instance strat
func verifyPostgresChange(postgresConfig *rds.CreateDBInstanceInput, foundConfig *rds.DBInstance) *rds.ModifyDBInstanceInput {
	updateFound := false

	mi := &rds.ModifyDBInstanceInput{}
	mi.DBInstanceIdentifier = postgresConfig.DBInstanceIdentifier

	if *postgresConfig.DeletionProtection != *foundConfig.DeletionProtection {
		mi.DeletionProtection = postgresConfig.DeletionProtection
		updateFound = true
	}
	if *postgresConfig.Port != *foundConfig.Endpoint.Port {
		mi.DBPortNumber = postgresConfig.Port
		updateFound = true
	}
	if *postgresConfig.BackupRetentionPeriod != *foundConfig.BackupRetentionPeriod {
		mi.BackupRetentionPeriod = postgresConfig.BackupRetentionPeriod
		updateFound = true
	}
	if *postgresConfig.DBInstanceClass != *foundConfig.DBInstanceClass {
		mi.DBInstanceClass = postgresConfig.DBInstanceClass
		updateFound = true
	}
	if *postgresConfig.PubliclyAccessible != *foundConfig.PubliclyAccessible {
		mi.PubliclyAccessible = postgresConfig.PubliclyAccessible
		updateFound = true
	}
	if *postgresConfig.AllocatedStorage != *foundConfig.AllocatedStorage {
		mi.AllocatedStorage = postgresConfig.AllocatedStorage
		updateFound = true
	}
	if *postgresConfig.EngineVersion != *foundConfig.EngineVersion {
		mi.EngineVersion = postgresConfig.EngineVersion
		updateFound = true
	}
	if !updateFound {
		return nil
	}
	return mi
}

// verify postgres create config
func (p *AWSPostgresProvider) verifyPostgresCreateConfig(ctx context.Context, pg *v1alpha1.Postgres, postgresCreateConfig *rds.CreateDBInstanceInput) error {
	if postgresCreateConfig.DeletionProtection == nil {
		postgresCreateConfig.DeletionProtection = aws.Bool(defaultAwsPostgresDeletionProtection)
	}
	if postgresCreateConfig.MasterUsername == nil {
		postgresCreateConfig.MasterUsername = aws.String(defaultAwsPostgresUser)
	}
	if postgresCreateConfig.MasterUserPassword == nil {
		postgresCreateConfig.MasterUserPassword = aws.String(defaultAwsPostgresPassword)
	}
	if postgresCreateConfig.Port == nil {
		postgresCreateConfig.Port = aws.Int64(defaultAwsPostgresPort)
	}
	if postgresCreateConfig.DBName == nil {
		postgresCreateConfig.DBName = aws.String(defaultAwsPostgresDatabase)
	}
	if postgresCreateConfig.BackupRetentionPeriod == nil {
		postgresCreateConfig.BackupRetentionPeriod = aws.Int64(defaultAwsBackupRetentionPeriod)
	}
	if postgresCreateConfig.DBInstanceClass == nil {
		postgresCreateConfig.DBInstanceClass = aws.String(defaultAwsDBInstanceClass)
	}
	if postgresCreateConfig.PubliclyAccessible == nil {
		postgresCreateConfig.PubliclyAccessible = aws.Bool(defaultAwsPubliclyAccessible)
	}
	if postgresCreateConfig.AllocatedStorage == nil {
		postgresCreateConfig.AllocatedStorage = aws.Int64(defaultAwsAllocatedStorage)
	}
	if postgresCreateConfig.EngineVersion == nil {
		postgresCreateConfig.EngineVersion = aws.String(defaultAwsEngineVersion)
	}
	if postgresCreateConfig.EngineVersion != nil {
		if !resources.Contains(defaultSupportedEngineVersions, *postgresCreateConfig.EngineVersion) {
			postgresCreateConfig.EngineVersion = aws.String(defaultAwsEngineVersion)
		}
	}
	instanceName, err := buildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve postgres config")
	}
	if postgresCreateConfig.DBInstanceIdentifier == nil {
		postgresCreateConfig.DBInstanceIdentifier = aws.String(instanceName)
	}
	postgresCreateConfig.Engine = aws.String(defaultAwsEngine)
	return nil
}

// verify postgres delete config
func (p *AWSPostgresProvider) verifyPostgresDeleteConfig(ctx context.Context, pg *v1alpha1.Postgres, postgresCreateConfig *rds.CreateDBInstanceInput, postgresDeleteConfig *rds.DeleteDBInstanceInput) error {
	instanceIdentifier, err := buildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve postgres config")
	}
	if postgresDeleteConfig.DBInstanceIdentifier == nil {
		if postgresCreateConfig.DBInstanceIdentifier == nil {
			postgresCreateConfig.DBInstanceIdentifier = aws.String(instanceIdentifier)
		}
		postgresDeleteConfig.DBInstanceIdentifier = postgresCreateConfig.DBInstanceIdentifier
	}
	if postgresDeleteConfig.DeleteAutomatedBackups == nil {
		postgresDeleteConfig.DeleteAutomatedBackups = aws.Bool(defaultAwsDeleteAutomatedBackups)
	}
	if postgresDeleteConfig.SkipFinalSnapshot == nil {
		postgresDeleteConfig.SkipFinalSnapshot = aws.Bool(defaultAwsSkipFinalSnapshot)
	}
	if postgresDeleteConfig.FinalDBSnapshotIdentifier == nil && !*postgresDeleteConfig.SkipFinalSnapshot {
		postgresDeleteConfig.FinalDBSnapshotIdentifier = aws.String(instanceIdentifier)
	}
	return nil
}

func GeneratePassword() (string, error) {
	generatedPassword, err := uuid.NewRandom()
	if err != nil {
		return "", errorUtil.Wrap(err, "error generating password")
	}
	return strings.Replace(generatedPassword.String(), "-", "", 10), nil
}

func (p *AWSPostgresProvider) CreateSecret(ctx context.Context, s *v1.Secret) error {
	or, err := controllerutil.CreateOrUpdate(ctx, p.Client, s, func(existing runtime.Object) error {
		return nil
	})
	if err != nil {
		return errorUtil.Wrapf(err, "failed to create or update secret %s, action was %s", s.Name, or)
	}
	return nil
}

func buildDefaultPostgresSecret(ps *v1alpha1.Postgres) *v1.Secret {
	password, err := GeneratePassword()
	if err != nil {
		return nil
	}
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ps.Name + defaultCredSecSuffix,
			Namespace: ps.Namespace,
		},
		StringData: map[string]string{
			defaultPostgresUserKey:     defaultAwsPostgresUser,
			defaultPostgresPasswordKey: password,
		},
		Type: v1.SecretTypeOpaque,
	}
}
