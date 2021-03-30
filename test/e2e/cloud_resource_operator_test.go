package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"k8s.io/client-go/rest"
)

const (
	postgresName    = "example-postgres"
	redisName       = "example-redis"
	blobstorageName = "example-blobstorage"
)

var (
	retryInterval  = time.Second * 10
	timeout        = time.Minute * 5
	testingContext *TestingContext
	croNamespace   = "cloud-resource-operator"
	restConfig     *rest.Config
	t              GinkgoTInterface
)

var _ = Describe("cloud resource operator", func() {

	BeforeEach(func() {
		restConfig = cfg
		t = GinkgoT()
		testingContext, err = NewTestingContext(restConfig)
		if err != nil {
			t.Log("Failed to create context")
		}
	})

	// Redis tests:

	It("Redis - basic tests", func() {
		if err = OpenshiftRedisBasicTest(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Redis - verify connection", func() {
		if err = OpenshiftVerifyRedisConnection(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Redis - verify deployment recovery", func() {
		if err = OpenshiftVerifyRedisDeploymentRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Redis - verify service recovery", func() {
		if err = OpenshiftVerifyRedisServiceRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Redis - verify PVC recovery", func() {
		if err = OpenshiftVerifyRedisPVCRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Redis - verify deployment update", func() {
		if err = OpenshiftVerifyRedisDeploymentUpdate(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})

	// Blobstorage tests:

	It("Blobstorage - basic tests", func() {
		if err = OpenshiftBlobstorageBasicTest(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})

	// Postgres tests:

	It("Postgres - basic tests", func() {
		if err = OpenshiftPostgresBasicTest(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres secret", func() {
		if err = OpenshiftVerifyPostgresSecretTest(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres connection", func() {
		if err = OpenshiftVerifyPostgresConnection(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres permission", func() {
		if err = OpenshiftVerifyPostgresPermission(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres deployment recovery", func() {
		if err = OpenshiftVerifyPostgresDeploymentRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres service recovery", func() {
		if err = OpenshiftVerifyPostgresServiceRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres PVC recovery", func() {
		if err = OpenshiftVerifyPostgresPVCRecovery(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})
	It("Postgres - verify postgres deployment update", func() {
		if err = OpenshiftVerifyPostgresDeploymentUpdate(t, testingContext, croNamespace); err != nil {
			t.Fatal(err)
		}
	})

})
