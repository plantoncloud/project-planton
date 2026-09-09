# A Traffic Manager profile and a Front Door origin group are the rooms their endpoints and origins stand in on a diagram

## What changed

- **`AzureTrafficManagerProfile` and `AzureFrontDoorOriginGroup` are container kinds.** The kind metadata already said it in words -- "every endpoint is created inside a referenced profile" and "an origin is an ARM child of a referenced origin group", the same words the DNS record uses about its zone -- and now says it with `container_kind: true`, so the containment resolvers nest those kinds where Azure puts them: Traffic Manager endpoints inside the profile that steers traffic to them, Front Door origins inside the origin group that load-balances them (which in turn stands inside its Front Door profile).
- **Three references that point AT one of those rooms are containment-exempt.** A nested Traffic Manager endpoint's `target_profile_id` names the child profile it forwards to; the endpoint lives in its own parent profile. A Front Door route's `origin_group_id` names the group that answers its requests; the route lives in its endpoint. A Front Door rule's route-configuration override names the group it redirects to; the rule set lives in its profile. Each is access, not placement, and had the container marks landed alone, all three would have become placement by omission -- a route drawn inside the origin group it forwards to, or torn between its endpoint and that group.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) gains exactly two `contained` lines (`AzureTrafficManagerEndpointSpec.profile_id`, `AzureFrontDoorOriginSpec.origin_group_id`) and three `exempt` lines (the references above). Nothing else in the registry moved: these five are every typed reference into either kind across the catalog.

## Why

`container_kind` says a kind is a box other resources nest inside; `containment_exempt` says a reference into such a box is access, not placement. Azure's own portal shows a Traffic Manager profile as a table of its endpoints and an origin group as a table of its origins, and ARM's resource paths nest them (`trafficmanagerprofiles/{p}/azureEndpoints/{e}`, `profiles/{p}/originGroups/{g}/origins/{o}`). Drawing the parent as a card and each child as a card beside it hides that shape; the marks let a diagram draw what Azure draws. The exemptions and the marks travel together because each is only truthful beside the other.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the two placements and the three exemptions
grep -n -A8 'AzureTrafficManagerProfile = 2189' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n -A8 'AzureFrontDoorOriginGroup = 2082' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n containment_exempt catalog/azure/azuretrafficmanagerendpoint/v1alpha1/spec.proto catalog/azure/azurefrontdoorroute/v1alpha1/spec.proto catalog/azure/azurefrontdoorruleset/v1alpha1/spec.proto
```
