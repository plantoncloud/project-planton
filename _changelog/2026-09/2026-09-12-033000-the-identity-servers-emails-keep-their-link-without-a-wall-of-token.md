# The identity server's emails keep their link without a wall of token

## What changed

- **A labelled fallback link beneath the button.** The password-reset, account-update, and email-verification emails the identity server sends no longer spell out their action link beneath the button. Keycloak's action token runs to about a thousand characters; spelled out it was the largest element on the page, buried the button, and verified nothing a person could check by eye. The emails now offer "Or open the password reset link." (and its siblings), a short link in the quiet register whose href is the full URL -- so a client that strips the button's styling still leaves a way in. The text twins keep the URL, because plain text must.
- **The shared layout gained the atom.** The macro comes from the platform's email-templates source (`fallbackAction`), regenerated into `theme/email/html/template.ftl` beside the existing `fallbackLink`, which stays for links short enough to read and compare.
- **The rendering test reads the reviewed footer.** `TestEmailTheme_RendersThroughKeycloak` now asserts the reply-to as the `mailto:` link the layout renders, and asserts the labelled fallback.

## Why

An email's one job is to get the person to the button; a thousand characters of opaque token beneath it works against that and pretends to a verifiability it does not have. One shared atom, generated from one source, keeps the identity server's emails and the platform's emails the same design.

## How to check

- `cd operator && go test -tags requires_docker ./internal/keycloak/ -run TestEmailTheme_RendersThroughKeycloak` renders the account-update email through a real Keycloak and a Mailpit relay and asserts the labelled fallback.
- On a running platform: request "Forgot password?" from the sign-in page and open the email; the button and one short link, never a wall.
