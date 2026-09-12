package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// Temporal deploys and monitors Temporal via its official Helm chart.
// Depends on PostgreSQL because Temporal uses it for persistence and
// visibility storage.
type Temporal struct{ Base }

func (t *Temporal) Name() string                                { return "temporal" }
func (t *Temporal) Dependencies(_ *v1.PlantonPlatform) []string { return []string{"postgresql"} }
func (t *Temporal) IsEnabled(_ *v1.PlantonPlatform) bool        { return true }

func (t *Temporal) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", t.Name())

	chartData := resources.LoadTemporalChart()
	values := resources.TemporalHelmValues(planton.Name, planton.Namespace)

	rendered, err := resources.RenderHelmChart(
		chartData,
		fmt.Sprintf("%s-temporal", planton.Name),
		planton.Namespace,
		values,
	)
	if err != nil {
		return Result{}, fmt.Errorf("rendering Temporal chart: %w", err)
	}

	if err := t.ApplyManifests(ctx, c, planton, rendered); err != nil {
		return Result{}, fmt.Errorf("applying Temporal manifests: %w", err)
	}

	frontendDeploy := resources.TemporalFrontendServiceName(planton.Name)
	ready, err := t.IsDeploymentReady(ctx, c, frontendDeploy, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking Temporal readiness: %w", err)
	}
	if !ready {
		log.Info("Temporal not ready")
		// The chart's schema Job creates Temporal's databases and tables;
		// until it finishes, every server pod exits on "schema_version does
		// not exist" and restarts. Read the Job FIRST so those restarts are
		// reported as the wait they are, not as a crash loop -- and a Job
		// that failed is named as the cause instead of the pods it starves.
		if expl := t.explainJobs(ctx, c, planton.Namespace, map[string]string{
			"app.kubernetes.io/instance":  fmt.Sprintf("%s-temporal", planton.Name),
			"app.kubernetes.io/component": "database",
		}, "Temporal schema setup"); expl != nil {
			return Result{Ready: false, Reason: expl.Reason, Object: expl.Object, Message: expl.Message}, nil
		}
		return t.NotReady(ctx, c, planton.Namespace, DeploymentRef(frontendDeploy), "Waiting for Temporal frontend"), nil
	}

	log.Info("Temporal ready")
	return Result{Ready: true, Message: "Temporal healthy"}, nil
}
