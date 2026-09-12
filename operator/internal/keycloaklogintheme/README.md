# keycloak-login-theme

The Planton design system translated to the self-hosted identity server: the sign-in form, forced password update, profile completion, error, and logout screens, and the emails the identity server sends (a password reset, a verification link, an administrator's required action, the admin console's test message). On a self-hosted install these ARE the product's front door: the console's own `/login` route is only a redirect interstitial, so the identity server's login page is what every teammate on an adopting team sees every working day, and its password-reset email is often the first email a teammate receives from the install.

## How Keycloak theming works (primer)

Keycloak renders every user-facing screen server-side from a **theme**: a named directory of static files under `/opt/keycloak/themes/<name>/`, split by type (`login/`, `account/`, `email/`). A theme is **not code** -- it is:

1. **`theme.properties`** per type -- the manifest. Its `parent=` line is the core mechanism: a theme extends a built-in one and overrides only what it wants. Everything not overridden keeps the parent's templates and styling. Any other key in the manifest is a **theme property** the type's FreeMarker templates can read as `properties.<key>`.
2. **CSS + images + fonts** under `resources/` -- for a visual reskin of the pages, all that is needed.
3. **FreeMarker templates** (`html/*.ftl`, `text/*.ftl`, `messages/*.properties`) -- the emails have no stylesheet to reskin, so they are templates by nature; the login pages deliberately override none (overridden page templates rot when Keycloak upgrades, and CSS reached everything).

One realm setting -- `loginTheme` -- selects the theme for the login pages; the server-wide `--spi-theme--default=planton` flag the operator passes selects it for every other type, email included. Without an `email` type in this theme the server would fail to render any message at all, which is why the type exists even where it inherits.

## The two types this theme ships

**`login/`** extends `keycloak.v2` (Keycloak's bundled PatternFly-v5 login theme): every login-family screen Keycloak can ever serve stays functional and styled, and this theme lays the Planton palette on top. A from-scratch theme (`parent=base`) would leave any screen nobody thought to override as unstyled raw HTML -- on exactly the surface where an unstyled page reads as "something is broken". Two verified-against-26.3 subtleties encoded in `theme/login/theme.properties`: `styles=` entries resolve child-first, falling back to the parent, so the list re-declares the parent's `css/styles.css` and appends ours; and `darkMode=false` pins the page to the Planton dark design (dark by design, matching the console's default -- not by OS accident).

**`email/`** extends `keycloak` (the bundled email type) and carries the product's design for the messages:

- `html/template.ftl` and `text/template.ftl` are **generated**, never edited here. The platform repository authors every email the product sends as React Email (`product/libs/typescript/infra/email-templates`) and compiles the shared layout and its atoms into these FreeMarker macros (`emailLayout`, `heading`, `paragraph`, `muted`, `button`, `fallbackLink`, `facts`, `fact`, `code`, `textLayout`). An invitation from the control plane and a password reset from the identity server therefore wear one frame by construction. The platform's `make email-templates` writes both files (the header line names the generator); a change to the design is a change there.
- `html/*.ftl` and `text/*.ftl` for the messages this theme authors (`password-reset`, `executeActions`, `email-verification`, `email-test`) compose those macros in the product's voice; Keycloak renders both parts of every email. Messages this theme does not author (the event notifications) inherit the parent's body inside the generated frame, because the parent's templates import `template.ftl` and child-theme files resolve first.
- `messages/messages_en.properties` overrides the subjects and inbox preview lines; every other message key keeps Keycloak's default.
- `theme.properties` is the one file **rendered per install**: `Files(facts)` appends the install's `brandName`, `consoleUrl`, `consoleHost`, and `replyTo` as theme properties, and the templates read them (`${properties.brandName}`). That is how a password-reset email names the install that sent it and offers its reply-to for questions. The operator fills the facts from the platform resource on every pass (the sending name from `spec.email.from.name`, the front door's URL, `spec.email.replyTo`), and `Hash(facts)` changes with them, so a renamed sender rolls the identity pod and reaches the next email.

## Why delivery is a ConfigMap, not a custom image

The operator materializes `Files(facts)` into a ConfigMap mounted at `/opt/keycloak/themes/planton/`. The alternative -- baking the theme into a custom Keycloak image -- was rejected deliberately: the identity server is a security-critical component, and adopters should be able to verify that what runs is the **official, pinned, unmodified upstream image**. A theme tweak is an operator release, never an identity-server rebuild, and `Hash(facts)` lets the operator roll the pod when the theme or the facts change (Keycloak caches themes; a restart is what makes a new version take effect).

## The token-translation contract

Every color in `theme/login/resources/css/planton.css` is a **deliberate, documented copy** of the console's design tokens (the platform's theme package). Generating CSS from the TypeScript tokens would mean a cross-language build pipeline for a dozen hex values that change rarely -- disproportionate. The contract instead: **change a token in the console theme, change it here in the same commit.** `theme_test.go` pins the load-bearing values byte-exact so drift fails a test instead of shipping. The email layout is the exception that proves the rule: it IS generated from the platform's tokens, because it is a whole design, not a dozen values.

One value flows the other way on purpose: `--planton-focus-border` (`#696741`) is the design system's single non-monochrome accent, defined in the console theme as the focused-input border and reused here so "you are typing here" looks identical on both surfaces.

## Layout rule: a real directory, embedded whole

`theme/` is the theme exactly as Keycloak lays it out on disk, embedded with `go:embed`. Every file under it ships; there is no allowlist to keep in sync, each file is edited in its own language (CSS, SVG, FreeMarker, properties), and the platform's generator writes a file, not a Go constant. **The directory listing is the manifest of what ships into Keycloak.** Bundling the font (`inter-latin.woff2`, licensed under the [SIL Open Font License 1.1](https://github.com/rsms/inter/blob/master/LICENSE.txt)) keeps the sign-in page fetching **nothing** from third parties -- no font-CDN call from a credential screen, identical rendering air-gapped; the email layout likewise carries no image and no remote font.

## API

- `ThemeName` -- the theme directory / `loginTheme` value (`planton`).
- `EmailFacts` -- what the emails say about the install: `BrandName`, `ConsoleURL` (`ConsoleHost()` derives the host), `ReplyTo`.
- `Files(facts) map[string][]byte` -- every file, keyed by path under the theme root (`login/...`, `email/...`), with the email manifest rendered for the install. Kubernetes-free by design; the operator owns the ConfigMap shape.
- `Hash(facts) string` -- deterministic content fingerprint for restart-on-change.

## Proof

`theme_test.go` pins the artifacts, the tokens, the rendered facts, and the shape of every generated and authored template. `internal/keycloak`'s Docker-gated suite boots a real Keycloak 26.3 with this theme mounted and the flag set, sends an account-action email through a real relay, and reads the rendered message back from the inbox: the layout is FreeMarker the server accepts, the facts reach the words, and both parts arrive (`make test-realm-convergence` runs it with the rest).

## Consumers

- the operator's identity component mounts the theme, renders the facts, and selects the theme in the realm import and the server flag.
