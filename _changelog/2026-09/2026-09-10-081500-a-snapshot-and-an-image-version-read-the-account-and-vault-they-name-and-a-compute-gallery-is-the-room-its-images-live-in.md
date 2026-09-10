# A snapshot and an image version read the account and vault they name, and a Compute Gallery is the room its images live in

## What changed

- **Four references in the Azure compute family are containment-exempt.** A disk snapshot imported from a VHD names the storage account holding the blob (`AzureDiskSnapshotSpec.storage_account_id`); a snapshot carrying legacy Azure Disk Encryption settings names the Key Vault holding its disk-encryption secret and its key-encryption key (`AzureDiskSnapshotDiskEncryptionKey.source_vault_id`, `AzureDiskSnapshotKeyEncryptionKey.source_vault_id`); a gallery image version built from a page blob names the storage account holding it (`AzureComputeGalleryImageVersion.storage_account_id`). Each of those resources READS out of the account or vault and lives in its own resource group (or its gallery), so on a diagram the reference is access, not placement -- the same reasoning a Network Watcher flow log's storage account and a virtual machine's Key Vault secrets already carry.
- **`AzureComputeGallery` is a container kind.** The kind metadata already said it in words -- "image definitions live inside it" -- and ARM nests them (`{gallery_id}/images/{name}`); it now says it with `container_kind: true`, so the containment resolvers nest every image definition inside the library that publishes it. The image ids a VM, a scale set, or a disk boots from are plain strings and never place anything inside the gallery, so no exemption needs to travel with the mark.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly four lines from `contained` to `exempt` and gains exactly one `contained` line (`AzureComputeGalleryImageSpec.gallery_name`, the only typed reference into a gallery across the catalog). Nothing else moved.

## Why

`containment_exempt` says a reference into a container is access, not placement; `container_kind` says a kind is a box other resources nest inside. Without the exemptions, a snapshot restored from a customer's VHD would be drawn inside the storage account it was read from, and a snapshot with disk-encryption settings inside the vault its key sits in -- a backup artifact drawn as a resident of what it merely consulted. Without the mark, a gallery and its image definitions draw as peer cards joined by lines, hiding the library shape Azure's own portal shows.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the four exemptions and the gallery placement
grep -n -A8 'AzureComputeGallery = 2205' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n containment_exempt catalog/azure/azuredisksnapshot/v1alpha1/spec.proto catalog/azure/azurecomputegalleryimage/v1alpha1/spec.proto
```
