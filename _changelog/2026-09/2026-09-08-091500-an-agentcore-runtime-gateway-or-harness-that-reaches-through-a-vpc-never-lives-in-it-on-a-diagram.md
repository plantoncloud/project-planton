# An AgentCore runtime, gateway, or harness that reaches through a VPC never lives in it on a diagram

## What changed

- **The three AgentCore managed VPC endpoint blocks are containment-exempt on both of their network references.** `AwsBedrockAgentCoreManagedVpcEndpoint.vpc_id` and `.subnet_ids` on the agent runtime, `AwsBedrockAgentCoreGatewayManagedVpcEndpoint.vpc_id` and `.subnet_ids` on the gateway, and `AwsBedrockAgentCoreManagedVpcEndpoint.vpc_id` and `.subnet_ids` on the evaluation harness. A managed endpoint is an AWS-managed private PATH through your VPC so the runtime, gateway, or harness can reach a private OIDC provider or a private backend; the resource itself is not deployed inside that VPC. Until now the references were placement by omission, so a gateway whose one target sat behind a private endpoint would have been drawn inside the VPC it merely reaches into.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly those six lines from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. Where an AgentCore resource genuinely runs inside a network -- a runtime, a browser tool, or a harness in VPC network mode attaching session interfaces to your subnets -- its `VpcConfig.subnets` stays placement and the diagram draws it in the VPC. A private endpoint is the other thing: a door punched through the VPC wall for outbound reach. The same verdict already stands on a Lambda's VPC subnets and an EventBridge pipe's task subnets. On a diagram the runtime, the gateway, and the harness now stand where their own network places them, with a line into the VPC they reach through.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the six exempt lines
grep -n containment_exempt catalog/aws/awsbedrockagentcoreruntime/v1alpha1/spec.proto catalog/aws/awsbedrockagentcoregateway/v1alpha1/spec.proto catalog/aws/awsbedrockagentcoreevaluation/v1alpha1/spec.proto
```
