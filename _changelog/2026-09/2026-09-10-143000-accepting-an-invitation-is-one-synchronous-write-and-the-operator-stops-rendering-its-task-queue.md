# Accepting an invitation is one synchronous write, and the operator stops rendering its task queue

## What changed

- **The operator no longer renders `TEMPORAL_TASK_QUEUE_USER_INVITATION` for the control plane.** Platform `v0.0.60` accepts an invitation as one synchronous membership write -- the same shape as every other seat the platform writes -- so the background workflow that used to process acceptances, and the task queue it polled, no longer exist. The boot-contract fixture records the environment with that one name removed.
- **`platformversion.MinimumSupported` stays `v0.0.60`** and its rationale grows a sentence: the same release that stopped needing the placeholder email credentials also stopped needing this queue name, and an older control plane requires all of them to boot at all.

## Why

A self-hosted install brings people in by link: an administrator hands a personal invitation or the organization's invite link over Slack, the person signs in and accepts. Acceptance used to run as a background job that created the person's account along the way; accounts now come from exactly one place (the first sign-in), so accepting is nothing but "seat this person with these roles and record it" -- a request that either succeeds or explains itself, with no job to watch and no queue to configure. The operator's job is to render exactly the environment the control plane reads; a variable nothing reads is a lie waiting to mislead the next person who greps for it.

## How to check

```bash
cd operator && go test ./internal/resources -run TestBootContract   # the rendered environment matches the fixture; TEMPORAL_TASK_QUEUE_USER_INVITATION is absent
cd operator && go test ./internal/platformversion                   # the floor is still v0.0.60
```

Live: on a Kind cluster with this operator and platform `v0.0.60`, the control-plane Deployment carries no `TEMPORAL_TASK_QUEUE_USER_INVITATION`, the platform reaches `Ready`, and `planton invite`, `planton invite-link create|rotate|revoke|list`, and an acceptance through the API all complete with zero `ERROR` lines in the control-plane log.
