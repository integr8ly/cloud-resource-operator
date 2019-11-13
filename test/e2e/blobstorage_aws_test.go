package e2e

import (
	"testing"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	errorUtil "github.com/pkg/errors"
)

const managed = "managed"

func AwsBlobstorageBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testBlobstorage, namespace, err := GetBasicBlobstorage(ctx, managed)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get blobstorage")
	}

	// verify blobstorage create
	if err := BlobstorageCreateTest(t, f, testBlobstorage, namespace); err != nil {
		return errorUtil.Wrapf(err, "create blobstorage test failure")
	}

	// verify blobstorage delete
	if err := BlobstorageDeleteTest(t, f, testBlobstorage, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete blobstorage test failure")
	}

	t.Logf("blobstorage basic aws test pass")
	return nil

}
