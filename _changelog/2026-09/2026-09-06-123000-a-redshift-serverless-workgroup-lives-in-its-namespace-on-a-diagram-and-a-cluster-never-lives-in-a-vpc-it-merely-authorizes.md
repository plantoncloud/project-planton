# A Redshift Serverless workgroup lives in its namespace on a diagram, and a Redshift cluster never lives in a VPC it merely authorizes

## What changed

- **`AwsRedshiftServerlessWorkgroupSpec.subnet_ids` is containment-exempt.** The namespace is the container kind (`container_kind: true`), and the workgroup is the compute plane that attaches to it -- the box AWS's own model draws around its workgroups. Until now the workgroup's subnets were also placement, so a workgroup referencing its namespace and three subnets named two rooms that are not nested in each other, and the diagram had no honest way to choose (the platform's resolver fell back to topological order, which can land the workgroup in a single subnet). Now the namespace places the workgroup and the subnets are the network it reaches into by lines -- the verdict an ECS service already carries for its subnets while its cluster places it.
- **`AwsRedshiftServerlessWorkgroupEndpointAccess.subnet_ids` is containment-exempt.** A managed VPC endpoint's subnets are the CONSUMING VPC's; the workgroup is exposed inside them, never deployed into them.
- **`AwsRedshiftClusterEndpointAuthorization.vpc_ids` is containment-exempt.** An endpoint authorization admits another account's VPCs to the cluster; the cluster never lives inside a VPC it authorizes. The cluster's own `subnet_ids` stay placement.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly those three lines from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. The workgroup carried two placement claims into two rooms that cannot nest, which is the one shape the doctrine cannot draw truthfully. Its spec already says which room is home ("a workgroup computes; the data it serves lives on the namespace it attaches to"), so the subnets are the access path and the exemption records it. The other two fields were placement by omission on references that only ever meant "let these reach me".

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the three exempt lines
grep -n containment_exempt catalog/aws/awsredshiftserverlessworkgroup/v1alpha1/spec.proto catalog/aws/awsredshiftcluster/v1alpha1/spec.proto
```
