package types

import (
	"fmt"
	"testing"
)

func TestStatusMessage_WrapError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		sm   StatusMessage
		args args
		want StatusMessage
	}{
		{
			name: "test error message displays as expected",
			sm:   "test",
			args: args{err: fmt.Errorf("error")},
			want: "test: error",
		},
		{
			name: "test error is ignored when nil",
			sm:   "test",
			args: args{err: nil},
			want: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sm.WrapError(tt.args.err); got != tt.want {
				t.Errorf("WrapError() = %v, want %v", got, tt.want)
			}
		})
	}
}
