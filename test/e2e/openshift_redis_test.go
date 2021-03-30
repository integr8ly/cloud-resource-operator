package e2e

import (
	goctx "context"
	"fmt"
	types2 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"

	// appsv1 "k8s.io/api/apps/v1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OpenshiftRedisBasicTest(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, _, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	//verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify redis delete
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// verifies a connection can be made to a postgres resource
func OpenshiftVerifyRedisConnection(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, namespace, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// verify redis connection
	if err := VerifyRedisConnectionTest(t, ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify connection")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify redis delete
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

//tests deployment recovery on manual delete of deployment
func OpenshiftVerifyRedisDeploymentRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, namespace, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis deployment
	if err := ctx.Client.Delete(goctx.TODO(), GetTestDeployment(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis deployment")
	}

	// wait for redis deployment
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test service recovery on manual delete of service
func OpenshiftVerifyRedisServiceRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, namespace, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis service
	if err := ctx.Client.Delete(goctx.TODO(), GetTestService(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis service")
	}

	// wait for redis service
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test pvc recovery on manual delete of pvc
func OpenshiftVerifyRedisPVCRecovery(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, namespace, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis pvc
	if err := ctx.Client.Delete(goctx.TODO(), GetTestPVC(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis pvc")
	}

	// wait for redis service
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test manual updates to redis deployment image, waits to see if cro reconciles and returns image to what is expected
func OpenshiftVerifyRedisDeploymentUpdate(t TestingTB, ctx *TestingContext, namespace string) error {
	testRedis, namespace, err := getBasicTestRedis(ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	rd := &appsv1.Deployment{}
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
		return errorUtil.Wrapf(err, "failed to get redis deployment")
	}

	// get wanted container
	wantedContainer := rd.Spec.Template.Spec.Containers[0].Image

	// set unwanted container
	rd.Spec.Template.Spec.Containers[0].Image = "registry.redhat.io/rhscl/redis-5-rhel7"

	// update redis deployment
	if err := ctx.Client.Update(goctx.TODO(), rd); err != nil {
		return errorUtil.Wrapf(err, "failed to update redis service")
	}

	// wait for redis deployment
	if err := WaitForDeployment(t, ctx.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// get updated redis deployment
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
		return errorUtil.Wrapf(err, "failed to get redis deployment")
	}

	// verify the image is what we want
	if rd.Spec.Template.Spec.Containers[0].Image != wantedContainer {
		return errorUtil.New("redis failed to reconcile to wanted image")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(ctx, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, ctx, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// creates redis resource, verifies everything is as expected
func redisCreateTest(t TestingTB, ctx *TestingContext, testRedis *v1alpha1.Redis, namespace string) error {
	// create redis resource
	if err := ctx.Client.Create(goctx.TODO(), testRedis); err != nil {
		return errorUtil.Wrapf(err, "could not create example redis")
	}

	t.Logf("created %s resource", testRedis.Name)

	// poll cr for complete status phase
	rcr := &v1alpha1.Redis{}

	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rcr); err != nil {
			if k8serr.IsNotFound(err) {
				return false, errorUtil.Wrapf(err, "Redis failed to get redis, retrying")
			}
			return true, err
		}

		if rcr.Status.Phase == types2.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return err
	}

	// get created secret
	sec := v1.Secret{}
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: rcr.Status.SecretRef.Namespace, Name: rcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"port", "uri"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %s value not found", k))
		}
	}
	t.Logf("%s secret created successfully", rcr.Status.SecretRef.Name)

	return nil
}

// removes redis resources and verifies all components have been cleaned up
func redisDeleteTest(t TestingTB, ctx *TestingContext, testRedis *v1alpha1.Redis, namespace string) error {
	// delete redis resource
	if err := ctx.Client.Delete(goctx.TODO(), testRedis); err != nil {
		return errorUtil.Wrapf(err, "failed  to delete example redis")
	}

	err := wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, testRedis); err != nil {
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

	pvc := GetTestPVC(redisName, namespace)
	err = wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, pvc); err != nil {
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

	service := GetTestService(redisName, namespace)
	err = wait.Poll(time.Second*10, time.Minute*2, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, service); err != nil {
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
	t.Logf("all redis resources have been cleaned")

	return nil
}

// // verify that the deployment status is available
func verifySuccessfulRedisDeploymentStatus(ctx *TestingContext, namespace string) error {
	rd := &appsv1.Deployment{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
			return true, errorUtil.Wrapf(err, "could not get redis deployment")
		}
		for _, s := range rd.Status.Conditions {
			if s.Type == appsv1.DeploymentAvailable {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return errorUtil.Wrapf(err, "failed to poll redis deployment")
	}
	return nil
}

func getBasicTestRedis(ctx *TestingContext, namespace string) (*v1alpha1.Redis, string, error) {
	return &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
		Spec: types2.ResourceTypeSpec{
			SecretRef: &types2.SecretRef{
				Name:      "example-redis-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}, namespace, nil
}
