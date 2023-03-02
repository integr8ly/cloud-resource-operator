package gcpiface

import (
	"cloud.google.com/go/monitoring/apiv3"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"context"
	"errors"
	"fmt"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type MetricApi interface {
	ListTimeSeries(context.Context, *monitoringpb.ListTimeSeriesRequest, ...gax.CallOption) ([]*monitoringpb.TimeSeries, error)
}

type metricClient struct {
	MetricApi
	metricService *monitoring.MetricClient
	logger        *logrus.Entry
}

func NewMetricAPI(ctx context.Context, opt option.ClientOption, logger *logrus.Entry) (MetricApi, error) {
	cloudMetricClient, err := monitoring.NewMetricClient(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &metricClient{
		metricService: cloudMetricClient,
		logger:        logger,
	}, nil
}

func (c *metricClient) ListTimeSeries(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest, opts ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
	c.logger.Infof("listing time series with filter '%s'", req.Filter)
	timeSeriesIterator := c.metricService.ListTimeSeries(ctx, req, opts...)
	var timeSeries []*monitoringpb.TimeSeries
	for {
		ts, err := timeSeriesIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			var ae *apierror.APIError
			if errors.As(err, &ae) {
				return nil, err
			}
			return nil, err
		}
		timeSeries = append(timeSeries, ts)
	}
	if len(timeSeries) == 0 {
		return nil, fmt.Errorf("could not find any time series")
	}
	return timeSeries, nil
}
