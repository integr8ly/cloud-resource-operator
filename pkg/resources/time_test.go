package resources

import (
	"testing"
	"time"
)

func TestSafeTimeDereference(t *testing.T) {
	testTime := time.Now()
	type args struct {
		time *time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "test empty string is returned on nil input",
			args: args{
				time: nil,
			},
			want: time.Time{},
		},
		{
			name: "test value is returned",
			args: args{
				time: &[]time.Time{testTime}[0],
			},
			want: testTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeTimeDereference(tt.args.time); got.Unix() != tt.want.Unix() {
				t.Errorf("SafeTimeDereference() = %v, want %v", got.Unix(), tt.want.Unix())
			}
		})
	}
}
