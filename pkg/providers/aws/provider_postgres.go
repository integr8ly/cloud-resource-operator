package aws

import (
	"context"
	"encoding/json"
	"strings"
	"time"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	postgresProviderName                 = "aws-rds"
	defaultAwsPostgresDeletionProtection = true
	defaultAwsPostgresPort               = 5432
	defaultAwsPostgresUser               = "postgres"
	defaultAwsAllocatedStorage           = 20
	defaultAwsPostgresDatabase           = "postgres"
	defaultAwsBackupRetentionPeriod      = 31
	defaultAwsDBInstanceIdentifier       = "test-identifier"
	defaultAwsDBInstanceClass            = "db.t2.small"
	defaultAwsEngine                     = "postgres"
	defaultAwsEngineVersion              = "10.6"
	defaultAwsPubliclyAccessible         = false

	defaultAwsIdentifierLength = 40

	defaultCredentialsSec      = "-aws-rds-credentials"
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
	postgresCfg, stratCfg, err := p.getPostgresConfig(ctx, r)
	if err != nil {
		msg := "failed to retrieve aws RDS cluster config for instance"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrapf(err, msg)
	}
	if postgresCfg.DBInstanceIdentifier == nil {
		postgresCfg.DBInstanceIdentifier = aws.String(r.Name)
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
	if err := p.verifyPostgresConfig(ctx, cr, postgresCfg); err != nil {
		msg := "failed to verify aws postgres cluster configuration"
		return nil, v1alpha1.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	credSec := &v1.Secret{}
	if err := p.Client.Get(ctx, types.NamespacedName{Name: cr.Name + defaultCredentialsSec, Namespace: cr.Namespace}, credSec); err != nil {
		return nil, "failed to retrieve postgres credential secret", errorUtil.Wrap(err, "failed to retrieve postgres credential secret")
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
	return nil, "postgres instance in progress", nil
}

func (p *AWSPostgresProvider) DeletePostgres(ctx context.Context, r *v1alpha1.Postgres) (v1alpha1.StatusMessage, error) {
	//todo delete the cred secret as part of delete func
	return "", nil
}

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

func (p *AWSPostgresProvider) getPostgresConfig(ctx context.Context, r *v1alpha1.Postgres) (*rds.CreateDBInstanceInput, *StrategyConfig, error) {
	stratCfg, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, r.Spec.Tier)
	if err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to read aws strategy config")
	}
	if stratCfg.Region == "" {
		stratCfg.Region = DefaultRegion
	}

	postgresConfig := &rds.CreateDBInstanceInput{}
	if err := json.Unmarshal(stratCfg.RawStrategy, postgresConfig); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws postgres cluster configuration")
	}
	return postgresConfig, stratCfg, nil
}

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

func (p *AWSPostgresProvider) verifyPostgresConfig(ctx context.Context, pg *v1alpha1.Postgres, postgresConfig *rds.CreateDBInstanceInput) error {
	if postgresConfig.DeletionProtection == nil {
		postgresConfig.DeletionProtection = aws.Bool(defaultAwsPostgresDeletionProtection)
	}
	if postgresConfig.MasterUsername == nil {
		postgresConfig.MasterUsername = aws.String(defaultAwsPostgresUser)
	}
	if postgresConfig.MasterUserPassword == nil {
		postgresConfig.MasterUserPassword = aws.String(defaultAwsPostgresPassword)
	}
	if postgresConfig.Port == nil {
		postgresConfig.Port = aws.Int64(defaultAwsPostgresPort)
	}
	if postgresConfig.DBName == nil {
		postgresConfig.DBName = aws.String(defaultAwsPostgresDatabase)
	}
	if postgresConfig.BackupRetentionPeriod == nil {
		postgresConfig.BackupRetentionPeriod = aws.Int64(defaultAwsBackupRetentionPeriod)
	}
	if postgresConfig.DBInstanceClass == nil {
		postgresConfig.DBInstanceClass = aws.String(defaultAwsDBInstanceClass)
	}
	if postgresConfig.PubliclyAccessible == nil {
		postgresConfig.PubliclyAccessible = aws.Bool(defaultAwsPubliclyAccessible)
	}
	if postgresConfig.AllocatedStorage == nil {
		postgresConfig.AllocatedStorage = aws.Int64(defaultAwsAllocatedStorage)
	}
	if postgresConfig.EngineVersion == nil {
		postgresConfig.EngineVersion = aws.String(defaultAwsEngineVersion)
	}
	if postgresConfig.EngineVersion != nil {
		if !resources.Contains(defaultSupportedEngineVersions, *postgresConfig.EngineVersion) {
			postgresConfig.EngineVersion = aws.String(defaultAwsEngineVersion)
		}
	}
	instanceName, err := buildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultAwsIdentifierLength)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve postgres config")
	}
	if postgresConfig.DBInstanceIdentifier == nil {
		postgresConfig.DBInstanceIdentifier = aws.String(instanceName)
	}
	postgresConfig.Engine = aws.String(defaultAwsEngine)
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
			Name:      ps.Name + defaultCredentialsSec,
			Namespace: ps.Namespace,
		},
		StringData: map[string]string{
			defaultPostgresUserKey:     defaultAwsPostgresUser,
			defaultPostgresPasswordKey: password,
		},
		Type: v1.SecretTypeOpaque,
	}
}
