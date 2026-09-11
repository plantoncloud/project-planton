# A backup vault is the room its policies and protected items live in

## What changed

- **`AzureRecoveryServicesVault` and `AzureDataProtectionBackupVault` are container kinds.** The kind metadata already said it in words -- "backup policies and protected items are ARM children of a vault", "backup policies and backup instances are ARM children of a vault" -- and now says it with `container_kind: true`, so the containment resolvers nest those kinds where Azure puts them: the two Recovery Services policies, the two protected items, and the backup container registration inside their Recovery Services vault; the Data Protection policy and instance inside their Data Protection vault. The golden gains seven placements, one per child, each through the `recovery_vault_name` or `vault_id` reference the child already carries.
- **Four Data Protection backup instance references are containment-exempt.** A blob or Data Lake instance PROTECTS a storage account (`AzureDataProtectionBackupInstanceBlobStorage.storage_account_id`, `AzureDataProtectionBackupInstanceDataLakeStorage.storage_account_id`); a disk or Kubernetes instance WRITES its snapshots into a resource group (`AzureDataProtectionBackupInstanceDisk.snapshot_resource_group_name`, `AzureDataProtectionBackupInstanceKubernetesCluster.snapshot_resource_group_name`). The instance itself is an ARM child of its vault, so on a diagram each of those references is access, not placement -- the rule the Kubernetes variant's `kubernetes_cluster_id` already carries. Before this change a blob instance was drawn inside the storage account it protects and a disk instance inside the group its snapshots land in.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) gains seven `contained` lines into the two vaults and moves exactly four lines from `contained` to `exempt`. Nothing else moved.

## Why

`container_kind` says a kind is a box other resources nest inside; `containment_exempt` says a reference into such a box is access, not placement. Azure's own portal shows a vault as blades of its backup policies and its protected items, and ARM's resource paths nest them (`vaults/{v}/backupPolicies/{p}`, `vaults/{v}/backupFabrics/Azure/protectionContainers/{c}/protectedItems/{i}`, `backupVaults/{v}/backupInstances/{i}`). Drawing the vault as a card with a fan of lines hides that shape -- and left a Data Protection policy, which names only its vault, with no room at all. The marks and the exemptions travel together because each is only truthful beside the other: with the marks alone, a backup instance would still be torn between its vault and the account or group it reaches into.

Every reference into either vault across the Azure catalog comes from one of these seven children, so the marks create no other placement.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the seven placements and the four exemptions
grep -n -A8 'AzureRecoveryServicesVault = 2175' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n -A8 'AzureDataProtectionBackupVault = 2180' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n containment_exempt catalog/azure/azuredataprotectionbackupinstance/v1alpha1/spec.proto
```
