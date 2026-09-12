# The operator allocates every unconditional status slot once

## What changed

- **The `PlantonPlatform` status is allocated once, together.** The identity server's slot joins the data services, the policy engine, the control plane, and the console in the one base allocation; the two post-hoc nil-checks that re-created the identity and OpenFGA slots on a status that lacked them are gone. Slots that follow a dial (the front door, the runner, the secrets manager, the graph store) keep syncing in both directions on every reconcile, because a running platform can flip those.
- **The platform catalog kind's control profile says what the platform does.** `KubernetesPlantonPlatform`'s `rbac` control evidence names the identity server for sign-in and the policy engine for fine-grained relationship authorization on every request; it no longer describes the policy engine as an opt-in component field.

## Why

A fresh platform's status carries every unconditional slot from its first reconcile. Code that re-allocates a slot "a status allocated without it" could only serve a platform created by a release that allocated fewer slots, and every deployment today is disposable and reinstalled, so that code protects nobody and would be read for the life of the operator. The one place a slot is allocated is now the one place a reader looks. The control profile sentence described a field the kind's schema does not have; a control profile is read by people deciding whether a component meets their bar, so it says what the platform actually does.

## How to check

```bash
make operator-test                                         # unit and envtest suites, the status package included
cd operator && go vet ./internal/status/ && gofmt -l internal/status/   # nothing listed
go test -count=1 ./pkg/compliance/controlprofile/ ./pkg/anatomy/        # the control profile parses
rg -n -i "backfill|allocated without|on upgrade" operator/internal/status/status.go   # nothing
```
