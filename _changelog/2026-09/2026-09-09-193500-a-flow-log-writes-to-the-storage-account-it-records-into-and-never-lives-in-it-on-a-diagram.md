# A flow log writes to the storage account it records into and never lives in it on a diagram

## What changed

- **The Network Watcher flow log's storage account reference is containment-exempt.** `AzureNetworkWatcherFlowLogSpec.storage_account_id` names the storage account the flow-log files land in. The flow log itself is a Network Watcher resource that lives in the watcher's resource group (`NetworkWatcherRG` on most subscriptions) and records one network, subnet, or interface; it is never a thing inside the account. Until now the reference was placement by omission, so a flow log would have been drawn inside the storage account it merely writes to.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly that one line from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. The storage account is a container because its containers, shares, queues, and tables are created into it; a flow log is created into a Network Watcher and writes files across the wire. The diagnostic setting's `storage_account_id` already carries the same exemption for the same reason -- a resource that delivers records to an account stands beside it with a line, the way a log delivery stands beside its bucket. The flow log now reads the same way.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the exempt line
grep -n containment_exempt catalog/azure/azurenetworkwatcherflowlog/v1alpha1/spec.proto
```
