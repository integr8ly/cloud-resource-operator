package e2e

import (
	goctx "context"
	"fmt"

	t1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OpenshiftBlobstorageBasicTest(t TestingTB, ctx *TestingContext, namespace string) error {
	testBlobstorage, namespace, err := getBasicBlobstorage(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get blobstorage")
	}

	// verify blobstorage create
	if err := blobstorageCreateTest(t, ctx, testBlobstorage, namespace); err != nil {
		return errorUtil.Wrapf(err, "create blobstorage test failure")
	}

	// verify blobstorage delete
	if err := blobstorageDeleteTest(t, ctx, testBlobstorage, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete blobstorage test failure")
	}

	t.Logf("blobstorage basic test pass")
	return nil
}

// creates blobstorage resource, verifies everything is as expected
func blobstorageCreateTest(t TestingTB, ctx *TestingContext, testBlobstorage *v1alpha1.BlobStorage, namespace string) error {
	// create blobstorage resource
	if err := ctx.Client.Create(goctx.TODO(), testBlobstorage); err != nil {
		return errorUtil.Wrapf(err, "could not create example blobstorage")
	}
	t.Logf("created %s resource", testBlobstorage.Name)

	// poll cr for complete status phase
	bcr := &v1alpha1.BlobStorage{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: blobstorageName}, bcr); err != nil {
			return true, errorUtil.Wrapf(err, "could not get blobstorage cr")
		}
		if bcr.Status.Phase == t1.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("blobstorage status phase %s", bcr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: bcr.Status.SecretRef.Namespace, Name: bcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"bucketName", "bucketRegion", "credentialKeyID", "credentialSecretKey"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %s value not found", k))
		}
	}

	t.Logf("%s secret created successfully", bcr.Status.SecretRef.Name)
	return nil
}

// removes blobstorage resource and verifies all components have been cleaned up
func blobstorageDeleteTest(t TestingTB, ctx *TestingContext, testBlobstorage *v1alpha1.BlobStorage, namespace string) error {
	// delete blobstorage resource
	if err := ctx.Client.Delete(goctx.TODO(), testBlobstorage); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example blobstorage")
	}

	sec := v1.Secret{}
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: testBlobstorage.Spec.SecretRef.Name}, &sec); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return errorUtil.Wrapf(err, "could not get secret")
	}
	t.Logf("%s custom resource deleted", testBlobstorage.Name)

	return nil
}

func getBasicBlobstorage(ctx *TestingContext, namespace string) (*v1alpha1.BlobStorage, string, error) {

	return &v1alpha1.BlobStorage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blobstorageName,
			Namespace: namespace,
		},
		Spec: t1.ResourceTypeSpec{
			SecretRef: &t1.SecretRef{
				Name:      "example-blobstorage-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}, namespace, nil
}
