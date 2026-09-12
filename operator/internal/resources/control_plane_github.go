package resources

import (
	"encoding/json"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The install's GitHub declaration reaches the control plane the way the
// identity-federation facts do: as ONE file the operator keeps current in a
// ConfigMap mounted whole, never as a fan of environment variables. A change
// to the declaration -- a host added, an App registered, a webhook posture
// corrected -- updates the file in place and never rolls the pod; the
// control plane re-reads it when it answers a wizard. App private keys ride
// the email credentials' shape: projected files under one directory, the PEM
// exactly as GitHub generated it, so nobody encodes a key by hand and a
// rotated key is live on the next token mint.
//
// The facts JSON is a cross-repo contract: the control plane's reader parses
// exactly this shape and pins the same fixture (testdata/github-facts.json).
// A field change here is a contract change -- move both sides and the floor.
const (
	// GithubFactsKey is the single data key inside the facts ConfigMap;
	// GithubFactsMountPath is where the control plane mounts it (the whole
	// directory -- a subPath mount would freeze kubelet's in-place updates).
	GithubFactsKey       = "facts.json"
	GithubFactsMountPath = "/etc/planton/github"

	githubFactsVolumeName = "github-facts"

	// GithubCredentialsMountPath is the directory the App credentials volume
	// is mounted at; each host's files live under its own hostname directory.
	GithubCredentialsMountPath = "/etc/planton/github-credentials"

	githubCredentialsVolumeName = "github-credentials"

	GithubAppPrivateKeyFileName    = "private-key.pem"
	GithubAppWebhookSecretFileName = "webhook-secret"

	// GithubDefaultHost is the host every install works with when nothing is
	// declared: the github.com adopter's posture, unchanged.
	GithubDefaultHost = "github.com"
)

// GithubFactsConfigMapName holds the GitHub facts the operator ADVERTISES to
// the product: which hosts are declared, which carry an install App and where
// its files are, and whether each can deliver webhooks. It exists on EVERY
// install (an empty declaration renders the github.com default), so the
// control plane's mount never depends on late-created-volume semantics:
// "{crName}-github-facts".
func GithubFactsConfigMapName(crName string) string {
	return fmt.Sprintf("%s-github-facts", crName)
}

// GithubFactsFilePath is the in-container path of the facts file, injected
// into the control plane as a STATIC env var -- the value never changes, so
// facts updates never roll the pod.
func GithubFactsFilePath() string {
	return GithubFactsMountPath + "/" + GithubFactsKey
}

// GithubAppCredentialFilePath is where one host's named credential file
// lives inside the control-plane container.
func GithubAppCredentialFilePath(host, fileName string) string {
	return path.Join(GithubCredentialsMountPath, host, fileName)
}

// GithubBinding is the resolved spec.github: the hosts in declaration order,
// each with its App (preflighted by the component -- a host whose Secret is
// missing arrives here with App nil and a reason) and its webhook verdict.
// nil means nothing is declared, and the renderer says what that means out
// loud: github.com, no install App, webhooks judged by the front door.
type GithubBinding struct {
	Hosts     []GithubHostBinding
	HostLogin bool
}

// GithubHostBinding is one host as the control plane will learn it.
type GithubHostBinding struct {
	Host string

	// App is the install App on this host, nil when none is declared or the
	// declaration could not be honored (AppUnavailableReason then says why).
	App                  *GithubAppBinding
	AppUnavailableReason string

	// WebhooksReachable is the verdict; WebhooksPosture is how it was
	// reached (auto, reachable, unreachable) and WebhooksReason the sentence
	// a team reads beside it.
	WebhooksReachable bool
	WebhooksPosture   string
	WebhooksReason    string
}

// GithubAppBinding is an install App's identity and where its secrets live.
type GithubAppBinding struct {
	ClientID             string
	PrivateKeySecretName string
	PrivateKeySecretKey  string
	// WebhookSecretName is empty when the App declares no webhook secret.
	WebhookSecretName string
	WebhookSecretKey  string
}

// GithubFacts is the operator→product projection contract for GitHub.
type GithubFacts struct {
	// Hosts in the order the wizard offers them. Never empty: an install
	// that declares nothing carries the github.com default.
	Hosts []GithubHostFacts `json:"hosts"`
	// HostLogin is whether connections may use the control plane's own
	// GitHub sign-in.
	HostLogin bool `json:"hostLogin"`
}

// GithubHostFacts is one host's facts.
type GithubHostFacts struct {
	Host string `json:"host"`
	// Declared is false only for the github.com default an undeclared
	// install carries -- the wizard offers it, but knows nobody chose it.
	Declared bool `json:"declared"`
	// App is the install App on this host, or null.
	App *GithubAppFacts `json:"app"`
	// AppUnavailableReason is the sentence the wizard shows beside the
	// greyed one-click card when App is null.
	AppUnavailableReason string              `json:"appUnavailableReason,omitempty"`
	Webhooks             GithubWebhooksFacts `json:"webhooks"`
}

// GithubAppFacts is where the control plane finds an install App's identity.
type GithubAppFacts struct {
	ClientID       string `json:"clientId"`
	PrivateKeyFile string `json:"privateKeyFile"`
	// WebhookSecretFile is empty when the App declares no webhook secret.
	WebhookSecretFile string `json:"webhookSecretFile,omitempty"`
}

// GithubWebhooksFacts is one host's webhook verdict and how it was reached.
type GithubWebhooksFacts struct {
	Reachable bool   `json:"reachable"`
	Posture   string `json:"posture"`
	Reason    string `json:"reason"`
}

// GithubFactsFrom renders the facts from the binding. A nil binding is the
// undeclared install: the github.com default with the front-door verdict
// the caller supplies through defaultWebhooksReachable (the door's own
// internet reachability) so the fact stays true where nothing was declared.
func GithubFactsFrom(binding *GithubBinding, defaultWebhooksReachable bool) GithubFacts {
	if binding == nil || len(binding.Hosts) == 0 {
		facts := GithubFacts{Hosts: []GithubHostFacts{{
			Host:                 GithubDefaultHost,
			Declared:             false,
			AppUnavailableReason: PlatformAppUnavailableReason,
			Webhooks: GithubWebhooksFacts{
				Reachable: defaultWebhooksReachable,
				Posture:   "auto",
				Reason:    FrontDoorWebhooksReason(defaultWebhooksReachable),
			},
		}}}
		if binding != nil {
			facts.HostLogin = binding.HostLogin
		}
		return facts
	}
	facts := GithubFacts{HostLogin: binding.HostLogin}
	for _, h := range binding.Hosts {
		hostFacts := GithubHostFacts{
			Host:                 h.Host,
			Declared:             true,
			AppUnavailableReason: h.AppUnavailableReason,
			Webhooks: GithubWebhooksFacts{
				Reachable: h.WebhooksReachable,
				Posture:   h.WebhooksPosture,
				Reason:    h.WebhooksReason,
			},
		}
		if h.App != nil {
			hostFacts.App = &GithubAppFacts{
				ClientID:       h.App.ClientID,
				PrivateKeyFile: GithubAppCredentialFilePath(h.Host, GithubAppPrivateKeyFileName),
			}
			if h.App.WebhookSecretName != "" {
				hostFacts.App.WebhookSecretFile = GithubAppCredentialFilePath(h.Host, GithubAppWebhookSecretFileName)
			}
		} else if hostFacts.AppUnavailableReason == "" {
			hostFacts.AppUnavailableReason = PlatformAppUnavailableReason
		}
		facts.Hosts = append(facts.Hosts, hostFacts)
	}
	return facts
}

// FrontDoorWebhooksReason is the sentence for the auto posture, in the words
// the connection wizard shows a team; the component uses the same words for
// every declared host left on auto.
func FrontDoorWebhooksReason(reachable bool) string {
	if reachable {
		return "the install's front door is on the public internet, so this host can deliver webhooks to it; pushes trigger runs"
	}
	return "the install's front door is not on the public internet, so this host cannot deliver webhooks to it; Planton checks GitHub for pushes instead, and runs start from Planton"
}

// GithubFactsConfigMap renders the facts ConfigMap.
func GithubFactsConfigMap(crName, namespace string, facts GithubFacts, ownerRef *metav1.OwnerReference) (*corev1.ConfigMap, error) {
	payload, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("marshaling GitHub facts: %w", err)
	}
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GithubFactsConfigMapName(crName),
			Namespace: namespace,
		},
		Data: map[string]string{GithubFactsKey: string(payload)},
	}
	if ownerRef != nil {
		configMap.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return configMap, nil
}

// githubFactsVolume is the facts ConfigMap mounted whole on the control
// plane, on every install (the ConfigMap exists on every install too;
// Optional is belt-and-braces only).
func githubFactsVolume(crName string) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: githubFactsVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: GithubFactsConfigMapName(crName)},
				Optional:             new(true),
			},
		},
	}
	mount := corev1.VolumeMount{Name: githubFactsVolumeName, MountPath: GithubFactsMountPath, ReadOnly: true}
	return volume, mount
}

// githubCredentialsVolume renders the one projected volume that carries every
// install App's secrets, each host's files under its own directory, plus its
// read-only mount. nil when no host declares an App: no volume is rendered
// for nothing to mount. Whole-directory, never subPath, so a rotated key is
// live on the next token mint.
func githubCredentialsVolume(binding *GithubBinding) (*corev1.Volume, *corev1.VolumeMount) {
	sources := githubCredentialSources(binding)
	if len(sources) == 0 {
		return nil, nil
	}
	volume := &corev1.Volume{
		Name:         githubCredentialsVolumeName,
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: sources}},
	}
	mount := &corev1.VolumeMount{Name: githubCredentialsVolumeName, MountPath: GithubCredentialsMountPath, ReadOnly: true}
	return volume, mount
}

// githubCredentialSources projects each App's Secret keys onto the fixed
// file names under the host's directory. Two Apps may share one Secret with
// different keys; the projection is per (Secret, key), so nothing collides.
func githubCredentialSources(binding *GithubBinding) []corev1.VolumeProjection {
	if binding == nil {
		return nil
	}
	var sources []corev1.VolumeProjection
	for _, h := range binding.Hosts {
		if h.App == nil {
			continue
		}
		sources = append(sources, secretProjection(h.App.PrivateKeySecretName,
			corev1.KeyToPath{Key: h.App.PrivateKeySecretKey, Path: path.Join(h.Host, GithubAppPrivateKeyFileName)}))
		if h.App.WebhookSecretName != "" {
			sources = append(sources, secretProjection(h.App.WebhookSecretName,
				corev1.KeyToPath{Key: h.App.WebhookSecretKey, Path: path.Join(h.Host, GithubAppWebhookSecretFileName)}))
		}
	}
	return sources
}

// githubFactsEnvVars is the one static variable that tells the control plane
// where the facts file is. The value never changes, so it never rolls the
// pod.
func githubFactsEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{{Name: "PLANTON_GITHUB_FACTS_FILE", Value: GithubFactsFilePath()}}
}

// githubDefaultHostBinding returns the github.com entry of a binding, or nil.
// The platform release the floor admits still reads GitHub through
// environment variables shaped for one host; the renderer feeds those from
// the github.com entry until the platform reads the facts file, at which
// point the variables and this helper leave together with the floor.
func githubDefaultHostBinding(binding *GithubBinding) *GithubHostBinding {
	if binding == nil {
		return nil
	}
	for i := range binding.Hosts {
		if binding.Hosts[i].Host == GithubDefaultHost {
			return &binding.Hosts[i]
		}
	}
	return nil
}
