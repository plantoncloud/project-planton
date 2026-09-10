package component

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// realmStateRecord is what the operator remembers about the identity server's
// realm between passes: the fingerprint of each credential it last handed
// over, and the broker endpoints it last discovered. Keycloak masks secret
// config on read, so a fingerprint against a record is the only way to tell a
// rotation from a steady state without writing every pass or never.
//
// One record, one writer: the pass reads it once (readRealmState), each step
// advances the field it earned in memory -- the federation fields where
// verification succeeds, the email field after a clean convergence -- and
// the pass writes it once at the end when it changed. The Secret is applied
// with server-side apply under one field manager, so two partial writers
// would each drop the other's keys; the single writer is what makes a
// second fingerprint safe.
type realmStateRecord struct {
	// FederationCredentialSHA fingerprints the directory bind credential (or
	// the broker client secret) last written to the realm.
	FederationCredentialSHA string
	// OIDCEndpointsJSON is the broker issuer's last-discovered endpoints,
	// replayed on steady-state passes so a hand-deleted broker can be
	// recreated without an on-cadence discovery fetch.
	OIDCEndpointsJSON string
	// EmailCredentialSHA fingerprints the mail relay's password, OAuth2
	// client secret, or API key last written to the realm's smtpServer.
	EmailCredentialSHA string
}

// readRealmState returns the recorded state, empty when the record does not
// exist yet (a first-ever pass rotates every credential once, the documented
// worst case of a lost record).
func (id *Identity) readRealmState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) realmStateRecord {
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{
		Name: resources.IdentityRealmStateSecretName(planton.Name), Namespace: planton.Namespace,
	}, &secret)
	if err != nil {
		return realmStateRecord{}
	}
	return realmStateRecord{
		FederationCredentialSHA: string(secret.Data[resources.IdentityRealmStateFederationCredentialKey]),
		OIDCEndpointsJSON:       string(secret.Data[resources.IdentityRealmStateOIDCEndpointsKey]),
		EmailCredentialSHA:      string(secret.Data[resources.IdentityRealmStateEmailCredentialKey]),
	}
}

// persistRealmState is the pass's one write of the record, only when it
// moved. Worst case of a lost write: one redundant credential write and one
// extra discovery fetch next pass -- log, never fail.
func (id *Identity) persistRealmState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, recorded, record realmStateRecord) {
	if record == recorded {
		return
	}
	if err := id.writeRealmState(ctx, c, planton, record); err != nil {
		logf.FromContext(ctx).WithValues("component", id.Name()).
			Error(err, "Failed to record realm state; the next pass repeats one rotation write")
	}
}

// writeRealmState records the pass's state. Empty fields are omitted from
// the apply so the key is removed rather than written blank; a caller that
// wants a field preserved keeps the recorded value on the record it passes.
func (id *Identity) writeRealmState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, record realmStateRecord) error {
	data := map[string][]byte{}
	for key, value := range map[string]string{
		resources.IdentityRealmStateFederationCredentialKey: record.FederationCredentialSHA,
		resources.IdentityRealmStateOIDCEndpointsKey:        record.OIDCEndpointsJSON,
		resources.IdentityRealmStateEmailCredentialKey:      record.EmailCredentialSHA,
	} {
		if value != "" {
			data[key] = []byte(value)
		}
	}
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.IdentityRealmStateSecretName(planton.Name),
			Namespace: planton.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	if ownerRef := id.OwnerReferenceFor(planton); ownerRef != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return id.ApplyTypedObject(ctx, c, secret)
}
