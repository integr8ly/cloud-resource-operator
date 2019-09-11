package aws

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	errorUtil "github.com/pkg/errors"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultProviderCredentialName = "cloud-resources-aws-credentials"

	defaultCredentialsKeyIDName     = "aws_access_key_id"
	defaultCredentialsSecretKeyName = "aws_secret_access_key"
)

var (
	operatorEntries = []v1.StatementEntry{
		{
			Effect: "Allow",
			Action: []string{
				"s3:CreateBucket",
				"s3:DeleteBucket",
				"s3:ListBucket",
				"s3:ListAllMyBuckets",
				"s3:GetObject",
				"elasticache:CreateReplicationGroup",
				"elasticache:DeleteReplicationGroup",
				"elasticache:DescribeReplicationGroups",
			},
			Resource: "*",
		},
	}
)

func buildPutBucketObjectEntries(bucket string) []v1.StatementEntry {
	return []v1.StatementEntry{
		{
			Effect: "Allow",
			Action: []string{
				"s3:*",
			},
			Resource: fmt.Sprintf("arn:aws:s3:::%s", bucket),
		},
		{
			Effect: "Allow",
			Action: []string{
				"s3:*",
			},
			Resource: fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
		},
	}
}

type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

//go:generate moq -out credentials_moq.go . CredentialManagerInterface
type CredentialManagerInterface interface {
	ReconcileProviderCredentials(ctx context.Context, ns string) (*AWSCredentials, error)
	ReconcileBucketOwnerCredentials(ctx context.Context, name, ns, bucket string) (*AWSCredentials, *v1.CredentialsRequest, error)
	ReconcileCredentials(ctx context.Context, name string, ns string, entries []v1.StatementEntry) (*v1.CredentialsRequest, *AWSCredentials, error)
}

type CredentialManager struct {
	ProviderCredentialName string
	Client                 client.Client
}

func NewCredentialManager(client client.Client) *CredentialManager {
	return &CredentialManager{
		ProviderCredentialName: defaultProviderCredentialName,
		Client:                 client,
	}
}

// Ensure the credentials the AWS provider requires are available
func (m *CredentialManager) ReconcileProviderCredentials(ctx context.Context, ns string) (*AWSCredentials, error) {
	_, creds, err := m.ReconcileCredentials(ctx, m.ProviderCredentialName, ns, operatorEntries)
	if err != nil {
		return nil, err
	}
	return creds, nil
}

func (m *CredentialManager) ReconcileBucketOwnerCredentials(ctx context.Context, name, ns, bucket string) (*AWSCredentials, *v1.CredentialsRequest, error) {
	cr, creds, err := m.ReconcileCredentials(ctx, name, ns, buildPutBucketObjectEntries(bucket))
	if err != nil {
		return nil, nil, err
	}
	return creds, cr, nil
}

func (m *CredentialManager) ReconcileCredentials(ctx context.Context, name string, ns string, entries []v1.StatementEntry) (*v1.CredentialsRequest, *AWSCredentials, error) {
	cr, err := m.reconcileCredentialRequest(ctx, name, ns, entries)
	if err != nil {
		return nil, nil, errorUtil.Wrapf(err, "failed to reconcile aws credential request %s", name)
	}
	err = wait.PollImmediate(time.Second*5, time.Minute*5, func() (done bool, err error) {
		if err = m.Client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return cr.Status.Provisioned, nil
	})
	if err != nil {
		return nil, nil, errorUtil.Wrap(err, "timed out waiting for credential request to become provisioned")
	}
	awsCreds, err := m.reconcileAWSCredentials(ctx, cr)
	if err != nil {
		return nil, nil, errorUtil.Wrapf(err, "failed to reconcile aws credentials from credential request %s", cr.Name)
	}
	return cr, awsCreds, nil
}

func (m *CredentialManager) reconcileCredentialRequest(ctx context.Context, name string, ns string, entries []v1.StatementEntry) (*v1.CredentialsRequest, error) {
	codec, err := v1.NewCodec()
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to create provider codec")
	}
	providerSpec, err := codec.EncodeProviderSpec(&v1.AWSProviderSpec{
		TypeMeta: controllerruntime.TypeMeta{
			Kind: "AWSProviderSpec",
		},
		StatementEntries: entries,
	})
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to encode provider spec")
	}
	cr := &v1.CredentialsRequest{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	controllerutil.CreateOrUpdate(ctx, m.Client, cr, func(existing runtime.Object) error {
		r := existing.(*v1.CredentialsRequest)
		r.Spec.ProviderSpec = providerSpec
		r.Spec.SecretRef = v12.ObjectReference{
			Name:      name,
			Namespace: ns,
		}
		return nil
	})
	return cr, nil
}

func (m *CredentialManager) reconcileAWSCredentials(ctx context.Context, cr *v1.CredentialsRequest) (*AWSCredentials, error) {
	sec := &v12.Secret{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: cr.Spec.SecretRef.Name, Namespace: cr.Spec.SecretRef.Namespace}, sec)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to get aws credentials secret %s", cr.Spec.SecretRef.Name)
	}
	awsAccessKeyID := string(sec.Data[defaultCredentialsKeyIDName])
	awsSecretAccessKey := string(sec.Data[defaultCredentialsSecretKeyName])
	if awsAccessKeyID == "" {
		return nil, errorUtil.New(fmt.Sprintf("aws access key id is undefined in secret %s", sec.Name))
	}
	if awsSecretAccessKey == "" {
		return nil, errorUtil.New(fmt.Sprintf("aws secret access key is undefined in secret %s", sec.Name))
	}
	return &AWSCredentials{
		AccessKeyID:     awsAccessKeyID,
		SecretAccessKey: awsSecretAccessKey,
	}, nil
}
