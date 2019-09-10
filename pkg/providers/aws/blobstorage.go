package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"

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
	defaultRegion = "eu-west-1"

	dataBucketName            = "bucketName"
	dataS3CredentialKeyID     = "credentialKeyID"
	dataS3CredentialSecretKey = "credentialSecretKey"

	defaultFinalizer = "finalizers.aws.cloud-resources-operator.integreatly.org"
)

// AWSDeploymentDetails Provider-specific details about the AWS S3 bucket created
type AWSs3DeploymentDetails struct {
	BucketName          string
	CredentialKeyID     string
	CredentialSecretKey string
}

func (d *AWSs3DeploymentDetails) Data() map[string][]byte {
	return map[string][]byte{
		dataBucketName:            []byte(d.BucketName),
		dataS3CredentialKeyID:     []byte(d.CredentialKeyID),
		dataS3CredentialSecretKey: []byte(d.CredentialSecretKey),
	}
}

// AWSBlobStorageProvider BlobStorageProvider implementation for AWS S3
type AWSBlobStorageProvider struct {
	Client            client.Client
	CredentialManager *CredentialManager
	ConfigManager     *ConfigManager
}

func NewAWSBlobStorageProvider(client client.Client) *AWSBlobStorageProvider {
	return &AWSBlobStorageProvider{
		Client:            client,
		CredentialManager: NewCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (p *AWSBlobStorageProvider) GetName() string {
	return string(providers.AWSDeploymentStrategy)
}

func (p *AWSBlobStorageProvider) SupportsStrategy(d string) bool {
	return d == providers.AWSDeploymentStrategy
}

// CreateStorage Create S3 bucket from strategy config and credentials to interact with it
func (p *AWSBlobStorageProvider) CreateStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (*providers.BlobStorageInstance, error) {
	// handle provider-specific finalizer
	if bs.GetDeletionTimestamp() == nil {
		resources.AddFinalizer(&bs.ObjectMeta, defaultFinalizer)
		if err := p.Client.Update(ctx, bs); err != nil {
			return nil, errorUtil.Wrapf(err, "failed to add finalizer to instance")
		}
	}

	// info about the bucket to be created
	bucketCreateCfg, stratCfg, err := p.getS3BucketConfig(ctx, bs)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to retrieve aws s3 bucket config for instance %s", bs.Name)
	}
	if bucketCreateCfg.Bucket == nil {
		bucketCreateCfg.Bucket = aws.String(fmt.Sprintf("%s-%s", bs.Namespace, bs.Name))
	}

	// create the credentials to be used by the end-user, whoever created the blobstorage instance
	endUserCredsName := fmt.Sprintf("cloud-resources-aws-s3-%s-credentials", bs.Name)
	endUserCreds, _, err := p.CredentialManager.ReconcileBucketOwnerCredentials(ctx, endUserCredsName, bs.Namespace, *bucketCreateCfg.Bucket)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to reconcile s3 put object credentials")
	}

	// create the credentials to be used by the aws resource providers, not to be used by end-user
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, bs.Namespace)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to reconcile aws blob storage provider credentials")
	}

	// setup aws s3 sdk session
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	}))
	s3svc := s3.New(sess)

	// the aws access key can sometimes still not be registered in aws on first try, so loop
	var existingBuckets []*s3.Bucket
	err = wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		listOutput, err := s3svc.ListBuckets(nil)
		if err != nil {
			return false, nil
		}
		existingBuckets = listOutput.Buckets
		return true, nil
	})
	if err != nil {
		return nil, errorUtil.Wrapf(err, "timed out waiting to list s3 buckets")
	}

	// pre-create the blobstorageinstance that will be returned if everything is successful
	bsi := &providers.BlobStorageInstance{
		DeploymentDetails: &AWSs3DeploymentDetails{
			BucketName:          *bucketCreateCfg.Bucket,
			CredentialKeyID:     endUserCreds.AccessKeyID,
			CredentialSecretKey: endUserCreds.SecretAccessKey,
		},
	}

	// create bucket if it doesn't already exist, if it does exist then use the existing bucket
	var foundBucket *s3.Bucket
	for _, b := range existingBuckets {
		if *b.Name == *bucketCreateCfg.Bucket {
			foundBucket = b
			break
		}
	}
	if foundBucket != nil {
		return bsi, nil
	}
	_, err = s3svc.CreateBucket(bucketCreateCfg)
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to create s3 bucket")
	}
	return bsi, nil
}

// DeleteStorage Delete S3 bucket and credentials to add objects to it
func (p *AWSBlobStorageProvider) DeleteStorage(ctx context.Context, bs *v1alpha1.BlobStorage) error {
	// resolve bucket information for bucket created by provider
	bucketCreateCfg, stratCfg, err := p.getS3BucketConfig(ctx, bs)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to retrieve aws s3 bucket config for instance %s", bs.Name)
	}
	if bucketCreateCfg.Bucket == nil {
		bucketCreateCfg.Bucket = aws.String(fmt.Sprintf("%s-%s", bs.Namespace, bs.Name))
	}

	// get provider aws creds so the bucket can be deleted
	providerCreds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, bs.Namespace)
	if err != nil {
		return errorUtil.Wrap(err, "failed to reconcile aws provider credentials")
	}
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	}))

	// delete the bucket that was created by the provider
	s3svc := s3.New(sess)
	_, err = s3svc.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: bucketCreateCfg.Bucket,
	})
	s3err, isAWSErr := err.(awserr.Error)
	if err != nil && !isAWSErr {
		return errorUtil.Wrapf(err, "failed to delete s3 bucket %s", *bucketCreateCfg.Bucket)
	}
	if err != nil && isAWSErr {
		if s3err.Code() != s3.ErrCodeNoSuchBucket {
			return errorUtil.Wrapf(err, "failed to delete aws s3 bucket %s, aws error", *bucketCreateCfg.Bucket)
		}
	}
	err = s3svc.WaitUntilBucketNotExists(&s3.HeadBucketInput{
		Bucket: bucketCreateCfg.Bucket,
	})
	if err != nil {
		return errorUtil.Wrapf(err, "failed to wait for s3 bucket deletion, %s", *bucketCreateCfg.Bucket)
	}

	// remove the credentials request created by the provider
	putObjCredsName := fmt.Sprintf("cloud-resources-aws-s3-%s-credentials", bs.Name)
	putObjCredReq := &v1.CredentialsRequest{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      putObjCredsName,
			Namespace: bs.Namespace,
		},
	}
	if err := p.Client.Delete(ctx, putObjCredReq); err != nil {
		return errorUtil.Wrapf(err, "failed to delete credential request %s", putObjCredsName)
	}

	// remove the finalizer added by the provider
	resources.RemoveFinalizer(&bs.ObjectMeta, defaultFinalizer)
	if err := p.Client.Update(ctx, bs); err != nil {
		return errorUtil.Wrapf(err, "failed to update instance as part of finalizer reconcile")
	}
	return nil
}

func (p *AWSBlobStorageProvider) getS3BucketConfig(ctx context.Context, bs *v1alpha1.BlobStorage) (*s3.CreateBucketInput, *StrategyConfig, error) {
	stratCfg, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.BlobStorageResourceType, bs.Spec.Tier)
	if err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to read aws strategy config")
	}
	if stratCfg.Region == "" {
		stratCfg.Region = defaultRegion
	}

	// unmarshal the s3 bucket config
	s3config := &s3.CreateBucketInput{}
	if err = json.Unmarshal(stratCfg.RawStrategy, s3config); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal aws s3 configuration")
	}
	return s3config, stratCfg, nil
}
