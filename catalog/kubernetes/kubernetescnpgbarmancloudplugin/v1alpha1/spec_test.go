package kubernetescnpgbarmancloudpluginv1alpha1

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

func TestKubernetesCnpgBarmanCloudPlugin(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "KubernetesCnpgBarmanCloudPlugin Suite")
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

var _ = ginkgo.Describe("KubernetesCnpgBarmanCloudPlugin Validation Tests", func() {
	var input *KubernetesCnpgBarmanCloudPlugin

	ginkgo.BeforeEach(func() {
		input = &KubernetesCnpgBarmanCloudPlugin{
			ApiVersion: "kubernetes.planton.dev/v1alpha1",
			Kind:       "KubernetesCnpgBarmanCloudPlugin",
			Metadata: &shared.CloudResourceMetadata{
				Name: "test-barman-plugin",
			},
			Spec: &KubernetesCnpgBarmanCloudPluginSpec{
				Namespace: literal("cnpg-system"),
			},
		}
	})

	ginkgo.Describe("When valid input is passed", func() {
		ginkgo.It("minimal spec (a literal operator namespace -- the resident-operator shape) should not return a validation error", func() {
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace as a reference to the operator resource's namespace output should be valid", func() {
			input.Spec.Namespace = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesCloudNativePgOperator, "cnpg", "status.outputs.namespace")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace as a reference to a KubernetesNamespace should be valid (an explicit kind override on the reference)", func() {
			input.Spec.Namespace = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesNamespace, "cnpg-system", "spec.name")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("replicas of 1 should be valid (gte 1 boundary)", func() {
			input.Spec.Replicas = int32Ptr(1)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("warm-standby replicas should be valid", func() {
			input.Spec.Replicas = int32Ptr(2)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("chart_version unset should be valid (the default applies)", func() {
			input.Spec.ChartVersion = nil
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("a pinned chart_version should be valid", func() {
			input.Spec.ChartVersion = stringPtr("0.7.0")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("crds.install false should be valid (something else manages the ObjectStore CRD)", func() {
			input.Spec.Crds = &KubernetesCnpgBarmanCloudPluginCrds{Install: boolPtr(false)}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("full-surface spec with every block populated should be valid", func() {
			input.Spec = &KubernetesCnpgBarmanCloudPluginSpec{
				Namespace:       valueFrom(cloudresourcekind.CloudResourceKind_KubernetesCloudNativePgOperator, "cnpg", "status.outputs.namespace"),
				CreateNamespace: false,
				ChartVersion:    stringPtr("0.7.0"),
				Crds:            &KubernetesCnpgBarmanCloudPluginCrds{Install: boolPtr(true)},
				Replicas:        int32Ptr(2),
				Resources: &kubernetes.ContainerResources{
					Requests: &kubernetes.CpuMemory{Cpu: "50m", Memory: "64Mi"},
					Limits:   &kubernetes.CpuMemory{Cpu: "200m", Memory: "256Mi"},
				},
				Image: &KubernetesCnpgBarmanCloudPluginImage{
					Repository: "my-mirror.example.com/cloudnative-pg/plugin-barman-cloud",
					Tag:        "v0.13.0",
				},
				SidecarImage: &KubernetesCnpgBarmanCloudPluginImage{
					Repository: "my-mirror.example.com/cloudnative-pg/plugin-barman-cloud-sidecar",
					Tag:        "v0.13.0",
				},
				ImagePullSecrets:  []string{"registry-pull"},
				PriorityClassName: "system-cluster-critical",
				NodeSelector:      map[string]string{"node-role": "control"},
				Tolerations: []*kubernetes.WorkloadToleration{
					{Key: "dedicated", Operator: "Equal", Value: "operators", Effect: "NoSchedule"},
				},
				HelmValues: "additionalArgs:\n  - --log-level=info\n",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("When invalid input is passed", func() {
		ginkgo.It("missing namespace should fail (required -- the plugin must land in the operator's namespace)", func() {
			input.Spec.Namespace = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero replicas should fail (gte 1)", func() {
			input.Spec.Replicas = int32Ptr(0)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("a wrong kind constant should fail", func() {
			input.Kind = "KubernetesCloudNativePgOperator"
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})
	})
})
