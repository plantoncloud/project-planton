# The identity server's emails name the install in the header and carry the alert and quote shapes

## What changed

- **`operator/internal/keycloaklogintheme/theme/email/html/template.ftl` is regenerated from the platform's email layout.** The header now sets the install's console host under the wordmark, so a person who runs two installs (or a staging beside a production) knows which one sent the message at the top of it, not only in the footer. The footer's reply-to is a `mailto:` link. The layout's stylesheet carries the dark-mode colors for the shapes the platform's emails gained (a left-edge notice with a glyph in the edge color, a quote block, an unpadded inline literal), so a message body composed with them renders the same in a client that honors `prefers-color-scheme`.
- The `email/text/template.ftl` twin is unchanged in content; the theme package's tests pass.

## Why

The platform and the identity server share one email design by construction: the platform's layout is compiled into this theme's FreeMarker macros, so a change made for the invitation and alert emails reaches the password-reset and account-action emails without a second author. The header change answers the first thing a reader checks ("which install sent this") in the header, where mail clients show it before the body; the added shapes keep the two senders' vocabularies identical for the messages that will use them.
