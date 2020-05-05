package openshift

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	varPlaceholder = "REPLACE_ME"
)

type BlobStorageDeploymentDetails struct {
	data map[string]string
}

func (d *BlobStorageDeploymentDetails) Data() map[string][]byte {
	convertedData := map[string][]byte{}
	for key, value := range d.data {
		convertedData[key] = []byte(value)
	}
	return convertedData
}

var _ providers.BlobStorageProvider = (*BlobStorageProvider)(nil)

type BlobStorageProvider struct {
	Client client.Client
	Logger *logrus.Entry
}

func NewBlobStorageProvider(c client.Client, l *logrus.Entry) *BlobStorageProvider {
	return &BlobStorageProvider{
		Client: c,
		Logger: l,
	}
}

func (b BlobStorageProvider) GetName() string {
	return "openshift-blobstorage"
}

func (b BlobStorageProvider) SupportsStrategy(s string) bool {
	return providers.OpenShiftDeploymentStrategy == s
}

func (b BlobStorageProvider) GetReconcileTime(bs *v1alpha1.BlobStorage) time.Duration {
	return time.Second * 10
}

func (b BlobStorageProvider) CreateStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (*providers.BlobStorageInstance, croType.StatusMessage, error) {
	// default to an empty s3 set of credentials for now. in the future. this should determine the cloud provider being
	// used by checking the infrastructure cr.
	dd := &BlobStorageDeploymentDetails{
		data: map[string]string{
			aws.DetailsBlobStorageBucketName:          varPlaceholder,
			aws.DetailsBlobStorageCredentialSecretKey: varPlaceholder,
			aws.DetailsBlobStorageCredentialKeyID:     varPlaceholder,
			aws.DetailsBlobStorageBucketRegion:        varPlaceholder,
		},
	}

	if bs.Spec.SecretRef.Namespace == "" {
		bs.Spec.SecretRef.Namespace = bs.Namespace
	}

	secName := bs.Spec.SecretRef.Name
	if bs.Status.SecretRef != nil && bs.Status.SecretRef.Name != "" {
		secName = bs.Status.SecretRef.Name
	}
	secNamespace := bs.Spec.SecretRef.Namespace
	if bs.Status.SecretRef != nil && bs.Status.SecretRef.Namespace != "" {
		secNamespace = bs.Status.SecretRef.Namespace
	}
	sec := &v1.Secret{}
	if err := b.Client.Get(ctx, client.ObjectKey{Name: secName, Namespace: secNamespace}, sec); err != nil && !errors.IsNotFound(err) {
		return nil, "failed to reconcile", err
	}

	for key, value := range sec.Data {
		dd.data[key] = string(value)
	}

	return &providers.BlobStorageInstance{
		DeploymentDetails: dd,
	}, "reconcile complete", nil
}

func (b BlobStorageProvider) DeleteStorage(ctx context.Context, bs *v1alpha1.BlobStorage) (croType.StatusMessage, error) {
	return "deletion complete", nil
}
