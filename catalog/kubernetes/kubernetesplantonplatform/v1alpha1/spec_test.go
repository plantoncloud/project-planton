package kubernetesplantonplatformv1alpha1

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/plantonhq/planton/shared"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	foreignkeyv1 "github.com/plantonhq/planton/shared/foreignkey/v1"
)

func TestKubernetesPlantonPlatformSpec(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "KubernetesPlantonPlatformSpec Validation Tests")
}

func strPtr(value string) *string { return &value }

func literalRef(value string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_Value{Value: value},
	}
}

// gatewayRef is a valueFrom against a KubernetesGateway resource; an empty
// fieldPath leans on the field's annotated default.
func gatewayRef(name, fieldPath string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_ValueFrom{
			ValueFrom: &foreignkeyv1.ValueFromRef{
				Kind:      cloudresourcekind.CloudResourceKind_KubernetesGateway,
				Name:      name,
				FieldPath: fieldPath,
			},
		},
	}
}

// minimalValidPlatform is the zero-config case: a namespace and a version
// — the whole platform boots from exactly this, reachable over
// port-forward with the first console visitor becoming the admin.
func minimalValidPlatform() *KubernetesPlantonPlatform {
	return &KubernetesPlantonPlatform{
		ApiVersion: "kubernetes.planton.dev/v1alpha1",
		Kind:       "KubernetesPlantonPlatform",
		Metadata: &shared.CloudResourceMetadata{
			Name: "planton",
		},
		Spec: &KubernetesPlantonPlatformSpec{
			Namespace:       literalRef("planton"),
			CreateNamespace: true,
			Version:         "v0.0.45",
		},
	}
}

var _ = ginkgo.Describe("KubernetesPlantonPlatformSpec Validation Tests", func() {

	ginkgo.Describe("When valid input is passed", func() {

		ginkgo.It("should not return a validation error for the zero-config platform", func() {
			err := protovalidate.Validate(minimalValidPlatform())
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept platform-wide storage settings", func() {
			input := minimalValidPlatform()
			input.Spec.Storage = &KubernetesPlantonPlatformStorage{
				StorageClassName: "gp3",
				Size:             "800Gi",
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept an ingress with hostname and cert-manager TLS", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				Tls: &KubernetesPlantonPlatformIngressTls{
					Issuer: &KubernetesPlantonPlatformCertManagerIssuer{
						Name: "letsencrypt",
					},
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept an ingress with a brought certificate Secret", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				Tls: &KubernetesPlantonPlatformIngressTls{
					SecretName: "planton-tls",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept a magic-DNS ingress (enabled, no hostname, no tls)", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{Enabled: true}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		// The Gateway API front door: the Gateway's listener already serves
		// the hostname over HTTPS, so no tls block is needed.
		ginkgo.It("should accept a Gateway API front door pinned to one listener, no tls", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{
					Name:        literalRef("main"),
					Namespace:   literalRef("istio-ingress"),
					SectionName: "https",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept a Gateway API front door with a cert-manager issuer", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{Name: literalRef("main")},
				Tls: &KubernetesPlantonPlatformIngressTls{
					Issuer: &KubernetesPlantonPlatformCertManagerIssuer{
						Name: "letsencrypt",
						Kind: strPtr("ClusterIssuer"),
					},
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		// The reachability declaration: every word is accepted on an enabled
		// door, and "private" is accepted even on a disabled one because it is
		// true there (a port-forward door is reached only from the machine
		// running it). Only the contradiction is refused, in the invalid block.
		ginkgo.It("should accept every reachability word on an enabled door", func() {
			for _, word := range []string{"auto", "public", "private"} {
				input := minimalValidPlatform()
				input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
					Enabled:      true,
					Hostname:     "planton.example.com",
					Reachability: strPtr(word),
				}
				err := protovalidate.Validate(input)
				gomega.Expect(err).To(gomega.BeNil(), "reachability %q", word)
			}
		})

		ginkgo.It("should accept reachability private on a disabled ingress", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{Reachability: strPtr("private")}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept runner workload identity and database growth", func() {
			input := minimalValidPlatform()
			replicas := int32(2)
			input.Spec.Runner = &KubernetesPlantonPlatformRunner{
				ServiceAccountAnnotations: map[string]string{
					"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/planton-runner",
				},
			}
			input.Spec.Database = &KubernetesPlantonPlatformDatabase{
				Postgresql: &KubernetesPlantonPlatformPostgresql{
					Replicas:    &replicas,
					StorageSize: "20Gi",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept the AWS secret backend with its config", func() {
			input := minimalValidPlatform()
			input.Spec.Bootstrap = &KubernetesPlantonPlatformBootstrap{
				SecretBackend: &KubernetesPlantonPlatformSecretBackend{
					Type: "awsSecretsManager",
					AwsSecretsManager: &KubernetesPlantonPlatformAwsSecretsManager{
						Region:    "us-east-1",
						KmsKeyArn: "arn:aws:kms:us-east-1:123456789012:key/abc",
					},
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept a license from a Secret reference", func() {
			input := minimalValidPlatform()
			input.Spec.License = &KubernetesPlantonPlatformLicense{
				SecretKeyRef: &KubernetesPlantonPlatformLicenseSecretKeyRef{
					Name: "planton-license",
					Key:  "key",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept a pre-seeded admin email", func() {
			input := minimalValidPlatform()
			input.Spec.Identity = &KubernetesPlantonPlatformIdentity{
				AdminEmail: "admin@example.com",
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("When invalid input is passed", func() {

		ginkgo.It("should fail when version is missing", func() {
			input := minimalValidPlatform()
			input.Spec.Version = ""
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail when namespace is missing", func() {
			input := minimalValidPlatform()
			input.Spec.Namespace = nil
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on TLS without a hostname", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled: true,
				Tls: &KubernetesPlantonPlatformIngressTls{
					SecretName: "planton-tls",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on TLS with BOTH a Secret and an issuer", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				Tls: &KubernetesPlantonPlatformIngressTls{
					SecretName: "planton-tls",
					Issuer: &KubernetesPlantonPlatformCertManagerIssuer{
						Name: "letsencrypt",
					},
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		// The two Gateway rules mirror the operator's CRD word for word, so a
		// declaration the catalog accepts is one the API server accepts.
		ginkgo.It("should fail when gateway_ref and ingress_class_name name two front doors", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx",
				GatewayRef:       &KubernetesPlantonPlatformGatewayRef{Name: literalRef("main")},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on gateway_ref with a brought certificate Secret (the listener owns it)", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{Name: literalRef("main")},
				Tls:        &KubernetesPlantonPlatformIngressTls{SecretName: "planton-tls"},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on reachability public with the ingress disabled (a port-forward door is never public)", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{Reachability: strPtr("public")}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("reached only through kubectl port-forward"))
		})

		ginkgo.It("should fail on a reachability word outside auto, public, private", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:      true,
				Hostname:     "planton.example.com",
				Reachability: strPtr("internet"),
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on a gateway_ref without a name", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{Namespace: literalRef("istio-ingress")},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		// The Gateway may be Planton's own: both fields are KubernetesGateway
		// foreign keys, so an infra chart wires them with valueFrom and the
		// platform deploys after the Gateway it attaches to.
		ginkgo.It("should accept gateway_ref name and namespace as references to a KubernetesGateway", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{
					Name:        gatewayRef("management-gateway", ""),
					Namespace:   gatewayRef("management-gateway", ""),
					SectionName: "https",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should accept a referenced Gateway name beside a literal namespace", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{
					Name:      gatewayRef("management-gateway", "status.outputs.gateway_name"),
					Namespace: literalRef("istio-ingress"),
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).To(gomega.BeNil())
		})

		ginkgo.It("should fail on a gateway_ref name that is present but empty", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{Name: literalRef("")},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on a gateway_ref namespace that is present but empty", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &KubernetesPlantonPlatformGatewayRef{Name: literalRef("main"), Namespace: literalRef("")},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on TLS with NEITHER a Secret nor an issuer", func() {
			input := minimalValidPlatform()
			input.Spec.Ingress = &KubernetesPlantonPlatformIngress{
				Enabled:  true,
				Hostname: "planton.example.com",
				Tls:      &KubernetesPlantonPlatformIngressTls{},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on the AWS secret backend without its config", func() {
			input := minimalValidPlatform()
			input.Spec.Bootstrap = &KubernetesPlantonPlatformBootstrap{
				SecretBackend: &KubernetesPlantonPlatformSecretBackend{
					Type: "awsSecretsManager",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on a license with BOTH key and secret reference", func() {
			input := minimalValidPlatform()
			input.Spec.License = &KubernetesPlantonPlatformLicense{
				Key: "plk_FAKE_PLACEHOLDER_VALUE",
				SecretKeyRef: &KubernetesPlantonPlatformLicenseSecretKeyRef{
					Name: "planton-license",
					Key:  "key",
				},
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on a malformed storage size", func() {
			input := minimalValidPlatform()
			input.Spec.Storage = &KubernetesPlantonPlatformStorage{
				Size: "ten gigs",
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on a malformed admin email", func() {
			input := minimalValidPlatform()
			input.Spec.Identity = &KubernetesPlantonPlatformIdentity{
				AdminEmail: "not-an-email",
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on an unknown IaC provisioner", func() {
			input := minimalValidPlatform()
			provisioner := "pulumi"
			input.Spec.Bootstrap = &KubernetesPlantonPlatformBootstrap{
				IacProvisioner: &provisioner,
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})

		ginkgo.It("should fail on an out-of-range gateway port", func() {
			input := minimalValidPlatform()
			port := int32(70000)
			input.Spec.Gateway = &KubernetesPlantonPlatformGateway{
				LocalPort: &port,
			}
			err := protovalidate.Validate(input)
			gomega.Expect(err).NotTo(gomega.BeNil())
		})
	})
})
