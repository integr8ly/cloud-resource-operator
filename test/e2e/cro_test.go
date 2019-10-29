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
	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	retryInterval = time.Second * 20
	timeout       = time.Second * 60
)

func TestCRO(t *testing.T) {
	redisList := &v1alpha1.Redis{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Redis",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, redisList); err != nil {
		t.Fatalf("failed to add Redis custom resource scheme to framework: %v", err)
	}

	postgresList := &v1alpha1.Postgres{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Postgres",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, postgresList); err != nil {
		t.Fatalf("failed to add Postgres custom resource scheme to framework: %v", err)
	}

	// run subtests
	t.Run("cro-group", func(t *testing.T) {
		t.Run("Cluster", CROCluster)
	})
}

func postgresTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return errorUtil.Wrapf(err, "could not get namespace")
	}
	examplePostgres := &v1alpha1.Postgres{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Postgres",
			APIVersion: "integreatly.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-postgres",
			Namespace: namespace,
		},
		Spec: v1alpha1.PostgresSpec{
			SecretRef: &v1alpha1.SecretRef{
				Name:      "postgres-credentials",
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

	// wait from postgres deployment
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-postgres", 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment")
	}

	// get created secret
	sec := v1.Secret{}
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: "postgres-credentials"}, &sec); err != nil {
			return true, errorUtil.Wrapf(err, "could not get secret")
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	// check for expected key values
	for _, k := range []string{"user", "database", "password"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %v value not found", k))
		}
	}

	// delete postgres resource
	if err := f.Client.Delete(goctx.TODO(), examplePostgres); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example Postgres")
	}

	return nil
}

func CROCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
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
	if err = postgresTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

func getCleanupOptions(t *testing.T) *framework.CleanupOptions {
	return &framework.CleanupOptions{
		TestContext:   framework.NewTestCtx(t),
		Timeout:       timeout,
		RetryInterval: retryInterval,
	}
}
