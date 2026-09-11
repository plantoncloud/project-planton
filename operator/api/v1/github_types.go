package v1

// GithubSpec is what the install knows about GitHub, declared once by the
// platform team and read by every organization's connection wizard: which
// GitHub hosts the company uses (github.com, a GitHub Enterprise Server, or
// both), which of them carry a GitHub App registered for the whole install
// so teams connect in one click, and whether each host can deliver webhooks
// to this install -- a fact about the network only the platform team knows.
//
// Nothing declared is a complete, correct posture: github.com with no
// install App, "bring your own App" and host login as the ways in, webhooks
// judged by whether the front door is on the public internet. Every field
// here refines that for the people who need more.
type GithubSpec struct {
	// hosts lists the GitHub hosts this install works with, in the order the
	// connection wizard offers them. Declare github.com to say it explicitly
	// or to give it an App; declare a GitHub Enterprise Server to make it the
	// first choice. A host that is not declared can still be typed by hand
	// in a connection, without an install App.
	// +kubebuilder:validation:MaxItems=8
	// +listType=map
	// +listMapKey=host
	// +optional
	Hosts []GithubHostSpec `json:"hosts,omitempty"`

	// hostLogin lets connections use a GitHub sign-in the control plane's own
	// process carries (a GITHUB_TOKEN in its environment). Off by default on
	// a shared install: the token would act for everyone. The desktop's local
	// instance turns it on because the machine's own sign-in is the person's.
	// +optional
	HostLogin bool `json:"hostLogin,omitempty"`
}

// GithubHostSpec is one GitHub host and what the install offers on it.
type GithubHostSpec struct {
	// host is the hostname as people type it in a browser: github.com, or a
	// GitHub Enterprise Server such as github.example.com. A hostname, never
	// a URL -- the control plane derives the API address (api.github.com for
	// github.com, host/api/v3 for an enterprise server).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)+$`
	Host string `json:"host"`

	// app is a GitHub App registered on this host for the whole install.
	// With it, every organization connects in one click and gets webhooks
	// and check runs; without it, teams bring their own App. The App must be
	// registered on THIS host: an App on github.com cannot sign for an
	// enterprise server.
	// +optional
	App *GithubAppSpec `json:"app,omitempty"`

	// webhooks is whether this host can deliver webhooks to the install.
	// auto (the default) judges by the front door: reachable when the door
	// is on the public internet. Declare reachable for a GitHub Enterprise
	// Server that shares the install's private network -- it can deliver
	// even when the internet cannot -- and unreachable when a public host
	// cannot reach a door that is public only inside a corporate perimeter.
	// Where webhooks cannot arrive, Planton polls GitHub for pushes and every
	// team is told so.
	// +kubebuilder:default=auto
	// +optional
	Webhooks GithubWebhooksPosture `json:"webhooks,omitempty"`
}

// GithubWebhooksPosture is how the install knows whether a GitHub host can
// deliver webhooks to it.
// +kubebuilder:validation:Enum=auto;reachable;unreachable
type GithubWebhooksPosture string

const (
	// GithubWebhooksAuto follows the front door: reachable when the door is
	// on the public internet.
	GithubWebhooksAuto GithubWebhooksPosture = "auto"
	// GithubWebhooksReachable declares that the host can deliver regardless
	// of the internet (a same-network enterprise server).
	GithubWebhooksReachable GithubWebhooksPosture = "reachable"
	// GithubWebhooksUnreachable declares that the host cannot deliver, so
	// Planton polls for pushes.
	GithubWebhooksUnreachable GithubWebhooksPosture = "unreachable"
)

// GithubAppSpec is the identity of a GitHub App registered for the install
// on one host. The App's private key stays in a Secret the adopter owns,
// exactly as GitHub hands it (a PEM file); the operator mounts it as a file
// for the control plane and nobody encodes it by hand.
type GithubAppSpec struct {
	// clientId is the App's client ID from its settings page -- the JWT
	// issuer when the control plane mints installation tokens. Not a secret.
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientId"`

	// privateKeySecretRef names the Secret key holding the App's private key
	// as GitHub generated it: the PEM text, unencoded.
	PrivateKeySecretRef SecretKeyRef `json:"privateKeySecretRef"`

	// webhookSecretRef names the Secret key holding the webhook secret set
	// on the App, so deliveries are verified as the App's own. Without it
	// deliveries from this host are accepted unverified -- acceptable only
	// on a host that cannot deliver at all.
	// +optional
	WebhookSecretRef *SecretKeyRef `json:"webhookSecretRef,omitempty"`
}

// GitHub declaration echo for status.github: how the install's GitHub is
// declared, never whether GitHub accepted anything.
const (
	// GithubModeNotConfigured is the echo when spec.github is absent: the
	// github.com defaults apply.
	GithubModeNotConfigured = "NotConfigured"
)
