# A failover group lives on its primary, and a Cosmos DB role lives in its account

## What changed

- **A SQL failover group's partner server is containment-exempt.** `AzureMssqlFailoverGroupPartnerServer.server_id` names the secondary server the group replicates databases TO. The group is created ON its primary (`AzureMssqlFailoverGroupSpec.server_id`, which stays placement) and points at the partner, so on a diagram the partner reference is access, not placement. Before this change both references were placement, and a group naming two servers of the same kind was promoted to the room they share -- the resource group beside both servers, or nothing at all when the partner lives in another group -- instead of standing on its primary.
- **A Cosmos DB SQL role definition's assignable scopes are containment-exempt.** `AzureCosmosdbSqlRoleDefinitionSpec.assignable_scopes` lists WHERE a custom data-plane role may be granted; WHERE the definition itself lives is its `cosmosdb_account_id`, which stays placement. The same rule the subscription-level `AzureRoleDefinitionSpec.assignable_scopes` already carries.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly two lines from `contained` to `exempt`. Nothing else moved.

## Why

`containment_exempt` says a reference into a container is access, not placement. `AzureMssqlServer` and `AzureCosmosdbAccount` are container kinds (databases, pools, and roles are created into them), so without the exemption a failover group would be drawn beside its servers instead of on the primary it is anchored to, and a role definition whose scopes are authored into a database would be drawn inside that database instead of in the account that owns it. The partner server and the assignable scopes stay on the picture as lines, which is what they are.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the two exemptions
grep -n containment_exempt catalog/azure/azuremssqlfailovergroup/v1alpha1/spec.proto catalog/azure/azurecosmosdbsqlroledefinition/v1alpha1/spec.proto
```
