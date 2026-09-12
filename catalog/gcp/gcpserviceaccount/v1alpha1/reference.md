# GcpServiceAccount

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `gcp.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

GcpServiceAccountSpec defines the configuration for a Google Cloud service account —
the identity that workloads (GKE pods, Cloud Run services, Cloud Functions, Compute
instances, CI/CD pipelines) authenticate as when calling Google Cloud APIs.

Identity vs. access: this resource creates the identity and can optionally attach
broad role lists at the project or organization scope. For fine-grained, composable
grants — one role, one member, per resource node — prefer the GcpProjectIamMember
component, which references this service account's `member` output directly.

Keyless by default: no user-managed key is created unless `create_key` is set.
Prefer keyless patterns (Workload Identity on GKE, service-account impersonation,
Workload Identity Federation for external CI/CD) over exported keys — a key is a
long-lived credential that must be rotated and protected.

## Example

```yaml
apiVersion: gcp.planton.dev/v1alpha1
kind: GcpServiceAccount
metadata:
  name: my-sample-service-account
spec:
  # Service account ID (6-30 characters; lowercase letters, digits, hyphens;
  # starts with a letter, cannot end with a hyphen)
  serviceAccountId: my-sample-sa

  # GCP project ID where the service account will be created.
  # Omit to use the provider's default project.
  projectId:
    value: my-gcp-project-123

  # Human-readable identity shown in the GCP console (falls back to metadata.name)
  displayName: Sample Service Account

  # What this identity is for — surfaces in the console and gcloud describe
  description: Sample identity used to exercise the module locally

  # User-managed key: omit entirely for keyless (recommended). When present,
  # the block's fields shape the key — e.g. keepers as the rotation trigger:
  #   userManagedKey:
  #     keepers:
  #       rotation: "2026-08"
  # This canonical example stays keyless.

  # Project-level IAM roles to grant to this service account (additive grants)
  projectIamRoles:
    - roles/logging.logWriter
    - roles/monitoring.metricWriter

  # Organization-level IAM roles (requires orgId to be set)
  # orgId: "123456789012"
  # orgIamRoles:
  #   - roles/resourcemanager.organizationViewer
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.serviceAccountId` | `string` | yes |  |  |
| `spec.projectId` | `string \| valueFrom` |  |  | GcpProject (`status.outputs.project_id`) |
| `spec.displayName` | `string` |  |  |  |
| `spec.description` | `string` |  |  |  |
| `spec.disabled` | `bool` |  | `false` |  |
| `spec.userManagedKey` | `GcpServiceAccountUserManagedKey` |  |  |  |
| `spec.userManagedKey.algorithm` | `string` |  |  |  |
| `spec.userManagedKey.privateKeyType` | `string` |  |  |  |
| `spec.userManagedKey.publicKeyType` | `string` |  |  |  |
| `spec.userManagedKey.publicKeyData` | `string` |  |  |  |
| `spec.userManagedKey.keepers` | `map<string, string>` |  |  |  |
| `spec.userManagedKey.deletionPolicy` | `string` |  |  |  |
| `spec.projectIamRoles` | `[]string` |  |  |  |
| `spec.orgId` | `string` |  |  |  |
| `spec.orgIamRoles` | `[]string` |  |  |  |
| `spec.createIgnoreAlreadyExists` | `bool` |  |  |  |
| `spec.deletionPolicy` | `string` |  |  |  |

## Field Details

### spec.serviceAccountId

`string` · required

The short account ID that forms the service account email:
<service_account_id>@<project>.iam.gserviceaccount.com.
Must be 6-30 characters, start with a lowercase letter, and contain only
lowercase letters, digits, and hyphens (cannot end with a hyphen).
Immutable: changing it destroys and recreates the service account, which
invalidates every IAM binding and workload identity that references the old email.

- rule: {"required":true,"string":{"minLen":"6","maxLen":"30","pattern":"^[a-z]([-a-z0-9]*[a-z0-9])?$"}}

### spec.projectId

`string | valueFrom`

The GCP project in which the service account is created.
Can be a literal project ID or a reference to a GcpProject resource.
If omitted, the provider's default project is used — set it explicitly in
multi-project layouts so the identity's home project is unambiguous.

- references: GcpProject (`status.outputs.project_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: GcpProject, name: <that resource's name>, fieldPath: status.outputs.project_id}} -- a bare string does not parse

### spec.displayName

`string`

Human-readable display name shown in the GCP console (max 100 characters
in GCP; not validated here because the API truncates rather than rejects).
If omitted, the resource's metadata name is used.
Mutable: can be changed without recreating the service account.

### spec.description

`string`

Human-readable description of what this service account is for (max 256 bytes).
Surfaces in the GCP console and `gcloud iam service-accounts describe` — write it
for the operator who finds this identity two years from now.
Mutable: can be changed without recreating the service account.

- rule: {"string":{"maxLen":"256"}}

### spec.disabled

`bool` · optional (explicit presence)

Whether the service account is disabled. A disabled service account keeps its
IAM bindings but cannot authenticate — tokens are rejected until it is re-enabled.
Useful as a kill switch during incident response or for staged decommissioning
(disable first, observe breakage, then delete).

- default: `false`

### spec.userManagedKey

`GcpServiceAccountUserManagedKey`

Create a user-managed key for this service account. Omit for keyless
(the recommended default — prefer Workload Identity, impersonation, or
federation wherever the workload supports it). When present, a key is
created with the configured algorithm and formats, and the private key
(unless public_key_data supplies your own public key) is exported in
stack outputs as `key_base64` — treat that output as a live credential.

- rule: public_key_data (upload flow) cannot be combined with private_key_type or public_key_type (generate flow)

### spec.userManagedKey.algorithm

`string`

Algorithm used to generate the key:
  ""                  -- GCP default ("KEY_ALG_RSA_2048")
  "KEY_ALG_RSA_2048"  -- 2048-bit RSA (the standard choice)
  "KEY_ALG_RSA_1024"  -- 1024-bit RSA (legacy; weaker — avoid for
                         new keys)
Create-time only: changing it replaces the key.

- rule: algorithm must be one of: KEY_ALG_RSA_2048, KEY_ALG_RSA_1024

### spec.userManagedKey.privateKeyType

`string`

Output format of the generated private key:
  ""                             -- GCP default
                                    ("TYPE_GOOGLE_CREDENTIALS_FILE")
  "TYPE_GOOGLE_CREDENTIALS_FILE" -- the standard JSON credentials file
  "TYPE_PKCS12_FILE"             -- PKCS#12 bundle (password "notasecret";
                                    for legacy tooling that requires p12)

- rule: private_key_type must be one of: TYPE_GOOGLE_CREDENTIALS_FILE, TYPE_PKCS12_FILE

### spec.userManagedKey.publicKeyType

`string`

Output format of the public key:
  ""                    -- GCP default ("TYPE_X509_PEM_FILE")
  "TYPE_X509_PEM_FILE"  -- X.509 certificate PEM
  "TYPE_RAW_PUBLIC_KEY" -- raw public key bytes
  "TYPE_NONE"           -- do not return the public key

- rule: public_key_type must be one of: TYPE_X509_PEM_FILE, TYPE_RAW_PUBLIC_KEY, TYPE_NONE

### spec.userManagedKey.publicKeyData

`string`

Your own public key (base64-encoded X.509 PEM) — the UPLOAD flow: the
matching private key never leaves your custody and GCP returns no
private key material (the key_base64 stack output stays empty).
The strongest key posture when a user-managed key is unavoidable.

### spec.userManagedKey.keepers

`map<string, string>`

Arbitrary key/value pairs whose CHANGE forces a new key to be
generated — the idiomatic rotation trigger. Set e.g.
{"rotation": "2026-08"} and bump the value on your rotation cadence;
the old key is destroyed and a fresh one exported.

### spec.userManagedKey.deletionPolicy

`string`

Deletion policy for the key itself:
  ""        -- same as "DELETE" (provider default)
  "DELETE"  -- the key is deleted on destroy
  "PREVENT" -- destroy FAILS while this key exists

- rule: deletion_policy must be one of: DELETE, PREVENT

### spec.projectIamRoles

`[]string`

IAM roles granted to this service account at the PROJECT scope, e.g.
["roles/logging.logWriter", "roles/storage.admin"]. Grants are additive
(member-level): they never clobber other members' bindings on the same role.
For grants that should be first-class, referenceable nodes in the resource
graph (visible dependencies, independent lifecycle), use GcpProjectIamMember
instead of this list.

### spec.orgId

`string`

The numeric organization ID (e.g. "123456789012") — required only when
org_iam_roles is set, to identify which organization receives the grants.

### spec.orgIamRoles

`[]string`

IAM roles granted to this service account at the ORGANIZATION scope, e.g.
["roles/resourcemanager.organizationViewer"]. Requires org_id. Grants are
additive (member-level). Org-scope grants affect every project under the
organization — grant sparingly.

### spec.createIgnoreAlreadyExists

`bool`

If true, creating the service account succeeds (as a no-op adoption)
when an account with the same email already exists, instead of
failing. Useful for idempotent bootstrap flows that may race other
provisioning paths onto well-known identity names.

### spec.deletionPolicy

`string`

Deletion policy — what happens when this resource is destroyed:
  ""        -- same as "DELETE" (provider default)
  "DELETE"  -- the service account is deleted
  "PREVENT" -- destroy FAILS; a guard rail for identities whose
               deletion would invalidate IAM bindings fleet-wide
Mutable in place.

- rule: deletion_policy must be one of: DELETE, PREVENT

## Validation Rules

- `org_roles_require_org_id`: org_id is required when org_iam_roles is set

## Outputs

Reference an output from another manifest as `valueFrom: {kind: GcpServiceAccount, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.email` | `string` | The service account email: <service_account_id>@<project>.iam.gserviceaccount.com. The most common reference handle — workload configs (GKE, Cloud Run, Cloud Functions, Compute) attach the identity by email. |
| `status.outputs.member` | `string` | The IAM member string for this service account: "serviceAccount:<email>". Feed this directly into IAM grants (GcpProjectIamMember's member field) — it is the exact format GCP IAM policies expect, so no string assembly is needed. |
| `status.outputs.unique_id` | `string` | The stable, unique numeric ID GCP assigns to the service account. Unlike the email, it is never reused if the account is deleted and recreated — use it where a tamper-proof identity reference matters (e.g. audit tooling). |
| `status.outputs.name` | `string` | The fully-qualified resource name: projects/<project>/serviceAccounts/<email>. Used by APIs that address the service account as a resource (key management, IAM policy on the service account itself, Workload Identity bindings). |
| `status.outputs.key_base64` | `string` | Base64-encoded private key, populated only when spec.user_managed_key requested the generate flow (empty for keyless accounts and for the public_key_data upload flow). This is a live, long-lived credential — the engines mark it secret in state; handle it like a password and prefer keyless patterns entirely. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.projectId` | GcpProject | `status.outputs.project_id` |

## Referenced By

Fields on other kinds that can point at this resource:

| Kind | Field | Reads |
|---|---|---|
| GcpArtifactRegistryRepo | `spec.iamMembers[].member` | `status.outputs.member` |
| GcpCloudComposerEnvironment | `spec.nodeConfig.serviceAccount` | `status.outputs.email` |
| GcpCloudFunction | `spec.buildConfig.serviceAccount` | `status.outputs.name` |
| GcpCloudFunction | `spec.serviceConfig.serviceAccountEmail` | `status.outputs.email` |
| GcpCloudFunction | `spec.trigger.eventTrigger.serviceAccountEmail` | `status.outputs.email` |
| GcpCloudRun | `spec.serviceAccount` | `status.outputs.email` |
| GcpCloudRunJob | `spec.template.serviceAccount` | `status.outputs.email` |
| GcpCloudSchedulerJob | `spec.httpTarget.oauthToken.serviceAccountEmail` | `status.outputs.email` |
| GcpCloudSchedulerJob | `spec.httpTarget.oidcToken.serviceAccountEmail` | `status.outputs.email` |
| GcpCloudTasksQueue | `spec.httpTarget.oauthToken.serviceAccountEmail` | `status.outputs.email` |
| GcpCloudTasksQueue | `spec.httpTarget.oidcToken.serviceAccountEmail` | `status.outputs.email` |
| GcpComputeInstance | `spec.serviceAccount.email` | `status.outputs.email` |
| GcpComputeMig | `spec.template.serviceAccount.email` | `status.outputs.email` |
| GcpDataprocCluster | `spec.clusterConfig.gceConfig.serviceAccount` | `status.outputs.email` |
| GcpEventarcMessageBus | `spec.pipelines[].authentication.googleOidc.serviceAccount` | `status.outputs.email` |
| GcpEventarcMessageBus | `spec.pipelines[].authentication.oauthToken.serviceAccount` | `status.outputs.email` |
| GcpEventarcTrigger | `spec.serviceAccount` | `status.outputs.email` |
| GcpGcsBucket | `spec.iamMembers[].member` | `status.outputs.member` |
| GcpGkeCluster | `spec.clusterAutoscaling.autoProvisioningDefaults.serviceAccount` | `status.outputs.email` |
| GcpGkeNodePool | `spec.nodeConfig.serviceAccount` | `status.outputs.email` |
| GcpGkeWorkloadIdentityBinding | `spec.serviceAccountEmail` | `status.outputs.email` |
| GcpKmsKeyIamMember | `spec.member` | `status.outputs.member` |
| GcpPlantonRunner | `spec.serviceAccount` | `status.outputs.email` |
| GcpProjectIamMember | `spec.member` | `status.outputs.member` |
| GcpPubSubSubscription | `spec.pushConfig.oidcToken.serviceAccountEmail` | `status.outputs.email` |
| GcpPubSubSubscription | `spec.bigqueryConfig.serviceAccountEmail` | `status.outputs.email` |
| GcpPubSubSubscription | `spec.cloudStorageConfig.serviceAccountEmail` | `status.outputs.email` |
| GcpPubSubSubscription | `spec.messageTransforms[].aiInference.serviceAccountEmail` | `status.outputs.email` |
| GcpPubSubTopic | `spec.ingestionDataSourceSettings.awsKinesis.gcpServiceAccount` | `status.outputs.email` |
| GcpPubSubTopic | `spec.ingestionDataSourceSettings.awsMsk.gcpServiceAccount` | `status.outputs.email` |
| GcpPubSubTopic | `spec.ingestionDataSourceSettings.azureEventHubs.gcpServiceAccount` | `status.outputs.email` |
| GcpPubSubTopic | `spec.ingestionDataSourceSettings.confluentCloud.gcpServiceAccount` | `status.outputs.email` |
| GcpPubSubTopic | `spec.messageTransforms[].aiInference.serviceAccountEmail` | `status.outputs.email` |
| GcpSecretManagerSecret | `spec.iamMembers[].member` | `status.outputs.member` |
| GcpServiceAccountIamMember | `spec.serviceAccountId` | `status.outputs.name` |
| GcpServiceAccountIamMember | `spec.member` | `status.outputs.member` |
| GcpVertexAiDeployedIndex | `spec.authConfig.allowedIssuers` | `status.outputs.email` |
| GcpVertexAiNotebook | `spec.serviceAccount` | `status.outputs.email` |
| GcpWorkflow | `spec.serviceAccount` | `status.outputs.email` |
| KubernetesCertManager | `spec.workloadIdentity.gke.serviceAccountEmail` | `status.outputs.email` |
| KubernetesExternalDns | `spec.workloadIdentity.gke.serviceAccountEmail` | `status.outputs.email` |
| KubernetesExternalSecretsOperator | `spec.workloadIdentity.gke.serviceAccountEmail` | `status.outputs.email` |
| KubernetesMongodb | `spec.backup.storages[].gcs.credentials.serviceAccountKey` | `status.outputs.key_base64` |
| KubernetesOpenBao | `spec.autoUnseal.gcpKms.workloadIdentityServiceAccount` | `status.outputs.email` |
| KubernetesPostgres | `spec.workloadIdentity.gke.serviceAccountEmail` | `status.outputs.email` |
| KubernetesServiceAccount | `spec.workloadIdentity.gke.serviceAccountEmail` | `status.outputs.email` |
| KubernetesVelero | `spec.backupStorage.gcs.workloadIdentityServiceAccountEmail` | `status.outputs.email` |

## See Also

- [Overview](../README.md)
