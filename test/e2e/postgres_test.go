package e2e

import (
	goctx "context"
	"database/sql"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"

	_ "github.com/lib/pq"
	errorUtil "github.com/pkg/errors"
)

func VerifyPostgresPermissionsTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx, postgres v1alpha1.Postgres, namespace string) error {
	// poll cr for complete status phase
	pcr := &v1alpha1.Postgres{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pcr); err != nil {
		return errorUtil.Wrapf(err, "could not get resource")
	}
	t.Logf("postgres status phase %s", pcr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: pcr.Status.SecretRef.Namespace, Name: pcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// get connection details from secret
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		sec.Data["host"], string(sec.Data["port"]), sec.Data["username"], sec.Data["password"], sec.Data["database"])

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return errorUtil.Wrapf(err, "could not verify postgres connection string")
	}
	t.Logf("successfully verified %s", psqlInfo)

	if err := db.Close(); err != nil {
		return errorUtil.Wrapf(err, "failed to close postgres connection")
	}

	return nil
}
