# A Studio space that mounts a file system never lives in it on a diagram

## What changed

- **`AwsSagemakerDomainSpaceCustomFileSystem.file_system_id` is containment-exempt.** A SageMaker Studio space that mounts an existing EFS file system names it so the space's apps can read and write it. That reference is a mount, not a home: the domain is deployed in its VPC, and the file system is a room of its own that access points are created into. Until now the reference was placement by omission, so a domain whose space mounted an EFS would have been drawn inside the file system's room instead of in its network.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly that one line from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. The domain-level mount (`AwsSagemakerDomainEfsFileSystemConfig.file_system_id`) already carries the exemption for exactly this reason; the space-level twin did not, so the same mount drew two different pictures depending on where in the domain it was authored. On a diagram the domain now stands in its VPC with a line to the file system whichever level mounts it.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the exempt line
grep -n containment_exempt catalog/aws/awssagemakerdomain/v1alpha1/spec.proto
```
