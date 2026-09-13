# A Prometheus scraper reads the EKS cluster it scrapes and never lives in it on a diagram

## What changed

- **The Managed Prometheus scraper's EKS cluster reference is containment-exempt.** `AwsManagedPrometheusScraperEksSource.cluster_arn` names the cluster the scraper reads metrics from. The scraper's collectors run on AWS-managed network interfaces in the subnets the same source block names; they are never workloads inside the cluster. Until now the reference was placement by omission, so a scraper pointed at an EKS cluster would have been drawn inside the cluster it merely reads -- or, because it also names two subnets, inside whichever of the three candidates happened to sort last.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly that one line from `contained` to `exempt`; nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. Every other reference into an EKS cluster in the catalog is a resource that genuinely runs in the cluster -- a node group, an add-on, a Fargate profile, an access entry, a Batch environment on EKS -- and stays placement. The scraper is the one reader: it observes the cluster from outside. On a diagram it now stands where its subnets place it, inside the VPC beside the cluster, with a line to the cluster it scrapes.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the exempt line
grep -n containment_exempt catalog/aws/awsmanagedprometheusscraper/v1alpha1/spec.proto
```
