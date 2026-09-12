# What reaches into a Key Vault never lives in it, and a custom role lives where it is defined

## What changed

- **Five Key Vault references are containment-exempt.** An AI Foundry hub stores its secrets in a vault and unwraps its encryption key through one (`AzureAiFoundrySpec.key_vault_id`, `AzureAiFoundryEncryption.key_vault_id`); a Machine Learning workspace does the same (`AzureMachineLearningWorkspaceSpec.key_vault_id`, `AzureMachineLearningWorkspaceEncryption.key_vault_id`); a Data Factory linked service of the Key Vault variant points at the vault other linked services resolve their secrets through (`AzureDataFactoryLinkedServiceKeyVault.key_vault_id`). Each of those resources WRITES INTO, UNWRAPS THROUGH, or POINTS AT the vault and lives somewhere else -- the hub and the workspace in their own resource group, the linked service in its factory -- so on a diagram every one of these references is access, not placement. The AKS cluster's service-mesh certificate authority, a virtual machine's and a scale set's vault-sourced secrets, and the External Secrets store's vault URL already carried the exemption; these five now agree with them. The references a key, a secret, and a certificate carry to their vault stay placement: those three ARE created inside it.
- **A role definition's assignable scopes are containment-exempt.** `AzureRoleDefinitionSpec.assignable_scopes` lists WHERE a custom role may be granted; WHERE the definition itself lives is its `scope`, which stays placement. A definition assignable in two other resource groups was torn between three rooms.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly six lines from `contained` to `exempt`. Nothing else moved.

## Why

`containment_exempt` says a reference into a container is access, not placement. Key Vault is a container kind (its keys, secrets, and certificates are created into it), so without the exemption a workspace, a hub, or a linked service that merely uses a vault would be drawn INSIDE it on a project diagram and on the organization estate -- and a diagram that puts a machine-learning workspace inside a Key Vault is confidently wrong about where things live. The vault stays on the picture as a line from each of them, which is what it is.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the six exemptions
grep -n containment_exempt catalog/azure/azureaifoundry/v1alpha1/spec.proto catalog/azure/azuremachinelearningworkspace/v1alpha1/spec.proto catalog/azure/azuredatafactorylinkedservice/v1alpha1/spec.proto catalog/azure/azureroledefinition/v1alpha1/spec.proto
```
