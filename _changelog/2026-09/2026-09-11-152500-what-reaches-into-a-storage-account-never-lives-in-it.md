# What reaches into a storage account never lives in it

## What changed

- **Eleven storage-account references are containment-exempt.** An AI Foundry hub and a Machine Learning workspace use a storage account as their default workspace storage (`AzureAiFoundrySpec.storage_account_id`, `AzureMachineLearningWorkspaceSpec.storage_account_id`); a Cognitive Services account reads training data from one (`AzureCognitiveAccountStorage.storage_account_id`); a Data Factory linked service is the factory's connection to a blob or Data Lake store (`AzureDataFactoryLinkedServiceAzureBlobStorage.service_endpoint`, `AzureDataFactoryLinkedServiceDataLakeStorageGen2.url`) and a blob-event trigger listens to one (`AzureDataFactoryTriggerBlobEvent.storage_account_id`); an Event Grid subscription dead-letters into one and delivers into a queue in one (`AzureEventgridEventSubscriptionDeadLetter.storage_account_id`, `AzureEventgridEventSubscriptionStorageQueueDestination.storage_account_id`); a data collection rule delivers collected data into one (`AzureMonitorDataCollectionRuleStorageBlobDestination.storage_account_id`, `AzureMonitorDataCollectionRuleStorageTableDirect.storage_account_id`); and a Recovery Services backup container registers one with a vault (`AzureBackupContainerStorageAccountSpec.storage_account_id`). Every one of those resources READS, WRITES, LISTENS TO, or REGISTERS the account and lives somewhere else -- in its own resource group, its factory, its topic, or its vault -- so on a diagram each reference is access, not placement. Each field's comment now says so.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly eleven lines from `contained` to `exempt`. Nothing else moved.

## Why

`containment_exempt` says a reference into a container is access, not placement. The storage account is a container kind (its blob containers, file shares, queues, tables, Data Lake filesystems, encryption scopes, and local users are created into it), so without the exemption a workspace, a linked service, an event subscription, or a monitoring rule that references the account it writes to was drawn INSIDE that account -- a consumer drawn as a resident of what it merely uses. The Network Watcher flow log, the diagnostic setting, the Event Hub capture destination, the Function App's storage, and the Container App environment's storage registration already carried the same exemption for the same reason; these eleven now agree with them. The references stay on the picture as lines, which is what they are.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the eleven exemptions
grep -rn containment_exempt catalog/azure/azureaifoundry catalog/azure/azurecognitiveaccount catalog/azure/azuredatafactorylinkedservice catalog/azure/azuredatafactorytrigger catalog/azure/azureeventgrideventsubscription catalog/azure/azuremachinelearningworkspace catalog/azure/azuremonitordatacollectionrule catalog/azure/azurebackupcontainerstorageaccount --include=spec.proto
```
