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

const defaultNeo4jStorageSize = "10Gi"

// Neo4j deploys and monitors the Neo4j graph database via the official Helm
// chart. Optional component, gated by spec.components.graph.enabled.
type Neo4j struct{ Base }

func (n *Neo4j) Name() string                                { return "neo4j" }
func (n *Neo4j) Dependencies(_ *v1.PlantonPlatform) []string { return nil }

func (n *Neo4j) IsEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Components != nil &&
		planton.Spec.Components.Graph != nil &&
		planton.Spec.Components.Graph.Enabled
}

func (n *Neo4j) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", n.Name())

	var componentSize resource.Quantity
	var componentClass string
	if planton.Spec.Components != nil && planton.Spec.Components.Graph != nil {
		componentSize = planton.Spec.Components.Graph.StorageSize
		componentClass = planton.Spec.Components.Graph.StorageClassName
	}
	storageSize := effectiveStorageSize(planton, componentSize, defaultNeo4jStorageSize)
	storageClass := effectiveStorageClass(planton, componentClass)

	chartData := resources.LoadNeo4jChart()
	values := resources.Neo4jHelmValues(planton.Name, storageSize, storageClass)

	rendered, err := resources.RenderHelmChart(
		chartData,
		fmt.Sprintf("%s-neo4j", planton.Name),
		planton.Namespace,
		values,
	)
	if err != nil {
		return Result{}, fmt.Errorf("rendering Neo4j chart: %w", err)
	}

	if err := n.ApplyManifests(ctx, c, planton, rendered); err != nil {
		return Result{}, fmt.Errorf("applying Neo4j manifests: %w", err)
	}

	stsName := fmt.Sprintf("%s-neo4j", planton.Name)
	ready, err := n.IsStatefulSetReady(ctx, c, stsName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking Neo4j readiness: %w", err)
	}
	if !ready {
		log.Info("Neo4j not ready")
		return n.NotReady(ctx, c, planton.Namespace, StatefulSetRef(stsName), "Waiting for Neo4j"), nil
	}

	log.Info("Neo4j ready")
	return Result{Ready: true, Message: "Neo4j healthy"}, nil
}
