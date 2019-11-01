package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	appsv1 "k8s.io/api/apps/v1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	postgresName = "example-postgres"
	redisName    = "example-redis"
)

var (
	retryInterval = time.Second * 20
	timeout       = time.Minute * 5
)

func TestCRO(t *testing.T) {
	// adding redis scheme to framework
	redisList := &v1alpha1.Redis{}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, redisList); err != nil {
		t.Fatalf("failed to add Redis custom resource scheme to framework: %v", err)
	}

	// adding postgres scheme to framework
	postgresList := &v1alpha1.Postgres{}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, postgresList); err != nil {
		t.Fatalf("failed to add Postgres custom resource scheme to framework: %v", err)
	}

	// run subtests
	t.Run("cro-group", func(t *testing.T) {
		t.Run("Cluster", CROCluster)
	})
}

func CROCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	// get global framework variables
	f := framework.Global
	// wait for cloud-resource-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "cloud-resource-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// run postgres test
	if err = postgresBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run redis test
	if err = redisBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

// basic test, creates postgres resource, checks deployment has been created, the status has been updated.
// the secret has been created and populated, deletes the postgres resource and checks all resources has been deleted
func postgresBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return errorUtil.Wrapf(err, "could not get namespace")
	}

	examplePostgres := &v1alpha1.Postgres{
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
	// create postgres resource
	if err := f.Client.Create(goctx.TODO(), examplePostgres, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create example Postgres")
	}
	t.Logf("created %s resource", examplePostgres.Name)

	// poll cr for complete status phase
	pcr := &v1alpha1.Postgres{}
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
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

	// delete postgres resource
	if err := f.Client.Delete(goctx.TODO(), examplePostgres); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example Postgres")
	}
	t.Logf("%s custom resource deleted", examplePostgres.Name)

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

func redisBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return errorUtil.Wrapf(err, "could not get namespace")
	}

	exampleRedis := &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
		Spec: v1alpha1.RedisSpec{
			SecretRef: &v1alpha1.SecretRef{
				Name:      "example-redis-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}

	// create redis resource
	if err := f.Client.Create(goctx.TODO(), exampleRedis, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create example redis")
	}
	t.Logf("created %s resource", exampleRedis.Name)

	// poll cr for complete status phase
	rcr := &v1alpha1.Redis{}
	err = wait.Poll(retryInterval, time.Minute*6, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rcr); err != nil {
			return true, errorUtil.Wrapf(err, "could not get redis cr")
		}
		if rcr.Status.Phase == v1alpha1.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("redis status phase %s", rcr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: rcr.Status.SecretRef.Namespace, Name: rcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"port", "uri"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %s value not found", k))
		}
	}
	t.Logf("%s secret created successfully", rcr.Status.SecretRef.Name)

	// delete redis resource
	if err := f.Client.Delete(goctx.TODO(), exampleRedis); err != nil {
		return errorUtil.Wrapf(err, "failed  to delete example redis")
	}

	// check resources have been cleaned up
	rd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rd, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment deletion")
	}

	rpvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rpvc, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get persistent volume claim deletion")
	}

	rs := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rs, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get service deletion")
	}
	t.Logf("all redis resources have been cleaned")

	return nil
}

// returns cleanup options
func getCleanupOptions(t *testing.T) *framework.CleanupOptions {
	return &framework.CleanupOptions{
		TestContext:   framework.NewTestCtx(t),
		Timeout:       timeout,
		RetryInterval: retryInterval,
	}
}
