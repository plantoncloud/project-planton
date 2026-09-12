# The realm owns a password policy and brute-force protection, because people now choose their own passwords

## What changed

- **The realm's owned settings gain `passwordPolicy` (`length(12) and notUsername and notEmail`) and `bruteForceProtected: true`.** Both are written by the realm import on a fresh install and re-asserted by the reconciler on every pass, so a realm created before this operator gains them on its next pass and an administrator who relaxes them in the Keycloak console is reverted. The lockout thresholds stay Keycloak's defaults and remain admin-tunable: the switch is the posture, the numbers are not product decisions.
- **Two new constants in `internal/resources` carry the values with their reasons**, and the import-agreement test pins that the import and the owned set say the same thing.

No boot-contract change: the control plane's environment is untouched, and the platform floor stays where it is.

## Why

Until now nobody chose a password on a self-hosted realm: the first admin received a generated one and replaced it at first sign-in, and every other person came from a directory. Platform releases from `v0.0.60` onward let an invited teammate create their own sign-in account from the join page with a password they choose, on a door strangers can reach. A realm with no policy accepts a one-character password there, and a realm without brute-force detection lets a client hammer one account's password without limit. The moment people choose their own passwords, both settings are load-bearing, and load-bearing settings belong to the operator -- on every install, self-healing, never dependent on an administrator remembering to set them. The control plane mirrors the length floor in its own field validation so the join page refuses before a too-short password ever leaves it; the realm setting is the authority, and the identity server remains the only place a credential is ever verified.

## How to check

```bash
cd operator && go test ./internal/keycloak -run 'TestIdentityRealmImportAgreesWithOwnedSet|TestOwnedRealmSettingsCarryTheCredentialPosture'
cd operator && go test ./internal/resources
cd operator && make test-realm-convergence   # requires Docker: a fresh realm carries both keys after one pass and the second pass is clean
```

Live: on a Kind cluster with this operator, the realm's Admin API representation reads `"passwordPolicy": "length(12) and notUsername and notEmail"` and `"bruteForceProtected": true`; changing either in the Keycloak console is reverted within a reconcile; a person creating an account through an invitation with a nine-character password is refused with "The password must be at least 12 characters long."
