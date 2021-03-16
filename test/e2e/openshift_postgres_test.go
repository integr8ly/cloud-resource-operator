package e2e

import (
	goctx "context"
	"fmt"
	types2 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// basic test, creates postgres resource, checks deployment has been created, the status has been updated.
// the secret has been created and populated, deletes the postgres resource and checks all resources has been deleted
func OpenshiftPostgresBasicTest(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// verify postgres create
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify postgres delete
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// tests to verify the postgres string created from secret is valid
func OpenshiftVerifyPostgresSecretTest(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres permission
	if err := VerifyPostgresSecretTest(t, ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "verify postgres permissions failure")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// verifies a connection can be made to a postgres resource
func OpenshiftVerifyPostgresConnection(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres connection
	if err := VerifyPostgresConnectionTest(t, ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify connection")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// verifies postgres user has permissions
func OpenshiftVerifyPostgresPermission(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// verify postgres permissions
	if err := VerifyPostgresPermissionTest(t, ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify permissions")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil

}

// tests deployment recovery on manual delete of deployment
func OpenshiftVerifyPostgresDeploymentRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres deployment
	if err := ctx.Client.Delete(goctx.TODO(), GetTestDeployment(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres deployment")
	}

	// wait for postgres deployment
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// test service recovery on manual delete of service
func OpenshiftVerifyPostgresServiceRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres service
	if err := ctx.Client.Delete(goctx.TODO(), GetTestService(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres service")
	}

	// wait for postgres service
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// tests pvc recovery on manual delete of pvc
func OpenshiftVerifyPostgresPVCRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// delete postgres service
	if err := ctx.Client.Delete(goctx.TODO(), GetTestPVC(postgresName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete postgres service")
	}

	// wait for postgres service
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// test manually updates postgres deployment image, waits to see if cro reconciles and returns image to what is expected
func OpenshiftVerifyPostgresDeploymentUpdate(t TestingTB, ctx *TestingContext, namespace string) error {
	testPostgres, namespace, err := getBasicTestPostgres(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres")
	}

	// setup postgres
	if err := postgresCreateTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "create postgres test failure")
	}

	// new different postgres deployment
	upd := &appsv1.Deployment{}
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres deployment")
	}

	// get wanted container
	wantedContainer := upd.Spec.Template.Spec.Containers[0].Image

	// set unwanted container
	upd.Spec.Template.Spec.Containers[0].Image = "openshift/postgresql-92-centos7"

	// update postgres deployment
	if err := ctx.Client.Update(goctx.TODO(), upd); err != nil {
		return errorUtil.Wrapf(err, "failed to update postgres service")
	}

	// wait for postgres deployment
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, postgresName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get postgres re-deployment")
	}

	// get updated postgres deployment
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
		return errorUtil.Wrapf(err, "failed to get postgres deployment")
	}

	// verify the image is what we want
	if upd.Spec.Template.Spec.Containers[0].Image != wantedContainer {
		return errorUtil.New("postgres failed to reconcile to wanted image")
	}

	// verify deployment is available
	if err := verifySuccessfulPostgresDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup postgres
	if err := postgresDeleteTest(t, ctx, testPostgres, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete postgres test failure")
	}

	return nil
}

// Creates postgres resource, verifies everything is as expected
func postgresCreateTest(t TestingTB, ctx *TestingContext, testPostgres *v1alpha1.Postgres, namespace string) error {
	// create postgres resource
	if err := ctx.Client.Create(goctx.TODO(), testPostgres); err != nil {
		return errorUtil.Wrapf(err, "could not create example Postgres")
	}
	t.Logf("created %s resource", testPostgres.Name)

	// poll cr for complete status phase
	pcr := &v1alpha1.Postgres{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pcr); err != nil {
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
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: pcr.Status.SecretRef.Namespace, Name: pcr.Status.SecretRef.Name}, &sec); err != nil {
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
func postgresDeleteTest(t TestingTB, ctx *TestingContext, testPostgres *v1alpha1.Postgres, namespace string) error {
	// delete postgres resource
	if err := ctx.Client.Delete(goctx.TODO(), testPostgres); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example Postgres")
	}
	t.Logf("%s custom resource deleted", testPostgres.Name)

	err := wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, testPostgres); err != nil {
			if k8serr.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	pvc := GetTestPVC(postgresName, namespace)
	err = wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pvc); err != nil {
			if k8serr.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	service := GetTestService(postgresName, namespace)
	err = wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, service); err != nil {
			if k8serr.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	t.Logf("all postgres resources have been cleaned")

	return nil
}

// verify that the deployment status is available
func verifySuccessfulPostgresDeploymentStatus(ctx *TestingContext, namespace string) error {
	// get postgres deployment
	upd := &appsv1.Deployment{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, upd); err != nil {
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

func getBasicTestPostgres(ctx *TestingContext, namespace string) (*v1alpha1.Postgres, string, error) {
	return &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresName,
			Namespace: namespace,
		},
		Spec: types2.ResourceTypeSpec{
			SecretRef: &types2.SecretRef{
				Name:      "example-postgres-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}, namespace, nil
}
