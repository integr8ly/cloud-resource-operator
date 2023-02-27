package gcpiface

import (
	redis "cloud.google.com/go/redis/apiv1"
	"context"
	"github.com/googleapis/gax-go/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	redispb "google.golang.org/genproto/googleapis/cloud/redis/v1"
)

type RedisAPI interface {
	DeleteInstance(context.Context, *redispb.DeleteInstanceRequest, ...gax.CallOption) (*redis.DeleteInstanceOperation, error)
	CreateInstance(context.Context, *redispb.CreateInstanceRequest, ...gax.CallOption) (*redis.CreateInstanceOperation, error)
	GetInstance(context.Context, *redispb.GetInstanceRequest, ...gax.CallOption) (*redispb.Instance, error)
	UpdateInstance(context.Context, *redispb.UpdateInstanceRequest, ...gax.CallOption) (*redis.UpdateInstanceOperation, error)
	UpgradeInstance(context.Context, *redispb.UpgradeInstanceRequest, ...gax.CallOption) (*redis.UpgradeInstanceOperation, error)
	RescheduleMaintenance(context.Context, *redispb.RescheduleMaintenanceRequest, ...gax.CallOption) (*redis.RescheduleMaintenanceOperation, error)
}

type redisClient struct {
	RedisAPI
	redisService *redis.CloudRedisClient
	logger       *logrus.Entry
}

func NewRedisAPI(ctx context.Context, opt option.ClientOption, logger *logrus.Entry) (RedisAPI, error) {
	cloudRedisClient, err := redis.NewCloudRedisClient(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &redisClient{
		redisService: cloudRedisClient,
		logger:       logger,
	}, nil
}

func (c *redisClient) DeleteInstance(ctx context.Context, req *redispb.DeleteInstanceRequest, opts ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
	c.logger.Infof("deleting gcp redis instance %s", req.Name)
	return c.redisService.DeleteInstance(ctx, req, opts...)
}

func (c *redisClient) CreateInstance(ctx context.Context, req *redispb.CreateInstanceRequest, opts ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
	c.logger.Infof("creating gcp redis instance %s", req.Instance.Name)
	return c.redisService.CreateInstance(ctx, req, opts...)
}

func (c *redisClient) GetInstance(ctx context.Context, req *redispb.GetInstanceRequest, opts ...gax.CallOption) (*redispb.Instance, error) {
	c.logger.Infof("fetching gcp redis instance %s", req.Name)
	instance, err := c.redisService.GetInstance(ctx, req, opts...)
	if instance != nil {
		c.logger.Infof("found gcp redis instance %s", req.Name)
	}
	return instance, err
}

func (c *redisClient) UpdateInstance(ctx context.Context, req *redispb.UpdateInstanceRequest, opts ...gax.CallOption) (*redis.UpdateInstanceOperation, error) {
	c.logger.Infof("updating gcp redis instance %s", req.Instance.Name)
	return c.redisService.UpdateInstance(ctx, req, opts...)
}

func (c *redisClient) UpgradeInstance(ctx context.Context, req *redispb.UpgradeInstanceRequest, opts ...gax.CallOption) (*redis.UpgradeInstanceOperation, error) {
	c.logger.Infof("upgrading gcp redis instance %s", req.Name)
	return c.redisService.UpgradeInstance(ctx, req, opts...)
}

func (c *redisClient) RescheduleMaintenance(ctx context.Context, req *redispb.RescheduleMaintenanceRequest, opts ...gax.CallOption) (*redis.RescheduleMaintenanceOperation, error) {
	c.logger.Infof("upgrading gcp redis instance %s", req.Name)
	return c.redisService.RescheduleMaintenance(ctx, req, opts...)
}

type MockRedisClient struct {
	RedisAPI
	DeleteInstanceFn        func(context.Context, *redispb.DeleteInstanceRequest, ...gax.CallOption) (*redis.DeleteInstanceOperation, error)
	CreateInstanceFn        func(context.Context, *redispb.CreateInstanceRequest, ...gax.CallOption) (*redis.CreateInstanceOperation, error)
	GetInstanceFn           func(context.Context, *redispb.GetInstanceRequest, ...gax.CallOption) (*redispb.Instance, error)
	UpdateInstanceFn        func(context.Context, *redispb.UpdateInstanceRequest, ...gax.CallOption) (*redis.UpdateInstanceOperation, error)
	UpgradeInstanceFn       func(context.Context, *redispb.UpgradeInstanceRequest, ...gax.CallOption) (*redis.UpgradeInstanceOperation, error)
	RescheduleMaintenanceFn func(context.Context, *redispb.RescheduleMaintenanceRequest, ...gax.CallOption) (*redis.RescheduleMaintenanceOperation, error)
}

func GetMockRedisClient(modifyFn func(redisClient *MockRedisClient)) *MockRedisClient {
	mock := &MockRedisClient{
		DeleteInstanceFn: func(ctx context.Context, request *redispb.DeleteInstanceRequest, opts ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
			return &redis.DeleteInstanceOperation{}, nil
		},
		CreateInstanceFn: func(ctx context.Context, request *redispb.CreateInstanceRequest, opts ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
			return &redis.CreateInstanceOperation{}, nil
		},
		GetInstanceFn: func(ctx context.Context, request *redispb.GetInstanceRequest, opts ...gax.CallOption) (*redispb.Instance, error) {
			return &redispb.Instance{}, nil
		},
		UpdateInstanceFn: func(ctx context.Context, request *redispb.UpdateInstanceRequest, opts ...gax.CallOption) (*redis.UpdateInstanceOperation, error) {
			return &redis.UpdateInstanceOperation{}, nil
		},
		UpgradeInstanceFn: func(ctx context.Context, request *redispb.UpgradeInstanceRequest, opts ...gax.CallOption) (*redis.UpgradeInstanceOperation, error) {
			return &redis.UpgradeInstanceOperation{}, nil
		},
		RescheduleMaintenanceFn: func(ctx context.Context, request *redispb.RescheduleMaintenanceRequest, opts ...gax.CallOption) (*redis.RescheduleMaintenanceOperation, error) {
			return &redis.RescheduleMaintenanceOperation{}, nil
		},
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func (m *MockRedisClient) DeleteInstance(ctx context.Context, req *redispb.DeleteInstanceRequest, opts ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
	if m.DeleteInstanceFn != nil {
		return m.DeleteInstanceFn(ctx, req, opts...)
	}
	return &redis.DeleteInstanceOperation{}, nil
}

func (m *MockRedisClient) CreateInstance(ctx context.Context, req *redispb.CreateInstanceRequest, opts ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
	if m.CreateInstanceFn != nil {
		return m.CreateInstanceFn(ctx, req, opts...)
	}
	return &redis.CreateInstanceOperation{}, nil
}

func (m *MockRedisClient) GetInstance(ctx context.Context, req *redispb.GetInstanceRequest, opts ...gax.CallOption) (*redispb.Instance, error) {
	if m.GetInstanceFn != nil {
		return m.GetInstanceFn(ctx, req, opts...)
	}
	return &redispb.Instance{}, nil
}

func (m *MockRedisClient) UpdateInstance(ctx context.Context, req *redispb.UpdateInstanceRequest, opts ...gax.CallOption) (*redis.UpdateInstanceOperation, error) {
	if m.UpdateInstanceFn != nil {
		return m.UpdateInstanceFn(ctx, req, opts...)
	}
	return &redis.UpdateInstanceOperation{}, nil
}

func (m *MockRedisClient) UpgradeInstance(ctx context.Context, req *redispb.UpgradeInstanceRequest, opts ...gax.CallOption) (*redis.UpgradeInstanceOperation, error) {
	if m.UpgradeInstanceFn != nil {
		return m.UpgradeInstanceFn(ctx, req, opts...)
	}
	return &redis.UpgradeInstanceOperation{}, nil
}

func (m *MockRedisClient) RescheduleMaintenance(ctx context.Context, req *redispb.RescheduleMaintenanceRequest, opts ...gax.CallOption) (*redis.RescheduleMaintenanceOperation, error) {
	if m.RescheduleMaintenanceFn != nil {
		return m.RescheduleMaintenanceFn(ctx, req, opts...)
	}
	return &redis.RescheduleMaintenanceOperation{}, nil
}
