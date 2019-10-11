package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	errorUtil "github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	blobstorageProviderName    = "aws-s3"
	defaultAwsBucketNameLength = 40
	// default create options
	dataBucketName          = "bucketName"
	dataCredentialKeyID     = "credentialKeyID"
	dataCredentialSecretKey = "credentialSecretKey"
)

// BlobStorageDeploymentDetails Provider-specific details about the AWS S3 bucket created
type BlobStorageDeploymentDetails struct {
	BucketName          string
	CredentialKeyID     string
	CredentialSecretKey string
}

func (d *BlobStorageDeploymentDetails) Data() map[string][]byte {
	return map[string][]byte{
		dataBucketName:          []byte(d.BucketName),
		dataCredentialKeyID:     []byte(d.CredentialKeyID),
		dataCredentialSecretKey: []byte(d.CredentialSecretKey),
	}
}

var _ providers.BlobStorageProvider = (*BlobStorageProvider)(nil)

// BlobStorageProvider implementation for AWS S3
type BlobStorageProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewAWSBlobStorageProvider(client client.Client, logger *logrus.Entry) *BlobStorageProvider {
	return &BlobStorageProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": blobstorageProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigMapConfigManager(client),
	}
}

func (p *BlobStorageProvider) GetName() string {
	return blobstorageProviderName
}

func (p *BlobStorageProvider) SupportsStrategy(d string) bool {
	return d == providers.AWSDeploymentStrategy
}

// CreateStorage Create S3 bucket from strategy config and credentials to interact with it
func (p *BlobStorageProvider) CreateStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (*providers.BlobStorageInstance, v1alpha1.StatusMessage, error) {
	// handle provider-specific finalizer
	if err := resources.CreateFinalizer(ctx, p.Client, bs, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	// info about the bucket to be created
	p.Logger.Infof("getting aws s3 bucket config for blob storage instance %s", bs.Name)
	bucketCreateCfg, stratCfg, err := p.buildS3BucketConfig(ctx, bs)
	if err != nil {
		errMsg := "failed to build s3 bucket config"
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// create the credentials to be used by the end-user, whoever created the blobstorage instance
	endUserCredsName := buildEndUserCredentialsNameFromBucket(*bucketCreateCfg.Bucket)
	p.Logger.Infof("creating end-user credentials with name %s for managing s3 bucket %s", endUserCredsName, *bucketCreateCfg.Bucket)
	endUserCreds, _, err := p.CredentialManager.ReoncileBucketOwnerCredentials(ctx, endUserCredsName, bs.Namespace, *bucketCreateCfg.Bucket)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile s3 end-user credentials for blob storage instance %s", bs.Name)
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// create the credentials to be used by the aws resource providers, not to be used by end-user
	p.Logger.Infof("creating provider credentials for creating s3 buckets, in namespace %s", bs.Namespace)
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, bs.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile aws blob storage provider credentials for blob storage instance %s", bs.Name)
		return nil, v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// setup aws s3 sdk session
	p.Logger.Infof("creating new aws sdk session in region %s", stratCfg.Region)
	s3svc := createS3Session(stratCfg, providerCreds)

	// create bucket if it doesn't already exist, if it does exist then use the existing bucket
	p.Logger.Infof("reconciling aws s3 bucket %s", *bucketCreateCfg.Bucket)
	msg, err := p.reconcileBucketCreate(ctx, s3svc, bucketCreateCfg)
	if err != nil {
		return nil, msg, errorUtil.Wrapf(err, string(msg))
	}

	// blobstorageinstance that will be returned if everything is successful
	bsi := &providers.BlobStorageInstance{
		DeploymentDetails: &BlobStorageDeploymentDetails{
			BucketName:          *bucketCreateCfg.Bucket,
			CredentialKeyID:     endUserCreds.AccessKeyID,
			CredentialSecretKey: endUserCreds.SecretAccessKey,
		},
	}

	p.Logger.Infof("creation handler for blob storage instance %s in namespace %s finished successfully", bs.Name, bs.Namespace)
	return bsi, msg, nil
}

// DeleteStorage Delete S3 bucket and credentials to add objects to it
func (p *BlobStorageProvider) DeleteStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (v1alpha1.StatusMessage, error) {
	p.Logger.Infof("deleting blob storage instance %s via aws s3", bs.Name)

	// resolve bucket information for bucket created by provider
	p.Logger.Infof("getting aws s3 bucket config for blob storage instance %s", bs.Name)
	bucketCreateCfg, stratCfg, err := p.buildS3BucketConfig(ctx, bs)
	if err != nil {
		errMsg := "failed to build s3 bucket config"
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// get provider aws creds so the bucket can be deleted
	p.Logger.Infof("creating provider credentials for creating s3 buckets, in namespace %s", bs.Namespace)
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, bs.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile aws provider credentials for blob storage instance %s", bs.Name)
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// create new s3 session
	p.Logger.Infof("creating new aws sdk session in region %s", stratCfg.Region)
	s3svc := createS3Session(stratCfg, providerCreds)

	// delete the bucket that was created by the provider
	return p.reconcileBucketDelete(ctx, bs, s3svc, bucketCreateCfg)
}

func createS3Session(stratCfg *StrategyConfig, providerCreds *AWSCredentials) s3iface.S3API {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	}))
	return s3.New(sess)
}

func (p *BlobStorageProvider) reconcileBucketDelete(ctx context.Context, bs *v1alpha1.BlobStorage, s3svc s3iface.S3API, bucketCfg *s3.CreateBucketInput) (v1alpha1.StatusMessage, error) {
	buckets, err := getS3buckets(s3svc)
	if err != nil {
		return "error getting s3 buckets", err
	}

	// check if the bucket has already been deleted
	var foundBucket *s3.Bucket
	for _, i := range buckets {
		if *i.Name == *bucketCfg.Bucket {
			foundBucket = i
			break
		}
	}

	// check if bucket exists and delete the bucket
	if foundBucket != nil {
		_, err = s3svc.DeleteBucket(&s3.DeleteBucketInput{
			Bucket: bucketCfg.Bucket,
		})
		s3err, isAwsErr := err.(awserr.Error)
		if err != nil && (!isAwsErr || s3err.Code() != s3.ErrCodeNoSuchBucket) {
			errMsg := fmt.Sprintf("failed to delete s3 bucket: %s", err)
			return v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
		}
	}

	// build end user credential name
	endUserCredsName := buildEndUserCredentialsNameFromBucket(*bucketCfg.Bucket)

	// remove the credentials request created by the provider
	p.Logger.Infof("deleting end-user credential request %s in namespace %s", endUserCredsName, bs.Namespace)
	endUserCredsReq := &v1.CredentialsRequest{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      endUserCredsName,
			Namespace: bs.Namespace,
		},
	}
	if err := p.Client.Delete(ctx, endUserCredsReq); err != nil {
		if !errors.IsNotFound(err) {
			errMsg := fmt.Sprintf("failed to delete credential request %s", endUserCredsName)
			return v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
		}
		p.Logger.Infof("could not find credential request %s, already deleted, continuing", endUserCredsName)
	}

	// remove the finalizer
	resources.RemoveFinalizer(&bs.ObjectMeta, DefaultFinalizer)
	if err := p.Client.Update(ctx, bs); err != nil {
		errMsg := "failed to update blob storage cr as part of finalizer reconcile"
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}
	return v1alpha1.StatusEmpty, nil
}

func (p *BlobStorageProvider) reconcileBucketCreate(ctx context.Context, s3svc s3iface.S3API, bucketCfg *s3.CreateBucketInput) (v1alpha1.StatusMessage, error) {
	// the aws access key can sometimes still not be registered in aws on first try, so loop
	p.Logger.Infof("listing existing aws s3 buckets")
	buckets, err := getS3buckets(s3svc)
	if err != nil {
		errMsg := "failed to list existing aws s3 buckets, credentials could be reconciling"
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// check if bucket already exists
	p.Logger.Infof("checking if aws s3 bucket %s already exists", *bucketCfg.Bucket)
	var foundBucket *s3.Bucket
	for _, b := range buckets {
		if *b.Name == *bucketCfg.Bucket {
			foundBucket = b
			break
		}
	}
	if foundBucket != nil {
		errMsg := fmt.Sprintf("using bucket %s", *foundBucket.Name)
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// create bucket
	p.Logger.Infof("bucket %s not found, creating bucket", *bucketCfg.Bucket)
	_, err = s3svc.CreateBucket(bucketCfg)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create s3 bucket %s", *bucketCfg.Bucket)
		return v1alpha1.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	p.Logger.Infof("reconcile for aws s3 bucket completed successfully, bucket created")
	return "successfully created", nil
}

// function to get s3 buckets, used to check/wait on AWS credentials
func getS3buckets(s3svc s3iface.S3API) ([]*s3.Bucket, error) {
	var existingBuckets []*s3.Bucket
	err := wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		listOutput, err := s3svc.ListBuckets(nil)
		if err != nil {
			return false, nil
		}
		existingBuckets = listOutput.Buckets
		return true, nil
	})
	if err != nil {
		return nil, errorUtil.Wrap(err, "timed out waiting to list s3 buckets")
	}
	return existingBuckets, nil
}

func (p *BlobStorageProvider) buildS3BucketConfig(ctx context.Context, bs *v1alpha1.BlobStorage) (*s3.CreateBucketInput, *StrategyConfig, error) {
	// info about the bucket to be created
	p.Logger.Infof("getting aws s3 bucket config for blob storage instance %s", bs.Name)
	bucketCreateCfg, stratCfg, err := p.getS3BucketConfig(ctx, bs)
	if err != nil {
		return nil, nil, errorUtil.Wrapf(err, fmt.Sprintf("failed to retrieve aws s3 bucket config for blob storage instance %s", bs.Name))
	}

	// cluster infra info
	p.Logger.Info("getting cluster id from infrastructure for bucket naming")
	bucketName, err := buildInfraNameFromObject(ctx, p.Client, bs.ObjectMeta, defaultAwsBucketNameLength)
	if err != nil {
		return nil, nil, errorUtil.Wrapf(err, fmt.Sprintf("failed to retrieve aws s3 bucket config for blob storage instance %s", bs.Name))
	}
	if bucketCreateCfg.Bucket == nil {
		bucketCreateCfg.Bucket = aws.String(bucketName)
	}
	return bucketCreateCfg, stratCfg, nil
}

func (p *BlobStorageProvider) getS3BucketConfig(ctx context.Context, bs *v1alpha1.BlobStorage) (*s3.CreateBucketInput, *StrategyConfig, error) {
	stratCfg, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.BlobStorageResourceType, bs.Spec.Tier)
	if err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to read aws strategy config")
	}
	if stratCfg.Region == "" {
		p.Logger.Debugf("region not set in deployment strategy configuration, using default region %s", DefaultRegion)
		stratCfg.Region = DefaultRegion
	}

	// delete the s3 bucket created by the provider
	s3cbi := &s3.CreateBucketInput{}
	if err = json.Unmarshal(stratCfg.CreateStrategy, s3cbi); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws s3 configuration")
	}
	return s3cbi, stratCfg, nil
}

func buildEndUserCredentialsNameFromBucket(b string) string {
	return fmt.Sprintf("cro-aws-s3-%s-creds", b)
}
