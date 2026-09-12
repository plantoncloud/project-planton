package component

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A Deployment is ready when its rollout is complete, never merely because
// one replica is available: during an image change the previous pod stays
// available until the new one is, and a readiness test that stopped there
// would report the platform Ready at a new version while the old release
// still served.
func TestDeploymentRolloutComplete(t *testing.T) {
	type counts struct{ observed, total, updated, available int64 }
	deployment := func(generation int64, replicas *int64, c *counts) *unstructured.Unstructured {
		status := map[string]any{}
		if c != nil {
			status = map[string]any{
				"observedGeneration": c.observed,
				"replicas":           c.total,
				"updatedReplicas":    c.updated,
				"availableReplicas":  c.available,
			}
		}
		d := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{}, "status": status}}
		d.SetGeneration(generation)
		if replicas != nil {
			_ = unstructured.SetNestedField(d.Object, *replicas, "spec", "replicas")
		}
		return d
	}
	one, two, zero := int64(1), int64(2), int64(0)

	cases := []struct {
		name string
		d    *unstructured.Unstructured
		want bool
	}{
		{"fresh rollout complete, replicas unset defaults to one",
			deployment(1, nil, &counts{1, 1, 1, 1}), true},
		{"just created, no status yet",
			deployment(1, &one, nil), false},
		{"rolling: the new pod exists but is not available, the old one still serves",
			deployment(2, &one, &counts{observed: 2, total: 2, updated: 1, available: 1}), false},
		{"rolling: the new pod is not created yet",
			deployment(2, &one, &counts{observed: 2, total: 1, updated: 0, available: 1}), false},
		{"rolling: the new pod is available, the old one is terminating",
			deployment(2, &one, &counts{observed: 2, total: 2, updated: 1, available: 2}), false},
		{"the controller has not observed the new spec yet",
			deployment(2, &one, &counts{observed: 1, total: 1, updated: 1, available: 1}), false},
		{"rollout complete after an image change",
			deployment(2, &one, &counts{observed: 2, total: 1, updated: 1, available: 1}), true},
		{"two replicas, one still unavailable",
			deployment(1, &two, &counts{1, 2, 2, 1}), false},
		{"two replicas, both available and current",
			deployment(1, &two, &counts{1, 2, 2, 2}), true},
		{"scaled to zero is never ready",
			deployment(1, &zero, &counts{1, 0, 0, 0}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deploymentRolloutComplete(tc.d); got != tc.want {
				t.Fatalf("deploymentRolloutComplete = %v, want %v", got, tc.want)
			}
		})
	}
}
