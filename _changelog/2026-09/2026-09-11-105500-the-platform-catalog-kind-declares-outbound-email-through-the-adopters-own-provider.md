# The platform catalog kind declares outbound email through the adopter's own provider

## What changed

- **`KubernetesPlantonPlatform.spec.email` mirrors the operator's `spec.email`.** An infra chart, a Terraform module, or a Pulumi program that declares a self-hosted platform now declares its mail provider in the same manifest: `from{address, name}` and `reply_to` as the sending identity both the control plane and the identity server use; exactly one of `smtp{host, port, security, credentials_secret_name, oauth2{user, token_url, scope, client_id, client_secret_ref}, ca_bundle_secret_ref}` or `resend{api_key_secret_ref}`. Closed vocabularies stay strings with `in:` lists (`security`: `starttls` | `tls` | `none`); defaulted scalars (`port` 587, `security` starttls, `from.name` Planton) are `optional` and render on presence only, so an omitted value is left to the CRD's own default. The three rules the operator's definition enforces are mirrored word for word as message CEL, so a manifest is refused at validation in the same words the cluster would use: exactly one provider; one way into a relay (a credentials Secret or OAuth2, never both); no credentials over `security: none`. The field names the first operator chart whose definition knows it (0.14.1).
- **Both engines render only what the manifest declared, in lockstep.** Terraform's `locals.email_body` and the Pulumi module's `platformSpecBody` render the block through the same null-prune and presence idioms every other block uses; the offline `tofu plan` and `pulumi preview` over the full-surface e2e manifest produce the identical `email` block.
- **One Secret reference message for the whole spec.** `KubernetesPlantonPlatformLicenseSecretKeyRef` is renamed `KubernetesPlantonPlatformSecretKeyRef` and carries every by-reference credential on the spec: the license key, the relay's CA bundle, the OAuth2 client secret, the Resend API key. This is the per-kind shape the other Kubernetes kinds already use (`KubernetesLokiSecretKeyRef`, `KubernetesGrafanaSecretKeyRef`, ...). The wire shape is unchanged; only the generated type name moves.
- **Preset `05-email-smtp`** shows the Office 365 relay with a basic-auth Secret, and teaches the OAuth2 and private-CA variants in comments; the four existing presets link to it. The GUIDE gains "Declare email once, and both senders use it"; the README's spec table, the catalog page's Key Configuration, and the compliance profile's `enc-in-transit` control name the field; the e2e manifest exercises the relay with a CA bundle; the reference is regenerated.

## Why

A self-hosted Planton a team runs day to day needs to reach people: invitations that land in inboxes, alerts someone reads, a "Forgot password?" that works. The operator already delivers one email declaration to both senders; until now the only way to make it was a hand edit of the platform manifest after install. The catalog kind is one of the three surfaces a platform is declared from, and a field the operator knows but the catalog does not is a field a GitOps tree cannot declare. Credentials stay Secret names and Secret key references, never values, because a manifest is a file people commit.

## How to check

```bash
go test ./catalog/kubernetes/kubernetesplantonplatform/...   # accept and refusal cases, incl. the three mirrored rules in their sentences
cd catalog/kubernetes/kubernetesplantonplatform/iac/tf && planton tofu init --manifest ../../e2e/manifest.yaml --module-dir . && planton tofu plan --manifest ../../e2e/manifest.yaml --module-dir .   # the email block rendered as declared
planton validate-manifest catalog/kubernetes/kubernetesplantonplatform/presets/05-email-smtp.yaml
planton secret-coverage --check
```
