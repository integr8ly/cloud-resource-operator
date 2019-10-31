package resources

import (
	"os"
	"testing"
	"time"
)

func TestGetForcedReconcileTimeOrDefault(t *testing.T) {
	type args struct {
		defaultTo time.Duration
	}
	var tests = []struct {
		name                  string
		want                  time.Duration
		envForceReconcileTime string
		args                  args
	}{
		{
			name: "test function returns default",
			args: args{
				defaultTo: time.Second * 60,
			},
			want: time.Second * 60,
		},
		{
			name: "test accepts env var and returns value",
			args: args{
				defaultTo: time.Second * 60,
			},
			envForceReconcileTime: "30",
			want:                  time.Nanosecond * 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envForceReconcileTime != "" {
				if err := os.Setenv(EnvForceReconcileTimeout, tt.envForceReconcileTime); err != nil {
					t.Errorf("GetReconcileTime() err = %v", err)
				}
			}
			if got := GetForcedReconcileTimeOrDefault(tt.args.defaultTo); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
