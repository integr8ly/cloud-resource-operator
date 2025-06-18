package moq_aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type MockCloudWatchClient struct {
	GetMetricDataFn func(ctx context.Context, input *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

func BuildMockCloudWatchClient(modifyFn func(*MockCloudWatchClient)) *MockCloudWatchClient {
	mock := &MockCloudWatchClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func (m *MockCloudWatchClient) GetMetricData(ctx context.Context, input *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	if m.GetMetricDataFn != nil {
		return m.GetMetricDataFn(ctx, input, optFns...)
	}
	return &cloudwatch.GetMetricDataOutput{}, nil
}

func BuildMockMetricDataResult(modifyFn func(*cloudwatchtypes.MetricDataResult)) *cloudwatchtypes.MetricDataResult {
	mock := &cloudwatchtypes.MetricDataResult{
		StatusCode: cloudwatchtypes.StatusCodeComplete,
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}
