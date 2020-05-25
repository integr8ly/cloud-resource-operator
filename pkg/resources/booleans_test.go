package resources

import "testing"

func TestBtof64(t *testing.T) {
	type args struct {
		b bool
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "test true",
			args: args{b: true},
			want: float64(1),
		},
		{
			name: "test false",
			args: args{b: false},
			want: float64(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Btof64(tt.args.b); got != tt.want {
				t.Errorf("Btof64() = %v, want %v", got, tt.want)
			}
		})
	}
}
