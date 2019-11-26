package aws

import (
	"context"
	"errors"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockS3Svc struct {
	s3iface.S3API
	wantErrList       bool
	wantErrCreate     bool
	wantErrDelete     bool
	wantErrWaitDelete bool
	bucketNames       []string
}

func (s *mockS3Svc) ListBuckets(lbi *s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	if s.wantErrList {
		return nil, errors.New("mock aws s3 client error")
	}
	buckets := make([]*s3.Bucket, 0)
	for _, bName := range s.bucketNames {
		buckets = append(buckets, &s3.Bucket{
			Name: aws.String(bName),
		})
	}
	fmt.Println("Setting buckets", buckets)
	cbo := &s3.ListBucketsOutput{
		Buckets: buckets,
	}
	return cbo, nil
}

func (s *mockS3Svc) CreateBucket(cbi *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	if s.wantErrCreate {
		return nil, errors.New("mock aws s3 client error")
	}
	return &s3.CreateBucketOutput{}, nil
}

func (s *mockS3Svc) DeleteBucket(dbi *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	if s.wantErrDelete {
		return nil, errors.New("mock aws s3 client error")
	}
	return &s3.DeleteBucketOutput{}, nil
}

func (s *mockS3Svc) ListObjectsV2(*s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{}, nil
}

func buildTestBlobStorageCR() *v1alpha1.BlobStorage {
	return &v1alpha1.BlobStorage{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
	}
}

func TestBlobStorageProvider_reconcileBucket(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx       context.Context
		s3svc     s3iface.S3API
		bucketCfg *s3.CreateBucketInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test aws s3 bucket already exists",
			fields: fields{
				Client:            nil,
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx: context.TODO(),
				s3svc: &mockS3Svc{
					bucketNames: []string{"test"},
				},
				bucketCfg: &s3.CreateBucketInput{
					Bucket: aws.String("test"),
				},
			},
			wantErr: false,
		},
		{
			name: "test aws s3 bucket is created if doesn't exist",
			fields: fields{
				Client:            nil,
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx: context.TODO(),
				s3svc: &mockS3Svc{
					bucketNames: []string{"test"},
				},
				bucketCfg: &s3.CreateBucketInput{
					Bucket: aws.String("test2"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BlobStorageProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			if _, err := p.reconcileBucketCreate(tt.args.ctx, tt.args.s3svc, tt.args.bucketCfg); (err != nil) != tt.wantErr {
				t.Errorf("reconcileBucket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlobStorageProvider_reconcileBucketDelete(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build test scheme", err)

	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx             context.Context
		s3svc           s3iface.S3API
		bucketCfg       *s3.CreateBucketInput
		bucketDeleteCfg *S3DeleteStrat
		bs              *v1alpha1.BlobStorage
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test successful delete",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx:   context.TODO(),
				s3svc: &mockS3Svc{},
				bucketCfg: &s3.CreateBucketInput{
					Bucket: aws.String("test"),
				},
				bucketDeleteCfg: &S3DeleteStrat{
					ForceBucketDeletion: aws.Bool(false),
				},
				bs: buildTestBlobStorageCR(),
			},
			wantErr: false,
		},
		{
			name: "test error on failed bucket delete",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx: context.TODO(),
				s3svc: &mockS3Svc{
					wantErrDelete: true,
					bucketNames:   []string{"test"},
				},
				bucketCfg: &s3.CreateBucketInput{
					Bucket: aws.String("test"),
				},
				bucketDeleteCfg: &S3DeleteStrat{
					ForceBucketDeletion: aws.Bool(false),
				},
				bs: buildTestBlobStorageCR(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BlobStorageProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			if _, err := p.reconcileBucketDelete(tt.args.ctx, tt.args.bs, tt.args.s3svc, tt.args.bucketCfg, tt.args.bucketDeleteCfg); (err != nil) != tt.wantErr {
				t.Errorf("reconcileBucketDelete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlobStorageProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		b *v1alpha1.BlobStorage
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "test short reconcile when the cr is not complete",
			args: args{
				b: &v1alpha1.BlobStorage{
					Status: v1alpha1.BlobStorageStatus{
						Phase: types.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				b: &v1alpha1.BlobStorage{
					Status: v1alpha1.BlobStorageStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BlobStorageProvider{}
			if got := p.GetReconcileTime(tt.args.b); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_TagBlobStorage(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build test scheme", err)

	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx            context.Context
		bucketName     string
		bs             *v1alpha1.BlobStorage
		stratCfgRegion string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "test tagging completes",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx:            context.TODO(),
				bucketName:     "test",
				bs:             buildTestBlobStorageCR(),
				stratCfgRegion: "test",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BlobStorageProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.TagBlobStorage(tt.args.ctx, tt.args.bucketName, tt.args.bs, tt.args.stratCfgRegion)
			if (err != nil) != tt.wantErr {
				t.Errorf("TagBlobStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TagBlobStorage() got = %v, want %v", got, tt.want)
			}
		})
	}
}
