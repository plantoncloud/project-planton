# A self-hosted install declares its mail provider, and the operator delivers it

## What changed

- **`spec.email` on `PlantonPlatform`.** One block declares the mail provider every sender on the install uses -- the control plane and, next, the identity server: the address the install sends as (`from.address`, required; `from.name`, default `Planton`; `replyTo`), and exactly one arm. `smtp` points at any relay (Exchange Online, Google Workspace, an internal smart host, or a vendor's SMTP endpoint) with `host`, `port` (default 587), `security` (`starttls` required-upgrade by default, `tls` implicit, `none` plaintext), and one way in: `credentialsSecretName` (a `kubernetes.io/basic-auth` Secret), `oauth2` (a client-credentials grant for XOAUTH2 -- `user`, `tokenUrl`, `scope`, `clientId`, `clientSecretRef`), or nothing for a relay that admits the cluster by address; `caBundleSecretRef` trusts a relay behind a private CA. `resend` carries `apiKeySecretRef`. Admission rules refuse both arms, no arm, two ways in, and any credential over a plaintext connection, each in a sentence naming the ways out.
- **The operator delivers the declaration; it never probes the relay.** Provider, sender identity, and relay facts render as `PLANTON_EMAIL_*` environment; every secret value (password, client secret, CA bundle, API key) renders as a file under `/etc/planton/email/` from one projected volume with fixed file names, each named by a static `*_FILE` variable -- so a rotated Secret is live on the next send with no pod roll. Without `spec.email` the operator renders `PLANTON_EMAIL_PROVIDER=none` out loud (the control plane reads an unset provider as the hosted arm). Two setup hints (`PLANTON_EMAIL_SETUP_HINT_MANIFEST`, `PLANTON_EMAIL_SETUP_HINT_SECRET_COMMAND`) carry the platform's own name and namespace for the console to show verbatim.
- **Every referenced Secret is preflighted before the pod is rendered.** A missing Secret or key is reported on the control-plane component in words -- which Secret, which key, what type to create it as, which field to remove -- and the pod is rendered as if no email were declared, so the platform keeps running and nothing sits in FailedMount.
- **`status.email` and an `EMAIL` column** echo the declared arm (`NotConfigured`, `SMTP`, `Resend`) in the `LICENSE` column's grammar: configuration, never a verdict.
- **One `SecretKeyRef` type** in the API package replaces the two identical narrowed Secret references the license and the identity provider each carried; the generated schema is unchanged apart from one description string.
- The boot-contract fixture records the new names. The floor stays at `v0.0.60`: no published platform reads the `smtp` arm yet, and the floor moves to the release that does.

## Why

An install a team can join is not yet an install a team can run. Email is what turns an invitation into something that reaches a person, an alert into something someone reads, and a forgotten password into a solved problem instead of an admin-console rescue. The declaration is shaped for the mail teams that will actually configure it: Exchange Online after Microsoft retires password submission (OAuth2), an internal relay behind a corporate CA (the CA bundle), a smart host that allow-lists the cluster (no credential) -- and it refuses the one thing a manifest must never cause, a company's mail password crossing the network in the clear.

## How to check

```bash
cd operator && go test ./internal/controller -run TestControllers          # admission: the accepted shapes and every refusal's sentence
cd operator && go test ./internal/resources ./internal/component ./internal/status   # rendering, preflight wording, the column
cd operator && go test ./internal/resources -run TestBootContract          # the rendered environment matches the fixture
```

Live: on a Kind cluster with this operator, a platform without `spec.email` reaches `Ready` with `PLANTON_EMAIL_PROVIDER=none` and `EMAIL: NotConfigured`; after `spec.email.smtp` with a basic-auth Secret, the `EMAIL` column reads `SMTP`, the control-plane Deployment carries the relay facts and the two `*_FILE` paths with no Secret-backed env, the files inside a pod hold the Secret's values, editing the Secret changes the file with zero restarts, deleting the Secret puts the sentence above on the control-plane component and renders the pod as `none` again, and the admission refusals each print their sentence.
