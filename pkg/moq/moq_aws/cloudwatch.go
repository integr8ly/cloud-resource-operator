package moq_aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
)

type MockCloudWatchClient struct {
	cloudwatchiface.CloudWatchAPI
	GetMetricDataFn func(input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error)
}

func BuildMockCloudWatchClient(modifyFn func(*MockCloudWatchClient)) *MockCloudWatchClient {
	mock := &MockCloudWatchClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}

func (m *MockCloudWatchClient) GetMetricData(input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
	return m.GetMetricDataFn(input)
}

func BuildMockMetricDataResult(modifyFn func(*cloudwatch.MetricDataResult)) *cloudwatch.MetricDataResult {
	mock := &cloudwatch.MetricDataResult{
		StatusCode: aws.String(cloudwatch.StatusCodeComplete),
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}
