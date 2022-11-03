package gcpiface

import (
	"context"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

type SQLAdminService interface {
	InstancesList(string) (*sqladmin.InstancesListResponse, error)
	DeleteInstance(context.Context, string, string) (*sqladmin.Operation, error)
	CreateInstance(context.Context, string, *sqladmin.DatabaseInstance) (*sqladmin.Operation, error)
	ModifyInstance(context.Context, string, string, *sqladmin.DatabaseInstance) (*sqladmin.Operation, error)
}

// MockSqlClient mock client
type MockSqlClient struct {
	SQLAdminService
	InstancesListFn  func(string) (*sqladmin.InstancesListResponse, error)
	DeleteInstanceFn func(context.Context, string, string) (*sqladmin.Operation, error)
	CreateInstanceFn func(context.Context, string, *sqladmin.DatabaseInstance) (*sqladmin.InstancesInsertCall, error)
	ModifyInstanceFn func(context.Context, string, string, *sqladmin.DatabaseInstance) (*sqladmin.DatabaseInstance, error)
}

func (m *MockSqlClient) InstancesList(project string) (*sqladmin.InstancesListResponse, error) {
	if m.InstancesListFn != nil {
		return m.InstancesListFn(project)
	}
	return &sqladmin.InstancesListResponse{
		Items: []*sqladmin.DatabaseInstance{},
	}, nil
}

func (m *MockSqlClient) DeleteInstance(ctx context.Context, projectID, instanceName string) (*sqladmin.Operation, error) {
	if m.DeleteInstanceFn != nil {
		return m.DeleteInstanceFn(ctx, projectID, instanceName)
	}
	return nil, nil
}

func (m *MockSqlClient) CreateInstance(ctx context.Context, projectID string, instance *sqladmin.DatabaseInstance) (*sqladmin.InstancesInsertCall, error) {
	if m.CreateInstanceFn != nil {
		return m.CreateInstanceFn(ctx, projectID, instance)
	}
	return nil, nil

}

func (m *MockSqlClient) ModifyInstance(ctx context.Context, projectID string, instance *sqladmin.DatabaseInstance) (*sqladmin.DatabaseInstance, error) {
	if m.ModifyInstanceFn != nil {
		return m.ModifyInstanceFn(ctx, projectID, instance.Name, instance)
	}
	return nil, nil
}

func GetMockSQLClient(modifyFn func(sqlClient *MockSqlClient)) *MockSqlClient {
	mock := &MockSqlClient{}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return mock
}
