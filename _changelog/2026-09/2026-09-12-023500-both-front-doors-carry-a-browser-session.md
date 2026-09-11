# Both front doors carry a browser session

## What changed

- **The in-cluster gateway raises nginx's header buffers.** The identity and console routes of the operator-rendered gateway (`operator/internal/resources/gateway.go`) set `proxy_buffer_size 32k` together with `proxy_buffers 4 32k` and `proxy_busy_buffers_size 64k` (nginx checks the three against each other at parse time and refuses to start when only the first is raised), and the server block sets `large_client_header_buffers 4 32k`. The console's sign-in callback answers with a session cookie larger than nginx's 8k default response-header buffer; below it nginx answered `502 upstream sent too big header` before the browser could hand the sign-in ticket back to the CLI, so `planton login` on a private (localhost) install never completed. The browser then sends the same cookie back on every request, which the client-header buffers now admit.
- **The Ingress door sets the same fact.** When ingress-nginx is detected, the operator's Ingress annotations gain `nginx.ingress.kubernetes.io/proxy-buffer-size: 32k` beside the existing buffering, timeout, and body-size annotations (`operator/internal/resources/ingress.go`); ingress-nginx's default is 4k, so a hostname install failed the same callback the same way. A user's own annotation still wins on conflict, as before. Gateway API `HTTPRoute` installs have no nginx in the path and are unaffected. Ingress-nginx's request-side buffer is a controller-wide setting, not a per-Ingress annotation, so that side is documented rather than rendered.

## Why

An install's front door must carry the session its own console issues. `proxy_buffering off` (already set for gRPC-Web streaming) does not enlarge the buffer nginx reads a response header into; `proxy_buffer_size` does, and it is the one directive that matters here. Rendering the fact on both doors follows the shape the operator already keeps between `gateway.go` and `ingress.go`: one front-door truth, expressed once per door.

## How to check

- `cd operator && go test ./internal/resources/` (the gateway test asserts both directives; the Ingress test asserts the annotation and that a user's override still wins).
- On a running platform with the tree operator: `planton login` completes through the browser; the gateway log shows no `upstream sent too big header` on `/api/device/auth/callback`.
