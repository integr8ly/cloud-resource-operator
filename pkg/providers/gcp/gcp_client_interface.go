package gcp

import (
	"context"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

type SQLAdminService interface {
	InstancesList(string) (*sqladmin.InstancesListResponse, error)
	DeleteInstance(context.Context, string, string) (*sqladmin.Operation, error)
}

// mock client
type mockSqlClient struct {
	SQLAdminService
	instancesListFn  func(string) (*sqladmin.InstancesListResponse, error)
	deleteInstanceFn func(context.Context, string, string) (*sqladmin.Operation, error)
}

func (m *mockSqlClient) InstancesList(project string) (*sqladmin.InstancesListResponse, error) {
	if m.instancesListFn != nil {
		return m.instancesListFn(project)
	}
	return &sqladmin.InstancesListResponse{
		Items: []*sqladmin.DatabaseInstance{},
	}, nil
}

func (m *mockSqlClient) DeleteInstance(ctx context.Context, projectID, instanceName string) (*sqladmin.Operation, error) {
	if m.deleteInstanceFn != nil {
		return m.deleteInstanceFn(ctx, projectID, instanceName)
	}
	return nil, nil
}

func getMockSQLClient(modifyFn func(sqlClient *mockSqlClient)) *mockSqlClient {
	mock := &mockSqlClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}
