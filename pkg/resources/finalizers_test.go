package resources

import (
	"testing"

	controllerruntime "sigs.k8s.io/controller-runtime"
)

func TestAddFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		existingFinalizers []string
		finalizer          string
		expectedLength     int
	}{
		{
			name:               "test finalizer is appended when identical one doesn't exist",
			existingFinalizers: []string{},
			finalizer:          "test",
			expectedLength:     1,
		},
		{
			name:               "test finalizer is not appended when identical one already exists",
			existingFinalizers: []string{"test"},
			finalizer:          "test",
			expectedLength:     1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			om := &controllerruntime.ObjectMeta{
				Finalizers: tc.existingFinalizers,
			}
			AddFinalizer(om, tc.finalizer)
			if len(om.GetFinalizers()) != tc.expectedLength {
				t.Fatalf("unexpected finalizer length, expected %d but got %d", tc.expectedLength, len(om.GetFinalizers()))
			}
		})
	}
}

func TestRemoveFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		existingFinalizers []string
		finalizer          string
		expectedLength     int
	}{
		{
			name:               "test removing non-existent finalizer does nothing",
			existingFinalizers: []string{},
			finalizer:          "test",
			expectedLength:     0,
		},
		{
			name:               "test removing existing finalizer",
			existingFinalizers: []string{"test"},
			finalizer:          "test",
			expectedLength:     0,
		},
		{
			name:               "test removing existing finalizer from collection keeps other finalizers",
			existingFinalizers: []string{"test", "test2"},
			finalizer:          "test",
			expectedLength:     1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			om := &controllerruntime.ObjectMeta{
				Finalizers: tc.existingFinalizers,
			}
			RemoveFinalizer(om, tc.finalizer)
			if len(om.GetFinalizers()) != tc.expectedLength {
				t.Fatalf("unexpected finalizer length, expected %d but got %d", tc.expectedLength, len(om.GetFinalizers()))
			}
		})
	}
}

func TestHasFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		existingFinalizers []string
		finalizer          string
		expectedResult     bool
	}{
		{
			name:               "test returns true when finalizer is present",
			existingFinalizers: []string{"test"},
			finalizer:          "test",
			expectedResult:     true,
		},
		{
			name:               "test returns false when finalizer isn't present",
			existingFinalizers: []string{"test"},
			finalizer:          "test2",
			expectedResult:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			om := &controllerruntime.ObjectMeta{
				Finalizers: tc.existingFinalizers,
			}
			if HasFinalizer(om, tc.finalizer) != tc.expectedResult {
				t.Fatalf("unexpected result, expected %t but got %t", tc.expectedResult, HasFinalizer(om, tc.finalizer))
			}
		})
	}
}
