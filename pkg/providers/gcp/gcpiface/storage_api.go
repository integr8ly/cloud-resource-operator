package gcpiface

import (
	"cloud.google.com/go/iam"
	"cloud.google.com/go/storage"
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type StorageAPI interface {
	CreateBucket(ctx context.Context, bucket, projectID string, attrs *storage.BucketAttrs) error
	DeleteBucket(ctx context.Context, bucket string) error
	SetBucketPolicy(ctx context.Context, bucket, identity, role string) error
	ListObjects(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error)
	GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error)
	DeleteObject(ctx context.Context, bucket, object string) error
}

type storageClient struct {
	StorageAPI
	storageService *storage.Client
	logger         *logrus.Entry
}

func NewStorageAPI(ctx context.Context, opt option.ClientOption, logger *logrus.Entry) (StorageAPI, error) {
	cloudStorageClient, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &storageClient{
		storageService: cloudStorageClient,
		logger:         logger,
	}, nil
}

func (c *storageClient) CreateBucket(ctx context.Context, bucket, projectID string, attrs *storage.BucketAttrs) error {
	c.logger.Infof("creating bucket %q", bucket)
	bucketHandle := c.storageService.Bucket(bucket)
	err := bucketHandle.Create(ctx, projectID, attrs)
	if err != nil {
		return err
	}
	return nil
}

func (c *storageClient) DeleteBucket(ctx context.Context, bucket string) error {
	c.logger.Infof("deleting bucket %q", bucket)
	bucketHandle := c.storageService.Bucket(bucket)
	err := bucketHandle.Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (c *storageClient) SetBucketPolicy(ctx context.Context, bucket, identity, role string) error {
	c.logger.Infof("setting policy on bucket %q", bucket)
	bucketHandle := c.storageService.Bucket(bucket)
	policy, err := bucketHandle.IAM().Policy(ctx)
	if err != nil {
		return err
	}
	policy.Add(identity, iam.RoleName(role))
	if err = bucketHandle.IAM().SetPolicy(ctx, policy); err != nil {
		return err
	}
	return nil
}

func (c *storageClient) ListObjects(ctx context.Context, bucket string, query *storage.Query) ([]*storage.ObjectAttrs, error) {
	c.logger.Infof("listing objects from bucket %q", bucket)
	objectIterator := c.storageService.Bucket(bucket).Objects(ctx, query)
	var objects []*storage.ObjectAttrs
	for {
		oa, err := objectIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		objects = append(objects, oa)
	}
	return objects, nil
}

func (c *storageClient) GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
	c.logger.Infof("fetching object %q from bucket %q", object, bucket)
	objectHandle := c.storageService.Bucket(bucket).Object(object)
	attrs, err := objectHandle.Attrs(ctx)
	if err != nil {
		return nil, err
	}
	return attrs, nil
}

func (c *storageClient) DeleteObject(ctx context.Context, bucket, object string) error {
	c.logger.Infof("deleting object %q from bucket %q", object, bucket)
	objectHandle := c.storageService.Bucket(bucket).Object(object)
	err := objectHandle.Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}
