package e2e

import (
	goctx "context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	appsv1 "k8s.io/api/apps/v1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// basic test, creates postgres resource, checks deployment has been created, the status has been updated.
// the secret has been created and populated, deletes the postgres resource and checks all resources has been deleted
func PostgresBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return errorUtil.Wrapf(err, "could not get namespace")
	}

	testPostgres := &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
		Spec: v1alpha1.PostgresSpec{
			SecretRef: &v1alpha1.SecretRef{
				Name:      "example-postgres-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}

	if err := postgresCreateTest(t, f, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	if err := postgresDeleteTest(t, f, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

func postgresCreateTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx, testPostgres *v1alpha1.Postgres, namespace string) error {
	// create postgres resource
	if err := f.Client.Create(goctx.TODO(), testPostgres, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create example Postgres")
	}
	t.Logf("created %s resource", testPostgres.Name)

	// poll cr for complete status phase
	pcr := &v1alpha1.Postgres{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pcr); err != nil {
			return true, errorUtil.Wrapf(err, "could not get postgres cr")
		}
		if pcr.Status.Phase == v1alpha1.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("postgres status phase %s", pcr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: pcr.Status.SecretRef.Namespace, Name: pcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"username", "database", "password", "host", "port"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %v value not found", k))
		}
	}
	t.Logf("%s secret created successfully", pcr.Status.SecretRef.Name)

	return nil
}

func postgresDeleteTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx, testPostgres *v1alpha1.Postgres, namespace string) error {
	// delete postgres resource
	if err := f.Client.Delete(goctx.TODO(), testPostgres); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example Postgres")
	}
	t.Logf("%s custom resource deleted", testPostgres.Name)

	// check resources have been cleaned up
	pd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, pd, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment deletion")
	}

	ppvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, ppvc, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get persistent volume claim deletion")
	}

	ps := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, ps, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get service deletion")
	}
	t.Logf("all postgres resources have been cleaned")

	return nil
}
