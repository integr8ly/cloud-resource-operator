package e2e

import (
	goctx "context"
	"fmt"
	types2 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
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
func OpenshiftPostgresBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// verify postgres create
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify postgres delete
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// tests to verify the postgres string created from secret is valid
func OpenshiftVerifyPostgresSecretTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres permission
	if err := VerifyPostgresSecretTest(t, f, namespace); err != nil {
		return errorUtil.Wrapf(err, "verify postgres permissions failure")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// verifies a connection can be made to a postgres resource
func OpenshiftVerifyPostgresConnection(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres connection
	if err := VerifyPostgresConnectionTest(t, f, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify connection")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// verifies postgres user has permissions
func OpenshiftVerifyPostgresPermission(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres permissions
	if err := VerifyPostgresPermissionTest(t, f, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify permissions")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil

}

// tests deployment recovery on manual delete of deployment
func OpenshiftVerifyPostgresDeploymentRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres deployment
	if err := f.Client.Delete(goctx.TODO(), GetTestDeployment(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres deployment")
	}

	// wait for postgres deployment
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// test service recovery on manual delete of service
func OpenshiftVerifyPostgresServiceRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres service
	if err := f.Client.Delete(goctx.TODO(), GetTestService(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres service")
	}

	// wait for postgres service
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// tests pvc recovery on manual delete of pvc
func OpenshiftVerifyPostgresPVCRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres service
	if err := f.Client.Delete(goctx.TODO(), GetTestPVC(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres service")
	}

	// wait for postgres service
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// test manually updates postgres deployment image, waits to see if cro reconciles and returns image to what is expected
func OpenshiftVerifyPostgresDeploymentUpdate(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// new different postgres deployment
	upd := &appsv1.Deployment{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres deployment")
	}

	// get wanted container
	wantedContainer := upd.Spec.Template.Spec.Containers[0].Image

	// set unwanted container
	upd.Spec.Template.Spec.Containers[0].Image = "openshift/postgresql-92-centos7"

	// update postgres deployment
	if err := f.Client.Update(goctx.TODO(), upd); err != nil {
		return errorUtil.Wrapf(err, "failed to update postgres service")
	}

	// wait for postgres deployment
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// get updated postgres deployment
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres deployment")
	}

	// verify the image is what we want
	if upd.Spec.Template.Spec.Containers[0].Image != wantedContainer {
		return errorUtil.New("postgres failed to reconcile to wanted image")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, f, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// Creates postgres resource, verifies everything is as expected
func postgresCreateTest(t *testing.T, f *framework.Framework, testPostgres *v1alpha1.Postgres, namespace string) error {
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
		if pcr.Status.Phase == types2.StatusPhase("complete") {
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

// removes postgres resource and verifies all components have been cleaned up
func postgresDeleteTest(t *testing.T, f *framework.Framework, testPostgres *v1alpha1.Postgres, namespace string) error {
	// delete postgres resource
	if err := f.Client.Delete(goctx.TODO(), testPostgres); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example Postgres")
	}
	t.Logf("%s custom resource deleted", testPostgres.Name)

	// check resources have been cleaned up
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestDeployment(postgresName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment deletion")
	}

	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestPVC(postgresName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get persistent volume claim deletion")
	}

	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestService(postgresName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get service deletion")
	}
	t.Logf("all postgres resources have been cleaned")

	return nil
}

// verify that the deployment status is available
func verifySuccessfulPostgresDeploymentStatus(f *framework.Framework, namespace string) error {
	// get postgres deployment
	upd := &appsv1.Deployment{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
			return true, errorUtil.Wrapf(err, "could not get postgres deployment")
		}
		for _, s := range upd.Status.Conditions {
			if s.Type == appsv1.DeploymentAvailable {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return errorUtil.Wrapf(err, "failed to poll postgres deployment")
	}

	return nil
}

func getBasicTestPostgres(ctx framework.TestCtx) (*v1alpha1.Postgres, string, error) {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return nil, "", errorUtil.Wrapf(err, "could not get namespace")
	}

	return &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
		Spec: v1alpha1.PostgresSpec{
			SecretRef: &types2.SecretRef{
				Name:      "example-postgres-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
			Env: &types2.Env{
				Name:  "DEFAULT_ORGANIZATION_TAG",
				Value: "integreatly.org/",
			},
			Labels: &types2.Labels{
				ProductName: "productname",
			},
		},
	}, namespace, nil
}
