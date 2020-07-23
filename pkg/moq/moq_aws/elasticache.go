package moq_aws

import (
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
)

type MockElastiCacheClient struct {
	elasticacheiface.ElastiCacheAPI
	DescribeReplicationGroupsFn func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error)
}

func BuildMockElastiCacheClient(modifyFn func(client *MockElastiCacheClient)) *MockElastiCacheClient {
	mock := &MockElastiCacheClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func (m *MockElastiCacheClient) DescribeReplicationGroups(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
	return m.DescribeReplicationGroupsFn(input)
}
