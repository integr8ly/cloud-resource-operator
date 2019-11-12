package e2e

import (
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
)

const (
	postgresName    = "example-postgres"
	redisName       = "example-redis"
	blobstorageName = "example-blobstorage"
	smtpName        = "example-smtp"
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

	// adding blob storage scheme to framework
	blobstorageList := &v1alpha1.BlobStorage{}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, blobstorageList); err != nil {
		t.Fatalf("failed to add Blobstorage custom resource scheme to framework: %v", err)
	}

	// adding smtp scheme to framework
	smtpList := &v1alpha1.SMTPCredentialSet{}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, smtpList); err != nil {
		t.Fatalf("failed to add SMTP custom resource scheme to framework: %v", err)
	}

	// run subtests
	t.Run("cro-openshift-postgres-test", func(t *testing.T) {
		t.Run("Cluster", OpenshiftPostgresTestCluster)
	})

	t.Run("cro-openshift-redis-test", func(t *testing.T) {
		t.Run("Cluster", OpenshiftRedisTestCluster)
	})

	t.Run("cro-openshift-blobstorage-test", func(t *testing.T) {
		t.Run("Cluster", OpenshiftBlobstorageTestCluster)
	})

	t.Run("cro-openshift-smtp-test", func(t *testing.T) {
		t.Run("Cluster", OpenshiftSMTPTestCluster)
	})

}

// setup openshift postgres test env and executes subtests
func OpenshiftPostgresTestCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("initialized cluster resources")

	// get global framework variables
	f := framework.Global

	// run postgres test
	if err = OpenshiftPostgresBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres valid secret test
	if err = OpenshiftVerifyPostgresSecretTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres connection test
	if err = OpenshiftVerifyPostgresConnection(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres permission test
	if err = OpenshiftVerifyPostgresPermission(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres deployment recover test
	if err = OpenshiftVerifyPostgresDeploymentRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres service recover test
	if err = OpenshiftVerifyPostgresServiceRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres pvc recover test
	if err = OpenshiftVerifyPostgresPVCRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run postgres deployment update recover test
	if err = OpenshiftVerifyPostgresDeploymentUpdate(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

// setup openshift redis environment and executes sub tests
func OpenshiftRedisTestCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("initialized cluster resources")

	// get global framework variables
	f := framework.Global

	// run redis test
	if err = OpenshiftRedisBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run verify redis connection test
	if err = OpenshiftVerifyRedisConnection(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run redis deployment recover test
	if err = OpenshiftVerifyRedisDeploymentRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run redis service recover test
	if err = OpenshiftVerifyRedisServiceRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run redis pvc recover test
	if err = OpenshiftVerifyRedisPVCRecovery(t, f, *ctx); err != nil {
		t.Fatal(err)
	}

	// run redis deployment update recover test
	if err = OpenshiftVerifyRedisDeploymentUpdate(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

func OpenshiftBlobstorageTestCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("initialized cluster resources")

	f := framework.Global

	// run blobstorage test
	if err = OpenshiftBlobstorageBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

func OpenshiftSMTPTestCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(getCleanupOptions(t))
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("initialized cluster resources")

	f := framework.Global

	// run smtp test
	if err = OpenshiftSMTPBasicTest(t, f, *ctx); err != nil {
		t.Fatal(err)
	}
}

// returns cleanup options
func getCleanupOptions(t *testing.T) *framework.CleanupOptions {
	return &framework.CleanupOptions{
		TestContext:   framework.NewTestCtx(t),
		Timeout:       timeout,
		RetryInterval: retryInterval,
	}
}
