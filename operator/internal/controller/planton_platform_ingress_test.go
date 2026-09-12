/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	plantonaiv1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// Ingress component behavior through the controller: the friction ladder's
// preflights and the one-hostname routing contract. envtest runs no ingress
// controller, so no address is ever published -- which is exactly what the
// auto-hostname waiting path needs.
var _ = Describe("PlantonPlatform ingress", func() {
	const namespace = "default"

	newReconciler := func() *PlantonPlatformReconciler {
		return &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	// reconcileTwice runs initialization and then a component pass.
	reconcileTwice := func(nn types.NamespacedName) {
		reconciler := newReconciler()
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
	}

	createPlatform := func(name string, ingress *plantonaiv1.IngressSpec) *plantonaiv1.PlantonPlatform {
		p := &plantonaiv1.PlantonPlatform{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: plantonaiv1.PlantonPlatformSpec{
				Version: "v1.0.0",
				Ingress: ingress,
			},
		}
		Expect(k8sClient.Create(ctx, p)).To(Succeed())
		return p
	}

	deletePlatform := func(p *plantonaiv1.PlantonPlatform) {
		_ = k8sClient.Delete(context.Background(), p)
	}

	createIngressClass := func(name, controller string) *networkingv1.IngressClass {
		class := &networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       networkingv1.IngressClassSpec{Controller: controller},
		}
		Expect(k8sClient.Create(ctx, class)).To(Succeed())
		return class
	}

	Context("with an explicit hostname and an existing IngressClass", func() {
		It("creates the host-pinned Ingress, advertises the URL, and points the console at it", func() {
			class := createIngressClass("nginx-explicit", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-explicit", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx-explicit",
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.IngressName(p.Name), Namespace: namespace,
			}, &ing)).To(Succeed())

			Expect(ing.Spec.Rules).To(HaveLen(1))
			Expect(ing.Spec.Rules[0].Host).To(Equal("planton.example.com"))
			paths := ing.Spec.Rules[0].HTTP.Paths
			Expect(paths).To(HaveLen(7))
			Expect(paths[0].Path).To(Equal(resources.APIPathPrefix), "one plain prefix rule covers the whole API surface")
			Expect(paths[0].Backend.Service.Port.Name).To(Equal("grpc-web"), "the browser dialect port, never raw gRPC")
			Expect(paths[1].Path).To(Equal("/storage"), "the storage relay rides the control plane's API port")
			Expect(paths[2].Path).To(Equal("/idp"), "the identity server rides the same hostname")
			Expect(paths[3].Path).To(Equal(resources.OIDCDiscoveryPath), "the keyless issuer's discovery document at the door's root")
			Expect(paths[4].Path).To(Equal(resources.OIDCJWKSPath), "and its keys")
			Expect(paths[5].Path).To(Equal(resources.WebhooksPathPrefix), "inbound webhooks under their own namespace")
			for _, idx := range []int{3, 4, 5} {
				Expect(paths[idx].Backend.Service.Port.Name).To(Equal("webhook"), "path %s reaches the control plane's webhook port", paths[idx].Path)
			}
			Expect(paths[6].Path).To(Equal("/"))
			Expect(ing.Annotations).To(HaveKeyWithValue(
				"nginx.ingress.kubernetes.io/proxy-buffering", "off"),
				"nginx detected via the IngressClass controller value")

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConsoleURL).To(Equal("http://planton.example.com"))
			Expect(updated.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPrivate),
				"auto reads a plain-HTTP door as private: no cloud accepts it as an issuer")
			Expect(updated.Status.Components.Ingress).NotTo(BeNil())
			Expect(updated.Status.Components.Ingress.Phase).To(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring("unencrypted"),
				"plain HTTP must be stated, not silent")
		})
	})

	Context("with a named IngressClass that does not exist", func() {
		It("explains the failure and lists the classes that DO exist", func() {
			class := createIngressClass("traefik-real", "traefik.io/ingress-controller")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-badclass", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "does-not-exist",
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress).NotTo(BeNil())
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring(`"does-not-exist" not found`))
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring("traefik-real"))
		})
	})

	Context("with auto-hostname while no address is published", func() {
		It("waits with an actionable message and holds the console back", func() {
			class := createIngressClass("nginx-auto", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-auto", &plantonaiv1.IngressSpec{
				Enabled:          true,
				IngressClassName: "nginx-auto",
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress.Message).To(
				ContainSubstring("set spec.ingress.hostname"),
				"the wait must tell the user their way out")
			Expect(updated.Status.ConsoleURL).To(BeEmpty())

			// The host-less Ingress was still admitted so the controller can
			// publish an address on a real cluster.
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.IngressName(p.Name), Namespace: namespace,
			}, &ing)).To(Succeed())
			Expect(ing.Spec.Rules[0].Host).To(BeEmpty())

			// The console's own wait-for-URL gate cannot be observed here:
			// its controlplane dependency never becomes Ready in envtest.
			// It is unit-tested at the component level instead.
		})
	})

	Context("with a BYO TLS secret that does not exist", func() {
		It("fails preflight with a named, actionable message", func() {
			class := createIngressClass("nginx-tls", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-tls", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx-tls",
				TLS:              &plantonaiv1.IngressTLSSpec{SecretName: "missing-cert"},
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring(`"missing-cert" not found`))
		})
	})

	Context("with a cert-manager issuer", func() {
		// A helper shaping the fixture Certificate the way ingress-shim
		// creates it: located by spec.secretName (the operator's contract),
		// carrying a Ready condition with cert-manager's own message.
		newCertificate := func(name, secretName string, ready bool, message string) *unstructured.Unstructured {
			cert := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "cert-manager.io/v1",
				"kind":       "Certificate",
				"metadata":   map[string]any{"name": name, "namespace": namespace},
				"spec":       map[string]any{"secretName": secretName},
				"status": map[string]any{
					"conditions": []any{map[string]any{
						"type":    "Ready",
						"status":  map[bool]string{true: "True", false: "False"}[ready],
						"message": message,
					}},
				},
			}}
			return cert
		}

		It("holds readiness until the certificate is issued, guiding the DNS task, while the URL keeps the rest of the platform converging", func() {
			class := createIngressClass("nginx-issuer", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-issuer", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx-issuer",
				TLS: &plantonaiv1.IngressTLSSpec{
					Issuer: &plantonaiv1.CertManagerIssuerRef{Name: "lab-ca", Kind: "ClusterIssuer"},
				},
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			// No Certificate yet (envtest runs no cert-manager): the wait
			// names the missing actor, and the advertised URL is ALREADY
			// published so nothing else waits on the certificate.
			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress.Phase).NotTo(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring("cert-manager"))
			Expect(updated.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPublic),
				"auto on an HTTPS hostname resolves public the moment the URL is published, before the certificate is issued")
			Expect(updated.Status.ConsoleURL).To(Equal("https://planton.example.com"),
				"the URL must publish during the certificate wait -- the platform converges while the person points DNS")

			// The Ingress asks cert-manager via the annotation and targets
			// the derived Secret.
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.IngressName(p.Name), Namespace: namespace,
			}, &ing)).To(Succeed())
			Expect(ing.Annotations).To(HaveKeyWithValue("cert-manager.io/cluster-issuer", "lab-ca"))
			Expect(ing.Spec.TLS).To(HaveLen(1))
			Expect(ing.Spec.TLS[0].SecretName).To(Equal(resources.IngressTLSSecretName(p.Name)))

			// A pending Certificate: cert-manager's own reason is relayed and
			// the DNS task is named.
			cert := newCertificate("ing-issuer-cert", resources.IngressTLSSecretName(p.Name), false,
				"Issuing certificate as Secret does not exist")
			Expect(k8sClient.Create(ctx, cert)).To(Succeed())
			defer func() { _ = k8sClient.Delete(context.Background(), cert) }()

			reconcileTwice(nn)
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress.Phase).NotTo(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(updated.Status.Components.Ingress.Message).To(
				ContainSubstring("Issuing certificate as Secret does not exist"),
				"cert-manager's own reason must be relayed verbatim")
			Expect(updated.Status.Components.Ingress.Message).To(
				ContainSubstring("point planton.example.com at"),
				"the wait must name the DNS task -- with or without a published address")

			// Issued: the door is honestly Ready.
			cert.Object["status"] = map[string]any{
				"conditions": []any{map[string]any{"type": "Ready", "status": "True", "message": "Certificate is up to date"}},
			}
			Expect(k8sClient.Update(ctx, cert)).To(Succeed())

			reconcileTwice(nn)
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress.Phase).To(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(updated.Status.Components.Ingress.Message).To(ContainSubstring("https://planton.example.com"))
		})
	})

	Context("with ingress enabled (identity rides the toggle)", func() {
		It("seeds the identity slot and gates it on the database", func() {
			class := createIngressClass("nginx-identity", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-identity", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx-identity",
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			// A published platform always carries the identity server: the
			// slot exists without any identity config in the spec.
			Expect(updated.Status.Components.Identity).NotTo(BeNil())
			// PostgreSQL never becomes Ready in envtest, so identity must be
			// held at its dependency gate rather than deploying blind.
			Expect(updated.Status.Components.Identity.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			Expect(updated.Status.Components.Identity.Message).To(ContainSubstring("postgresql"))
		})

		It("serves no-ingress installs through the gateway front door with sign-in", func() {
			p := createPlatform("no-ingress", nil)
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress).To(BeNil())
			// The gateway IS the front door without ingress -- it deploys
			// immediately (no dependencies) and advertises the deterministic
			// port-forward URL everything else derives from.
			Expect(updated.Status.Components.Gateway).NotTo(BeNil())
			Expect(updated.Status.ConsoleURL).To(Equal("http://localhost:8080"))
			Expect(updated.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPrivate),
				"the port-forward door is private; the discovery it serves is honest about localhost")
			// Sign-in is unconditional: the identity slot exists without
			// ingress, held at its database dependency gate in envtest.
			Expect(updated.Status.Components.Identity).NotTo(BeNil())
			Expect(updated.Status.Components.Identity.Phase).To(Equal(plantonaiv1.ComponentPhasePending))

			// The gateway's front-door objects exist: the routing ConfigMap,
			// the Deployment, and the Service the port-forward targets.
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.GatewayConfigMapName(p.Name), Namespace: namespace,
			}, &cm)).To(Succeed())
			Expect(cm.Data[resources.GatewayConfigKey]).To(ContainSubstring("location "+resources.APIPathPrefix+" {"),
				"the API routes by the plain path namespace, the same rule the Ingress carries")
			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.GatewayServiceName(p.Name), Namespace: namespace,
			}, &svc)).To(Succeed())
		})
	})

	Context("when ingress is disabled after being enabled", func() {
		It("switches the front door back to the gateway and republishes the URL", func() {
			class := createIngressClass("nginx-toggle", "k8s.io/ingress-nginx")
			defer func() { _ = k8sClient.Delete(context.Background(), class) }()

			p := createPlatform("ing-toggle", &plantonaiv1.IngressSpec{
				Enabled:          true,
				Hostname:         "planton.example.com",
				IngressClassName: "nginx-toggle",
			})
			defer deletePlatform(p)

			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConsoleURL).To(Equal("http://planton.example.com"))
			Expect(updated.Status.Components.Gateway).To(BeNil(),
				"exactly one front door: no gateway while ingress serves")

			updated.Spec.Ingress.Enabled = false
			Expect(k8sClient.Update(ctx, &updated)).To(Succeed())
			reconcileTwice(nn)

			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Ingress).To(BeNil())
			Expect(updated.Status.Components.Gateway).NotTo(BeNil(),
				"the gateway returns as the front door when ingress retires")
			// Identity survives the switch -- sign-in is unconditional.
			Expect(updated.Status.Components.Identity).NotTo(BeNil())
			// The advertised URL is now the gateway's, republished in the
			// same pass that retired the ingress URL.
			Expect(updated.Status.ConsoleURL).To(Equal("http://localhost:8080"))
			Expect(updated.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPrivate),
				"the port-forward door is private; the discovery it serves is honest about localhost")
		})
	})
})
