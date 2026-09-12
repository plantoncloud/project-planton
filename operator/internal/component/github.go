package component

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The install's GitHub declaration is resolved here, purely from the spec
// and the front door's posture, and every Secret an install App names is
// preflighted before the Deployment projects it -- a missing Secret would
// otherwise hold the pod in FailedMount with no reason anyone can read.
// A host whose App cannot be honored keeps its place in the facts with the
// App absent and the reason beside it, so every other host still works and
// the wizard says exactly why the one-click card is grey.

// githubSecretPreflight is the GitHub App's shape of the refusal sentence.
func githubSecretPreflight(host, field, contents string, keys ...string) secretPreflight {
	return secretPreflight{
		Noun: "GitHub App Secret", Field: field, Contents: contents,
		Consequence: fmt.Sprintf("teams connect to %s with their own GitHub App, not the install's", host),
		Keys:        keys,
	}
}

// resolveGithub turns spec.github into the binding the renderer reads. The
// front door's posture supplies the verdict for hosts on the auto webhooks
// posture (and for the undeclared github.com default). Every App Secret is
// preflighted; the first finding is returned as the sentence the component
// reports, with that host's App left out of the binding. A nil spec returns
// nil: the renderer knows what nothing declared means.
func resolveGithub(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, doorPublic bool) (*resources.GithubBinding, string, error) {
	g := planton.Spec.Github
	if g == nil {
		return nil, "", nil
	}
	binding := &resources.GithubBinding{HostLogin: g.HostLogin}
	firstRefusal := ""
	for i := range g.Hosts {
		h := &g.Hosts[i]
		hostBinding := resources.GithubHostBinding{Host: h.Host}
		hostBinding.WebhooksPosture, hostBinding.WebhooksReachable, hostBinding.WebhooksReason = webhooksVerdict(h, doorPublic)

		if h.App != nil {
			app, refusal, err := preflightGithubApp(ctx, c, planton.Namespace, i, h)
			if err != nil {
				return nil, "", err
			}
			if refusal != "" {
				hostBinding.AppUnavailableReason = refusal
				if firstRefusal == "" {
					firstRefusal = refusal
				}
			} else {
				hostBinding.App = app
			}
		}
		binding.Hosts = append(binding.Hosts, hostBinding)
	}
	return binding, firstRefusal, nil
}

// webhooksVerdict decides whether a host can deliver webhooks: the
// declaration when it states a posture, the front door when it says auto.
func webhooksVerdict(h *v1.GithubHostSpec, doorPublic bool) (posture string, reachable bool, reason string) {
	switch h.Webhooks {
	case v1.GithubWebhooksReachable:
		return string(h.Webhooks), true,
			fmt.Sprintf("the install declares that %s can deliver webhooks to it (they share a network); pushes trigger runs", h.Host)
	case v1.GithubWebhooksUnreachable:
		return string(h.Webhooks), false,
			fmt.Sprintf("the install declares that %s cannot deliver webhooks to it; Planton checks GitHub for pushes instead, and runs start from Planton", h.Host)
	default:
		return string(v1.GithubWebhooksAuto), doorPublic, resources.FrontDoorWebhooksReason(doorPublic)
	}
}

// preflightGithubApp checks the Secrets one host's App names and returns the
// binding, or the refusal sentence when a Secret or key is missing.
func preflightGithubApp(ctx context.Context, c client.Client, namespace string, index int, h *v1.GithubHostSpec) (*resources.GithubAppBinding, string, error) {
	field := fmt.Sprintf("spec.github.hosts[%d].app", index)
	key := h.App.PrivateKeySecretRef
	msg, err := preflightSecretKeys(ctx, c, namespace, key.Name,
		githubSecretPreflight(h.Host, field+".privateKeySecretRef", "the App's private key as GitHub generated it, PEM text", key.Key))
	if msg != "" || err != nil {
		return nil, msg, err
	}
	app := &resources.GithubAppBinding{
		ClientID:             h.App.ClientID,
		PrivateKeySecretName: key.Name,
		PrivateKeySecretKey:  key.Key,
	}
	if wh := h.App.WebhookSecretRef; wh != nil {
		msg, err := preflightSecretKeys(ctx, c, namespace, wh.Name,
			githubSecretPreflight(h.Host, field+".webhookSecretRef", "the webhook secret set on the App", wh.Key))
		if msg != "" || err != nil {
			return nil, msg, err
		}
		app.WebhookSecretName = wh.Name
		app.WebhookSecretKey = wh.Key
	}
	return app, "", nil
}
