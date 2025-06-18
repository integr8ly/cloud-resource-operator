package moq_aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
)

type MockElastiCacheClient struct {
	DescribeReplicationGroupsFn func(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error)
}

func BuildMockElastiCacheClient(modifyFn func(client *MockElastiCacheClient)) *MockElastiCacheClient {
	mock := &MockElastiCacheClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func (m *MockElastiCacheClient) DescribeReplicationGroups(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error) {
	if m.DescribeReplicationGroupsFn != nil {
		return m.DescribeReplicationGroupsFn(ctx, input, optFns...)
	}
	return &elasticache.DescribeReplicationGroupsOutput{}, nil
}
