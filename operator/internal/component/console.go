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

// Console deploys the Planton web console (Next.js) as a Kubernetes Deployment
// connected to the control plane via its ClusterIP Service.
type Console struct{ Base }

func (co *Console) Name() string                                { return "console" }
func (co *Console) Dependencies(_ *v1.PlantonPlatform) []string { return []string{"controlplane"} }

func (co *Console) IsEnabled(_ *v1.PlantonPlatform) bool { return true }

func (co *Console) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", co.Name())
	ownerRef := co.OwnerReferenceFor(planton)

	cfg := resources.ConsoleConfig{
		CRName:    planton.Name,
		Namespace: planton.Namespace,
		Version:   planton.Spec.Version,
		OwnerRef:  ownerRef,
		Replicas:  1,
	}

	if planton.Spec.Console != nil {
		if planton.Spec.Console.Replicas != nil {
			cfg.Replicas = *planton.Spec.Console.Replicas
		}
		if planton.Spec.Console.Image != nil {
			cfg.ImageRepository = planton.Spec.Console.Image.Repository
			cfg.ImageTag = planton.Spec.Console.Image.Tag
		}
		cfg.ExternalConfigSecretName = planton.Spec.Console.ExternalConfigSecretName
	}

	publicURL, resolved := frontDoorURL(planton)
	if !resolved {
		// Deploying now would mean a second rollout once the URL resolves;
		// wait one pass instead of deploying twice.
		log.Info("Console waiting for the public URL")
		return Result{Ready: false, Message: "Waiting for the public URL (ingress address not yet resolved)"}, nil
	}
	cfg.PublicURL = publicURL
	if frontDoorRoutesNativeGRPC(planton) {
		cfg.GRPCEndpoint = resources.GRPCEndpoint(publicURL)
	}

	// The console always signs in through the bundled identity server --
	// through the gateway's port-forward front door or the ingress hostname.
	if publicURL != "" {
		if err := co.EnsureCredentialSecret(ctx, c,
			resources.ConsoleNextAuthSecretName(planton.Name), planton.Namespace,
			resources.ConsoleNextAuthSecretKey, ownerRef); err != nil {
			return Result{}, fmt.Errorf("ensuring console NextAuth Secret: %w", err)
		}
		realm := identityRealm(planton)
		cfg.Identity = &resources.ConsoleIdentityConfig{
			IssuerURL:         resources.IdentityIssuerURL(publicURL, realm),
			InternalIssuerURL: resources.IdentityInternalIssuerURL(planton.Name, planton.Namespace, realm),
			Realm:             realm,
		}
	}

	deploy := resources.ConsoleDeployment(cfg)
	if err := co.ApplyTypedObject(ctx, c, deploy); err != nil {
		return Result{}, fmt.Errorf("applying Console Deployment: %w", err)
	}

	svc := resources.ConsoleService(planton.Name, planton.Namespace, ownerRef)
	if err := co.ApplyTypedObject(ctx, c, svc); err != nil {
		return Result{}, fmt.Errorf("applying Console Service: %w", err)
	}

	deployName := resources.ConsoleDeploymentName(planton.Name)
	ready, err := co.IsDeploymentReady(ctx, c, deployName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking Console readiness: %w", err)
	}
	if !ready {
		log.Info("Console not ready")
		return co.NotReady(ctx, c, planton.Namespace, DeploymentRef(deployName), "Waiting for Console Deployment"), nil
	}

	log.Info("Console ready")
	return Result{Ready: true, Message: "Console healthy"}, nil
}
