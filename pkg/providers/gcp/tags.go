package gcp

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func buildDefaultRedisTags(ctx context.Context, client k8sclient.Client, r *v1alpha1.Redis) (map[string]string, error) {
	defaultTags, _, err := resources.GetDefaultResourceTags(ctx, client, r.Spec.Type, r.Name, r.ObjectMeta.Labels["productName"])
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to get default redis tags")
	}
	tags := make(map[string]string, len(defaultTags))
	for _, tag := range defaultTags {
		tags[tag.Key] = tag.Value
	}
	return tags, nil
}
