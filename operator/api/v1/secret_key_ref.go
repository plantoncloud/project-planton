package v1

// SecretKeyRef names one entry of one Secret in the resource's own namespace.
// It is the one shape every credential-by-reference on these kinds uses -- the
// license key, a directory bind password, an OIDC client secret, a private CA
// bundle, an email relay's API key -- so a reader who has met it once has met
// them all.
//
// A narrowed, CRD-local mirror of corev1.SecretKeySelector: embedding the core
// type would admit its optional flag, which has no meaning here. A declared
// credential reference must resolve; the operator preflights it and reports a
// missing Secret in words rather than rendering a pod that cannot start.
type SecretKeyRef struct {
	// name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key within the Secret.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}
