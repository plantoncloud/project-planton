# A self-hosted platform routes native gRPC by content type and tells device clients what it is

## What changed

- **The front-door route table gains a native gRPC row.** A request at the root prefix whose `content-type` is exactly `application/grpc` or `application/grpc+proto` is delivered to the control plane's raw gRPC Service port (`grpc`, the Netty server), not to the console. On the Gateway API door this renders as one HTTPRoute rule with two matches, each the root `PathPrefix` paired with an `Exact` header match -- core Gateway API, no implementation-specific syntax. The Gateway API's own precedence (longer prefix first, then the rule with more header matches) keeps `/rpc`, `/storage`, and `/idp` ahead of it and puts it ahead of the console catch-all. Both control-plane doors carry the disabled request timeout, because server streams flow on both.
- **The Ingress object and the built-in nginx gateway skip the row, and say so.** Neither has a portable header match. Rendering the row path-only would send every console page to the gRPC port, so those doors render the four path rules exactly as before; a native client behind them port-forwards the control plane Service's `grpc` port. The operator README's new "The Front Door" section states the split.
- **The console is told two facts to publish in its device discovery document.** `PLANTON_DEPLOYMENT_KIND` (`self_hosted`, the same declared fact the control plane already boots with -- one constant now, `DeploymentKindSelfHosted`) on every install, and `GRPC_ENDPOINT` (the public host on the public URL's port; `GRPCEndpoint(publicURL)`) only when the front door is the Gateway API edge, so the document never advertises an address that would answer with a console page. Both names join the boot-contract fixture; an older console ignores them, so the platform floor does not move.
- **The control plane's named ports are constants.** `controlPlaneGrpcPortName` and `controlPlaneGrpcWebPortName` replace the literals in the container ports, the Service, and the route table, so the doors and the Service can never disagree on a name.

## Why

The CLI, the runner, and every other native gRPC client dial a single-segment path fixed by the protocol; the route table's own comment explained why no portable path rule could tell that from a console page, and left native clients with no door at all on a self-hosted install. Measured on a live Gateway API install: native gRPC at the root returned the console's 404; framed gRPC pushed through the gRPC-Web door came back without trailers (`server closed the stream without sending trailers`), so a client-side workaround was not available either. The header a gRPC client always sends is the portable discriminator the table had not used. Publishing the deployment kind and the gRPC address from the deployment itself is what lets a client stop reading meaning into hostnames: `planton.planton.ai` and `dev.planton.ai` can both live under one domain and be different kinds, and the client learns which from the instance.

## How to check

```bash
cd operator && go build ./... && go test ./internal/...      # route table, HTTPRoute/Ingress/nginx renderers, console env, controller edge suite, boot contract
go test ./internal/resources -run 'TestHTTPRouteRoutesNativeGRPCByContentType|TestGRPCEndpoint|TestConsoleDeployment_DeviceDiscoveryFacts'
```

Live, after the operator upgrade on a Gateway API install: `kubectl get httproute <platform>-ingress -o yaml` shows the header-matched rule before the console catch-all, and a grpc-go health check against `<hostname>:443` with no path prefix returns `SERVING` with trailers.
