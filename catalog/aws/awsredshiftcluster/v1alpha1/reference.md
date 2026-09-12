# AwsRedshiftCluster

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `aws.planton.dev/v1alpha1`

AwsRedshiftClusterSpec defines an Amazon Redshift provisioned cluster --
a petabyte-scale columnar data warehouse for analytical (OLAP) SQL
workloads over structured and semi-structured data.

The cluster identifier comes from metadata.name. Subnets, security
groups, IAM roles, KMS keys, and Elastic IPs compose by reference --
this cluster never creates or mutates resources that deserve to be
their own nodes. Ingress rules belong on the referenced
AwsSecurityGroup nodes, never inside this cluster.

Audit logging and cross-region snapshot copy are cluster settings with
no identity of their own (AWS keys both by the cluster identifier), so
they are folded in as sub-messages rather than modeled as standalone
kinds. Redshift Serverless is a different product (namespaces +
workgroups, no cluster) and is deliberately not modeled here.

## Example

```yaml
apiVersion: aws.planton.dev/v1alpha1
kind: AwsRedshiftCluster
metadata:
  name: awsredshiftcluster-demo
spec:
  region: us-west-2
  subnetIds:
    - value: subnet-0a1b2c3d4e5f60001
    - value: subnet-0a1b2c3d4e5f60002
  nodeType: ra3.large
  masterUsername: hackadmin
  manageMasterPassword: true
  skipFinalSnapshot: true
  # Governance: cap what individual features may spend, pause the
  # warehouse outside business hours, and cap Spectrum scans.
  usageLimits:
    - featureType: spectrum
      limitType: data-scanned
      amount: 5
    - featureType: concurrency-scaling
      limitType: time
      amount: 60
      period: daily
      breachAction: disable
  scheduledActions:
    - name: awsredshiftcluster-demo-nightly-pause
      schedule: cron(0 4 * * ? *)
      iamRoleArn:
        value: arn:aws:iam::123456789012:role/redshift-scheduler
      pauseCluster: true
    - name: awsredshiftcluster-demo-morning-resume
      schedule: cron(0 13 * * ? *)
      iamRoleArn:
        value: arn:aws:iam::123456789012:role/redshift-scheduler
      resumeCluster: true
  # Cross-VPC access: a managed endpoint in the cluster's own subnet
  # group, plus a grant letting another account create endpoints.
  endpointAccesses:
    - endpointName: analytics-consumers
  endpointAuthorizations:
    - account: "210987654321"
  # Replace the default snapshot cadence with an existing account-level
  # schedule.
  snapshotScheduleIdentifier: every-12h
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.region` | `string` | yes |  |  |
| `spec.subnetIds` | `[]string \| valueFrom` |  |  | AwsSubnet (`status.outputs.subnet_id`) |
| `spec.clusterSubnetGroupName` | `string \| valueFrom` |  |  |  |
| `spec.securityGroupIds` | `[]string \| valueFrom` |  |  | AwsSecurityGroup (`status.outputs.security_group_id`) |
| `spec.availabilityZone` | `string` |  |  |  |
| `spec.availabilityZoneRelocationEnabled` | `bool` |  |  |  |
| `spec.publiclyAccessible` | `bool` |  |  |  |
| `spec.elasticIp` | `string \| valueFrom` |  |  | AwsElasticIp (`status.outputs.public_ip`) |
| `spec.enhancedVpcRouting` | `bool` |  |  |  |
| `spec.port` | `int32` |  |  |  |
| `spec.nodeType` | `string` | yes |  |  |
| `spec.numberOfNodes` | `int32` |  |  |  |
| `spec.clusterVersion` | `string` |  |  |  |
| `spec.databaseName` | `string` |  |  |  |
| `spec.masterUsername` | `string` |  |  |  |
| `spec.manageMasterPassword` | `bool` |  | `true` |  |
| `spec.masterPassword` | `string` (sensitive) |  |  |  |
| `spec.masterPasswordSecretKmsKeyId` | `string \| valueFrom` |  |  | AwsKmsKey (`status.outputs.key_arn`) |
| `spec.encrypted` | `bool` |  | `true` |  |
| `spec.kmsKeyId` | `string \| valueFrom` |  |  | AwsKmsKey (`status.outputs.key_arn`) |
| `spec.multiAz` | `bool` |  |  |  |
| `spec.iamRoles` | `[]string \| valueFrom` |  |  | AwsIamRole (`status.outputs.role_arn`) |
| `spec.defaultIamRoleArn` | `string \| valueFrom` |  |  | AwsIamRole (`status.outputs.role_arn`) |
| `spec.automatedSnapshotRetentionPeriod` | `int32` |  | `1` |  |
| `spec.manualSnapshotRetentionPeriod` | `int32` |  |  |  |
| `spec.preferredMaintenanceWindow` | `string` |  |  |  |
| `spec.maintenanceTrackName` | `string` |  |  |  |
| `spec.allowVersionUpgrade` | `bool` |  | `true` |  |
| `spec.applyImmediately` | `bool` |  |  |  |
| `spec.skipFinalSnapshot` | `bool` |  |  |  |
| `spec.finalSnapshotIdentifier` | `string` |  |  |  |
| `spec.snapshotIdentifier` | `string` |  |  |  |
| `spec.snapshotArn` | `string` |  |  |  |
| `spec.snapshotClusterIdentifier` | `string` |  |  |  |
| `spec.ownerAccount` | `string` |  |  |  |
| `spec.logging` | `AwsRedshiftClusterLogging` |  |  |  |
| `spec.logging.logDestinationType` | `string` | yes |  |  |
| `spec.logging.s3BucketName` | `string` |  |  |  |
| `spec.logging.s3KeyPrefix` | `string` |  |  |  |
| `spec.logging.logExports` | `[]string` |  |  |  |
| `spec.snapshotCopy` | `AwsRedshiftClusterSnapshotCopy` |  |  |  |
| `spec.snapshotCopy.destinationRegion` | `string` | yes |  |  |
| `spec.snapshotCopy.retentionPeriod` | `int32` |  |  |  |
| `spec.snapshotCopy.manualSnapshotRetentionPeriod` | `int32` |  |  |  |
| `spec.snapshotCopy.snapshotCopyGrantName` | `string` |  |  |  |
| `spec.clusterParameterGroupName` | `string` |  |  |  |
| `spec.parameters` | `[]AwsRedshiftClusterParameter` |  |  |  |
| `spec.parameters[].name` | `string` | yes |  |  |
| `spec.parameters[].value` | `string` | yes |  |  |
| `spec.parameterGroupFamily` | `string` |  |  |  |
| `spec.snapshotScheduleIdentifier` | `string` |  |  |  |
| `spec.usageLimits` | `[]AwsRedshiftClusterUsageLimit` |  |  |  |
| `spec.usageLimits[].featureType` | `string` | yes |  |  |
| `spec.usageLimits[].limitType` | `string` | yes |  |  |
| `spec.usageLimits[].amount` | `int64` |  |  |  |
| `spec.usageLimits[].period` | `string` |  |  |  |
| `spec.usageLimits[].breachAction` | `string` |  |  |  |
| `spec.scheduledActions` | `[]AwsRedshiftClusterScheduledAction` |  |  |  |
| `spec.scheduledActions[].name` | `string` | yes |  |  |
| `spec.scheduledActions[].description` | `string` |  |  |  |
| `spec.scheduledActions[].disabled` | `bool` |  |  |  |
| `spec.scheduledActions[].schedule` | `string` | yes |  |  |
| `spec.scheduledActions[].startTime` | `string` |  |  |  |
| `spec.scheduledActions[].endTime` | `string` |  |  |  |
| `spec.scheduledActions[].iamRoleArn` | `string \| valueFrom` | yes |  | AwsIamRole (`status.outputs.role_arn`) |
| `spec.scheduledActions[].pauseCluster` | `bool` |  |  |  |
| `spec.scheduledActions[].resumeCluster` | `bool` |  |  |  |
| `spec.scheduledActions[].resizeCluster` | `AwsRedshiftClusterResizeAction` |  |  |  |
| `spec.scheduledActions[].resizeCluster.classic` | `bool` |  |  |  |
| `spec.scheduledActions[].resizeCluster.clusterType` | `string` |  |  |  |
| `spec.scheduledActions[].resizeCluster.nodeType` | `string` |  |  |  |
| `spec.scheduledActions[].resizeCluster.numberOfNodes` | `int32` |  |  |  |
| `spec.endpointAccesses` | `[]AwsRedshiftClusterEndpointAccess` |  |  |  |
| `spec.endpointAccesses[].endpointName` | `string` | yes |  |  |
| `spec.endpointAccesses[].subnetGroupName` | `string` |  |  |  |
| `spec.endpointAccesses[].vpcSecurityGroupIds` | `[]string \| valueFrom` |  |  | AwsSecurityGroup (`status.outputs.security_group_id`) |
| `spec.endpointAuthorizations` | `[]AwsRedshiftClusterEndpointAuthorization` |  |  |  |
| `spec.endpointAuthorizations[].account` | `string` | yes |  |  |
| `spec.endpointAuthorizations[].vpcIds` | `[]string \| valueFrom` |  |  | AwsVpc (`status.outputs.vpc_id`) |
| `spec.endpointAuthorizations[].forceDelete` | `bool` |  |  |  |

## Field Details

### spec.region

`string` · required

The AWS region the cluster is created in. Must match the region of
the subnets, security groups, and KMS keys it references.
Example: "us-west-2", "eu-west-1".

- rule: {"string":{"minLen":"1"}}

### spec.subnetIds

`[]string | valueFrom`

Subnets for the cluster's Redshift subnet group. Provide at least
two subnets in DISTINCT availability zones so the cluster (and a
Multi-AZ standby, if enabled) has somewhere to land. Reference
AwsSubnet subnet_id outputs or pass literal subnet IDs. The module
manages the subnet group itself (pure glue: a named list of
subnets); alternatively point cluster_subnet_group_name at an
existing group. Changing the subnet group replaces the cluster.

- references: AwsSubnet (`status.outputs.subnet_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsSubnet, name: <that resource's name>, fieldPath: status.outputs.subnet_id}} -- a bare string does not parse

### spec.clusterSubnetGroupName

`string | valueFrom`

Name of an existing Redshift subnet group to place the cluster in,
instead of providing subnet_ids. Changing the subnet group replaces
the cluster.

- rule: write as {value: <literal>} or {valueFrom: {kind: <Kind>, name: <that resource's name>, fieldPath: status.outputs.<output>}} -- a bare string does not parse

### spec.securityGroupIds

`[]string | valueFrom`

Security groups attached to the cluster (the cluster's
vpc_security_group_ids). Empty uses the VPC's default security
group (the AWS default). Reference AwsSecurityGroup
security_group_id outputs or pass literal SG IDs -- warehouse
ingress rules (e.g. port 5439 from your BI tooling) belong on the
referenced AwsSecurityGroup node, never inside this cluster.

- references: AwsSecurityGroup (`status.outputs.security_group_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsSecurityGroup, name: <that resource's name>, fieldPath: status.outputs.security_group_id}} -- a bare string does not parse

### spec.availabilityZone

`string`

Pin the cluster to one availability zone. Empty lets AWS place it
-- preferred. Changing the zone on a live cluster requires
availability_zone_relocation_enabled; without relocation the pin is
create-time only.

### spec.availabilityZoneRelocationEnabled

`bool`

Allow the cluster to be relocated to another availability zone
during outages or on demand -- zero data loss, brief
unavailability. Requires RA3 node types and a cluster port in the
ranges 5431-5455 or 8191-8215 (an AWS relocation constraint).
Mutually exclusive with multi_az: relocation recovers by moving the
single cluster, Multi-AZ recovers by failing over to a standby.

### spec.publiclyAccessible

`bool`

Give the cluster a public IP so it can be reached from outside the
VPC. Off by default -- warehouses almost always stay private behind
VPC routing (and Query Editor / private BI reach them fine).

### spec.elasticIp

`string | valueFrom`

A static public IPv4 address for the cluster's leader node.
Requires publicly_accessible. Reference an AwsElasticIp public_ip
output or pass a literal Elastic IP ADDRESS (Redshift takes the IP
itself, not an allocation ID).

- references: AwsElasticIp (`status.outputs.public_ip`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsElasticIp, name: <that resource's name>, fieldPath: status.outputs.public_ip}} -- a bare string does not parse

### spec.enhancedVpcRouting

`bool`

Force all COPY and UNLOAD traffic between the cluster and data
repositories (S3, DynamoDB, ...) through the VPC instead of the
public internet -- enabling VPC flow logs, endpoints, and other
network controls to see and govern warehouse data movement.

### spec.port

`int32`

The port the cluster accepts connections on. 0 keeps the AWS
default (5439). Redshift accepts 1115-65535; if
availability_zone_relocation_enabled is on, AWS additionally
requires the port to be within 5431-5455 or 8191-8215.

### spec.nodeType

`string` · required

The compute/storage class of every node. RA3 classes (ra3.large,
ra3.xlplus, ra3.4xlarge, ra3.16xlarge) decouple compute from
managed storage that tiers between SSD and S3 automatically -- the
right call for nearly all new clusters and the only family that
supports multi_az and availability-zone relocation. DC2 classes
(dc2.large, dc2.8xlarge) are the legacy dense-compute family with
node-local SSD only. Resizing to a different class is an in-place
(but access-interrupting) classic/elastic resize, never a replace.

- rule: {"required":true}

### spec.numberOfNodes

`int32`

How many nodes the cluster runs. 0 keeps the AWS default (1, a
single-node cluster where leader and compute share one node).
2+ creates a multi-node cluster with a dedicated leader --
required for production and for multi_az. Resize is in-place.

### spec.clusterVersion

`string`

The Redshift engine version. Empty keeps the AWS default ("1.0" --
the only version family Redshift has ever shipped); actual engine
patches ride maintenance_track_name and allow_version_upgrade.

### spec.databaseName

`string`

The name of the first database created in the cluster. Empty keeps
the AWS default ("dev"). 1-64 characters: lowercase alphanumeric,
underscore, or dollar sign, starting with a letter or underscore
(AWS's CreateCluster contract).

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"pattern":"^[a-z_][0-9a-z_$]{0,63}$"}}

### spec.masterUsername

`string`

The admin username. Required for a brand-new cluster -- AWS has no
default and rejects a blank value at CreateCluster. 1-128
characters starting with a letter; letters, digits, and _.@+- are
legal. Only clusters restored from a snapshot leave it empty and
inherit the source's credentials. Create-time only -- changing it
replaces the cluster.

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"pattern":"^[A-Za-z][0-9A-Za-z_.@+-]{0,127}$"}}

### spec.manageMasterPassword

`bool`

Let AWS manage the admin password in Secrets Manager: AWS
generates it, stores it, rotates it on schedule, and no secret ever
touches this manifest or the IaC state. The managed secret's ARN is
exported as the master_password_secret_arn output. Mutually
exclusive with master_password -- and the recommended posture.

- default: `true`

### spec.masterPassword

`string` · sensitive

The admin password, supplied directly (8-64 chars with at least one
uppercase letter, one lowercase letter, and one digit). Stored in
IaC state -- prefer manage_master_password, which keeps the secret
in Secrets Manager entirely. Mutually exclusive with
manage_master_password.

### spec.masterPasswordSecretKmsKeyId

`string | valueFrom`

The KMS key that encrypts the Secrets Manager secret holding the
managed admin password. Empty uses the AWS-managed
aws/secretsmanager key. Only meaningful with
manage_master_password. Reference an AwsKmsKey key_arn output or
pass a literal key ARN.

- references: AwsKmsKey (`status.outputs.key_arn`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsKmsKey, name: <that resource's name>, fieldPath: status.outputs.key_arn}} -- a bare string does not parse

### spec.encrypted

`bool` · optional (explicit presence)

Encrypt cluster storage at rest. AWS defaults new clusters to
encrypted, and this spec keeps that default -- set false only for
a deliberate, exceptional reason. Toggling encryption on a live
cluster is an in-place but long-running migration.

- default: `true`

### spec.kmsKeyId

`string | valueFrom`

The KMS key for cluster storage encryption when encrypted is true.
Empty uses the AWS-managed Redshift service key. Reference an
AwsKmsKey key_arn output or pass a literal key ARN.

- references: AwsKmsKey (`status.outputs.key_arn`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsKmsKey, name: <that resource's name>, fieldPath: status.outputs.key_arn}} -- a bare string does not parse

### spec.multiAz

`bool`

Run a Multi-AZ deployment: compute in two availability zones with
automatic failover and a single endpoint. Requires RA3 node types
and a multi-node cluster. Mutually exclusive with
availability_zone_relocation_enabled.

### spec.iamRoles

`[]string | valueFrom`

IAM roles the cluster assumes to access other AWS services during
COPY, UNLOAD, CREATE EXTERNAL FUNCTION, and Redshift Spectrum
queries (S3, DynamoDB, Glue, Lambda, ...). AWS allows up to 10
associated roles. Reference AwsIamRole role_arn outputs or pass
literal role ARNs.

- references: AwsIamRole (`status.outputs.role_arn`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsIamRole, name: <that resource's name>, fieldPath: status.outputs.role_arn}} -- a bare string does not parse

### spec.defaultIamRoleArn

`string | valueFrom`

The IAM role assumed when a SQL command does not name one
explicitly (e.g. COPY ... IAM_ROLE default). Must also be present
in iam_roles. Reference an AwsIamRole role_arn output or pass a
literal role ARN.

- references: AwsIamRole (`status.outputs.role_arn`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsIamRole, name: <that resource's name>, fieldPath: status.outputs.role_arn}} -- a bare string does not parse

### spec.automatedSnapshotRetentionPeriod

`int32` · optional (explicit presence)

Days automated snapshots are retained, 0-35. 0 disables automated
snapshots entirely (not recommended); unset keeps the AWS default
(1 day). Production warehouses typically want 7+.

- default: `1`
- rule: {"int32":{"lte":35,"gte":0}}

### spec.manualSnapshotRetentionPeriod

`int32`

Days MANUAL snapshots are retained: 1-3653, or -1 to retain
indefinitely. 0 keeps the AWS default (-1, indefinite). Applies to
new manual snapshots taken after the change.

### spec.preferredMaintenanceWindow

`string`

The weekly maintenance window in UTC, format
"ddd:hh24:mi-ddd:hh24:mi" (e.g. "sat:03:00-sat:04:00"). Empty lets
AWS assign one.

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"pattern":"^(mon|tue|wed|thu|fri|sat|sun):([01][0-9]|2[0-3]):[0-5][0-9]-(mon|tue|wed|thu|fri|sat|sun):([01][0-9]|2[0-3]):[0-5][0-9]$"}}

### spec.maintenanceTrackName

`string`

The maintenance track the cluster follows. Empty keeps the AWS
default ("current" -- the latest approved release). "trailing"
stays one release behind; a snapshot restore can also land on a
named preview/source track inherited from its source cluster.

### spec.allowVersionUpgrade

`bool` · optional (explicit presence)

Permit engine version upgrades during the maintenance window. AWS
defaults this to true; disable only when upgrade timing must be
controlled manually.

- default: `true`

### spec.applyImmediately

`bool`

Apply modifications immediately instead of waiting for the next
maintenance window. Immediate changes can interrupt connections;
deferred changes wait quietly. AWS defaults to deferred.

### spec.skipFinalSnapshot

`bool`

Skip the final snapshot when the cluster is deleted. When false
(the safe default), final_snapshot_identifier must be set -- AWS
refuses to delete without knowing the snapshot name.

### spec.finalSnapshotIdentifier

`string`

The name for the final snapshot taken on deletion. Required when
skip_final_snapshot is false.

### spec.snapshotIdentifier

`string`

Restore the cluster from an existing snapshot by NAME at create
time. Create-time only; mutually exclusive with snapshot_arn. The
restored cluster inherits the snapshot's credentials, so
master_username stays empty.

### spec.snapshotArn

`string`

Restore the cluster from an existing snapshot by ARN at create
time -- the shape cross-account/cross-region snapshot shares use.
Create-time only; mutually exclusive with snapshot_identifier.

### spec.snapshotClusterIdentifier

`string`

The name of the cluster the source snapshot was taken from.
Required by AWS only when the snapshot name alone is ambiguous
(shared snapshots). Only meaningful alongside snapshot_identifier.

### spec.ownerAccount

`string`

The AWS account that owns the source snapshot, for restoring from
a snapshot shared by another account. Only meaningful alongside a
restore source.

### spec.logging

`AwsRedshiftClusterLogging`

Audit logging for the cluster -- connection attempts, user
activity, and user changes delivered to S3 or CloudWatch Logs. A
cluster setting keyed by the cluster itself (folded, never a
standalone node).

- rule: s3_bucket_name is required when log_destination_type is 's3'
- rule: log_exports must have at least one entry when log_destination_type is 'cloudwatch'

### spec.logging.logDestinationType

`string` · required

Where audit logs are delivered: "s3" writes log files to an S3
bucket (the bucket needs a policy granting the Redshift service
write access); "cloudwatch" streams them to CloudWatch Logs --
the modern destination with retention, metric filters, and
subscriptions.

- rule: {"required":true,"string":{"in":["s3","cloudwatch"]}}

### spec.logging.s3BucketName

`string`

The S3 bucket audit logs are written to. Required when
log_destination_type is "s3".

### spec.logging.s3KeyPrefix

`string`

An optional key prefix for log objects within the S3 bucket.

### spec.logging.logExports

`[]string`

Which audit log types to export: "connectionlog" (connection
attempts), "useractivitylog" (every executed query -- requires the
enable_user_activity_logging cluster parameter), "userlog" (user
create/alter/drop events). Required for "cloudwatch"; ignored for
"s3" (S3 delivery always carries all three).

- rule: {"repeated":{"unique":true,"items":{"string":{"in":["connectionlog","useractivitylog","userlog"]}}}}

### spec.snapshotCopy

`AwsRedshiftClusterSnapshotCopy`

Cross-region disaster recovery: automatically copy this cluster's
snapshots to another region. A cluster setting keyed by the
cluster itself (folded, never a standalone node).

- rule: retention_period must be between 1 and 35 when set -- 0 keeps the AWS default (7)
- rule: manual_snapshot_retention_period must be -1 (indefinite) or between 1 and 3653 when set -- 0 keeps the AWS default (indefinite)

### spec.snapshotCopy.destinationRegion

`string` · required

The region snapshots are copied to. Must differ from the cluster's
own region. Required.

- rule: {"required":true}

### spec.snapshotCopy.retentionPeriod

`int32`

Days copied AUTOMATED snapshots are retained in the destination
region, 1-35. 0 keeps the AWS default (7).

### spec.snapshotCopy.manualSnapshotRetentionPeriod

`int32`

Days copied MANUAL snapshots are retained in the destination
region: 1-3653, or -1 to retain indefinitely. 0 keeps the AWS
default (-1, indefinite).

### spec.snapshotCopy.snapshotCopyGrantName

`string`

The snapshot copy grant that lets Redshift encrypt copied
snapshots with a KMS key in the destination region. Required by
AWS when the cluster is KMS-encrypted; irrelevant otherwise.

### spec.clusterParameterGroupName

`string`

The name of an existing cluster parameter group to use. Mutually
exclusive with `parameters` -- either bring your own group or let
the module manage one from inline parameters.

### spec.parameters

`[]AwsRedshiftClusterParameter`

Cluster-level engine parameters (e.g. "require_ssl",
"enable_user_activity_logging", "wlm_json_configuration"), managed
as a dedicated parameter group owned by this cluster (the group is
glue -- a named parameter list -- so it stays folded). Mutually
exclusive with cluster_parameter_group_name.

### spec.parameters[].name

`string` · required

The parameter name (e.g. "require_ssl",
"enable_user_activity_logging", "max_concurrency_scaling_clusters",
"wlm_json_configuration"). Required.

- rule: {"required":true}

### spec.parameters[].value

`string` · required

The parameter value. Required.

- rule: {"required":true}

### spec.parameterGroupFamily

`string`

The parameter-group family for the managed group created from
`parameters`. Empty keeps "redshift-1.0" -- the long-standing
family AWS accepts on every cluster. AWS introduced "redshift-2.0"
with the Redshift patch 2.0 generation (new clusters' default
group is default.redshift-2.0); set it here when the managed group
should track that family. Only meaningful alongside `parameters`.

### spec.snapshotScheduleIdentifier

`string`

The identifier of an existing snapshot schedule to associate with
this cluster (AWS allows exactly ONE schedule per cluster -- the
schedule replaces the default automated-snapshot cadence with
explicit cron/rate definitions). The schedule itself is an
account-scoped resource shared by many clusters and is not created
here. Empty keeps AWS's default snapshot cadence.

### spec.usageLimits

`[]AwsRedshiftClusterUsageLimit`

Usage limits capping what individual Redshift features may consume
on this cluster -- Spectrum data scanned, concurrency-scaling time,
cross-region datasharing transfer -- each with a breach action from
logging to hard disable. Cluster-scoped settings keyed by the
cluster itself (folded, never standalone nodes).

- rule: limit_type must be 'data-scanned' for spectrum/cross-region-datasharing and 'time' for concurrency-scaling/extra-compute-for-automatic-optimization (AWS's CreateUsageLimit contract)

### spec.usageLimits[].featureType

`string` · required

The Redshift feature the limit applies to: "spectrum" (external S3
scans), "concurrency-scaling" (burst clusters),
"cross-region-datasharing" (data transferred to consumers in other
regions), or "extra-compute-for-automatic-optimization". Required;
changing it replaces the limit.

- rule: {"required":true,"string":{"in":["spectrum","concurrency-scaling","cross-region-datasharing","extra-compute-for-automatic-optimization"]}}

### spec.usageLimits[].limitType

`string` · required

How the limit is measured: "time" (minutes of feature usage) or
"data-scanned" (terabytes). AWS's contract pairs spectrum and
cross-region-datasharing with "data-scanned", concurrency-scaling
and extra-compute-for-automatic-optimization with "time"
(CEL-enforced). Required; changing it replaces the limit.

- rule: {"required":true,"string":{"in":["time","data-scanned"]}}

### spec.usageLimits[].amount

`int64`

The limit amount: minutes when limit_type is "time", terabytes
when it is "data-scanned". Must be positive.

- rule: {"int64":{"gt":"0"}}

### spec.usageLimits[].period

`string`

The period the amount applies to: "daily", "weekly", or "monthly".
Empty keeps the AWS default (monthly). A weekly period begins on
Sunday. Changing it replaces the limit.

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"in":["daily","weekly","monthly"]}}

### spec.usageLimits[].breachAction

`string`

What Redshift does when the limit is breached: "log" writes an
event to the system table, "emit-metric" additionally publishes a
CloudWatch metric, "disable" turns the feature off until the
period resets (spectrum and concurrency-scaling only -- AWS
rejects disable for cross-region datasharing). Empty keeps the AWS
default (log).

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"in":["log","emit-metric","disable"]}}

### spec.scheduledActions

`[]AwsRedshiftClusterScheduledAction`

Scheduled actions that pause, resume, or resize this cluster on a
cron/at schedule -- the standard nights-and-weekends cost lever for
non-production warehouses. Each entry keys one scheduled action
targeting this cluster.

- rule: exactly one of pause_cluster, resume_cluster, or resize_cluster must be set
- rule: start_time must be an RFC 3339 timestamp (e.g., '2026-09-01T00:00:00Z') when set
- rule: end_time must be an RFC 3339 timestamp (e.g., '2026-09-30T00:00:00Z') when set

### spec.scheduledActions[].name

`string` · required

The scheduled action's name: 1-63 lowercase alphanumeric/hyphen
characters. NOTE: names are unique per AWS ACCOUNT (not per
cluster) -- include something cluster-specific to avoid collisions
across clusters. Changing it replaces the action.

- rule: {"required":true,"string":{"pattern":"^[0-9a-z-]{1,63}$"}}

### spec.scheduledActions[].description

`string`

An optional description of what the action does and why.

### spec.scheduledActions[].disabled

`bool`

Suspend the action without deleting it. The zero value (false)
keeps the AWS default: the action is enabled and fires on
schedule.

### spec.scheduledActions[].schedule

`string` · required

When the action fires, in at() or cron() format -- e.g.
"cron(0 22 * * ? *)" (daily 22:00 UTC) or
"at(2026-09-01T03:00:00)". Cron fields are
Minutes Hours Day-of-month Month Day-of-week Year, in UTC.

- rule: {"required":true,"string":{"pattern":"^(at|cron)\\(.+\\)$"}}

### spec.scheduledActions[].startTime

`string`

When the schedule becomes active (RFC 3339, e.g.
"2026-09-01T00:00:00Z"). Empty activates it immediately.

### spec.scheduledActions[].endTime

`string`

When the schedule expires (RFC 3339). Empty keeps it active until
deleted.

### spec.scheduledActions[].iamRoleArn

`string | valueFrom` · required

The IAM role Redshift assumes to perform the action. The role's
TRUST POLICY must allow the "scheduler.redshift.amazonaws.com"
service principal to sts:AssumeRole (AWS validates the trust at
create -- a role trusting only redshift.amazonaws.com or ec2 is
rejected; the provider retries briefly on trust-propagation
delays). Reference an AwsIamRole role_arn output or pass a literal
role ARN.

- references: AwsIamRole (`status.outputs.role_arn`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsIamRole, name: <that resource's name>, fieldPath: status.outputs.role_arn}} -- a bare string does not parse

### spec.scheduledActions[].pauseCluster

`bool`

Pause the cluster (compute stops billing; storage persists).
Exactly one action arm must be set (CEL-enforced).

### spec.scheduledActions[].resumeCluster

`bool`

Resume a paused cluster. Exactly one action arm must be set
(CEL-enforced).

### spec.scheduledActions[].resizeCluster

`AwsRedshiftClusterResizeAction`

Resize the cluster to a new node count/type. Exactly one action
arm must be set (CEL-enforced).

### spec.scheduledActions[].resizeCluster.classic

`bool`

Force a CLASSIC resize (full data redistribution -- hours, but
works between any topologies) instead of the default elastic
resize (minutes, but constrained to compatible node counts).

### spec.scheduledActions[].resizeCluster.clusterType

`string`

The target cluster type. Empty keeps the current type; AWS derives
it from the node count otherwise.

- rule: {"ignore":"IGNORE_IF_ZERO_VALUE","string":{"in":["single-node","multi-node"]}}

### spec.scheduledActions[].resizeCluster.nodeType

`string`

The target node class (e.g. "ra3.large"). Empty keeps the current
class.

### spec.scheduledActions[].resizeCluster.numberOfNodes

`int32`

The target node count. 0 keeps the current count.

- rule: {"int32":{"gte":0}}

### spec.endpointAccesses

`[]AwsRedshiftClusterEndpointAccess`

Redshift-managed VPC endpoints that expose this cluster inside
another subnet group (same-account cross-VPC access without
peering; requires RA3 node types). Each entry keys one managed
endpoint; its private address and port are exported per endpoint on
the outputs contract.

### spec.endpointAccesses[].endpointName

`string` · required

The endpoint's name: 1-30 lowercase alphanumeric/hyphen
characters, unique within the cluster. Changing it replaces the
endpoint.

- rule: {"required":true,"string":{"pattern":"^[0-9a-z-]{1,30}$"}}

### spec.endpointAccesses[].subnetGroupName

`string`

The Redshift subnet group the endpoint lands in -- typically a
group in the CONSUMING VPC. Empty reuses the cluster's own subnet
group (module-managed or referenced), which yields an extra
endpoint in the cluster's own VPC. Changing it replaces the
endpoint.

### spec.endpointAccesses[].vpcSecurityGroupIds

`[]string | valueFrom`

Security groups attached to the endpoint's network interfaces.
Empty uses the VPC's default security group. Reference
AwsSecurityGroup security_group_id outputs or pass literal SG IDs.

- references: AwsSecurityGroup (`status.outputs.security_group_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsSecurityGroup, name: <that resource's name>, fieldPath: status.outputs.security_group_id}} -- a bare string does not parse

### spec.endpointAuthorizations

`[]AwsRedshiftClusterEndpointAuthorization`

Grants that authorize OTHER AWS accounts to create managed VPC
endpoints to this cluster (the grantor side of cross-account
access; the grantee creates its endpoint in its own account). Each
entry keys one per-account authorization.

### spec.endpointAuthorizations[].account

`string` · required

The AWS account ID being authorized (12 digits). Changing it
replaces the authorization.

- rule: {"required":true,"string":{"pattern":"^[0-9]{12}$"}}

### spec.endpointAuthorizations[].vpcIds

`[]string | valueFrom`

Restrict the grant to specific VPCs in the grantee account. Empty
authorizes ALL of the account's VPCs. Reference AwsVpc vpc_id
outputs or pass literal VPC IDs.

Containment-exempt: a grant admits another VPC's endpoints to the
cluster; the cluster never lives inside the VPC it authorizes.

- references: AwsVpc (`status.outputs.vpc_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsVpc, name: <that resource's name>, fieldPath: status.outputs.vpc_id}} -- a bare string does not parse

### spec.endpointAuthorizations[].forceDelete

`bool`

Revoke the authorization on delete even if the grantee still has
live endpoints against the cluster (their endpoints are deleted
too). Off by default: delete fails while grantee endpoints exist.

## Validation Rules

- `subnets_or_group`: provide at least two subnet_ids (distinct AZs) or an existing cluster_subnet_group_name
- `port_range`: port must be between 1115 and 65535 when set -- 0 keeps the AWS default (5439)
- `number_of_nodes_range`: number_of_nodes must be between 1 and 128 when set -- 0 keeps the AWS default (1, single-node)
- `password_xor_managed`: master_password cannot be set when manage_master_password is true -- pick one password strategy
- `master_password_complexity`: master_password must be 8-64 characters with at least one lowercase letter, one uppercase letter, and one digit, and must not contain /, @, quotes, or spaces
- `secret_kms_requires_managed`: master_password_secret_kms_key_id is only meaningful with manage_master_password -- it encrypts the AWS-managed secret
- `master_username_required_unless_derived`: master_username is required for a new cluster -- AWS rejects a blank username; only snapshot restores inherit credentials from their source
- `relocation_xor_multi_az`: availability_zone_relocation_enabled and multi_az are mutually exclusive -- relocation moves the single cluster between zones, Multi-AZ fails over to a standby
- `elastic_ip_requires_public`: elastic_ip requires publicly_accessible -- a private cluster has no public IP to bind the address to
- `manual_snapshot_retention_range`: manual_snapshot_retention_period must be -1 (indefinite) or between 1 and 3653 when set -- 0 keeps the AWS default (indefinite)
- `final_snapshot_id_required_when_not_skipping`: final_snapshot_identifier is required when skip_final_snapshot is false -- AWS refuses to delete the cluster without a final snapshot name
- `final_snapshot_identifier_format`: final_snapshot_identifier must be 1-255 alphanumeric/hyphen characters with no consecutive hyphens and no trailing hyphen
- `snapshot_id_xor_arn`: snapshot_identifier and snapshot_arn are mutually exclusive create-time restore sources
- `snapshot_cluster_requires_snapshot_id`: snapshot_cluster_identifier is only meaningful alongside snapshot_identifier -- it disambiguates a snapshot NAME, never an ARN
- `owner_account_requires_restore`: owner_account is only meaningful alongside a restore source (snapshot_identifier or snapshot_arn)
- `own_parameters_xor_existing_group`: parameters and cluster_parameter_group_name are mutually exclusive -- manage parameters here or bring an existing group
- `family_requires_parameters`: parameter_group_family is only meaningful alongside inline parameters -- an existing group carries its own family
- `usage_limits_unique`: usage_limits must be unique per (feature_type, limit_type, period) -- AWS allows one limit per feature/type/period combination, and an empty period means monthly
- `scheduled_action_names_unique`: scheduled action names must be unique
- `endpoint_access_names_unique`: endpoint access names must be unique
- `endpoint_authorization_accounts_unique`: endpoint authorization grantee accounts must be unique -- AWS keeps one authorization per account

## Outputs

Reference an output from another manifest as `valueFrom: {kind: AwsRedshiftCluster, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.cluster_identifier` | `string` | The cluster identifier (e.g. "analytics-warehouse"). |
| `status.outputs.cluster_arn` | `string` | The Amazon Resource Name of the cluster, for IAM policies and cross-service references. |
| `status.outputs.cluster_namespace_arn` | `string` | The namespace ARN of the cluster, used by Redshift data sharing and the Redshift Data API. |
| `status.outputs.endpoint` | `string` | The connection endpoint in "address:port" form, for SQL client connection strings. |
| `status.outputs.dns_name` | `string` | The DNS hostname of the cluster's leader node (without port), for building connection strings and DNS alias records. |
| `status.outputs.database_name` | `string` | The name of the first database in the cluster. Populated only when the spec set database_name: with it omitted, AWS creates its documented default initial database ("dev") but DescribeClusters echoes no name back, so both engines surface an empty string here (live-verified) -- clients relying on the default should connect to "dev". |
| `status.outputs.port` | `int32` | The port the cluster accepts connections on. |
| `status.outputs.subnet_group_name` | `string` | The name of the Redshift subnet group the cluster runs in (module-managed from subnet_ids or the referenced existing group). |
| `status.outputs.parameter_group_name` | `string` | The name of the cluster parameter group in use (module-managed from inline parameters or the referenced existing group). |
| `status.outputs.master_password_secret_arn` | `string` | The ARN of the AWS-managed admin-password secret in Secrets Manager. Populated only when manage_master_password is true -- the handle applications use to fetch credentials at runtime. |
| `status.outputs.endpoint_access_addresses` | `map<string, string>` | The private DNS addresses of the cluster's managed VPC endpoints, keyed by endpoint name (spec.endpoint_accesses entries) -- what consumers inside the endpoint's VPC put in connection strings. |
| `status.outputs.usage_limit_ids` | `map<string, string>` | The AWS-generated usage-limit IDs, keyed exactly as the module keys each limit: "<feature_type>/<limit_type>/<period>", with an unset period rendered as "monthly" (the AWS default). These are the handles `aws redshift delete-usage-limit` and state import take; AWS generates them at creation time. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.subnetIds` | AwsSubnet | `status.outputs.subnet_id` |
| `spec.securityGroupIds` | AwsSecurityGroup | `status.outputs.security_group_id` |
| `spec.elasticIp` | AwsElasticIp | `status.outputs.public_ip` |
| `spec.masterPasswordSecretKmsKeyId` | AwsKmsKey | `status.outputs.key_arn` |
| `spec.kmsKeyId` | AwsKmsKey | `status.outputs.key_arn` |
| `spec.iamRoles` | AwsIamRole | `status.outputs.role_arn` |
| `spec.defaultIamRoleArn` | AwsIamRole | `status.outputs.role_arn` |
| `spec.scheduledActions[].iamRoleArn` | AwsIamRole | `status.outputs.role_arn` |
| `spec.endpointAccesses[].vpcSecurityGroupIds` | AwsSecurityGroup | `status.outputs.security_group_id` |
| `spec.endpointAuthorizations[].vpcIds` | AwsVpc | `status.outputs.vpc_id` |

## Referenced By

Fields on other kinds that can point at this resource:

| Kind | Field | Reads |
|---|---|---|
| AwsBedrockKnowledgeBase | `spec.sql.provisioned.clusterIdentifier` | `status.outputs.cluster_identifier` |

## See Also

- [Overview](../README.md)
