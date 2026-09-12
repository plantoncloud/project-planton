# A container group, a backup instance, and a node pool reach what they name and never live in it

## What changed

- **Six references in the Azure container family are containment-exempt.** A Container Instance that mounts an Azure Files share names the storage account holding it twice (`AzureContainerInstanceVolumeAzureFile.storage_account_name` and `.storage_account_key`); a Data Protection backup instance that protects an AKS cluster names it (`AzureDataProtectionBackupInstanceKubernetesCluster.kubernetes_cluster_id`); an AKS node pool names the subnet its nodes attach to and, on traditional Azure CNI, the subnet its pods draw addresses from (`AzureAksNodePoolSpec.vnet_subnet_id`, `.pod_subnet_id`); an AKS cluster whose ingress add-on deploys an Application Gateway names the subnet that gateway goes into (`AzureAksClusterIngressApplicationGateway.subnet_id`). Each of those resources READS, MOUNTS, PROTECTS, or ATTACHES TO what it names and lives somewhere else -- the container group in its own resource group or subnet, the backup in its vault, the node pool in its cluster (an ARM child: `managedClusters/{cluster}/agentPools/{pool}`), the cluster in its own node subnet -- so on a diagram every one of these references is access, not placement. The Container App environment's storage registration already carried the exemption on the identical pair of storage-account fields; the container group now agrees with it.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly six lines from `contained` to `exempt`. Nothing else moved.

## Why

`containment_exempt` says a reference into a container is access, not placement. Without it, a container group mounting a share would be drawn inside the storage account it reads from; a backup would be drawn inside the cluster it protects; a node pool on its own subnet -- the shape Azure documents for segmenting an internet-exposed pool or for dynamic pod IP allocation -- would fall to a tie between its cluster and a subnet the cluster does not name, drawn by luck on a project diagram and left floating outside its cluster on the organization estate; and a cluster with an ingress add-on would be promoted out of its node subnet into the whole network. The pool's own subnet and the add-on's subnet stay on the picture as lines, which is what they are.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the six exemptions
grep -n containment_exempt catalog/azure/azurecontainerinstance/v1alpha1/spec.proto catalog/azure/azuredataprotectionbackupinstance/v1alpha1/spec.proto catalog/azure/azureaksnodepool/v1alpha1/spec.proto catalog/azure/azureakscluster/v1alpha1/spec.proto
```
