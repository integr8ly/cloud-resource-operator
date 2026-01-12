package aws

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// --- Fakes ---

type fakeS3ListBucketsErr struct{}

func (f fakeS3ListBucketsErr) CreateBucket(ctx context.Context, input *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return &s3.CreateBucketOutput{}, nil
}
func (f fakeS3ListBucketsErr) DeleteBucket(ctx context.Context, input *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	return &s3.DeleteBucketOutput{}, nil
}
func (f fakeS3ListBucketsErr) ListBuckets(ctx context.Context, input *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return nil, fmt.Errorf("boom")
}
func (f fakeS3ListBucketsErr) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}
func (f fakeS3ListBucketsErr) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{}, nil
}
func (f fakeS3ListBucketsErr) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}
func (f fakeS3ListBucketsErr) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{}, nil
}
func (f fakeS3ListBucketsErr) PutBucketTagging(ctx context.Context, input *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	return &s3.PutBucketTaggingOutput{}, nil
}
func (f fakeS3ListBucketsErr) DeleteObjects(ctx context.Context, input *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	return &s3.DeleteObjectsOutput{}, nil
}
func (f fakeS3ListBucketsErr) PutPublicAccessBlock(ctx context.Context, input *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	return &s3.PutPublicAccessBlockOutput{}, nil
}
func (f fakeS3ListBucketsErr) PutBucketEncryption(ctx context.Context, input *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	return &s3.PutBucketEncryptionOutput{}, nil
}

type fakeClientDeleteErr struct{ client.Client }

func (f fakeClientDeleteErr) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return fmt.Errorf("delete failed")
}

type fakeClientUpdateErr struct{ client.Client }

func (f fakeClientUpdateErr) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}
func (f fakeClientUpdateErr) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return fmt.Errorf("update failed")
}

// --- Tests ---

func Test_reconcileBucketCreate_ListBucketsError(t *testing.T) {
	p := &BlobStorageProvider{
		Logger: logrus.New().WithField("test", true),
	}
	bs := &v1alpha1.BlobStorage{}
	cfg := &s3.CreateBucketInput{Bucket: aws.String("my-bucket")}

	status, err := p.reconcileBucketCreate(context.TODO(), bs, fakeS3ListBucketsErr{}, cfg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	want := "failed to list existing aws s3 buckets, credentials could be reconciling"
	if string(status) != want {
		t.Fatalf("unexpected status. got=%q want=%q", status, want)
	}
}

func Test_removeCredsAndFinalizer_DeleteError(t *testing.T) {
	p := &BlobStorageProvider{
		Client: fakeClientDeleteErr{},
		Logger: logrus.New().WithField("test", true),
	}
	bs := &v1alpha1.BlobStorage{}
	cfg := &s3.CreateBucketInput{Bucket: aws.String("my-bucket")}
	del := &S3DeleteStrat{}

	err := p.removeCredsAndFinalizer(context.TODO(), bs, fakeS3ListBucketsErr{}, cfg, del)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := err.Error(); got == "" || !containsSubstr(got, "failed to delete credential request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_removeCredsAndFinalizer_UpdateError(t *testing.T) {
	p := &BlobStorageProvider{
		Client: fakeClientUpdateErr{},
		Logger: logrus.New().WithField("test", true),
	}
	bs := &v1alpha1.BlobStorage{}
	cfg := &s3.CreateBucketInput{Bucket: aws.String("my-bucket")}
	del := &S3DeleteStrat{}

	err := p.removeCredsAndFinalizer(context.TODO(), bs, fakeS3ListBucketsErr{}, cfg, del)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := err.Error(); got == "" || !containsSubstr(got, "failed to update blob storage cr as part of finalizer reconcile") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// helper
func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	// simple implementation to avoid pulling strings package for a tiny helper
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
