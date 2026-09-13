# The AI rooms: an OpenAI account, a Foundry hub, an ML workspace, and its endpoints become the rooms their children live in

## What changed

Five Azure AI and machine-learning kinds whose metadata already described their children as "an ARM child of its account / its workspace / its endpoint" now carry `kind_meta.container_kind: true`, so the platform draws those children inside them on a diagram, as the portal's own blades list them. Every reference INTO one of the five across the catalog was read: all ten are children that live inside their parent, so all ten become placements and nothing else travels as an exemption for the marks' sake. Five subnet reaches that would have torn a new room in half -- or that were already placing a PaaS account inside a firewall rule -- are marked `containment_exempt`. Each field's comment says why.

- **`AzureCognitiveAccount`** is a container: every model deployment and every project is an ARM child created onto the account (`accounts/{account}/deployments/{name}`, `.../projects/{name}`). Exempt: the account's network ACL `virtual_network_rules[].subnet_id` (a firewall rule -- the account admits traffic from the subnet and lives in its resource group) and its agent `network_injection.subnet_id` (the account injects agent compute into the subnet and never lives there, the Azure-SSIS runtime's rule).
- **`AzureAiFoundry`** is a container: every AI Foundry project is created inside its hub and deploys into the hub's resource group (the project spec carries no group of its own). The hub's vault and storage reaches are already access.
- **`AzureMachineLearningWorkspace`** is a container: datastores, compute clusters and instances, and online and batch endpoints are ARM children of the workspace (`workspaces/{ws}/datastores|computes|onlineEndpoints|batchEndpoints/{name}`). Exempt: a compute cluster's and a compute instance's `subnet_id` (an ARM child of its workspace whose nodes attach to the subnet -- the AKS node pool's rule; without the mark a compute on its own subnet would fall to a mixed-kind tie between the workspace and the subnet) and the workspace's `serverless_compute.subnet_id` (the workspace injects serverless nodes into the subnet and never lives there).
- **`AzureMachineLearningOnlineEndpoint` and `AzureMachineLearningBatchEndpoint`** are containers in their turn: every online deployment is an ARM child created under the endpoint whose traffic map routes to it, every batch deployment under the endpoint whose default-deployment pointer routes to it (`.../onlineEndpoints|batchEndpoints/{endpoint}/deployments/{name}`), and neither can exist without its endpoint -- so the deployments draw inside the endpoint, inside the workspace. A batch deployment's compute cluster is a reach and is not a room.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) gains ten `contained` lines -- every typed reference into one of the five, each from a kind that lives inside it -- and five `exempt` lines moved from `contained`.

This change also carries, word for word, five earlier Azure containment corrections that were proposed on their own and overlapped with it or with each other on regenerated files: a Network Watcher flow log writes to the storage account it records into; a container group, a backup instance, and an AKS node pool reach what they name; what reaches into a Key Vault never lives in it, and a role lives where it is defined; a SQL failover group lives on its primary and a Cosmos DB role in its account; what reaches into a storage account, an event hub, or a Service Bus topic never lives in it. Their own notes stand beside this one.

## Why

A diagram is truthful when what lives inside a thing is drawn inside it. An Azure OpenAI account with three model deployments drew as four unrelated cards with lines; a Machine Learning workspace with its compute, its datastores, and its endpoints drew as a card with eight leaves scattered outside every wall, because each child named only a parent the platform did not treat as a room. The marks give each child its room. The exemptions keep a neighbour that merely reaches into a room -- or a compute that attaches to a subnet -- from being drawn inside it, and keep the new workspace room from being torn between itself and a subnet the moment a compute names both.

The five travel with the ten rooms already in this change because every container mark regenerates the same kind-registry descriptor, and the five earlier corrections travel here because each edited a spec file another of them also edited: one change, one regeneration of every touched stub, one merge.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; 10 placements added, 5 reaches exempt beyond the ten rooms' lines
grep -n -B1 'container_kind: true' shared/cloudresourcekind/cloud_resource_kind.proto | grep -c Azure   # the Azure container kinds, fifteen more than before this change
grep -rln containment_exempt catalog/azure --include=spec.proto | wc -l
```
