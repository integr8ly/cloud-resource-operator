package resources

import (
	"testing"

	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourceTypeSpecFromObject(t *testing.T) {
	t.Parallel()

	postgres := &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{Name: "example-postgres", Namespace: "test"},
		Spec: croType.ResourceTypeSpec{
			Type: "openshift",
			Tier: "development",
			SecretRef: &croType.SecretRef{
				Name: "example-postgres-sec",
			},
		},
	}
	rts, err := resourceTypeSpecFromObject(postgres)
	if err != nil {
		t.Fatalf("postgres: unexpected error: %v", err)
	}
	if rts.SecretRef == nil || rts.SecretRef.Name != "example-postgres-sec" {
		t.Fatalf("postgres: unexpected secret ref: %+v", rts.SecretRef)
	}

	redis := &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{Name: "example-redis", Namespace: "test"},
		Spec: v1alpha1.RedisSpec{
			ResourceTypeSpec: croType.ResourceTypeSpec{
				Type: "openshift",
				Tier: "development",
				SecretRef: &croType.SecretRef{
					Name: "example-redis-sec",
				},
			},
			Engine: "redis",
		},
	}
	rts, err = resourceTypeSpecFromObject(redis)
	if err != nil {
		t.Fatalf("redis: unexpected error: %v", err)
	}
	if rts.SecretRef == nil || rts.SecretRef.Name != "example-redis-sec" {
		t.Fatalf("redis: unexpected secret ref: %+v", rts.SecretRef)
	}
}
