/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlantonIdentityProviderSpec connects a company's identity system to a
// self-hosted Planton: on-prem Active Directory over LDAP user federation, or
// Entra ID / ADFS / generic OIDC over identity brokering. Exactly one arm per
// resource -- a hybrid enterprise expresses both arms as two resources, each
// owned by the team that owns that directory.
//
// This is deliberately its own kind, not a PlantonPlatform field: identity
// config has a different owner (the company's IT) and a different change
// cadence than the platform spec, so applying, rotating, or fixing it never
// edits the platform manifest.
//
// The spec carries only federation INFRASTRUCTURE -- what the identity server
// needs to federate. Access policy (which directory groups grant which
// Planton roles) is product data with its own audit trail, managed inside
// Planton, never on this resource.
//
// +kubebuilder:validation:XValidation:rule="has(self.activeDirectory) != has(self.oidc)",message="exactly one of activeDirectory or oidc must be set"
type PlantonIdentityProviderSpec struct {
	// platformRef names the PlantonPlatform this identity config belongs to.
	// Optional: when empty, it resolves to the single PlantonPlatform in
	// this namespace; two platforms in the namespace with no ref is a status
	// error naming both candidates, never a guess. The resolution is
	// recorded in status.boundPlatform so it is visible, not implicit.
	// +optional
	PlatformRef *PlatformRef `json:"platformRef,omitempty"`

	// activeDirectory federates users from an on-prem Active Directory over
	// LDAP. The field set mirrors the AD config surface platform teams
	// already run elsewhere, so an existing setup transcribes directly.
	// +optional
	ActiveDirectory *ActiveDirectorySpec `json:"activeDirectory,omitempty"`

	// oidc brokers sign-in to an upstream OIDC identity provider (Entra ID,
	// ADFS, or any spec-compliant issuer).
	// +optional
	OIDC *OIDCBrokerSpec `json:"oidc,omitempty"`

	// signInButtonLabel is the text on the sign-in page's federation button.
	// Defaults to an arm-appropriate label when empty.
	// +kubebuilder:validation:MaxLength=64
	// +optional
	SignInButtonLabel string `json:"signInButtonLabel,omitempty"`
}

// PlatformRef names a PlantonPlatform in the same namespace.
type PlatformRef struct {
	// name of the PlantonPlatform resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ActiveDirectorySpec configures LDAP user federation against an on-prem
// Active Directory. The directory is the truth: federation is read-only in
// v1 (a directory write path is a liability nobody asked for), and every
// credential arrives by Secret reference -- there are deliberately NO inline
// credential fields, because a field that exists gets committed to git
// eventually.
//
// +kubebuilder:validation:XValidation:rule="!has(self.startTls) || !self.startTls || self.servers.all(s, s.startsWith('ldap://'))",message="startTls is only legal with ldap:// servers; ldaps:// is already TLS"
type ActiveDirectorySpec struct {
	// servers are the directory endpoints, tried in order. The scheme
	// decides transport: ldaps:// is TLS from the first byte, ldap:// is
	// plaintext unless startTls upgrades it.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.all(s, s.startsWith('ldap://') || s.startsWith('ldaps://'))",message="every server must use the ldap:// or ldaps:// scheme"
	Servers []string `json:"servers"`

	// startTls upgrades an ldap:// connection to TLS after connect.
	// +kubebuilder:default=false
	// +optional
	StartTLS bool `json:"startTls,omitempty"`

	// caBundleSecretRef points at a PEM CA bundle for verifying the
	// directory's TLS certificate -- the private-CA case, the classic
	// enterprise blocker. Omit it when the directory's certificate chains
	// to a public root.
	// +optional
	CABundleSecretRef *SecretKeyRef `json:"caBundleSecretRef,omitempty"`

	// bindDn is the service account the identity server binds as to search
	// the directory.
	// +kubebuilder:validation:MinLength=1
	BindDN string `json:"bindDn"`

	// bindCredentialSecretRef points at the bind password. Required by
	// reference: the password is never inline.
	BindCredentialSecretRef SecretKeyRef `json:"bindCredentialSecretRef"`

	// usersDn is the search base for people.
	// +kubebuilder:validation:MinLength=1
	UsersDN string `json:"usersDn"`

	// groupsDn is the search base for groups.
	// +kubebuilder:validation:MinLength=1
	GroupsDN string `json:"groupsDn"`

	// userObjectClasses filter which directory entries are users.
	// +kubebuilder:default={person,organizationalPerson,user}
	// +optional
	UserObjectClasses []string `json:"userObjectClasses,omitempty"`

	// The attribute mappings below default to Active Directory's standard
	// schema; generic-LDAP directories override them.

	// usernameAttribute is the sign-in name attribute.
	// +kubebuilder:default="sAMAccountName"
	// +optional
	UsernameAttribute string `json:"usernameAttribute,omitempty"`

	// emailAttribute maps the user's email -- the key Planton's grants and
	// invitations match on.
	// +kubebuilder:default="mail"
	// +optional
	EmailAttribute string `json:"emailAttribute,omitempty"`

	// firstNameAttribute maps the given name.
	// +kubebuilder:default="givenName"
	// +optional
	FirstNameAttribute string `json:"firstNameAttribute,omitempty"`

	// lastNameAttribute maps the surname.
	// +kubebuilder:default="sn"
	// +optional
	LastNameAttribute string `json:"lastNameAttribute,omitempty"`

	// groupNameAttribute maps a group's name.
	// +kubebuilder:default="cn"
	// +optional
	GroupNameAttribute string `json:"groupNameAttribute,omitempty"`

	// groupMemberAttribute is the group attribute listing members.
	// +kubebuilder:default="member"
	// +optional
	GroupMemberAttribute string `json:"groupMemberAttribute,omitempty"`

	// nestedGroups resolves group membership transitively (a member of
	// team-a inside engineering is a member of both).
	// +kubebuilder:default=true
	// +optional
	NestedGroups *bool `json:"nestedGroups,omitempty"`

	// editMode is pinned READ_ONLY in v1: Planton never writes to the
	// directory. The enum exists so a future writable mode is an additive
	// change, not a breaking one.
	// +kubebuilder:validation:Enum=READ_ONLY
	// +kubebuilder:default="READ_ONLY"
	// +optional
	EditMode string `json:"editMode,omitempty"`

	// syncPeriodMinutes is how often the identity server re-syncs users
	// from the directory.
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:default=60
	// +optional
	SyncPeriodMinutes int32 `json:"syncPeriodMinutes,omitempty"`
}

// OIDCBrokerSpec configures identity brokering to an upstream OIDC provider.
type OIDCBrokerSpec struct {
	// issuerUrl is the upstream issuer. For Entra ID this must be the
	// single-tenant form (https://login.microsoftonline.com/{tenant-id}/v2.0)
	// -- the /common and /organizations multiplexers cannot satisfy strict
	// issuer validation, and verification names that limit when it detects
	// them.
	// +kubebuilder:validation:Pattern=`^https?://.+`
	IssuerURL string `json:"issuerUrl"`

	// clientId of the app registration the company's IT created for
	// Planton.
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientId"`

	// clientSecretRef points at the app registration's client secret.
	// Required by reference: the secret is never inline.
	ClientSecretRef SecretKeyRef `json:"clientSecretRef"`

	// scopes requested from the upstream provider. Add the provider's group
	// scope when group claims need it.
	// +kubebuilder:default={openid,profile,email}
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// groupsClaim is where group memberships arrive in upstream tokens.
	// +kubebuilder:default="groups"
	// +optional
	GroupsClaim string `json:"groupsClaim,omitempty"`

	// subjectClaim is the upstream claim carrying the directory's stable id
	// for each user -- the id Planton correlates accounts, sync, and
	// offboarding on. The default "sub" is present on every OIDC provider;
	// Entra ID deployments should set "oid", which stays stable across app
	// registrations where Entra's sub does not.
	// +kubebuilder:default="sub"
	// +optional
	SubjectClaim string `json:"subjectClaim,omitempty"`
}

// ConditionBound is the condition tracking platform binding resolution on a
// PlantonIdentityProvider.
const ConditionBound = "Bound"

// ConditionProvisioned is the condition tracking federation provisioning on
// the bound platform's identity server. Its observedGeneration doubles as
// the verification cadence gate: the full verification (directory probes,
// user/group syncs) re-runs when the spec generation moves past it, when a
// referenced credential rotates, or after a repair -- never as a
// steady-state probe against the company's directory.
const ConditionProvisioned = "Provisioned"

// PlantonIdentityProviderStatus reports binding and verification as verdicts
// with plain-language messages -- a wrong bind credential shows a verdict,
// never a stack trace.
type PlantonIdentityProviderStatus struct {
	// conditions represent the current state. "Bound" tracks platform
	// binding resolution.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// boundPlatform is the resolved PlantonPlatform this identity config is
	// bound to -- recorded so an empty platformRef's resolution is visible,
	// not implicit.
	// +optional
	BoundPlatform string `json:"boundPlatform,omitempty"`

	// verification carries per-check verdicts from the last federation
	// verification pass (connection reached, TLS trusted, bind
	// authenticated, search bases readable, issuer discovered). Populated
	// by the federation provisioning reconcile.
	// +optional
	Verification *IdentityProviderVerification `json:"verification,omitempty"`
}

// IdentityProviderVerification is the manifest-side half of the live
// verification experience: each check is a named verdict with a message a
// person can act on.
type IdentityProviderVerification struct {
	// checks, in the order they ran.
	// +optional
	Checks []IdentityProviderVerificationCheck `json:"checks,omitempty"`
}

// IdentityProviderVerificationCheck is one verification verdict.
type IdentityProviderVerificationCheck struct {
	// name of the check (e.g. "connection", "tls", "bind", "usersSearch").
	Name string `json:"name"`

	// verdict of the check.
	// +kubebuilder:validation:Enum=Passed;Failed;Unknown
	Verdict string `json:"verdict"`

	// message explains the verdict in plain language, naming the remedy on
	// failure.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Platform",type=string,JSONPath=`.status.boundPlatform`
// +kubebuilder:printcolumn:name="Bound",type=string,JSONPath=`.status.conditions[?(@.type=='Bound')].status`
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=='Provisioned')].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PlantonIdentityProvider is the Schema for the plantonidentityproviders API
type PlantonIdentityProvider struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of PlantonIdentityProvider
	// +required
	Spec PlantonIdentityProviderSpec `json:"spec"`

	// status defines the observed state of PlantonIdentityProvider
	// +optional
	Status PlantonIdentityProviderStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PlantonIdentityProviderList contains a list of PlantonIdentityProvider
type PlantonIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PlantonIdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlantonIdentityProvider{}, &PlantonIdentityProviderList{})
}
