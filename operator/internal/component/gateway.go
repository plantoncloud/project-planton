package component

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// Gateway deploys the built-in front door for installs without ingress: one
// nginx proxy presenting the console, the browser-facing API, and sign-in on
// a single origin, reached with one kubectl port-forward command. It mirrors
// the ingress path layout exactly, so graduating an install to ingress
// changes the address, never the architecture.
//
// Enabled precisely when ingress is disabled -- the platform always has
// exactly one front door.
type Gateway struct{ Base }

func (g *Gateway) Name() string { return "gateway" }

// Dependencies is empty on purpose: like an ingress controller, the front
// door comes up immediately and serves 502s for backends that are still
// booting, so the port-forward command works (and shows honest progress)
// from the first minute of an install.
func (g *Gateway) Dependencies(_ *v1.PlantonPlatform) []string { return nil }

func (g *Gateway) IsEnabled(planton *v1.PlantonPlatform) bool {
	return !isIngressEnabled(planton)
}

func (g *Gateway) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", g.Name())
	ownerRef := g.OwnerReferenceFor(planton)

	if err := g.ApplyTypedObject(ctx, c, resources.GatewayConfigMap(planton.Name, planton.Namespace, ownerRef)); err != nil {
		return Result{}, fmt.Errorf("applying Gateway ConfigMap: %w", err)
	}

	// nginx reads its config once at startup; hashing the rendered config
	// into the pod template rolls the Deployment when routing changes.
	configHash := sha256.Sum256([]byte(resources.GatewayNginxConfig(planton.Name, planton.Namespace)))

	cfg := resources.GatewayConfig{
		CRName:     planton.Name,
		Namespace:  planton.Namespace,
		OwnerRef:   ownerRef,
		ConfigHash: hex.EncodeToString(configHash[:]),
	}
	if planton.Spec.Gateway != nil && planton.Spec.Gateway.Image != nil {
		cfg.ImageRepository = planton.Spec.Gateway.Image.Repository
		cfg.ImageTag = planton.Spec.Gateway.Image.Tag
	}

	if err := g.ApplyTypedObject(ctx, c, resources.GatewayDeployment(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying Gateway Deployment: %w", err)
	}
	if err := g.ApplyTypedObject(ctx, c, resources.GatewayService(planton.Name, planton.Namespace, ownerRef)); err != nil {
		return Result{}, fmt.Errorf("applying Gateway Service: %w", err)
	}

	// The gateway owns status.consoleUrl in port-forward mode, exactly as the
	// ingress component owns it when ingress is the front door. Published
	// before the readiness check so the URL every later component derives
	// from (identity, control plane, console) is available in this same pass.
	url, _ := frontDoorURL(planton)
	publishFrontDoor(planton, url)

	ready, err := g.IsDeploymentReady(ctx, c, resources.GatewayDeploymentName(planton.Name), planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking Gateway readiness: %w", err)
	}
	if !ready {
		log.Info("Gateway not ready")
		return g.NotReady(ctx, c, planton.Namespace, DeploymentRef(resources.GatewayDeploymentName(planton.Name)),
			"Waiting for the front-door gateway Deployment"), nil
	}

	log.Info("Gateway ready")
	// The status IS the instruction: nobody should have to compose the
	// port-forward command (its local port is load-bearing -- sign-in URLs
	// are pinned to it).
	return Result{Ready: true, Message: fmt.Sprintf(
		"Front door ready; open %s after running: %s",
		url, resources.GatewayPortForwardCommand(planton.Name, planton.Namespace, gatewayLocalPort(planton)))}, nil
}
