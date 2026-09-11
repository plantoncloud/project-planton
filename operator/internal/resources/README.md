# internal/resources

Resource builders, Helm chart rendering, credential generation, and embedded manifests for Kubernetes objects managed by the operator.

## Responsibility

This package generates Kubernetes resource objects for the operator to apply. It owns five concerns:

1. **Helm chart values builders**: Functions that construct Helm values maps for each data layer and supporting service component, rendered at runtime via the Helm SDK.
2. **Embedded chart archives**: `.tgz` Helm chart files embedded in the binary via `//go:embed`, rendered at runtime for air-gap compatibility.
3. **Embedded operator manifests**: Vendored upstream release YAML for sub-operators (CloudNativePG, Tekton Pipelines), applied via Server-Side Apply.
4. **Credential generation**: Secure password generation and Secret construction for operator-managed authentication.
5. **Connection abstraction**: Helpers resolving each data service's connection details behind one seam per service.

## Deployment Pattern (DD-13)

All data layer and supporting service components are deployed via Helm charts rendered at runtime using `RenderHelmChart()`. This is the single rendering pipeline for the entire operator:

```
Embedded .tgz → RenderHelmChart(chartData, releaseName, namespace, values) → []*unstructured.Unstructured → Server-Side Apply
```

## Object Type Strategy

- **Helm chart rendering** for chart-shipped components: Valkey (the redis-protocol cache), OpenFGA, Temporal, OpenBAO, Neo4j -- and for one shared sub-operator, the Barman Cloud backup plugin, whose chart renders into a release beside CloudNativePG rather than into a platform.
- **Unstructured** for third-party CRs the operator declares against sub-operators: the CloudNativePG `postgresql.cnpg.io/v1` Cluster and `ScheduledBackup`, and the backup plugin's `barmancloud.cnpg.io/v1` ObjectStore.
- **Typed Go objects** for operator-generated credential Secrets and bootstrap ConfigMaps.

## Helm Charts Embedded

| Component | Chart | Version | Mode |
|-----------|-------|---------|------|
| Valkey (redis-protocol cache) | bitnamicharts/valkey | 3.0.31 | Always (no mode field) |
| OpenFGA | openfga/openfga | 0.3.13 | Always |
| Temporal | temporal/temporal | 0.62.0 | Always |
| Barman Cloud plugin (CloudNativePG backups; a shared sub-operator, not a platform component) | cnpg/plugin-barman-cloud | 0.7.0 (plugin v0.13.0) | When cert-manager is present or a backup is declared |

## Credential Generation

`GeneratePassword()` produces 32-character URL-safe base64 passwords via `crypto/rand`. `NewCredentialSecret()` wraps them in a K8s Secret with owner references for cascading deletion. Each Helm chart references credentials via `existingSecret` values. The DataLayer phase uses get-or-create semantics: passwords are generated once and never regenerated.

PostgreSQL is the exception by design: CloudNativePG generates and owns the superuser credential Secret (`{cluster}-superuser`), so the credential lives and dies with the volumes it unlocks.

## PostgreSQL (CloudNativePG)

The platform's database is one `postgresql.cnpg.io/v1` Cluster per install, built by `NewPostgreSQLCluster` and reconciled by the CloudNativePG operator (vendored release, installed as a prerequisite via the detect-or-install gate). `spec.database.postgresql.replicas` turns the same cluster into a streaming-replication HA topology with automated failover.

`PostgreSQLConnection()` returns host, port, and credential references for the platform cluster: every consumer (control plane, identity server, OpenFGA, Temporal) connects through the cluster's read-write Service as the superuser -- the single-user, self-provisioned-databases contract shared with the desktop daemon's local instance.

Backups ride the Barman Cloud plugin (`postgresql_backup.go`, `barman_plugin_helm.go`): `NewObjectStore` renders where the backups go and how the database's pods authenticate (S3, GCS, Azure Blob, or R2; keyless via the Cluster's `serviceAccountTemplate` annotations, or an adopter-owned Secret referenced by name), `NewObjectStoreSettingsSecret` the non-secret values the plugin insists on reading from a Secret, `NewScheduledBackup` the base-backup schedule (always with an immediate first backup), and `NewPostgreSQLCluster` wires the `plugins` entry under a server name that carries the platform's UID so no two installs ever share an archive. A recovery replaces `initdb` with `bootstrap.recovery` from an `externalClusters` entry addressed through the plugin. Every builder is a pure function over plain options so the maps are pinned by tests; the component decides when each is applied.

## Service Host Helpers

Each component exports a `*ServiceHost(crName, namespace)` function returning the in-cluster FQDN. These are used by later phases to wire connection strings into application environment variables.
