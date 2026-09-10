package component

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/keycloak"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The federation half of the identity component: translating the bound
// PlantonIdentityProvider's spec (plus its referenced Secrets) into the
// realm reconciler's desired state, deciding when the verification pass is
// due, and writing the manifest's Provisioned condition and verification
// verdicts.
//
// Verification cadence is the load-bearing restraint here: the reconcile
// loop runs every 30 seconds, and a corporate directory probed on that
// rhythm is ~3,000 probes a day -- exactly what an enterprise security team
// pages on. Probes and syncs run only when the spec generation moved, a
// referenced credential rotated, or federation state was repaired; the
// verdicts persist in status between runs.

// federationBuild is one pass's translated desired state plus the
// bookkeeping the post-converge steps need.
type federationBuild struct {
	// fed is the reconciler's desired state; nil when buildErr is set (the
	// hands-off contract -- see keycloak.OwnedFederation).
	fed *keycloak.OwnedFederation

	// buildErr is the plain-language reason desired state could not be
	// built (a referenced Secret unreadable, discovery unreachable). It
	// lands on the Provisioned condition; federation is left untouched.
	buildErr string

	// verificationDue: the spec generation moved past the last verified
	// one, the credential rotated, or no verdicts exist yet.
	verificationDue bool

	// credentialSHA fingerprints the credential included in fed, recorded
	// in the state Secret after a successful pass.
	credentialSHA string

	// endpointsJSON is the freshly-discovered OIDC endpoints (broker arm,
	// verification passes only), recorded for recreate-without-refetch.
	endpointsJSON string

	// caBundle carries the CA Secret's name/key/content-hash into the
	// identity Deployment (LDAP arm with caBundleSecretRef only).
	caBundleSecretName string
	caBundleSecretKey  string
	caBundleHash       string
}

// buildFederationState translates the bound manifest into desired federation
// state. It never returns an error: every failure mode becomes buildErr,
// because a build failure is the MANIFEST's status to carry, never a reason
// to wedge the whole identity component (Keycloak keeps serving the users it
// already has).
func (id *Identity) buildFederationState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, idp *v1.PlantonIdentityProvider) *federationBuild {
	build := &federationBuild{}

	recorded := id.readFederationState(ctx, c, planton)
	provisioned := meta.FindStatusCondition(idp.Status.Conditions, v1.ConditionProvisioned)
	build.verificationDue = provisioned == nil ||
		provisioned.ObservedGeneration != idp.Generation ||
		idp.Status.Verification == nil

	switch {
	case idp.Spec.ActiveDirectory != nil:
		id.buildLDAPState(ctx, c, planton.Namespace, idp.Spec.ActiveDirectory, recorded, build)
	case idp.Spec.OIDC != nil:
		id.buildBrokerState(ctx, c, planton.Namespace, idp.Spec, recorded, build)
	}
	return build
}

func (id *Identity) buildLDAPState(ctx context.Context, c client.Client, namespace string, ad *v1.ActiveDirectorySpec, recorded map[string]string, build *federationBuild) {
	bindCredential, err := readSecretKey(ctx, c, namespace, ad.BindCredentialSecretRef)
	if err != nil {
		build.buildErr = fmt.Sprintf("the bind credential could not be read from Secret %s (key %s): %v -- federation is left untouched until it can be",
			ad.BindCredentialSecretRef.Name, ad.BindCredentialSecretRef.Key, err)
		return
	}
	build.credentialSHA = sha256Hex(bindCredential)
	rotate := recorded[resources.IdentityFederationStateCredentialKey] != build.credentialSHA
	if rotate {
		build.verificationDue = true
	}

	if ad.CABundleSecretRef != nil {
		caBundle, err := readSecretKey(ctx, c, namespace, *ad.CABundleSecretRef)
		if err != nil {
			build.buildErr = fmt.Sprintf("the CA bundle could not be read from Secret %s (key %s): %v -- federation is left untouched until it can be",
				ad.CABundleSecretRef.Name, ad.CABundleSecretRef.Key, err)
			return
		}
		build.caBundleSecretName = ad.CABundleSecretRef.Name
		build.caBundleSecretKey = ad.CABundleSecretRef.Key
		build.caBundleHash = sha256Hex(caBundle)
	}

	// The CRD's defaulting owns these values; the fallbacks below only
	// guard writes that bypassed admission defaulting (fake clients in
	// tests, hand-built objects) so a zero never reaches Keycloak -- a
	// fullSyncPeriod of 0 is a CONTINUOUS directory sync.
	syncPeriod := ad.SyncPeriodMinutes
	if syncPeriod <= 0 {
		syncPeriod = 60
	}
	build.fed = &keycloak.OwnedFederation{LDAP: &keycloak.OwnedLDAPFederation{
		Servers:            ad.Servers,
		StartTLS:           ad.StartTLS,
		UseTruststore:      ad.CABundleSecretRef != nil,
		BindDN:             ad.BindDN,
		BindCredential:     bindCredential,
		RotateCredential:   rotate,
		UsersDN:            ad.UsersDN,
		GroupsDN:           ad.GroupsDN,
		UserObjectClasses:  defaultStrings(ad.UserObjectClasses, []string{"person", "organizationalPerson", "user"}),
		UsernameAttribute:  defaultString(ad.UsernameAttribute, "sAMAccountName"),
		EmailAttribute:     defaultString(ad.EmailAttribute, "mail"),
		FirstNameAttribute: defaultString(ad.FirstNameAttribute, "givenName"),
		LastNameAttribute:  defaultString(ad.LastNameAttribute, "sn"),
		GroupNameAttribute: defaultString(ad.GroupNameAttribute, "cn"),
		GroupMemberAttr:    defaultString(ad.GroupMemberAttribute, "member"),
		NestedGroups:       ad.NestedGroups == nil || *ad.NestedGroups,
		SyncPeriodMinutes:  syncPeriod,
	}}
}

func (id *Identity) buildBrokerState(ctx context.Context, c client.Client, namespace string, spec v1.PlantonIdentityProviderSpec, recorded map[string]string, build *federationBuild) {
	oidc := spec.OIDC
	clientSecret, err := readSecretKey(ctx, c, namespace, oidc.ClientSecretRef)
	if err != nil {
		build.buildErr = fmt.Sprintf("the client secret could not be read from Secret %s (key %s): %v -- federation is left untouched until it can be",
			oidc.ClientSecretRef.Name, oidc.ClientSecretRef.Key, err)
		return
	}
	build.credentialSHA = sha256Hex(clientSecret)
	rotate := recorded[resources.IdentityFederationStateCredentialKey] != build.credentialSHA
	if rotate {
		build.verificationDue = true
	}

	// Endpoints: freshly discovered on verification passes (the same fetch
	// verifies the issuer), replayed from the state record otherwise -- the
	// steady-state reconcile never fetches the upstream's discovery
	// document. No record and no fresh discovery means the broker cannot be
	// created correctly: hands off, with the reason on the condition.
	var endpoints *keycloak.OIDCEndpoints
	if build.verificationDue {
		fresh, _, err := keycloak.DiscoverOIDC(ctx, &http.Client{Timeout: httpClientTimeout}, oidc.IssuerURL)
		if err == nil {
			endpoints = fresh
			if data, marshalErr := json.Marshal(fresh); marshalErr == nil {
				build.endpointsJSON = string(data)
			}
		}
	}
	if endpoints == nil && recorded[resources.IdentityFederationStateEndpointsKey] != "" {
		var replayed keycloak.OIDCEndpoints
		if err := json.Unmarshal([]byte(recorded[resources.IdentityFederationStateEndpointsKey]), &replayed); err == nil {
			endpoints = &replayed
		}
	}
	if endpoints == nil {
		build.buildErr = fmt.Sprintf("the issuer's discovery document could not be fetched from %s/.well-known/openid-configuration -- check the issuer URL and the cluster's egress; federation is left untouched until it can be", oidc.IssuerURL)
		return
	}

	build.fed = &keycloak.OwnedFederation{Broker: &keycloak.OwnedOIDCBroker{
		IssuerURL:        oidc.IssuerURL,
		ClientID:         oidc.ClientID,
		ClientSecret:     clientSecret,
		RotateCredential: rotate,
		Scopes:           defaultStrings(oidc.Scopes, []string{"openid", "profile", "email"}),
		GroupsClaim:      defaultString(oidc.GroupsClaim, "groups"),
		SubjectClaim:     defaultString(oidc.SubjectClaim, "sub"),
		DisplayName:      defaultString(spec.SignInButtonLabel, "Sign in with your organization"),
		Endpoints:        endpoints,
	}}
}

// federationForConverge maps the binding + build outcome onto the
// reconciler's nil-vs-empty contract.
func federationForConverge(boundIdp *v1.PlantonIdentityProvider, build *federationBuild) *keycloak.OwnedFederation {
	switch {
	case boundIdp == nil:
		// Nothing bound: sweep operator-named federation objects (the
		// manifest-deletion path).
		return &keycloak.OwnedFederation{}
	case build.buildErr != "":
		// Desired state unbuildable: hands off.
		return nil
	default:
		return build.fed
	}
}

// finishFederation runs after a successful convergence: the verification
// pass when due, the Provisioned condition + verdicts onto the manifest,
// and the state record for the next pass. Returns true when fresh verdicts
// were recorded (the facts projection re-stamps its observed-at only then).
func (id *Identity) finishFederation(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, idp *v1.PlantonIdentityProvider, build *federationBuild, report *keycloak.Report, verifyIn keycloak.VerifyInput) bool {
	log := logf.FromContext(ctx).WithValues("component", id.Name())

	// A repair pass re-verifies too: repaired federation state deserves
	// fresh verdicts (a non-federation repair triggering one extra
	// verification is a rare, harmless over-probe).
	if !build.verificationDue && report.Clean() {
		return false
	}

	verifyIn.Federation = build.fed
	checks, err := keycloak.Verify(ctx, verifyIn)
	if err != nil {
		id.setFederationStatus(ctx, c, idp, metav1.Condition{
			Type: v1.ConditionProvisioned, Status: metav1.ConditionFalse,
			Reason:             "VerificationError",
			Message:            "federation is provisioned but verification could not run: " + err.Error(),
			ObservedGeneration: idp.Generation,
		}, nil)
		return false
	}

	verification := &v1.IdentityProviderVerification{Checks: make([]v1.IdentityProviderVerificationCheck, 0, len(checks))}
	var failed []string
	for _, check := range checks {
		verification.Checks = append(verification.Checks, v1.IdentityProviderVerificationCheck{
			Name: check.Name, Verdict: string(check.Verdict), Message: check.Message,
		})
		if check.Verdict == keycloak.VerdictFailed {
			failed = append(failed, check.Name)
		}
	}

	condition := metav1.Condition{
		Type: v1.ConditionProvisioned, Status: metav1.ConditionTrue,
		Reason:             "Provisioned",
		Message:            "federation is provisioned on the identity server and every verification check passed",
		ObservedGeneration: idp.Generation,
	}
	if len(failed) > 0 {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "VerificationFailed"
		condition.Message = fmt.Sprintf("federation is provisioned but verification failed: %v -- each check's message in status.verification names the remedy", failed)
	}
	id.setFederationStatus(ctx, c, idp, condition, verification)

	if err := id.writeFederationState(ctx, c, planton, build); err != nil {
		// Worst case of a lost record: one redundant credential write and
		// one extra discovery fetch next pass -- log, never fail.
		log.Error(err, "Failed to record federation state; the next pass repeats one rotation write")
	}
	return true
}

// setFederationStatus writes the Provisioned condition (and verdicts when
// verification ran) onto a FRESH read of the manifest -- the binding pass
// already wrote its status earlier in this reconcile, so updating the stale
// in-memory copy would conflict.
func (id *Identity) setFederationStatus(ctx context.Context, c client.Client, idp *v1.PlantonIdentityProvider, condition metav1.Condition, verification *v1.IdentityProviderVerification) {
	log := logf.FromContext(ctx).WithValues("component", id.Name())

	var fresh v1.PlantonIdentityProvider
	if err := c.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, &fresh); err != nil {
		log.Error(err, "Failed to re-read PlantonIdentityProvider for federation status", "name", idp.Name)
		return
	}

	changed := meta.SetStatusCondition(&fresh.Status.Conditions, condition)
	if verification != nil && !verificationEqual(fresh.Status.Verification, verification) {
		fresh.Status.Verification = verification
		changed = true
	}
	if !changed {
		return
	}
	if err := c.Status().Update(ctx, &fresh); err != nil {
		// The next pass re-resolves from fresh reads; log-and-continue is
		// the whole recovery (the binding writer's posture).
		log.Error(err, "Failed to update PlantonIdentityProvider federation status", "name", idp.Name)
		return
	}
	log.Info("Updated PlantonIdentityProvider federation status",
		"name", idp.Name, "reason", condition.Reason, "checks", len(checksOrNone(verification)))
}

func checksOrNone(v *v1.IdentityProviderVerification) []v1.IdentityProviderVerificationCheck {
	if v == nil {
		return nil
	}
	return v.Checks
}

func verificationEqual(a, b *v1.IdentityProviderVerification) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	if len(a.Checks) != len(b.Checks) {
		return false
	}
	for i := range a.Checks {
		if a.Checks[i] != b.Checks[i] {
			return false
		}
	}
	return true
}

// readFederationState returns the recorded fingerprints/endpoints (empty map
// when the record does not exist yet).
func (id *Identity) readFederationState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) map[string]string {
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{
		Name: resources.IdentityFederationStateSecretName(planton.Name), Namespace: planton.Namespace,
	}, &secret)
	if err != nil {
		return map[string]string{}
	}
	state := make(map[string]string, len(secret.Data))
	for key, value := range secret.Data {
		state[key] = string(value)
	}
	return state
}

// writeFederationState records what this pass wrote, so the next pass can
// tell rotation from steady state without ever reading a masked value back.
func (id *Identity) writeFederationState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, build *federationBuild) error {
	data := map[string]string{
		resources.IdentityFederationStateCredentialKey: build.credentialSHA,
	}
	if build.endpointsJSON != "" {
		data[resources.IdentityFederationStateEndpointsKey] = build.endpointsJSON
	}
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.IdentityFederationStateSecretName(planton.Name),
			Namespace: planton.Namespace,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
	if ownerRef := id.OwnerReferenceFor(planton); ownerRef != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return id.ApplyTypedObject(ctx, c, secret)
}

// projectFederationFacts publishes the federation facts ConfigMap: the
// operator's ADVERTISEMENT of the bound identity manifest's declared arm and
// recorded verification verdicts, mounted read-only by the control plane so
// the console renders the connect journey and verification surface without
// the control plane needing any Kubernetes read path of its own (the CR
// stays the operator's record; this file is its curated projection).
//
// Called on EVERY reconcile -- content says configured=false when nothing is
// bound -- so the ConfigMap provably exists before the control plane's
// Deployment first renders (controlplane depends on identity), and the
// manifest-deletion sweep flips it back to configured=false the same pass.
//
// freshVerdicts stamps observedAt with now; otherwise the previously
// recorded stamp is preserved. The stamp means "when the gateway-side
// verdicts were recorded", never "when this file was last rewritten".
func (id *Identity) projectFederationFacts(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, boundIdp *v1.PlantonIdentityProvider, freshVerdicts bool) {
	log := logf.FromContext(ctx).WithValues("component", id.Name())

	facts := resources.IdentityFederationFacts{}
	if boundIdp != nil {
		// Re-read the manifest: this pass's status writes (the Provisioned
		// condition, fresh verdicts) landed on the API server, not on the
		// in-memory copy resolved at the top of the reconcile.
		var fresh v1.PlantonIdentityProvider
		if err := c.Get(ctx, types.NamespacedName{Name: boundIdp.Name, Namespace: boundIdp.Namespace}, &fresh); err != nil {
			log.Error(err, "Failed to re-read PlantonIdentityProvider for the facts projection", "name", boundIdp.Name)
			return
		}
		facts.Configured = true
		switch {
		case fresh.Spec.ActiveDirectory != nil:
			facts.Arm = "ldap"
			facts.ProviderLabel = defaultString(fresh.Spec.SignInButtonLabel, "Active Directory")
		case fresh.Spec.OIDC != nil:
			facts.Arm = "oidc"
			// The broker's default display name -- kept in lockstep with
			// buildBrokerState so the facts and the sign-in button agree.
			facts.ProviderLabel = defaultString(fresh.Spec.SignInButtonLabel, "Sign in with your organization")
		}
		facts.Provisioned = meta.IsStatusConditionTrue(fresh.Status.Conditions, v1.ConditionProvisioned)
		if fresh.Status.Verification != nil {
			for _, check := range fresh.Status.Verification.Checks {
				facts.Checks = append(facts.Checks, resources.IdentityFederationFactsCheck{
					Name: check.Name, Verdict: check.Verdict, Message: check.Message,
				})
			}
		}
		if freshVerdicts {
			facts.ObservedAt = time.Now().UTC().Format(time.RFC3339)
		} else {
			facts.ObservedAt = id.recordedFactsObservedAt(ctx, c, planton)
		}
	}

	configMap, err := resources.IdentityFederationFactsConfigMap(planton.Name, planton.Namespace, facts, id.OwnerReferenceFor(planton))
	if err != nil {
		log.Error(err, "Failed to render the federation facts ConfigMap")
		return
	}
	if err := id.ApplyTypedObject(ctx, c, configMap); err != nil {
		// The next pass rewrites the projection from scratch; log-and-
		// continue is the whole recovery (the status writer's posture).
		log.Error(err, "Failed to publish the federation facts ConfigMap")
	}
}

// recordedFactsObservedAt reads the previously projected observed-at stamp,
// or empty when no projection (or no stamp) exists yet.
func (id *Identity) recordedFactsObservedAt(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) string {
	var configMap corev1.ConfigMap
	err := c.Get(ctx, types.NamespacedName{
		Name: resources.IdentityFederationFactsConfigMapName(planton.Name), Namespace: planton.Namespace,
	}, &configMap)
	if err != nil {
		return ""
	}
	var recorded resources.IdentityFederationFacts
	if err := json.Unmarshal([]byte(configMap.Data[resources.IdentityFederationFactsKey]), &recorded); err != nil {
		return ""
	}
	return recorded.ObservedAt
}

// readSecretKey reads one key of a user-provided Secret in the platform's
// namespace.
func readSecretKey(ctx context.Context, c client.Client, namespace string, ref v1.SecretKeyRef) (string, error) {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, &secret); err != nil {
		return "", err
	}
	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q is not in Secret %s", ref.Key, ref.Name)
	}
	return string(value), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultStrings(value, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}
