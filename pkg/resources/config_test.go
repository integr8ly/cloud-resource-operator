package resources

import (
	"os"
	"testing"
	"time"
)

func TestGetReconcileTime(t *testing.T) {
	type args struct {
		recTime string
	}
	var tests = []struct {
		name string
		want time.Duration
		args args
	}{
		{
			name: "test function returns default",
			args: args{
				recTime: "",
			},
			want: time.Duration(DefaulReconcileTime),
		},
		{
			name: "test accepts env var and returns value",
			args: args{
				recTime: "30",
			},
			want: time.Duration(30),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.recTime != "" {
				err := os.Setenv("RECTIME", tt.args.recTime)
				if err != nil {
					t.Error(err)
				}
			}
			if got := GetReconcileTime(); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
