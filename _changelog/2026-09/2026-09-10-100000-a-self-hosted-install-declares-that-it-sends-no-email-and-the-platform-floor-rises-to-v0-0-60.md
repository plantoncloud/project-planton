# A self-hosted install declares that it sends no email, and the platform floor rises to v0.0.60

## What changed

- **The operator renders `PLANTON_EMAIL_PROVIDER=none` for the control plane** and no longer hands it placeholder email credentials (`RESEND_API_KEY`, `SENDGRID_API_KEY`, `SENDGRID_EMAIL_TEMPLATE_ID_USER_INVITATION`) or the invitation URL base path (`USER_INVITATION_URL_BASE_PATH`). Email is a declared capability of the install, like storage and authorization: a self-hosted platform has no email service unless it says so, and the control plane is told the truth instead of dialing a fake key and failing.
- **Invitation links compose from the console the operator already declares.** The control plane builds every invitation link from `PLANTON_CONSOLE_URL` (the install's front door, rendered since operator 0.13.0), so the separate base path is gone.
- **`platformversion.MinimumSupported` is `v0.0.60`.** An older control plane requires the placeholder key and the base path to boot at all, so under this operator it would never come up; the floor refuses it at the resource with the sentence naming the version to move to. The boot-contract fixture records the new environment.

## Why

On a fresh self-hosted install, "Invite Member" failed with an internal error: the control plane tried to email the invitation with the placeholder key before saving it, and the install had no email service. Platform `v0.0.60` makes invitations links first -- the invitation is saved, the email is a courtesy, and the create response says whether one went -- and reads `PLANTON_EMAIL_PROVIDER` to learn what this deployment can do. The operator's job is to declare the install's posture honestly; a placeholder credential was a lie the control plane had to discover at the worst moment.

## How to check

```bash
cd operator && go test ./internal/resources -run TestBootContract   # the rendered environment matches the fixture; the floor is v0.0.60
cd operator && go test ./internal/platformversion                   # v0.0.59 is refused, v0.0.60 is run
```

Live: on a Kind cluster with this operator and platform `v0.0.60`, the control-plane Deployment carries `PLANTON_EMAIL_PROVIDER=none` and none of the four retired names, the platform reaches `Ready`, and a `PlantonPlatform` declaring `v0.0.59` is refused within seconds with nothing created for it.
