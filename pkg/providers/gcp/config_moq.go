// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package gcp

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"sync"
)

// Ensure, that ConfigManagerMock does implement ConfigManager.
// If this is not the case, regenerate this file with moq.
var _ ConfigManager = &ConfigManagerMock{}

// ConfigManagerMock is a mock implementation of ConfigManager.
//
//	func TestSomethingThatUsesConfigManager(t *testing.T) {
//
//		// make and configure a mocked ConfigManager
//		mockedConfigManager := &ConfigManagerMock{
//			ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
//				panic("mock out the ReadStorageStrategy method")
//			},
//		}
//
//		// use mockedConfigManager in code that requires ConfigManager
//		// and then make assertions.
//
//	}
type ConfigManagerMock struct {
	// ReadStorageStrategyFunc mocks the ReadStorageStrategy method.
	ReadStorageStrategyFunc func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error)

	// calls tracks calls to the methods.
	calls struct {
		// ReadStorageStrategy holds details about calls to the ReadStorageStrategy method.
		ReadStorageStrategy []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Rt is the rt argument value.
			Rt providers.ResourceType
			// Tier is the tier argument value.
			Tier string
		}
	}
	lockReadStorageStrategy sync.RWMutex
}

// ReadStorageStrategy calls ReadStorageStrategyFunc.
func (mock *ConfigManagerMock) ReadStorageStrategy(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
	if mock.ReadStorageStrategyFunc == nil {
		panic("ConfigManagerMock.ReadStorageStrategyFunc: method is nil but ConfigManager.ReadStorageStrategy was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		Rt   providers.ResourceType
		Tier string
	}{
		Ctx:  ctx,
		Rt:   rt,
		Tier: tier,
	}
	mock.lockReadStorageStrategy.Lock()
	mock.calls.ReadStorageStrategy = append(mock.calls.ReadStorageStrategy, callInfo)
	mock.lockReadStorageStrategy.Unlock()
	return mock.ReadStorageStrategyFunc(ctx, rt, tier)
}

// ReadStorageStrategyCalls gets all the calls that were made to ReadStorageStrategy.
// Check the length with:
//
//	len(mockedConfigManager.ReadStorageStrategyCalls())
func (mock *ConfigManagerMock) ReadStorageStrategyCalls() []struct {
	Ctx  context.Context
	Rt   providers.ResourceType
	Tier string
} {
	var calls []struct {
		Ctx  context.Context
		Rt   providers.ResourceType
		Tier string
	}
	mock.lockReadStorageStrategy.RLock()
	calls = mock.calls.ReadStorageStrategy
	mock.lockReadStorageStrategy.RUnlock()
	return calls
}