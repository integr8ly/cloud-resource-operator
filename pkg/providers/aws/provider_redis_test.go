package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	v1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"
)

var (
	testLogger = logrus.WithFields(logrus.Fields{"testing": "true"})
)

type mockElasticacheClient struct {
	elasticacheiface.ElastiCacheAPI
}

// mock elasticache DescribeReplicationGroups output
func (m *mockElasticacheClient) DescribeReplicationGroups(*elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	return &elasticache.DescribeReplicationGroupsOutput{}, nil
}

// mock elasticache CreateReplicationGroup output
func (m *mockElasticacheClient) CreateReplicationGroup(*elasticache.CreateReplicationGroupInput) (*elasticache.CreateReplicationGroupOutput, error) {
	return &elasticache.CreateReplicationGroupOutput{}, nil
}

func TestAWSRedisProvider_newProvider(t *testing.T) {
	cases := []struct {
		name           string
		client         client.Client
		strategy       string
		expectedResult bool
	}{
		{
			"test supported strategy",
			fake.NewFakeClient(),
			"aws",
			true,
		},
		{
			"test unsupported strategy",
			fake.NewFakeClient(),
			"azure",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewAWSRedisProvider(tc.client, testLogger)
			supportsStrategy := p.SupportsStrategy(tc.strategy)
			if supportsStrategy != tc.expectedResult {
				t.Fatalf("unexpected outcome, expected %t but got %t", tc.expectedResult, supportsStrategy)
			}
		})
	}
}

func TestAWSRedisProvider_createRedisCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	err = v1alpha1.SchemeBuilder.AddToScheme(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	sc := &StrategyConfig{
		Region:      "eu-west-1",
		RawStrategy: json.RawMessage("{}"),
	}
	rawStratCfg, err := json.Marshal(sc)
	if err != nil {
		t.Fatal("failed to marshal strategy config", err)
	}

	cases := []struct {
		name           string
		instance       *v1alpha1.Redis
		client         client.Client
		configMgr      *ConfigManagerMock
		credentialMgr  *CredentialManagerMock
		expectedError  error
		expectedResult *providers.RedisCluster
	}{
		{
			name: "test redis cluster is created as expected",
			client: fake.NewFakeClientWithScheme(scheme, &v1alpha1.Redis{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			}, &corev1.ConfigMap{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "cloud-resources-aws-strategies",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"redis": fmt.Sprintf("{\"test\": %s}", string(rawStratCfg)),
				},
			}),
			instance: &v1alpha1.Redis{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			configMgr: &ConfigManagerMock{
				ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (config *StrategyConfig, e error) {
					return sc, nil
				},
			},
			expectedError:  nil,
			expectedResult: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &AWSRedisProvider{
				Client:            tc.client,
				CredentialManager: tc.credentialMgr,
				ConfigManager:     tc.configMgr,
			}
			redisConfig, _, err := p.getRedisConfig(context.TODO(), tc.instance)
			if err != nil {
				t.Fatal("", err)
			}

			mockSvc := &mockElasticacheClient{}
			redis, err := createRedisCluster(mockSvc, redisConfig)
			if redis != tc.expectedResult {
				t.Fatalf("unexpected outcome, expected %s but got %s", tc.expectedResult, redis)
			}
			if err != tc.expectedError {
				t.Fatalf("unexpected error, expected %s but got %s", tc.expectedError, redis)
			}
		})
	}
}

func TestAWSRedisProvider_deleteRedisCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	err = v1alpha1.SchemeBuilder.AddToScheme(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	sc := &StrategyConfig{
		Region:      "eu-west-1",
		RawStrategy: json.RawMessage("{}"),
	}
	rawStratCfg, err := json.Marshal(sc)
	if err != nil {
		t.Fatal("failed to marshal strategy config", err)
	}

	cases := []struct {
		name          string
		instance      *v1alpha1.Redis
		client        client.Client
		configMgr     *ConfigManagerMock
		credentialMgr *CredentialManagerMock
		expectedError error
	}{
		{
			name: "test redis cluster is deleted as expected",
			client: fake.NewFakeClientWithScheme(scheme, &v1alpha1.Redis{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			}, &corev1.ConfigMap{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "cloud-resources-aws-strategies",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"redis": fmt.Sprintf("{\"test\": %s}", string(rawStratCfg)),
				},
			}),
			instance: &v1alpha1.Redis{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			configMgr: &ConfigManagerMock{
				ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (config *StrategyConfig, e error) {
					return sc, nil
				},
			},
			credentialMgr: &CredentialManagerMock{
				ReoncileBucketOwnerCredentialsFunc: nil,
				ReconcileCredentialsFunc: func(ctx context.Context, name string, ns string, entries []v1.StatementEntry) (request *v1.CredentialsRequest, credentials *AWSCredentials, e error) {
					return &v1.CredentialsRequest{}, &AWSCredentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
				},
				ReconcileProviderCredentialsFunc: func(ctx context.Context, ns string) (credentials *AWSCredentials, e error) {
					return &AWSCredentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
				},
			},
			expectedError: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &AWSRedisProvider{
				Client:            tc.client,
				CredentialManager: tc.credentialMgr,
				ConfigManager:     tc.configMgr,
			}

			ctx := context.TODO()
			redisConfig, _, err := p.getRedisConfig(ctx, tc.instance)
			if err != nil {
				t.Fatal("", err)
			}

			mockSvc := &mockElasticacheClient{}
			err = p.deleteRedisCluster(mockSvc, redisConfig, ctx, tc.instance)
			if err != tc.expectedError {
				t.Fatalf("unexpected error, expected %s but got %s", tc.expectedError, err)
			}
		})
	}
}
