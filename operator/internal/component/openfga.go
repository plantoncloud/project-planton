package component

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/bootstrap"
	"github.com/plantonhq/planton/operator/internal/resources"
)

const (
	// fgaBootstrapStoreIDKey is the one fact the bootstrap ConfigMap carries:
	// the store's identity. The authorization model inside the store belongs to
	// the control plane's version, and the control plane establishes it itself
	// at boot; the operator never writes or records a model id.
	fgaBootstrapStoreIDKey = "store_id"
	httpClientTimeout      = 10 * time.Second
)

// OpenFGA deploys and monitors OpenFGA via its official Helm chart, then
// bootstraps the FGA store via the OpenFGA HTTP API.
// Depends on PostgreSQL because OpenFGA uses it as its datastore.
type OpenFGA struct{ Base }

func (o *OpenFGA) Name() string                                { return "openfga" }
func (o *OpenFGA) Dependencies(_ *v1.PlantonPlatform) []string { return []string{"postgresql"} }

// IsEnabled is always true: the policy engine is part of every platform, like
// its database. Every request the control plane authorizes is answered by it,
// so there is no platform shape that runs without it.
func (o *OpenFGA) IsEnabled(_ *v1.PlantonPlatform) bool { return true }

func (o *OpenFGA) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", o.Name())

	chartData := resources.LoadOpenFGAChart()
	values := resources.OpenFGAHelmValues(planton.Name, planton.Namespace)

	rendered, err := resources.RenderHelmChart(
		chartData,
		resources.OpenFGAServiceName(planton.Name),
		planton.Namespace,
		values,
	)
	if err != nil {
		return Result{}, fmt.Errorf("rendering OpenFGA chart: %w", err)
	}

	if err := o.ApplyManifests(ctx, c, planton, rendered); err != nil {
		return Result{}, fmt.Errorf("applying OpenFGA manifests: %w", err)
	}

	deployName := resources.OpenFGAServiceName(planton.Name)
	ready, err := o.IsDeploymentReady(ctx, c, deployName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking OpenFGA readiness: %w", err)
	}
	if !ready {
		log.Info("OpenFGA not ready")
		return o.NotReady(ctx, c, planton.Namespace, DeploymentRef(deployName), "Waiting for OpenFGA server"), nil
	}

	bootstrapped, err := o.ensureFGABootstrap(ctx, c, planton)
	if err != nil {
		return Result{}, fmt.Errorf("FGA bootstrap: %w", err)
	}
	if !bootstrapped {
		log.Info("FGA bootstrap in progress")
		return Result{Ready: false, Message: "Bootstrapping FGA store"}, nil
	}

	log.Info("OpenFGA ready")
	return Result{Ready: true, Message: "OpenFGA healthy, store bootstrapped"}, nil
}

func (o *OpenFGA) ensureFGABootstrap(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (bool, error) {
	log := logf.FromContext(ctx).WithValues("component", o.Name(), "step", "bootstrap")
	cmName := bootstrap.BootstrapConfigMapName(planton.Name)

	var existing corev1.ConfigMap
	cmExists := false
	getErr := c.Get(ctx, types.NamespacedName{Name: cmName, Namespace: planton.Namespace}, &existing)
	if getErr == nil {
		cmExists = true
		// Exactly the store id and nothing else: a ConfigMap carrying more (or
		// less) is rewritten to the contract on the next pass.
		if len(existing.Data) == 1 && existing.Data[fgaBootstrapStoreIDKey] != "" {
			return true, nil
		}
	} else if !apierrors.IsNotFound(getErr) {
		return false, fmt.Errorf("checking bootstrap ConfigMap: %w", getErr)
	}

	fgaURL := resources.OpenFGAHTTPURL(planton.Name, planton.Namespace)

	httpClient := &http.Client{Timeout: httpClientTimeout}
	result, err := bootstrap.EnsureFGABootstrap(ctx, httpClient, fgaURL, resources.OpenFGAStoreName)
	if err != nil {
		log.Error(err, "FGA bootstrap failed, will retry on next reconcile")
		return false, nil
	}

	data := map[string]string{
		fgaBootstrapStoreIDKey: result.StoreID,
	}

	if cmExists {
		existing.Data = data
		if err := c.Update(ctx, &existing); err != nil {
			return false, fmt.Errorf("updating bootstrap ConfigMap: %w", err)
		}
	} else {
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: planton.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": resources.ManagedByLabel,
				},
				OwnerReferences: []metav1.OwnerReference{*o.OwnerReferenceFor(planton)},
			},
			Data: data,
		}
		if err := c.Create(ctx, cm); err != nil {
			return false, fmt.Errorf("creating bootstrap ConfigMap: %w", err)
		}
	}

	log.Info("FGA bootstrap complete", "storeID", result.StoreID)
	return true, nil
}

// RBAC markers for resources the OpenFGA component manages.
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
