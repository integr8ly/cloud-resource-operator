package resources

import "testing"

func TestIsCompoundMetric(t *testing.T) {
	type args struct {
		metric string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "metric is compound",
			args: args{
				metric: RedisFreeableMemoryAverage,
			},
			want: true,
		},
		{
			name: "metric is not compound",
			args: args{
				metric: RedisCPUUtilizationAverage,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCompoundMetric(tt.args.metric); got != tt.want {
				t.Errorf("IsCompoundMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsComputedCpuMetric(t *testing.T) {
	type args struct {
		metric string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "metric is computed cpu",
			args: args{
				metric: RedisCPUUtilizationAverage,
			},
			want: true,
		},
		{
			name: "metric is not computed cpu",
			args: args{
				metric: RedisFreeableMemoryAverage,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsComputedCpuMetric(tt.args.metric); got != tt.want {
				t.Errorf("IsComputedCpuMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}
