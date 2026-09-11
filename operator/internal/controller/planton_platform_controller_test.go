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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	resource_ "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	plantonaiv1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/component"
	"github.com/plantonhq/planton/operator/internal/janitor"
	"github.com/plantonhq/planton/operator/internal/platformversion"
	"github.com/plantonhq/planton/operator/internal/resources"
	"github.com/plantonhq/planton/operator/internal/status"
)

var _ = Describe("PlantonPlatform Controller", func() {
	const (
		resourceName = "test-planton"
		namespace    = "default"
		timeout      = 10 * time.Second
		interval     = 250 * time.Millisecond
	)

	namespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: namespace,
	}

	Context("When creating a minimal PlantonPlatform resource", func() {
		var resource *plantonaiv1.PlantonPlatform

		BeforeEach(func() {
			resource = &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
				},
			}

			existing := &plantonaiv1.PlantonPlatform{}
			err := k8sClient.Get(ctx, namespacedName, existing)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			existing := &plantonaiv1.PlantonPlatform{}
			err := k8sClient.Get(ctx, namespacedName, existing)
			if err == nil {
				Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
			}
		})

		It("should initialize status to Pending on the first reconciliation", func() {
			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "first reconcile should request immediate requeue after initialization")

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(plantonaiv1.PhasePending))
			Expect(updated.Status.Version).To(Equal("v1.0.0"))
		})

		It("should initialize all component statuses to Pending", func() {
			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			Expect(updated.Status.Components.PostgreSQL).NotTo(BeNil())
			Expect(updated.Status.Components.PostgreSQL.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			Expect(updated.Status.Components.Redis).NotTo(BeNil())
			Expect(updated.Status.Components.Redis.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			// The policy engine is part of every platform, like its
			// database: the minimal footprint carries its slot.
			Expect(updated.Status.Components.OpenFGA).NotTo(BeNil(), "OpenFGA should be initialized on every platform")
			// The bundled secrets manager is integral: the version-only
			// manifest deploys it (opting out is the deliberate act).
			Expect(updated.Status.Components.OpenBAO).NotTo(BeNil(), "OpenBAO should be initialized by default")
			Expect(updated.Status.Components.OpenBAO.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			Expect(updated.Status.Components.Temporal).NotTo(BeNil())
			Expect(updated.Status.Components.Temporal.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			Expect(updated.Status.Components.ControlPlane).NotTo(BeNil(), "ControlPlane should be initialized")
			Expect(updated.Status.Components.ControlPlane.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
			Expect(updated.Status.Components.Console).NotTo(BeNil(), "Console should be initialized")
			Expect(updated.Status.Components.Console.Phase).To(Equal(plantonaiv1.ComponentPhasePending))
		})

		It("should set Ready condition to False", func() {
			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			Expect(updated.Status.Conditions).To(HaveLen(1))

			readyCond := findCondition(updated.Status.Conditions, plantonaiv1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should reconcile components after initialization and requeue", func() {
			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile: initializes status
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: reconciles components. In standalone mode,
			// components without dependencies begin deploying Helm charts.
			// The overall phase moves to Deploying while waiting for pods.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(plantonaiv1.PhaseDeploying))
		})
	})

	Context("When the PlantonPlatform resource does not exist", func() {
		It("should not return an error for a missing resource", func() {
			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent",
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})

		// The deletion path on a real API server: a platform's cluster-scoped
		// satellite (the token-reviewer pair, labelled with the platform's
		// UID) cannot be garbage-collected by a namespaced owner, so the
		// reconcile of the vanished resource hands it to the janitor. A
		// second platform's pair -- same name shape, different UID -- stays.
		It("should sweep the deleted platform's cluster-scoped satellites and leave a live platform's alone", func() {
			departed := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "departing", Namespace: namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0"},
			}
			Expect(k8sClient.Create(ctx, departed)).To(Succeed())
			staying := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "staying", Namespace: namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0"},
			}
			Expect(k8sClient.Create(ctx, staying)).To(Succeed())
			defer func() { _ = k8sClient.Delete(context.Background(), staying) }()

			pairFor := func(p *plantonaiv1.PlantonPlatform) (*rbacv1.ClusterRole, *rbacv1.ClusterRoleBinding) {
				cfg := resources.ControlPlaneConfig{
					CRName: p.Name, Namespace: p.Namespace,
					OwnerRef: &metav1.OwnerReference{Kind: "PlantonPlatform", Name: p.Name, UID: p.UID},
				}
				return resources.ControlPlaneTokenReviewerClusterRole(cfg), resources.ControlPlaneTokenReviewerClusterRoleBinding(cfg)
			}
			for _, p := range []*plantonaiv1.PlantonPlatform{departed, staying} {
				role, binding := pairFor(p)
				Expect(k8sClient.Create(ctx, role)).To(Succeed())
				Expect(k8sClient.Create(ctx, binding)).To(Succeed())
			}
			stayingRole, stayingBinding := pairFor(staying)
			defer func() {
				_ = k8sClient.Delete(context.Background(), stayingRole)
				_ = k8sClient.Delete(context.Background(), stayingBinding)
			}()

			Expect(k8sClient.Delete(ctx, departed)).To(Succeed())

			reconciler := &PlantonPlatformReconciler{
				Client:  k8sClient,
				Scheme:  k8sClient.Scheme(),
				Janitor: &janitor.Janitor{Client: k8sClient, SubOperators: []component.SubOperatorOptions{}},
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: departed.Name, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			departedRole, departedBinding := pairFor(departed)
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: departedRole.Name}, &rbacv1.ClusterRole{}))).To(BeTrue(),
				"the departed platform's ClusterRole must be swept")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: departedBinding.Name}, &rbacv1.ClusterRoleBinding{}))).To(BeTrue(),
				"the departed platform's ClusterRoleBinding must be swept")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stayingRole.Name}, &rbacv1.ClusterRole{})).To(Succeed(),
				"a live platform's ClusterRole must stay")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stayingBinding.Name}, &rbacv1.ClusterRoleBinding{})).To(Succeed(),
				"a live platform's ClusterRoleBinding must stay")
		})
	})

	Context("When a data component's volume is stuck Pending", func() {
		It("should explain the storage problem in the component status instead of a generic wait", func() {
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-explain",
					Namespace: namespace,
				},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), resource)
			}()

			// The cluster shape both real installs hit: classes exist, none
			// is default, and the platform pins nothing. Envtest runs no PV
			// controller, so the Pending phase is set explicitly -- exactly
			// the state a real claim sits in forever on such a cluster.
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "trident"},
				Provisioner: "csi.trident.netapp.io",
			}
			Expect(k8sClient.Create(ctx, sc)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), sc)
			}()

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					// A CloudNativePG instance claim carries the cluster
					// label; that label, not the name, is how the postgres
					// component knows the claim is its own before the
					// instance pod exists.
					Name:      "storage-explain-postgres-1",
					Namespace: namespace,
					Labels:    map[string]string{"cnpg.io/cluster": "storage-explain-postgres"},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource_.MustParse("10Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), pvc)
			}()
			pvc.Status.Phase = corev1.ClaimPending
			Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())

			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			nn := types.NamespacedName{Name: "storage-explain", Namespace: namespace}
			// First reconcile initializes status; the second runs components
			// (installing the CloudNativePG release into envtest when no
			// earlier spec already has).
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Envtest runs no deployment controller, so the CloudNativePG
			// controller never becomes available on its own; marking it
			// available lets the component pass its sub-operator gate, apply
			// the Cluster CR against the real (envtest-registered) CRD, and
			// reach the storage diagnosis under test.
			var cnpgDeploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "cnpg-controller-manager", Namespace: "cnpg-system",
			}, &cnpgDeploy)).To(Succeed())
			// A completed rollout, as the readiness test defines it.
			cnpgDeploy.Status.ObservedGeneration = cnpgDeploy.Generation
			cnpgDeploy.Status.Replicas = 1
			cnpgDeploy.Status.UpdatedReplicas = 1
			cnpgDeploy.Status.ReadyReplicas = 1
			cnpgDeploy.Status.AvailableReplicas = 1
			Expect(k8sClient.Status().Update(ctx, &cnpgDeploy)).To(Succeed())

			// The release also registered CloudNativePG's admission webhooks,
			// which nothing serves in envtest -- a Cluster apply would time
			// out against them. In a real cluster the gate's availability
			// check guarantees the webhook server is up before any Cluster
			// apply; deleting the configurations models "no controller runs
			// here" faithfully.
			for _, hookRef := range []struct{ kind, name string }{
				{"MutatingWebhookConfiguration", "cnpg-mutating-webhook-configuration"},
				{"ValidatingWebhookConfiguration", "cnpg-validating-webhook-configuration"},
			} {
				hook := &unstructured.Unstructured{}
				hook.SetGroupVersionKind(schema.GroupVersionKind{
					Group: "admissionregistration.k8s.io", Version: "v1", Kind: hookRef.kind,
				})
				hook.SetName(hookRef.name)
				_ = k8sClient.Delete(ctx, hook)
			}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.PostgreSQL).NotTo(BeNil())
			msg := updated.Status.Components.PostgreSQL.Message
			Expect(msg).To(ContainSubstring("no default StorageClass"),
				"the status must name the actual problem, got: %s", msg)
			Expect(msg).To(ContainSubstring("spec.storage.storageClassName"),
				"the status must name the fix, got: %s", msg)
			Expect(msg).To(ContainSubstring("trident"),
				"the status must list the classes that exist, got: %s", msg)
		})
	})

	Context("When the platform secret backend meets the vault opt-out", func() {
		// The CEL guard runs in the real API server (envtest applies the
		// actual CRD), so these specs pin the rule's semantics -- and the
		// suite bootstrap itself catches the malformed-CEL class (curly
		// quotes) that has shipped three times.
		It("should accept a platform secret backend on the default (vault-on) install", func() {
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-vault-default", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
					Bootstrap: &plantonaiv1.BootstrapSpec{
						SecretBackend: &plantonaiv1.BootstrapSecretBackendSpec{Type: "platform"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed(),
				"the vault runs by default; a platform backend needs no explicit enable")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should reject a platform secret backend when the vault is explicitly opted out", func() {
			off := false
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-vault-optout", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
					Vault:   &plantonaiv1.OpenBAOSpec{Enabled: &off},
					Bootstrap: &plantonaiv1.BootstrapSpec{
						SecretBackend: &plantonaiv1.BootstrapSecretBackendSpec{Type: "platform"},
					},
				},
			}
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred(),
				"a platform backend with no vault to store it in must be rejected at apply time")
			Expect(err.Error()).To(ContainSubstring("spec.vault.enabled: false"),
				"the rejection must explain itself, got: %v", err)
		})
	})

	Context("When a license declares both delivery forms", func() {
		// Same rationale as the vault CEL specs: the rule runs in the real
		// API server, so these pin its semantics and the suite bootstrap
		// catches malformed CEL before it ships.
		It("should accept either delivery form alone", func() {
			inline := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-license-inline", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
					License: &plantonaiv1.LicenseSpec{Key: "plk1.1.claims.signature"},
				},
			}
			Expect(k8sClient.Create(ctx, inline)).To(Succeed())
			Expect(k8sClient.Delete(ctx, inline)).To(Succeed())

			byRef := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-license-ref", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
					License: &plantonaiv1.LicenseSpec{
						SecretKeyRef: &plantonaiv1.SecretKeyRef{Name: "acme-license", Key: "license-key"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, byRef)).To(Succeed())
			Expect(k8sClient.Delete(ctx, byRef)).To(Succeed())
		})

		It("should reject a license with both key and secretKeyRef", func() {
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-license-both", Namespace: namespace},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
					License: &plantonaiv1.LicenseSpec{
						Key:          "plk1.1.claims.signature",
						SecretKeyRef: &plantonaiv1.SecretKeyRef{Name: "acme-license", Key: "license-key"},
					},
				},
			}
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred(),
				"two delivery forms for one key is ambiguous and must be rejected at apply time")
			Expect(err.Error()).To(ContainSubstring("at most one of key or secretKeyRef"),
				"the rejection must explain itself, got: %v", err)
		})
	})

	Context("When reconciling an already-initialized resource", func() {
		It("should skip initialization on subsequent reconciliations", func() {
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "already-init",
					Namespace: namespace,
				},
				Spec: plantonaiv1.PlantonPlatformSpec{
					Version: "v1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), resource)
			}()

			reconciler := &PlantonPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			nn := types.NamespacedName{Name: "already-init", Namespace: namespace}

			// First reconcile: initializes status, immediate requeue
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Second reconcile: skips initialization, runs phases
			result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "should not request immediate requeue")
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "should requeue after interval")
		})
	})
	Context("When spec.version names a platform this operator cannot run", func() {
		// The floor runs in the reconciler (an operator upgrade can outgrow a
		// running platform, which no admission rule sees); the shape rule
		// runs in the API server. Both are pinned here against the real CRD.
		reconcileTwice := func(name types.NamespacedName) reconcile.Result {
			reconciler := &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name}) // initializes status
			Expect(err).NotTo(HaveOccurred())
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name}) // judges the version
			Expect(err).NotTo(HaveOccurred())
			return result
		}

		It("should refuse a release below the floor before creating anything, and explain it where kubectl get shows it", func() {
			name := types.NamespacedName{Name: "below-floor", Namespace: namespace}
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v0.0.41-selfhosted-preview"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { Expect(k8sClient.Delete(ctx, resource)).To(Succeed()) }()

			result := reconcileTwice(name)
			Expect(result.Requeue).To(BeFalse(), "nothing to watch until the spec changes")
			Expect(result.RequeueAfter).To(BeZero())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, name, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(plantonaiv1.PhaseError))
			Expect(updated.Status.Version).To(Equal("v0.0.41-selfhosted-preview"), "the column echoes what was declared")

			supported := findCondition(updated.Status.Conditions, plantonaiv1.ConditionVersionSupported)
			Expect(supported).NotTo(BeNil())
			Expect(supported.Status).To(Equal(metav1.ConditionFalse))
			Expect(supported.Reason).To(Equal(platformversion.ReasonBelowOperatorMinimum))

			ready := findCondition(updated.Status.Conditions, plantonaiv1.ConditionReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(status.ReasonPlatformVersionUnsupported))
			Expect(ready.Message).To(Equal(supported.Message), "the Ready message is the one the Message column prints")
			Expect(ready.Message).To(ContainSubstring(platformversion.MinimumSupported))
			Expect(ready.Message).To(ContainSubstring("Nothing running was changed"))

			var deployments appsv1.DeploymentList
			Expect(k8sClient.List(ctx, &deployments)).To(Succeed())
			for i := range deployments.Items {
				for _, owner := range deployments.Items[i].OwnerReferences {
					Expect(owner.Name).NotTo(Equal(name.Name), "a refused platform must own no objects")
				}
			}

			// A refusal already recorded costs nothing more.
			before := updated.ResourceVersion
			reconciler := &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, name, &updated)).To(Succeed())
			Expect(updated.ResourceVersion).To(Equal(before), "an unchanged refusal must not write status again")
		})

		It("should resume the moment the version moves to the floor", func() {
			name := types.NamespacedName{Name: "moves-to-floor", Namespace: namespace}
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v0.0.41-selfhosted-preview"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { Expect(k8sClient.Delete(ctx, resource)).To(Succeed()) }()
			reconcileTwice(name)

			var current plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, name, &current)).To(Succeed())
			current.Spec.Version = platformversion.MinimumSupported
			Expect(k8sClient.Update(ctx, &current)).To(Succeed())

			result := reconcileTwice(name) // the version echo re-initializes, then components run
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "back on the component cadence")

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, name, &updated)).To(Succeed())
			Expect(updated.Status.Phase).NotTo(Equal(plantonaiv1.PhaseError))
			supported := findCondition(updated.Status.Conditions, plantonaiv1.ConditionVersionSupported)
			Expect(supported).NotTo(BeNil())
			Expect(supported.Status).To(Equal(metav1.ConditionTrue))
			Expect(supported.Reason).To(Equal(platformversion.ReasonSupported))
		})

		It("should refuse a version that is not a release at apply time, and accept a pre-release form", func() {
			notARelease := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-version-local", Namespace: namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "local"},
			}
			err := k8sClient.Create(ctx, notARelease)
			Expect(err).To(HaveOccurred(), "a version that names no release cannot be placed on the release line")
			Expect(err.Error()).To(ContainSubstring("vMAJOR.MINOR.PATCH"), "the rejection must explain itself, got: %v", err)
			Expect(err.Error()).To(ContainSubstring("image.tag"), "the rejection must name the way to run a custom build, got: %v", err)

			preRelease := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-version-prerelease", Namespace: namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: platformversion.MinimumSupported + "-rc.1"},
			}
			Expect(k8sClient.Create(ctx, preRelease)).To(Succeed(), "a pre-release suffix is a release form")
			Expect(k8sClient.Delete(ctx, preRelease)).To(Succeed())
		})
	})

	Context("When the platform release declares the operator it needs", func() {
		// The other direction of the version contract: the release's own
		// declaration (a label on its image, read through the reader) against
		// this operator's stamped release. Best-effort by design: a reader
		// that fails leaves the platform proceeding, and the answer is
		// remembered per version.
		It("refuses a release that needs a newer operator, in words, and proceeds when the registry cannot be read", func() {
			previous := platformversion.OperatorRelease
			platformversion.OperatorRelease = "v0.14.0"
			defer func() { platformversion.OperatorRelease = previous }()

			nn := types.NamespacedName{Name: "needs-newer-operator", Namespace: namespace}
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(context.Background(), resource) }()

			reader := &fakeRequirementReader{required: "v0.16.0"}
			reconciler := &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), RequirementReader: reader}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(plantonaiv1.PhaseError))
			supported := findCondition(updated.Status.Conditions, plantonaiv1.ConditionVersionSupported)
			Expect(supported).NotTo(BeNil())
			Expect(supported.Status).To(Equal(metav1.ConditionFalse))
			Expect(supported.Reason).To(Equal(platformversion.ReasonRequiresNewerOperator))
			Expect(supported.Message).To(ContainSubstring("needs operator v0.16.0 or newer"))
			Expect(supported.Message).To(ContainSubstring("this operator is v0.14.0"))
			Expect(supported.Message).To(ContainSubstring("helm upgrade planton-operator"))
			Expect(updated.Status.RequiredOperatorVersion).To(Equal("v0.16.0"))
			Expect(updated.Status.Components.PostgreSQL.Phase).To(Equal(plantonaiv1.ComponentPhasePending),
				"nothing was rendered for a refused release")

			// Remembered per version: a third reconcile reads nothing more.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(reader.calls).To(Equal(1), "the requirement is read once per version")

			// A registry the operator cannot reach: the platform proceeds and
			// VersionSupported stays True.
			failing := &fakeRequirementReader{err: errors.NewServiceUnavailable("registry unreachable")}
			offline := &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), RequirementReader: failing}
			_, err = offline.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			supported = findCondition(updated.Status.Conditions, plantonaiv1.ConditionVersionSupported)
			Expect(supported.Status).To(Equal(metav1.ConditionTrue), "an unreadable registry never refuses: %s", supported.Message)
		})
	})

	Context("When a component's pod cannot start", func() {
		// The front-door gateway has no dependencies, so it is the one
		// component the second reconcile actually runs on a minimal spec --
		// the right seam to prove the whole path from a kubelet's verdict on
		// a pod to the MESSAGE column and an Event. Envtest runs no
		// scheduler or kubelet, so the pod and its status are planted by the
		// test exactly as the kubelet would write them.
		It("names the pod, the image, and the fix on the component, the Ready condition, and an Event -- once", func() {
			nn := types.NamespacedName{Name: "pull-fails", Namespace: namespace}
			resource := &plantonaiv1.PlantonPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
				Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0"},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() { _ = k8sClient.Delete(context.Background(), resource) }()

			recorder := record.NewFakeRecorder(16)
			reconciler := &PlantonPlatformReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: recorder}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred(), "the second reconcile applies the gateway Deployment")

			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resources.GatewayDeploymentName(nn.Name), Namespace: namespace}, &deploy)).To(Succeed())

			const image = "ghcr.io/plantonhq/planton-gateway:v9.9.9-missing"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: resources.GatewayDeploymentName(nn.Name) + "-7d9f4-x1", Namespace: namespace,
					Labels: deploy.Spec.Selector.MatchLabels,
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "gateway", Image: image}}},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(context.Background(), pod) }()
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "gateway", Image: image,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff", Message: "Back-off pulling image \"" + image + "\": manifest unknown"}},
				}},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated plantonaiv1.PlantonPlatform
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			gw := updated.Status.Components.Gateway
			Expect(gw).NotTo(BeNil())
			Expect(gw.Reason).To(Equal(plantonaiv1.ComponentReasonImagePullFailed))
			Expect(gw.Object).NotTo(BeNil())
			Expect(gw.Object.Kind).To(Equal("Pod"))
			Expect(gw.Object.Name).To(Equal(pod.Name))
			Expect(gw.Message).To(ContainSubstring(image), "the status names the image, got: %s", gw.Message)
			Expect(gw.Message).To(ContainSubstring("manifest unknown"), "the registry's own words are relayed, got: %s", gw.Message)
			Expect(gw.Message).To(ContainSubstring("image override"), "the status names the fix, got: %s", gw.Message)
			Expect(gw.LastTransitionTime.IsZero()).To(BeFalse())

			ready := findCondition(updated.Status.Conditions, plantonaiv1.ConditionReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Reason).To(Equal(string(plantonaiv1.ComponentReasonImagePullFailed)),
				"the Ready condition carries the failing component's reason")
			Expect(ready.Message).To(HavePrefix("gateway: image "+image),
				"the MESSAGE column names the component and repeats its sentence, got: %s", ready.Message)
			Expect(ready.ObservedGeneration).To(Equal(updated.Generation))

			Eventually(recorder.Events, timeout, interval).Should(Receive(And(
				ContainSubstring("Warning"), ContainSubstring("ImagePullFailed"), ContainSubstring("gateway:"), ContainSubstring(image))))

			// The failure persists; the next reconcile changes nothing and
			// records nothing more.
			before := gw.LastTransitionTime
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Components.Gateway.LastTransitionTime.Equal(&before)).To(BeTrue(),
				"lastTransitionTime does not move while the same failure persists")
			Consistently(recorder.Events, 300*time.Millisecond, 50*time.Millisecond).ShouldNot(Receive(),
				"a persisting failure is one Event, not one per reconcile")
		})
	})

})

// fakeRequirementReader answers the operator requirement a release declares
// without a registry, and counts its calls.
type fakeRequirementReader struct {
	required string
	err      error
	calls    int
}

func (f *fakeRequirementReader) RequiredOperator(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.required, f.err
}

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
