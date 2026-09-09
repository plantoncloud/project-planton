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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	plantonaiv1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The Gateway API edge through the controller, against the Gateway API's REAL
// CRDs (testdata/crds): every object the operator writes is schema-validated
// by the API server, and the Gateway controller's verdict is simulated by
// writing route status the way a controller would. envtest runs no Gateway
// controller, so acceptance is what the tests decide.
var _ = Describe("PlantonPlatform Gateway API edge", func() {
	const (
		namespace        = "default"
		gatewayNamespace = "gw-system"
	)

	newReconciler := func() *PlantonPlatformReconciler {
		return &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}
	reconcileTwice := func(nn types.NamespacedName) {
		reconciler := newReconciler()
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
	}
	reconcileOnce := func(nn types.NamespacedName) {
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
	}

	createPlatform := func(name string, ingress *plantonaiv1.IngressSpec) *plantonaiv1.PlantonPlatform {
		p := &plantonaiv1.PlantonPlatform{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: ingress},
		}
		Expect(k8sClient.Create(ctx, p)).To(Succeed())
		return p
	}
	deletePlatform := func(p *plantonaiv1.PlantonPlatform) { _ = k8sClient.Delete(context.Background(), p) }

	gatewayGVK := schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.GatewayAPIVersion, Kind: "Gateway"}
	routeGVK := schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.GatewayAPIVersion, Kind: "HTTPRoute"}

	ensureNamespace := func(name string) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		_ = k8sClient.Create(ctx, ns)
	}

	// A Gateway shaped like a cluster team's shared front door: one HTTP and
	// one HTTPS listener for *.example.com, admitting routes from every
	// namespace, publishing an address.
	createGateway := func(name string, listeners []any, addresses []any) *unstructured.Unstructured {
		ensureNamespace(gatewayNamespace)
		gw := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gatewayGVK.GroupVersion().String(),
			"kind":       gatewayGVK.Kind,
			"metadata":   map[string]any{"name": name, "namespace": gatewayNamespace},
			"spec": map[string]any{
				"gatewayClassName": "test-class",
				"listeners":        listeners,
			},
		}}
		Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		if len(addresses) > 0 {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: gatewayNamespace}, gw)).To(Succeed())
			Expect(unstructured.SetNestedSlice(gw.Object, addresses, "status", "addresses")).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, gw)).To(Succeed())
		}
		return gw
	}
	deleteGateway := func(gw *unstructured.Unstructured) { _ = k8sClient.Delete(context.Background(), gw) }

	listener := func(name, protocol string, port int64, hostname, from string, certRefs ...string) map[string]any {
		l := map[string]any{
			"name": name, "protocol": protocol, "port": port,
			"allowedRoutes": map[string]any{"namespaces": map[string]any{"from": from}},
		}
		if hostname != "" {
			l["hostname"] = hostname
		}
		if protocol == "HTTPS" {
			refs := make([]any, 0, len(certRefs))
			for _, ref := range certRefs {
				refs = append(refs, map[string]any{"name": ref, "namespace": namespace, "kind": "Secret", "group": ""})
			}
			l["tls"] = map[string]any{"mode": "Terminate", "certificateRefs": refs}
		}
		return l
	}

	getRoute := func(p *plantonaiv1.PlantonPlatform) *unstructured.Unstructured {
		route := &unstructured.Unstructured{}
		route.SetGroupVersionKind(routeGVK)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resources.HTTPRouteName(p.Name), Namespace: namespace}, route)).To(Succeed())
		return route
	}

	// acceptRoute writes the status a Gateway controller writes when it
	// programs the route for the named parent.
	acceptRoute := func(p *plantonaiv1.PlantonPlatform, gatewayName string) {
		route := getRoute(p)
		parents := []any{map[string]any{
			"parentRef":      map[string]any{"group": resources.GatewayAPIGroup, "kind": "Gateway", "name": gatewayName, "namespace": gatewayNamespace},
			"controllerName": "example.com/test-controller",
			"conditions": []any{
				map[string]any{"type": "Accepted", "status": "True", "reason": "Accepted", "message": "Route was valid",
					"lastTransitionTime": "2026-01-01T00:00:00Z", "observedGeneration": int64(1)},
				map[string]any{"type": "ResolvedRefs", "status": "True", "reason": "ResolvedRefs", "message": "",
					"lastTransitionTime": "2026-01-01T00:00:00Z", "observedGeneration": int64(1)},
			},
		}}
		Expect(unstructured.SetNestedSlice(route.Object, parents, "status", "parents")).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, route)).To(Succeed())
	}

	ingressStatus := func(nn types.NamespacedName) (*plantonaiv1.ComponentStatus, string) {
		var updated plantonaiv1.PlantonPlatform
		Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
		Expect(updated.Status.Components.Ingress).NotTo(BeNil())
		return updated.Status.Components.Ingress, updated.Status.ConsoleURL
	}

	Context("attached to a shared Gateway with an HTTPS listener for the domain", func() {
		It("writes the route table as an HTTPRoute, advertises HTTPS from the listener, and is Ready once the Gateway accepts", func() {
			gw := createGateway("main", []any{
				listener("http", "HTTP", 80, "*.example.com", "All"),
				listener("https", "HTTPS", 443, "*.example.com", "All", "wildcard-example-com"),
			}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-https", &plantonaiv1.IngressSpec{
				Enabled:    true,
				Hostname:   "planton.example.com",
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "main", Namespace: gatewayNamespace},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			route := getRoute(p)
			hostnames, _, _ := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
			Expect(hostnames).To(Equal([]string{"planton.example.com"}))
			parents, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
			Expect(parents).To(HaveLen(1))
			parent := parents[0].(map[string]any)
			Expect(parent["name"]).To(Equal("main"))
			Expect(parent["namespace"]).To(Equal(gatewayNamespace))
			rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
			Expect(rules).To(HaveLen(8), "one rule per entry of the front-door route table")
			var paths []string
			for _, r := range rules {
				rule := r.(map[string]any)
				matches, _, _ := unstructured.NestedSlice(rule, "matches")
				path, _, _ := unstructured.NestedMap(matches[0].(map[string]any), "path")
				Expect(path["type"]).To(Equal("PathPrefix"), "every rule is Kubernetes-core path matching")
				paths = append(paths, path["value"].(string))
			}
			// The keyless issuer's two documents and the webhook namespace reach
			// the control plane's webhook port; the root prefix appears twice:
			// first narrowed to native gRPC by an Exact content-type header
			// match, then the console catch-all.
			Expect(paths).To(Equal([]string{
				resources.APIPathPrefix, resources.StoragePathPrefix, resources.IdentityPathPrefix,
				resources.OIDCDiscoveryPath, resources.OIDCJWKSPath, resources.WebhooksPathPrefix, "/", "/",
			}))
			for _, idx := range []int{3, 4, 5} {
				backends, _, _ := unstructured.NestedSlice(rules[idx].(map[string]any), "backendRefs")
				Expect(backends[0].(map[string]any)["port"]).To(Equal(int64(8086)), "rule %d delivers to the control plane's webhook port", idx)
			}
			grpcMatches, _, _ := unstructured.NestedSlice(rules[6].(map[string]any), "matches")
			grpcHeaders, _, _ := unstructured.NestedSlice(grpcMatches[0].(map[string]any), "headers")
			Expect(grpcHeaders).To(HaveLen(1), "the native-gRPC rule is the root prefix plus one header match")
			Expect(grpcHeaders[0].(map[string]any)["name"]).To(Equal(resources.GRPCContentTypeHeader))
			consoleMatches, _, _ := unstructured.NestedSlice(rules[7].(map[string]any), "matches")
			_, consoleHasHeaders, _ := unstructured.NestedSlice(consoleMatches[0].(map[string]any), "headers")
			Expect(consoleHasHeaders).To(BeFalse(), "the console catch-all is a bare prefix")
			timeouts, found, _ := unstructured.NestedMap(rules[0].(map[string]any), "timeouts")
			Expect(found).To(BeTrue(), "the API rule disables the request timeout for server streams")
			Expect(timeouts["request"]).To(Equal("0s"))
			_, found, _ = unstructured.NestedMap(rules[6].(map[string]any), "timeouts")
			Expect(found).To(BeTrue(), "the native-gRPC rule disables the request timeout for server streams")
			_, found, _ = unstructured.NestedMap(rules[7].(map[string]any), "timeouts")
			Expect(found).To(BeFalse(), "console pages keep the Gateway's default timeout")
			_, found, _ = unstructured.NestedMap(rules[3].(map[string]any), "timeouts")
			Expect(found).To(BeFalse(), "discovery documents are ordinary short responses")

			status, url := ingressStatus(nn)
			Expect(url).To(Equal("https://planton.example.com"), "the scheme follows the HTTPS listener, no tls block needed")
			var withReach plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &withReach)).To(Succeed())
			Expect(withReach.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPublic),
				"auto on an HTTPS listener resolves public, published beside the URL")
			Expect(status.Phase).NotTo(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(status.Message).To(ContainSubstring("waiting for the controller of Gateway gw-system/main to accept"))

			acceptRoute(p, "main")
			reconcileOnce(nn)
			status, _ = ingressStatus(nn)
			Expect(status.Phase).To(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(status.Message).To(ContainSubstring("Console at https://planton.example.com via Gateway gw-system/main"))
		})
	})

	Context("when the Gateway does not admit the platform", func() {
		It("publishes the declared word when the person overrides auto with private", func() {
			gw := createGateway("main-private", []any{
				listener("https", "HTTPS", 443, "*.example.com", "All", "wildcard-example-com"),
			}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-private", &plantonaiv1.IngressSpec{
				Enabled:      true,
				Hostname:     "planton.example.com",
				GatewayRef:   &plantonaiv1.GatewayParentRef{Name: "main-private", Namespace: gatewayNamespace},
				Reachability: plantonaiv1.IngressReachabilityPrivate,
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConsoleURL).To(Equal("https://planton.example.com"))
			Expect(updated.Status.Reachability).To(Equal(plantonaiv1.IngressReachabilityPrivate),
				"a declared word stands over the door's HTTPS shape")
		})

		It("names the missing Gateway and the ones that exist", func() {
			gw := createGateway("elsewhere", []any{listener("http", "HTTP", 80, "", "All")}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-missing", &plantonaiv1.IngressSpec{
				Enabled: true, Hostname: "planton.example.com",
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "nope", Namespace: gatewayNamespace},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			status, _ := ingressStatus(nn)
			Expect(status.Phase).NotTo(Equal(plantonaiv1.ComponentPhaseReady))
			Expect(status.Message).To(ContainSubstring("Gateway gw-system/nope not found"))
			Expect(status.Message).To(ContainSubstring("gw-system/elsewhere"))
		})

		It("explains a listener that does not allow routes from the platform's namespace", func() {
			gw := createGateway("same-only", []any{listener("http", "HTTP", 80, "", "Same")}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-ns-refused", &plantonaiv1.IngressSpec{
				Enabled: true, Hostname: "planton.example.com",
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "same-only", Namespace: gatewayNamespace},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			status, _ := ingressStatus(nn)
			Expect(status.Message).To(ContainSubstring(`do not allow routes from namespace "default"`))
			Expect(status.Message).To(ContainSubstring("allowedRoutes.namespaces"))
		})

		It("explains a hostname no listener covers", func() {
			gw := createGateway("other-domain", []any{listener("http", "HTTP", 80, "*.other.com", "All")}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-host-refused", &plantonaiv1.IngressSpec{
				Enabled: true, Hostname: "planton.example.com",
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "other-domain", Namespace: gatewayNamespace},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			status, _ := ingressStatus(nn)
			Expect(status.Message).To(ContainSubstring("no HTTP or HTTPS listener on Gateway gw-system/other-domain admits the hostname planton.example.com"))
			Expect(status.Message).To(ContainSubstring("*.other.com"))
		})
	})

	Context("with no hostname", func() {
		It("derives a magic-DNS hostname from the Gateway's published address, once", func() {
			gw := createGateway("addressed", []any{listener("http", "HTTP", 80, "", "All")},
				[]any{map[string]any{"type": "IPAddress", "value": "203.0.113.9"}})
			defer deleteGateway(gw)

			p := createPlatform("gw-auto", &plantonaiv1.IngressSpec{
				Enabled:    true,
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "addressed", Namespace: gatewayNamespace},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			_, url := ingressStatus(nn)
			Expect(url).To(Equal("http://gw-auto-default.203-0-113-9.sslip.io"))
			route := getRoute(p)
			Expect(route.GetAnnotations()).To(HaveKeyWithValue(resources.DerivedHostnameAnnotation, "gw-auto-default.203-0-113-9.sslip.io"),
				"the derivation is recorded so a re-published address never changes the URL sign-in was baked with")
		})
	})

	Context("with a cert-manager issuer", func() {
		It("asks for the certificate, grants the Gateway's namespace the reference, and names the one listener edit", func() {
			gw := createGateway("tls-main", []any{
				listener("https", "HTTPS", 443, "*.example.com", "All", "some-other-cert"),
			}, nil)
			defer deleteGateway(gw)

			p := createPlatform("gw-issuer", &plantonaiv1.IngressSpec{
				Enabled: true, Hostname: "planton.example.com",
				GatewayRef: &plantonaiv1.GatewayParentRef{Name: "tls-main", Namespace: gatewayNamespace},
				TLS:        &plantonaiv1.IngressTLSSpec{Issuer: &plantonaiv1.CertManagerIssuerRef{Name: "lab-ca", Kind: "ClusterIssuer"}},
			})
			defer deletePlatform(p)
			nn := types.NamespacedName{Name: p.Name, Namespace: namespace}
			reconcileTwice(nn)

			cert := &unstructured.Unstructured{}
			cert.SetGroupVersionKind(schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resources.CertificateName(p.Name), Namespace: namespace}, cert)).To(Succeed())
			dnsNames, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
			Expect(dnsNames).To(Equal([]string{"planton.example.com"}))
			issuerKind, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind")
			Expect(issuerKind).To(Equal("ClusterIssuer"))

			grant := &unstructured.Unstructured{}
			grant.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.ReferenceGrantAPIVersion, Kind: "ReferenceGrant"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resources.ReferenceGrantName(p.Name), Namespace: namespace}, grant)).To(Succeed(),
				"the grant lives in the Secret's namespace and permits the Gateway's namespace")
			from, _, _ := unstructured.NestedSlice(grant.Object, "spec", "from")
			Expect(from[0].(map[string]any)["namespace"]).To(Equal(gatewayNamespace))

			// cert-manager is not running in envtest: the operator waits on issuance first.
			status, _ := ingressStatus(nn)
			Expect(status.Message).To(ContainSubstring("waiting for the certificate for planton.example.com"))

			// Issued: now the only thing missing is the listener referencing the Secret.
			Expect(unstructured.SetNestedSlice(cert.Object, []any{map[string]any{"type": "Ready", "status": "True"}}, "status", "conditions")).To(Succeed())
			Expect(k8sClient.Update(ctx, cert)).To(Succeed())
			reconcileOnce(nn)
			status, _ = ingressStatus(nn)
			Expect(status.Message).To(ContainSubstring("must reference it in tls.certificateRefs"))
			Expect(status.Message).To(ContainSubstring("default/" + resources.IngressTLSSecretName(p.Name)))
			Expect(status.Message).To(ContainSubstring("ReferenceGrant permitting the cross-namespace reference is in place"))
		})
	})

	Context("admission rules", func() {
		It("rejects naming both front doors", func() {
			p := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-two-doors", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: true, Hostname: "planton.example.com", IngressClassName: "nginx",
					GatewayRef: &plantonaiv1.GatewayParentRef{Name: "main"},
				}},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gatewayRef and ingressClassName name two different front doors"))
		})

		It("rejects a brought certificate Secret on the Gateway edge", func() {
			p := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-gw-secret", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: true, Hostname: "planton.example.com",
					GatewayRef: &plantonaiv1.GatewayParentRef{Name: "main"},
					TLS:        &plantonaiv1.IngressTLSSpec{SecretName: "my-cert"},
				}},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("the Gateway's HTTPS listener owns the certificate"))
		})

		// The reachability declaration is judged for the one contradiction only:
		// "public" on a door that does not exist. A "private" declaration on a
		// disabled ingress is a true statement about a port-forward door, and the
		// other ingress fields (hostname, class, annotations) are likewise accepted
		// and ignored on a disabled block -- refusing the truth would make this the
		// one field coupled to enabled.
		It("rejects reachability public on a disabled ingress", func() {
			p := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-public-port-forward", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: false, Reachability: plantonaiv1.IngressReachabilityPublic,
				}},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reached only through kubectl port-forward"))
		})

		It("accepts reachability private on a disabled ingress and defaults an omitted declaration to auto", func() {
			private := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-private-port-forward", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: false, Reachability: plantonaiv1.IngressReachabilityPrivate,
				}},
			}
			Expect(k8sClient.Create(ctx, private)).To(Succeed())

			omitted := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-reachability-omitted", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: true, Hostname: "planton.example.com",
				}},
			}
			Expect(k8sClient.Create(ctx, omitted)).To(Succeed())
			stored := &plantonaiv1.PlantonPlatform{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: omitted.Name, Namespace: namespace}, stored)).To(Succeed())
			// The CRD default is what every consumer of the field relies on: a
			// manifest that never mentions reachability reads as "auto", never "".
			Expect(stored.Spec.Ingress.Reachability).To(Equal(plantonaiv1.IngressReachabilityAuto))
		})

		It("rejects a reachability word outside the declared three", func() {
			p := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-reachability-unknown", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Ingress: &plantonaiv1.IngressSpec{
					Enabled: true, Hostname: "planton.example.com", Reachability: "internet",
				}},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("supported values"))
		})
	})
})
