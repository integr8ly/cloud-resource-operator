package aws

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"

	croapis "github.com/integr8ly/cloud-resource-operator/apis"
	"github.com/openshift/cloud-credential-operator/pkg/apis"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockS3Svc struct {
	s3iface.S3API
	wantErrList   bool
	wantErrCreate bool
	wantErrDelete bool
	bucketNames   []string
}

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := croapis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = configv1.Install(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestCredentialsRequest() *cloudcredentialv1.CredentialsRequest {
	return &cloudcredentialv1.CredentialsRequest{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: cloudcredentialv1.CredentialsRequestSpec{
			SecretRef: corev1.ObjectReference{
				Name:      "test",
				Namespace: "test",
			},
		},
		Status: cloudcredentialv1.CredentialsRequestStatus{
			Provisioned: true,
			ProviderStatus: &runtime.RawExtension{
				Raw: []byte("{ \"user\":\"test\", \"policy\":\"test\" }"),
			},
		},
	}
}

func (s *mockS3Svc) ListBuckets(*s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	if s.wantErrList {
		return nil, errors.New("mock aws s3 client error")
	}
	buckets := make([]*s3.Bucket, 0)
	for _, bName := range s.bucketNames {
		buckets = append(buckets, &s3.Bucket{
			Name: aws.String(bName),
		})
	}
	cbo := &s3.ListBucketsOutput{
		Buckets: buckets,
	}
	return cbo, nil
}

func (s *mockS3Svc) CreateBucket(*s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	if s.wantErrCreate {
		return nil, errors.New("mock aws s3 client error")
	}
	return &s3.CreateBucketOutput{}, nil
}

func (s *mockS3Svc) DeleteBucket(*s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	if s.wantErrDelete {
		return nil, errors.New("mock aws s3 client error")
	}
	return &s3.DeleteBucketOutput{}, nil
}

func (s *mockS3Svc) ListObjectsV2(*s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{}, nil
}

func (s *mockS3Svc) PutBucketTagging(*s3.PutBucketTaggingInput) (*s3.PutBucketTaggingOutput, error) {
	return &s3.PutBucketTaggingOutput{}, nil
}

func (s *mockS3Svc) PutPublicAccessBlock(*s3.PutPublicAccessBlockInput) (*s3.PutPublicAccessBlockOutput, error) {
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (s *mockS3Svc) PutBucketEncryption(*s3.PutBucketEncryptionInput) (*s3.PutBucketEncryptionOutput, error) {
	return &s3.PutBucketEncryptionOutput{}, nil
}

func buildTestBlobStorageCR() *v1alpha1.BlobStorage {
	return &v1alpha1.BlobStorage{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "test",
			ResourceVersion: fakeResourceVersion,
		},
	}
}

func TestBlobStorageProvider_reconcileBucket(t *testing.T) {
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
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
			dummyBlobStorage := &v1alpha1.BlobStorage{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", ResourceVersion: fakeResourceVersion}}
			if _, err := p.reconcileBucketCreate(tt.args.ctx, dummyBlobStorage, tt.args.s3svc, tt.args.bucketCfg); (err != nil) != tt.wantErr {
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest()),
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
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				b: &v1alpha1.BlobStorage{
					Status: croType.ResourceTypeStatus{
						Phase: croType.PhaseComplete,
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
		bs             *v1alpha1.BlobStorage
		s3svc          s3iface.S3API
		stratCfgRegion string
		bucketName     string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    croType.StatusMessage
		wantErr bool
	}{
		{
			name: "test tagging completes",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestBlobStorageCR(), buildTestCredentialsRequest(), buildTestInfra()),
				Logger:            logrus.WithFields(logrus.Fields{}),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
			},
			args: args{
				ctx:            context.TODO(),
				bucketName:     "test",
				bs:             buildTestBlobStorageCR(),
				stratCfgRegion: "test",
				s3svc: &mockS3Svc{
					bucketNames: []string{"test"},
				},
			},
			want:    croType.StatusMessage("successfully created and tagged"),
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
			got, err := p.TagBlobStorage(tt.args.ctx, tt.args.bucketName, tt.args.bs, tt.args.stratCfgRegion, tt.args.s3svc)
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

func TestNewAWSBlobStorageProvider(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	type args struct {
		client func() client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name    string
		args    args
		want    *BlobStorageProvider
		wantErr bool
	}{
		{
			name: "successfully create new blob storage provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: false,
		},
		{
			name: "fail to create new blob storage provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						return errors.New("generic error")
					}
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAWSBlobStorageProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSBlobStorageProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSBlobStorageProvider() got = %v, want non-nil result", got)
			}
		})
	}
}
