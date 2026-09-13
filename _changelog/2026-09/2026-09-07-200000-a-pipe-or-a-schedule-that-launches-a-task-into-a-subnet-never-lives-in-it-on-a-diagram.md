# A pipe or a schedule that launches a task into a subnet never lives in it on a diagram

## What changed

- **`AwsEventBridgePipeEcsNetworkConfiguration.subnets` is containment-exempt.** A pipe whose target is an ECS task names the subnets that task launches into. Those subnets place the task, not the pipe -- the pipe is a managed service outside the network. Until now the reference was placement by omission, so a pipe with an ECS target was drawn inside the VPC (or one subnet) it only launched tasks into.
- **`AwsEventBridgePipeSelfManagedKafkaVpc.subnets` is containment-exempt.** A pipe reading from self-managed Kafka names the subnets its client reaches the brokers through. The pipe reaches INTO the VPC to poll; it is not deployed there -- the verdict a Lambda function's VPC subnets already carry.
- **`AwsEventBridgeScheduleNetworkConfiguration.subnets` is containment-exempt.** The same verdict for a schedule whose target is an ECS task.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly those three lines from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. The EventBridge rule's ECS target subnets (`AwsEventBridgeEcsNetworkConfiguration.subnets`) already carry the exemption for exactly this reason; the pipe's and the schedule's identical fields did not, so the same configuration drew three different pictures depending on which EventBridge kind launched the task. On a diagram all three now stand beside the network with a line in, and only the resources that live in a subnet are drawn inside it.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the three exempt lines
grep -n containment_exempt catalog/aws/awseventbridgepipe/v1alpha1/spec.proto catalog/aws/awseventbridgescheduler/v1alpha1/spec.proto
```
