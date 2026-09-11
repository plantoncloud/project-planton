# The policy engine is part of every platform the operator installs

## What changed

- **`spec.components.authorization` is gone from the `PlantonPlatform` definition.** OpenFGA is now installed on every platform the operator reconciles, the way PostgreSQL is: `openfga` reports `IsEnabled` unconditionally, the control plane always depends on it (its store id reaches the pod through the component's bootstrap ConfigMap, so the dependency turns a `CreateContainerConfigError` into an explained wait), and the OpenFGA status slot is always present -- allocated with the core components and backfilled onto a status that was allocated without it, so an upgraded platform never leaves its control plane waiting on a slot that does not exist. The `ComponentToggle` type had no other user and leaves with the field.
- **The control plane is never told which authorization arm to run.** The operator no longer renders `PLANTON_AUTHORIZATION_PROVIDER`; the control plane resolves its arm from the deployment's sign-in shape alone (an identity issuer means the policy engine). The FGA environment is always the real connection: the engine's in-cluster endpoint, the store id from the bootstrap ConfigMap, and `PLANTON_BOOTSTRAP_AUTHORIZATIONMODEL_MANAGE=true` so the control plane writes and pins its own model at boot. The placeholder branch that rendered `http://localhost:8088` and a `local` store id for platforms without an engine is deleted.
- **The boot-contract fixture loses one variable and the platform floor stays at `v0.0.60`.** An older control plane under this operator sees the provider variable unset -- which it already reads as the policy engine -- and the same real FGA connection it received whenever the toggle was on, so no platform release is required before this operator can run one. A platform resource that still declares `components.authorization` is refused by the API server's strict field validation (`unknown field "spec.components.authorization"`), which names the exact line to remove.

## Why

A self-hosted Planton with a team had two authorization truths, chosen once on the platform resource. With the engine off -- the zero-config default -- every signed-in account could do anything in every organization, and every screen that describes access (the roster's roles, an invitation's role preview, Remove Access) described something that was not enforced. With the engine on, each person had exactly the roles they were granted. Everything the product says about access was honest on one arm only, and every feature had to be proven twice.

The footprint argument for keeping the engine optional had worn thin: OpenFGA is one single-replica pod on the platform's own PostgreSQL, the operator already bootstraps its store, and the control plane already owns its model. Making the engine part of every platform removes the fork instead of documenting it. The desktop's local single-user instance is genuinely a different shape (one person, no roster) and keeps its owner arm; it does not run under this operator.

## How to check

```bash
cd operator && make manifests generate                      # the CRD bases and the chart's CRD template carry no components.authorization
make operator-test                                          # envtest: the minimal platform allocates an OpenFGA slot; the boot-contract fixture has no PLANTON_AUTHORIZATION_PROVIDER
cd operator && go test ./internal/component/ -run 'OpenFGA' # the control plane always depends on openfga and always wires its connection
```

Declaring a platform has one fewer decision in it:

```yaml
apiVersion: planton.ai/v1
kind: PlantonPlatform
metadata:
  name: planton
  namespace: planton
spec:
  version: v0.0.60
# no components.authorization -- the policy engine is always there
```

Adopters upgrading a platform that declared `components.authorization` remove the block; the definition no longer carries the field, and `kubectl apply` names it as unknown until they do.
