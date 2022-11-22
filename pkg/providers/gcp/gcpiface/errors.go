package gcpiface

import (
	"errors"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"net/http"
)

func IsNotFound(err error) bool {
	var httpErr *googleapi.Error
	if errors.As(err, &httpErr) {
		if httpErr.Code == http.StatusNotFound {
			return true
		}
	}
	var grpcErr *apierror.APIError
	if errors.As(err, &grpcErr) {
		if grpcErr.GRPCStatus().Code() == codes.NotFound {
			return true
		}
	}
	return false
}
