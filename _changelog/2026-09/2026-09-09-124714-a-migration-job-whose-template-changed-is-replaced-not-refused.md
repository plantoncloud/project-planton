# A migration Job whose template changed is replaced, not refused

## What changed

- **`ApplyManifests` honors Helm's before-hook-creation semantics for Jobs.** When the API server refuses a server-side apply of a `batch/v1 Job` because its pod template is immutable, the operator deletes the completed run (background propagation; its pods are done) and applies the rendered Job again, so the new migration runs once. Only that refusal is answered this way: a Job refused for any other validation reason, a non-Job with an immutable field, or a non-validation error still surfaces as the error it is, and nothing is deleted over it. An unchanged Job never reaches the branch, because applying identical content is a no-op.

## Why

Upgrading the operator from 0.9.1 to 0.10.0 on a live platform put the platform in `Error`: 0.10.0 carried the embedded OpenFGA chart's move to 0.3.13 (image v1.5.9 to v1.19.0), so the rendered `<platform>-openfga-migrate` Job differed from the one the previous operator had created, and every reconcile ended with `Job.batch "...-openfga-migrate" is invalid: spec.template: field is immutable`. The control plane and console kept serving, but the operator was stuck behind its own dependency order and could reconcile nothing downstream. Helm solves this for hooks by deleting the previous hook object before creating the new one; the operator renders those hooks itself and now does the same.

## How to check

```bash
cd operator && go test ./internal/component -run TestApplyManifests   # replaced on the immutable-template refusal; nothing deleted on any other refusal
```

Live: upgrade the operator on a platform whose OpenFGA chart moved; the platform returns to `Ready`, the migrate Job carries the new image, and `kubectl get job` shows one run.
