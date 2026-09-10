package kubernetescloudnativepgoperatorv1alpha1

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	kubernetes "github.com/plantonhq/planton/catalog/kubernetes"
	"github.com/plantonhq/planton/shared"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	foreignkeyv1 "github.com/plantonhq/planton/shared/foreignkey/v1"
)

func TestKubernetesCloudNativePgOperator(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "KubernetesCloudNativePgOperator Suite")
}

func int32Ptr(i int32) *int32    { return &i }
func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }

func literal(value string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_Value{Value: value},
	}
}

func valueFrom(kind cloudresourcekind.CloudResourceKind, name, fieldPath string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_ValueFrom{
			ValueFrom: &foreignkeyv1.ValueFromRef{
				Kind:      kind,
				Name:      name,
				FieldPath: fieldPath,
			},
		},
	}
}

var _ = ginkgo.Describe("KubernetesCloudNativePgOperator Validation Tests", func() {
	var input *KubernetesCloudNativePgOperator

	ginkgo.BeforeEach(func() {
		input = &KubernetesCloudNativePgOperator{
			ApiVersion: "kubernetes.planton.dev/v1alpha1",
			Kind:       "KubernetesCloudNativePgOperator",
			Metadata: &shared.CloudResourceMetadata{
				Name: "test-cnpg-operator",
			},
			Spec: &KubernetesCloudNativePgOperatorSpec{
				Namespace: literal("cnpg-system"),
			},
		}
	})

	ginkgo.Describe("When valid input is passed", func() {
		ginkgo.It("minimal spec should not return a validation error", func() {
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace as a reference should be valid", func() {
			input.Spec.Namespace = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesNamespace, "cnpg-system", "spec.name")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("replicas of 1 should be valid (gte 1 boundary)", func() {
			input.Spec.Replicas = int32Ptr(1)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("warm-standby replicas should be valid", func() {
			input.Spec.Replicas = int32Ptr(3)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("cluster-wide watch without namespaces should be valid", func() {
			input.Spec.Watch = &KubernetesCloudNativePgOperatorWatch{
				ClusterWide: boolPtr(true),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace-fenced watch with namespaces should be valid", func() {
			input.Spec.Watch = &KubernetesCloudNativePgOperatorWatch{
				ClusterWide: boolPtr(false),
				Namespaces:  []string{"team-a", "team-b"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("max_concurrent_reconciles of 1 should be valid (gte 1 boundary)", func() {
			input.Spec.MaxConcurrentReconciles = int32Ptr(1)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("raised max_concurrent_reconciles should be valid", func() {
			input.Spec.MaxConcurrentReconciles = int32Ptr(50)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("full-surface spec with every block populated should be valid", func() {
			input.Spec = &KubernetesCloudNativePgOperatorSpec{
				Namespace:       literal("cnpg-system"),
				CreateNamespace: true,
				ChartVersion:    stringPtr("0.29.0"),
				Crds: &KubernetesCloudNativePgOperatorCrds{
					Install: boolPtr(true),
				},
				Replicas: int32Ptr(2),
				Resources: &kubernetes.ContainerResources{
					Requests: &kubernetes.CpuMemory{Cpu: "100m", Memory: "128Mi"},
					Limits:   &kubernetes.CpuMemory{Cpu: "500m", Memory: "512Mi"},
				},
				Watch: &KubernetesCloudNativePgOperatorWatch{
					ClusterWide: boolPtr(false),
					Namespaces:  []string{"databases", "team-a"},
				},
				OperatorConfig: map[string]string{
					"INHERITED_ANNOTATIONS": "categories",
					"PULL_SECRET_NAME":      "registry-pull",
				},
				MaxConcurrentReconciles: int32Ptr(20),
				Monitoring: &KubernetesCloudNativePgOperatorMonitoring{
					PodMonitorEnabled: true,
					GrafanaDashboard:  true,
				},
				PriorityClassName: "system-cluster-critical",
				NodeSelector:      map[string]string{"node-role": "control"},
				Tolerations: []*kubernetes.WorkloadToleration{
					{Key: "dedicated", Operator: "Equal", Value: "operators", Effect: "NoSchedule"},
				},
				ImagePullSecrets: []string{"registry-pull"},
				Image: &KubernetesCloudNativePgOperatorImage{
					Repository: "my-mirror.example.com/cloudnative-pg",
					Tag:        "1.30.0",
				},
				HelmValues: "monitoring:\n  grafanaDashboard:\n    namespace: monitoring\n",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("When invalid input is passed", func() {
		ginkgo.It("missing namespace should fail (required)", func() {
			input.Spec.Namespace = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero replicas should fail (gte 1)", func() {
			input.Spec.Replicas = int32Ptr(0)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("cluster-wide watch with namespaces should fail (namespaces_require_fenced)", func() {
			input.Spec.Watch = &KubernetesCloudNativePgOperatorWatch{
				ClusterWide: boolPtr(true),
				Namespaces:  []string{"team-a"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("namespace-fenced watch without namespaces should fail (fenced_requires_namespaces)", func() {
			input.Spec.Watch = &KubernetesCloudNativePgOperatorWatch{
				ClusterWide: boolPtr(false),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("UNSET cluster_wide with namespaces should fail (the default is cluster-wide — the namespaces would be silently ignored)", func() {
			input.Spec.Watch = &KubernetesCloudNativePgOperatorWatch{
				Namespaces: []string{"team-a"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero max_concurrent_reconciles should fail (gte 1)", func() {
			input.Spec.MaxConcurrentReconciles = int32Ptr(0)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})
	})
})
