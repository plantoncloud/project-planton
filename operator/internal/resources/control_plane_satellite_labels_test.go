package resources

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// The token-reviewer pair is the platform's one cluster-scoped satellite. A
// namespaced owner cannot garbage-collect it, so it carries the platform's
// UID as a label and the janitor removes it once that UID names no live
// platform -- by UID, never by name, so a platform recreated under the same
// name keeps the grant its new reconcile applied.
func TestControlPlaneTokenReviewerPair_CarriesThePlatformUID(t *testing.T) {
	cfg := ControlPlaneConfig{
		CRName:    "planton",
		Namespace: "planton",
		OwnerRef:  &metav1.OwnerReference{Kind: "PlantonPlatform", Name: "planton", UID: types.UID("11111111-2222")},
	}
	role := ControlPlaneTokenReviewerClusterRole(cfg)
	binding := ControlPlaneTokenReviewerClusterRoleBinding(cfg)

	for _, labels := range []map[string]string{role.Labels, binding.Labels} {
		if labels[PlatformUIDLabel] != "11111111-2222" {
			t.Errorf("%s = %q, want the owning platform's UID", PlatformUIDLabel, labels[PlatformUIDLabel])
		}
		if labels["app.kubernetes.io/managed-by"] != ManagedByLabel {
			t.Errorf("the pair must still carry the operator's mark, got %q", labels["app.kubernetes.io/managed-by"])
		}
		if labels["app.kubernetes.io/instance"] != "planton" {
			t.Errorf("the component labels must stay intact, got %v", labels)
		}
	}
}

func TestControlPlaneTokenReviewerPair_WithoutAnOwner_CarriesNoUID(t *testing.T) {
	role := ControlPlaneTokenReviewerClusterRole(ControlPlaneConfig{CRName: "planton", Namespace: "planton"})
	if _, present := role.Labels[PlatformUIDLabel]; present {
		t.Error("no owner reference means no UID to stamp; an empty UID label would read as 'no live platform' and be swept")
	}
}
