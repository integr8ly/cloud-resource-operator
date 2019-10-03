package openshift

import (
	"context"
	"fmt"

	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	testLogger         = logrus.WithFields(logrus.Fields{"testing": "true"})
	testRedisName      = "test-redis"
	testRedisNamespace = "test-redis"
)

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := apis.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestRedisCR() *v1alpha1.Redis {
	return &v1alpha1.Redis{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      testRedisName,
			Namespace: testRedisNamespace,
		},
		Spec:   v1alpha1.RedisSpec{},
		Status: v1alpha1.RedisStatus{},
	}
}

func buildTestDeploymentReady() *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: "test-redis",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: "True",
				},
			},
		},
	}
}

func buildTestRedisCluster() *providers.RedisCluster {
	return &providers.RedisCluster{DeploymentDetails: &providers.RedisDeploymentDetails{
		URI:  fmt.Sprintf("%s.%s.svc.cluster.local", testRedisName, testRedisNamespace),
		Port: redisPort}}
}

func buildDefaultConfigManager() *ConfigManagerMock {
	return &ConfigManagerMock{
		ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (config *StrategyConfig, e error) {
			return &StrategyConfig{RawStrategy: []byte("{}")}, nil
		},
	}
}

func TestOpenShiftRedisProvider_SupportsStrategy(t *testing.T) {
	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		d string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "test openshift strategy is supported",
			fields: fields{
				Client:        nil,
				Logger:        testLogger,
				ConfigManager: &ConfigManagerMock{},
			},
			args: args{d: providers.OpenShiftDeploymentStrategy},
			want: true,
		},
		{
			name: "test aws strategy is not supported",
			fields: fields{
				Client:        nil,
				Logger:        logrus.WithFields(logrus.Fields{}),
				ConfigManager: nil,
			},
			args: args{d: providers.AWSDeploymentStrategy},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftRedisProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
			}
			if got := p.SupportsStrategy(tt.args.d); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenShiftRedisProvider_CreateRedis(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		ctx   context.Context
		redis *v1alpha1.Redis
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.RedisCluster
		wantErr bool
	}{
		{
			name: "test successful creation",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestRedisCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
			},
			args: args{
				ctx:   context.TODO(),
				redis: buildTestRedisCR(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test successful creation with deployment ready",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestDeploymentReady(), buildTestRedisCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
			},
			args: args{
				ctx:   context.TODO(),
				redis: buildTestRedisCR(),
			},
			want:    buildTestRedisCluster(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftRedisProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
			}
			got, _, err := p.CreateRedis(tt.args.ctx, tt.args.redis)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRedis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateRedis() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenShiftRedisProvider_DeleteRedis(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		ctx   context.Context
		redis *v1alpha1.Redis
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test successful deletion",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestDeploymentReady(), buildTestRedisCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
			},
			args: args{
				ctx:   context.TODO(),
				redis: buildTestRedisCR(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftRedisProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
			}
			if _, err := p.DeleteRedis(tt.args.ctx, tt.args.redis); (err != nil) != tt.wantErr {
				t.Errorf("DeleteRedis() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
