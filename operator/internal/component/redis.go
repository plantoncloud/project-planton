package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

const defaultRedisStorageSize = "1Gi"

// Redis deploys and monitors the platform's redis-protocol cache server via
// the Bitnami Valkey Helm chart -- the engine is Valkey (BSD-3-Clause), not
// Redis 8+ (RSALv2/SSPLv1/AGPLv3); see resources/valkey_helm.go for the
// rationale. The component keeps the "redis" role name across the CRD, status,
// and connection surfaces. There is no operator mode -- scaling is handled
// through chart values.
type Redis struct{ Base }

func (r *Redis) Name() string                                { return "redis" }
func (r *Redis) Dependencies(_ *v1.PlantonPlatform) []string { return nil }
func (r *Redis) IsEnabled(_ *v1.PlantonPlatform) bool        { return true }

func (r *Redis) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", r.Name())

	var componentSize resource.Quantity
	var componentClass string
	if planton.Spec.Database != nil && planton.Spec.Database.Redis != nil {
		componentSize = planton.Spec.Database.Redis.StorageSize
		componentClass = planton.Spec.Database.Redis.StorageClassName
	}
	storageSize := effectiveStorageSize(planton, componentSize, defaultRedisStorageSize)
	storageClass := effectiveStorageClass(planton, componentClass)

	if err := r.EnsureCredentialSecret(ctx, c,
		resources.RedisSecretName(planton.Name), planton.Namespace,
		resources.RedisSecretKey, r.OwnerReferenceFor(planton)); err != nil {
		return Result{}, fmt.Errorf("ensuring Redis credentials: %w", err)
	}

	chartData := resources.LoadValkeyChart()
	values := resources.ValkeyHelmValues(planton.Name, storageSize, storageClass)

	rendered, err := resources.RenderHelmChart(
		chartData,
		fmt.Sprintf("%s-redis", planton.Name),
		planton.Namespace,
		values,
	)
	if err != nil {
		return Result{}, fmt.Errorf("rendering Redis chart: %w", err)
	}

	if err := r.ApplyManifests(ctx, c, planton, rendered); err != nil {
		return Result{}, fmt.Errorf("applying Redis manifests: %w", err)
	}

	// The Valkey chart names the data-serving StatefulSet "primary" (the Redis
	// chart said "master").
	stsName := fmt.Sprintf("%s-redis-primary", planton.Name)
	ready, err := r.IsStatefulSetReady(ctx, c, stsName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking Redis readiness: %w", err)
	}
	if !ready {
		log.Info("Redis not ready")
		return r.NotReady(ctx, c, planton.Namespace, StatefulSetRef(stsName), "Waiting for Redis"), nil
	}

	log.Info("Redis ready")
	return Result{Ready: true, Message: "Redis healthy"}, nil
}
