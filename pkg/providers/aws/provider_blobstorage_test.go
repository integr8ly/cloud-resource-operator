package aws

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"

	croapis "github.com/integr8ly/cloud-resource-operator/api"
	"github.com/openshift/cloud-credential-operator/pkg/apis"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/mock"
)

type MockS3Client struct {
	s3.Client
	mock.Mock
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

// ListBuckets provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.ListBucketsOutput), args.Error(1)
}

// CreateBucket provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.CreateBucketOutput), args.Error(1)
}

// DeleteBucket provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.DeleteBucketOutput), args.Error(1)
}

// PutBucketTagging provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) PutBucketTagging(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.PutBucketTaggingOutput), args.Error(1)
}

// PutPublicAccessBlock provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.PutPublicAccessBlockOutput), args.Error(1)
}

// PutBucketEncryption provides a mock function with given fields: ctx, params, optFns
func (m *MockS3Client) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.PutBucketEncryptionOutput), args.Error(1)
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
		s3Client  *s3.Client
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
				s3Client: func() *s3.Client {
					mockS3 := new(MockS3Client)
					mockS3.On("ListBuckets", mock.Anything, mock.Anything, mock.Anything).Return(&s3.ListBucketsOutput{
						Buckets: []types.Bucket{
							{Name: aws.String("test")},
						},
					}, nil)
					return (*s3.Client)(unsafe.Pointer(mockS3))
				}(),
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
				s3Client: func() *s3.Client {
					mockS3 := new(MockS3Client)
					mockS3.On("ListBuckets", mock.Anything, mock.Anything, mock.Anything).Return(&s3.ListBucketsOutput{
						Buckets: []types.Bucket{
							{Name: aws.String("test")},
						},
					}, nil)
					return (*s3.Client)(unsafe.Pointer(mockS3))
				}(),
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
			if _, err := p.reconcileBucketCreate(tt.args.ctx, dummyBlobStorage, tt.args.s3Client, tt.args.bucketCfg); (err != nil) != tt.wantErr {
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
		s3Client        *s3.Client
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
				ctx: context.TODO(),
				s3Client: func() *s3.Client {
					mockS3 := new(MockS3Client)
					mockS3.On("DeleteBucket", mock.Anything, mock.Anything, mock.Anything).Return(&s3.DeleteBucketOutput{}, nil)
					return &mockS3.Client
				}(),
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
				s3Client: func() *s3.Client {
					mockS3 := new(MockS3Client)
					mockS3.On("DeleteBucket", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("mock aws s3 client error"))
					return &mockS3.Client
				}(),
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
			if _, err := p.reconcileBucketDelete(tt.args.ctx, tt.args.bs, tt.args.s3Client, tt.args.bucketCfg, tt.args.bucketDeleteCfg); (err != nil) != tt.wantErr {
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
		s3Client       *s3.Client
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
				s3Client: func() *s3.Client {
					mockS3 := new(MockS3Client)
					mockS3.On("PutBucketTagging", mock.Anything, mock.Anything, mock.Anything).Return(&s3.PutBucketTaggingOutput{}, nil)
					return &mockS3.Client
				}(),
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
			got, err := p.TagBlobStorage(tt.args.ctx, tt.args.bucketName, tt.args.bs, tt.args.stratCfgRegion, tt.args.s3Client)
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
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
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
