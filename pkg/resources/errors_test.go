package resources

import (
	"fmt"
	googleHTTP "google.golang.org/api/googleapi"
	grpcCodes "google.golang.org/grpc/codes"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net/http"
	"testing"
)

func TestIsNotFoundError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "error of type google http is not found",
			args: args{
				err: &googleHTTP.Error{
					Code: http.StatusNotFound,
				},
			},
			want: true,
		},
		{
			name: "error of type google grpc is not found",
			args: args{
				err: NewMockAPIError(grpcCodes.NotFound),
			},
			want: true,
		},
		{
			name: "error of type kubernetes is not found",
			args: args{
				err: errors.NewNotFound(schema.GroupResource{}, ""),
			},
			want: true,
		},
		{
			name: "error of unknown type",
			args: args{
				err: fmt.Errorf("generic error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := IsNotFoundError(tt.args.err); err != tt.want {
				t.Errorf("IsNotFoundError() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestIsConflictError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "error of type google http is not found",
			args: args{
				err: &googleHTTP.Error{
					Code: http.StatusConflict,
				},
			},
			want: true,
		},
		{
			name: "error of type google grpc is not found",
			args: args{
				err: NewMockAPIError(grpcCodes.AlreadyExists),
			},
			want: true,
		},
		{
			name: "error of type kubernetes is not found",
			args: args{
				err: errors.NewAlreadyExists(schema.GroupResource{}, ""),
			},
			want: true,
		},
		{
			name: "error of unknown type",
			args: args{
				err: fmt.Errorf("generic error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflictError(tt.args.err); got != tt.want {
				t.Errorf("IsConflictError() = %v, want %v", got, tt.want)
			}
		})
	}
}
