package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/bootstrap"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// defaultOpenBAOStorageSize sizes the vault's data volume. Its contents are
// KV secret payloads and Transit key material -- kilobytes each; 2Gi is
// headroom, not bulk. spec.storage.size / spec.vault.storageSize override.
const defaultOpenBAOStorageSize = "2Gi"

// OpenBAO deploys and monitors OpenBAO (open-source Vault fork), the bundled
// secrets manager. Deployed by default (spec.vault): it backs the credential
// store for pasted connection secrets, the default envelope-encryption key,
// and the OIDC issuer's signing key -- integral the way the database is.
// spec.vault.enabled: false is the deliberate opt-out.
//
// In auto-init mode, the component initializes OpenBAO via its HTTP API after
// the pod is running, stores unseal keys and root token in a Kubernetes Secret,
// and re-unseals on pod restart. In manual mode, the component waits for the
// user to initialize and unseal.
type OpenBAO struct{ Base }

func (o *OpenBAO) Name() string                                { return "openbao" }
func (o *OpenBAO) Dependencies(_ *v1.PlantonPlatform) []string { return nil }

// IsEnabled defaults to true: absence of spec.vault means deploy the bundled
// secrets manager. Must agree with isVaultEnabled (control_plane.go) and
// isOpenBAOEnabled (status package), or the reconciler, the control-plane
// wiring, and the status slot disagree about the component's existence.
func (o *OpenBAO) IsEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Vault == nil || planton.Spec.Vault.Enabled == nil ||
		*planton.Spec.Vault.Enabled
}

func (o *OpenBAO) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", o.Name())

	var componentSize resource.Quantity
	var componentClass string
	initMode := v1.OpenBAOInitModeAuto

	if vault := planton.Spec.Vault; vault != nil {
		componentSize = vault.StorageSize
		componentClass = vault.StorageClassName
		if vault.InitMode != "" {
			initMode = vault.InitMode
		}
	}
	storageSize := effectiveStorageSize(planton, componentSize, defaultOpenBAOStorageSize)
	storageClass := effectiveStorageClass(planton, componentClass)

	chartData := resources.LoadOpenBAOChart()
	values := resources.OpenBAOHelmValues(planton.Name, storageSize, storageClass)

	rendered, err := resources.RenderHelmChart(
		chartData,
		fmt.Sprintf("%s-openbao", planton.Name),
		planton.Namespace,
		values,
	)
	if err != nil {
		return Result{}, fmt.Errorf("rendering OpenBAO chart: %w", err)
	}

	if err := o.ApplyManifests(ctx, c, planton, rendered); err != nil {
		return Result{}, fmt.Errorf("applying OpenBAO manifests: %w", err)
	}

	// OpenBAO readiness probes fail when sealed/uninitialized, so checking
	// StatefulSet readiness would deadlock. Instead, check if the pod is in
	// Running phase (container started, API accessible) before proceeding
	// to initialization.
	releaseName := fmt.Sprintf("%s-openbao", planton.Name)
	podRunning, err := o.IsPodRunning(ctx, c, releaseName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking OpenBAO pod status: %w", err)
	}
	if !podRunning {
		log.Info("OpenBAO pod not yet running")
		// The chart's fullnameOverride makes the StatefulSet's name the
		// release name.
		return o.NotReady(ctx, c, planton.Namespace, StatefulSetRef(releaseName), "Waiting for OpenBAO pod"), nil
	}

	if initMode == v1.OpenBAOInitModeManual {
		return o.checkManualInit(ctx, planton)
	}

	return o.ensureAutoInit(ctx, c, planton)
}

func (o *OpenBAO) checkManualInit(ctx context.Context, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", o.Name(), "initMode", "manual")

	apiAddr := resources.OpenBAOAPIAddr(planton.Name, planton.Namespace)
	health, err := bootstrap.CheckOpenBAOHealth(ctx, http.DefaultClient, apiAddr)
	if err != nil {
		log.Info("OpenBAO health check failed (may not be ready yet)", "error", err.Error())
		return Result{Ready: false, Message: "Waiting for OpenBAO health endpoint"}, nil
	}

	if !health.Initialized || health.Sealed {
		log.Info("OpenBAO awaiting manual init/unseal",
			"initialized", health.Initialized, "sealed", health.Sealed)
		return Result{Ready: false, Message: "OpenBAO deployed, awaiting manual initialization and unseal " +
			"(the platform expects a KV v2 engine at secret/ and a Transit engine at transit/)"}, nil
	}

	log.Info("OpenBAO ready (manual init)")
	return Result{Ready: true, Message: "OpenBAO healthy (manual init; expects KV v2 at secret/ and Transit at transit/)"}, nil
}

func (o *OpenBAO) ensureAutoInit(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", o.Name(), "initMode", "auto")

	apiAddr := resources.OpenBAOAPIAddr(planton.Name, planton.Namespace)
	health, err := bootstrap.CheckOpenBAOHealth(ctx, http.DefaultClient, apiAddr)
	if err != nil {
		log.Info("OpenBAO health check failed (may not be ready yet)", "error", err.Error())
		return Result{Ready: false, Message: "Waiting for OpenBAO health endpoint"}, nil
	}

	initSecretName := resources.OpenBAOInitSecretName(planton.Name)

	if health.Initialized && !health.Sealed {
		// Steady state -- but the engine mounts are still ensured every pass
		// (idempotent GET + enable-if-missing): installs initialized before
		// mount-ensuring existed, or manually altered vaults, converge here.
		if err := o.ensureMountsFromInitSecret(ctx, c, planton, apiAddr, initSecretName); err != nil {
			return Result{}, err
		}
		// The key material must explain itself where it is read (the
		// one-time-password precedent). Ensured on the steady-state pass so
		// installs initialized before the note existed gain it; annotations
		// only, the key data is untouched.
		if err := o.EnsureSecretAnnotations(ctx, c, initSecretName, planton.Namespace,
			map[string]string{
				resources.OpenBAOInitSecretAnnotation: resources.OpenBAOInitSecretNote(planton.Name),
			}); err != nil {
			return Result{}, fmt.Errorf("annotating OpenBAO init secret: %w", err)
		}
		log.Info("OpenBAO ready (already initialized and unsealed)")
		return Result{Ready: true, Message: "OpenBAO healthy (auto-init)"}, nil
	}

	if !health.Initialized {
		log.Info("OpenBAO not initialized, running auto-init")
		initResult, err := bootstrap.InitializeOpenBAO(ctx, http.DefaultClient, apiAddr,
			resources.OpenBAOSecretShares, resources.OpenBAOSecretThreshold)
		if err != nil {
			return Result{}, fmt.Errorf("auto-initializing OpenBAO: %w", err)
		}

		keysJSON, err := json.Marshal(initResult.UnsealKeys)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling unseal keys: %w", err)
		}

		ownerRef := o.OwnerReferenceFor(planton)
		initSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      initSecretName,
				Namespace: planton.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": resources.ManagedByLabel,
				},
				Annotations: map[string]string{
					resources.OpenBAOInitSecretAnnotation: resources.OpenBAOInitSecretNote(planton.Name),
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				resources.OpenBAOInitSecretUnsealKeysKey: keysJSON,
				resources.OpenBAOInitSecretRootTokenKey:  []byte(initResult.RootToken),
			},
		}
		if ownerRef != nil {
			initSecret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		}

		if err := c.Create(ctx, initSecret); err != nil && !apierrors.IsAlreadyExists(err) {
			return Result{}, fmt.Errorf("creating OpenBAO init secret: %w", err)
		}

		log.Info("OpenBAO initialized, init secret created")

		if err := bootstrap.UnsealOpenBAO(ctx, http.DefaultClient, apiAddr, initResult.UnsealKeys); err != nil {
			return Result{}, fmt.Errorf("auto-unsealing OpenBAO: %w", err)
		}

		if err := bootstrap.EnsureOpenBAOMounts(ctx, http.DefaultClient, apiAddr, initResult.RootToken); err != nil {
			return Result{}, fmt.Errorf("ensuring OpenBAO secrets engines: %w", err)
		}

		log.Info("OpenBAO unsealed")
		return Result{Ready: true, Message: "OpenBAO healthy (auto-init)"}, nil
	}

	// Initialized but sealed: read keys from Secret and unseal.
	var initSecret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{
		Name: initSecretName, Namespace: planton.Namespace,
	}, &initSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return Result{
				Ready:   false,
				Message: "OpenBAO is initialized and sealed but init secret is missing; manual unseal required",
			}, nil
		}
		return Result{}, fmt.Errorf("reading OpenBAO init secret: %w", err)
	}

	var unsealKeys []string
	if err := json.Unmarshal(initSecret.Data[resources.OpenBAOInitSecretUnsealKeysKey], &unsealKeys); err != nil {
		return Result{}, fmt.Errorf("parsing unseal keys from secret: %w", err)
	}

	if err := bootstrap.UnsealOpenBAO(ctx, http.DefaultClient, apiAddr, unsealKeys); err != nil {
		return Result{}, fmt.Errorf("auto-unsealing OpenBAO: %w", err)
	}

	if err := o.ensureMountsFromInitSecret(ctx, c, planton, apiAddr, initSecretName); err != nil {
		return Result{}, err
	}

	log.Info("OpenBAO unsealed from stored keys")
	return Result{Ready: true, Message: "OpenBAO healthy (auto-unsealed)"}, nil
}

// ensureMountsFromInitSecret runs the idempotent engine-mount ensure using the
// root token from the init Secret. Manual-init installs (no init Secret) own
// their mounts; the platform's expected engines are documented in the manual
// path's status message instead.
func (o *OpenBAO) ensureMountsFromInitSecret(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, apiAddr, initSecretName string) error {
	var initSecret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{
		Name: initSecretName, Namespace: planton.Namespace,
	}, &initSecret); err != nil {
		return fmt.Errorf("reading OpenBAO init secret for mount ensure: %w", err)
	}
	rootToken := string(initSecret.Data[resources.OpenBAOInitSecretRootTokenKey])
	if err := bootstrap.EnsureOpenBAOMounts(ctx, http.DefaultClient, apiAddr, rootToken); err != nil {
		return fmt.Errorf("ensuring OpenBAO secrets engines: %w", err)
	}
	return nil
}
