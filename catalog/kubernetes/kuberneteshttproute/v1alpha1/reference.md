# KubernetesHttpRoute

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

KubernetesHttpRouteSpec defines a Kubernetes Gateway API HTTPRoute: a
namespaced set of rules that match HTTP requests (by hostname, path, header,
query param, or method), optionally transform them with filters, and forward
them to backend Services. Routes attach to a Gateway through parent_refs.

100% fidelity with the upstream Gateway API v1.6.1 HTTPRouteSpec
(kubernetes-sigs/gateway-api apis/v1/httproute_types.go), standard channel.
Upstream spec fields are flattened after the Planton namespaced envelope
(namespace). Experimental fields are intentionally excluded
because they are absent from the standard-channel CRD and the typed Pulumi
resource Planton provisions with, so they would have no deployable target:
  - CommonRouteSpec.useDefaultGateways
  - HTTPRouteRule.retry and HTTPRouteRule.sessionPersistence
  - the ExternalAuth filter variant (and its sub-config)

## Example

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesHttpRoute
metadata:
  name: test-route
spec:
  namespace:
    value: test-namespace
  parentRefs:
    - name:
        value: test-gateway
      sectionName: https
      port: 443
  hostnames:
    - app.example.com
  rules:
    - name: primary
      matches:
        - path:
            type: PathPrefix
            value: /api
          headers:
            - type: Exact
              name: x-tenant
              value: acme
          queryParams:
            - type: Exact
              name: version
              value: v2
          method: GET
      filters:
        - type: RequestHeaderModifier
          requestHeaderModifier:
            set:
              - name: x-forwarded-tier
                value: production
            add:
              - name: x-trace
                value: enabled
            remove:
              - x-legacy
        - type: RequestMirror
          requestMirror:
            backendRef:
              name:
                value: audit-svc
              port: 8080
            percent: 25
        - type: CORS
          cors:
            allowOrigins:
              - https://console.example.com
            allowCredentials: true
            allowMethods:
              - GET
              - POST
            allowHeaders:
              - authorization
            exposeHeaders:
              - x-request-id
            maxAge: 600
      backendRefs:
        - name:
            value: web
          port: 80
          weight: 90
        - name:
            value: web-canary
          port: 80
          weight: 10
          filters:
            - type: ResponseHeaderModifier
              responseHeaderModifier:
                add:
                  - name: x-canary
                    value: "true"
      timeouts:
        request: 30s
        backendRequest: 10s
    - name: redirect-old-host
      matches:
        - path:
            type: PathPrefix
            value: /old
      filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            hostname: app.example.com
            path:
              type: ReplacePrefixMatch
              replacePrefixMatch: /new
            statusCode: 308
    - name: rewrite
      matches:
        - path:
            type: PathPrefix
            value: /legacy
      filters:
        - type: URLRewrite
          urlRewrite:
            hostname: internal.example.com
            path:
              type: ReplacePrefixMatch
              replacePrefixMatch: /v2
      backendRefs:
        - name:
            value: legacy-adapter
          port: 8080
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.parentRefs` | `[]KubernetesGatewayApiParentReference` |  |  |  |
| `spec.parentRefs[].group` | `string` |  |  |  |
| `spec.parentRefs[].kind` | `string` | yes |  |  |
| `spec.parentRefs[].namespace` | `string` |  |  |  |
| `spec.parentRefs[].name` | `string \| valueFrom` | yes |  | KubernetesGateway (`status.outputs.gateway_name`) |
| `spec.parentRefs[].sectionName` | `string` | yes |  |  |
| `spec.parentRefs[].port` | `int32` |  |  |  |
| `spec.hostnames` | `[]string` |  |  |  |
| `spec.rules` | `[]KubernetesHttpRouteRule` | yes |  |  |
| `spec.rules[].name` | `string` | yes |  |  |
| `spec.rules[].matches` | `[]KubernetesHttpRouteMatch` |  |  |  |
| `spec.rules[].matches[].path` | `KubernetesHttpRoutePathMatch` |  |  |  |
| `spec.rules[].matches[].path.type` | `string` |  |  |  |
| `spec.rules[].matches[].path.value` | `string` |  |  |  |
| `spec.rules[].matches[].headers` | `[]KubernetesHttpRouteHeaderMatch` |  |  |  |
| `spec.rules[].matches[].headers[].type` | `string` |  |  |  |
| `spec.rules[].matches[].headers[].name` | `string` | yes |  |  |
| `spec.rules[].matches[].headers[].value` | `string` | yes |  |  |
| `spec.rules[].matches[].queryParams` | `[]KubernetesHttpRouteQueryParamMatch` |  |  |  |
| `spec.rules[].matches[].queryParams[].type` | `string` |  |  |  |
| `spec.rules[].matches[].queryParams[].name` | `string` | yes |  |  |
| `spec.rules[].matches[].queryParams[].value` | `string` | yes |  |  |
| `spec.rules[].matches[].method` | `string` |  |  |  |
| `spec.rules[].filters` | `[]KubernetesHttpRouteFilter` |  |  |  |
| `spec.rules[].filters[].type` | `string` | yes |  |  |
| `spec.rules[].filters[].requestHeaderModifier` | `KubernetesHttpRouteHeaderFilter` |  |  |  |
| `spec.rules[].filters[].requestHeaderModifier.set` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].filters[].requestHeaderModifier.set[].name` | `string` | yes |  |  |
| `spec.rules[].filters[].requestHeaderModifier.set[].value` | `string` | yes |  |  |
| `spec.rules[].filters[].requestHeaderModifier.add` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].filters[].requestHeaderModifier.add[].name` | `string` | yes |  |  |
| `spec.rules[].filters[].requestHeaderModifier.add[].value` | `string` | yes |  |  |
| `spec.rules[].filters[].requestHeaderModifier.remove` | `[]string` |  |  |  |
| `spec.rules[].filters[].responseHeaderModifier` | `KubernetesHttpRouteHeaderFilter` |  |  |  |
| `spec.rules[].filters[].responseHeaderModifier.set` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].filters[].responseHeaderModifier.set[].name` | `string` | yes |  |  |
| `spec.rules[].filters[].responseHeaderModifier.set[].value` | `string` | yes |  |  |
| `spec.rules[].filters[].responseHeaderModifier.add` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].filters[].responseHeaderModifier.add[].name` | `string` | yes |  |  |
| `spec.rules[].filters[].responseHeaderModifier.add[].value` | `string` | yes |  |  |
| `spec.rules[].filters[].responseHeaderModifier.remove` | `[]string` |  |  |  |
| `spec.rules[].filters[].requestMirror` | `KubernetesHttpRouteRequestMirrorFilter` |  |  |  |
| `spec.rules[].filters[].requestMirror.backendRef` | `KubernetesGatewayApiBackendObjectReference` | yes |  |  |
| `spec.rules[].filters[].requestMirror.backendRef.group` | `string` |  |  |  |
| `spec.rules[].filters[].requestMirror.backendRef.kind` | `string` | yes |  |  |
| `spec.rules[].filters[].requestMirror.backendRef.name` | `string \| valueFrom` | yes |  | KubernetesService (`status.outputs.service_name`) |
| `spec.rules[].filters[].requestMirror.backendRef.namespace` | `string` |  |  |  |
| `spec.rules[].filters[].requestMirror.backendRef.port` | `int32` |  |  |  |
| `spec.rules[].filters[].requestMirror.percent` | `int32` |  |  |  |
| `spec.rules[].filters[].requestMirror.fraction` | `KubernetesGatewayApiFraction` |  |  |  |
| `spec.rules[].filters[].requestMirror.fraction.numerator` | `int32` |  |  |  |
| `spec.rules[].filters[].requestMirror.fraction.denominator` | `int32` |  |  |  |
| `spec.rules[].filters[].requestRedirect` | `KubernetesHttpRouteRequestRedirectFilter` |  |  |  |
| `spec.rules[].filters[].requestRedirect.scheme` | `string` |  |  |  |
| `spec.rules[].filters[].requestRedirect.hostname` | `string` | yes |  |  |
| `spec.rules[].filters[].requestRedirect.path` | `KubernetesHttpRoutePathModifier` |  |  |  |
| `spec.rules[].filters[].requestRedirect.path.type` | `string` | yes |  |  |
| `spec.rules[].filters[].requestRedirect.path.replaceFullPath` | `string` |  |  |  |
| `spec.rules[].filters[].requestRedirect.path.replacePrefixMatch` | `string` |  |  |  |
| `spec.rules[].filters[].requestRedirect.port` | `int32` |  |  |  |
| `spec.rules[].filters[].requestRedirect.statusCode` | `int32` |  |  |  |
| `spec.rules[].filters[].urlRewrite` | `KubernetesHttpRouteUrlRewriteFilter` |  |  |  |
| `spec.rules[].filters[].urlRewrite.hostname` | `string` | yes |  |  |
| `spec.rules[].filters[].urlRewrite.path` | `KubernetesHttpRoutePathModifier` |  |  |  |
| `spec.rules[].filters[].urlRewrite.path.type` | `string` | yes |  |  |
| `spec.rules[].filters[].urlRewrite.path.replaceFullPath` | `string` |  |  |  |
| `spec.rules[].filters[].urlRewrite.path.replacePrefixMatch` | `string` |  |  |  |
| `spec.rules[].filters[].cors` | `KubernetesHttpRouteCorsFilter` |  |  |  |
| `spec.rules[].filters[].cors.allowOrigins` | `[]string` |  |  |  |
| `spec.rules[].filters[].cors.allowCredentials` | `bool` |  |  |  |
| `spec.rules[].filters[].cors.allowMethods` | `[]string` |  |  |  |
| `spec.rules[].filters[].cors.allowHeaders` | `[]string` |  |  |  |
| `spec.rules[].filters[].cors.exposeHeaders` | `[]string` |  |  |  |
| `spec.rules[].filters[].cors.maxAge` | `int32` |  |  |  |
| `spec.rules[].filters[].extensionRef` | `KubernetesGatewayApiLocalObjectReference` |  |  |  |
| `spec.rules[].filters[].extensionRef.group` | `string` | yes |  |  |
| `spec.rules[].filters[].extensionRef.kind` | `string` | yes |  |  |
| `spec.rules[].filters[].extensionRef.name` | `string` | yes |  |  |
| `spec.rules[].backendRefs` | `[]KubernetesHttpRouteBackendRef` |  |  |  |
| `spec.rules[].backendRefs[].group` | `string` |  |  |  |
| `spec.rules[].backendRefs[].kind` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].name` | `string \| valueFrom` | yes |  | KubernetesService (`status.outputs.service_name`) |
| `spec.rules[].backendRefs[].namespace` | `string` |  |  |  |
| `spec.rules[].backendRefs[].port` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].weight` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters` | `[]KubernetesHttpRouteFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].type` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier` | `KubernetesHttpRouteHeaderFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.set` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.set[].name` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.set[].value` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.add` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.add[].name` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.add[].value` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestHeaderModifier.remove` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier` | `KubernetesHttpRouteHeaderFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.set` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.set[].name` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.set[].value` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.add` | `[]KubernetesHttpRouteHeader` |  |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.add[].name` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.add[].value` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].responseHeaderModifier.remove` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror` | `KubernetesHttpRouteRequestMirrorFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef` | `KubernetesGatewayApiBackendObjectReference` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.group` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.kind` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.name` | `string \| valueFrom` | yes |  | KubernetesService (`status.outputs.service_name`) |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.namespace` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.port` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.percent` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.fraction` | `KubernetesGatewayApiFraction` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.fraction.numerator` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestMirror.fraction.denominator` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect` | `KubernetesHttpRouteRequestRedirectFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.scheme` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.hostname` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.path` | `KubernetesHttpRoutePathModifier` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.path.type` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.path.replaceFullPath` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.path.replacePrefixMatch` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.port` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].requestRedirect.statusCode` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite` | `KubernetesHttpRouteUrlRewriteFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite.hostname` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite.path` | `KubernetesHttpRoutePathModifier` |  |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite.path.type` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite.path.replaceFullPath` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].urlRewrite.path.replacePrefixMatch` | `string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors` | `KubernetesHttpRouteCorsFilter` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.allowOrigins` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.allowCredentials` | `bool` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.allowMethods` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.allowHeaders` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.exposeHeaders` | `[]string` |  |  |  |
| `spec.rules[].backendRefs[].filters[].cors.maxAge` | `int32` |  |  |  |
| `spec.rules[].backendRefs[].filters[].extensionRef` | `KubernetesGatewayApiLocalObjectReference` |  |  |  |
| `spec.rules[].backendRefs[].filters[].extensionRef.group` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].extensionRef.kind` | `string` | yes |  |  |
| `spec.rules[].backendRefs[].filters[].extensionRef.name` | `string` | yes |  |  |
| `spec.rules[].timeouts` | `KubernetesHttpRouteTimeouts` |  |  |  |
| `spec.rules[].timeouts.request` | `string` |  |  |  |
| `spec.rules[].timeouts.backendRequest` | `string` |  |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

Namespace in which the HTTPRoute is created. Backends in other namespaces,
and Gateways in other namespaces, are subject to the usual same-namespace /
ReferenceGrant rules.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.parentRefs

`[]KubernetesGatewayApiParentReference`

References to the parent resources (usually Gateways) this route attaches
to. Each parent must allow attachment from HTTPRoutes of this namespace.
Each reference's name defaults to a KubernetesGateway foreign key — wire
it with valueFrom in an infra chart and the route deploys after its
Gateway.

Flattened from the upstream CommonRouteSpec; the experimental
useDefaultGateways field is excluded (standard channel only).

- rule: {"repeated":{"maxItems":"32"}}

### spec.parentRefs[].group

`string` · optional (explicit presence)

Group of the referent.
When unspecified, "gateway.networking.k8s.io" is inferred.
Set to "" (empty string) for the core API group.

Upstream default: "gateway.networking.k8s.io"
Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.parentRefs[].kind

`string` · required · optional (explicit presence)

Kind of the referent.
Core parent kinds: Gateway (Gateway conformance), Service (Mesh conformance).

Upstream default: "Gateway"
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.parentRefs[].namespace

`string` · optional (explicit presence)

Namespace of the referent. When unspecified, refers to the local
namespace of the Route. Cross-namespace references require a
ReferenceGrant in the target namespace.

### spec.parentRefs[].name

`string | valueFrom` · required

Name of the referent. Defaults to a KubernetesGateway foreign key: in an
infra chart, wire it with valueFrom against the Gateway resource and the
route deploys after (and follows renames of) its Gateway. When the parent
is not a Planton-managed Gateway (a ListenerSet, a mesh Service, or an
externally created Gateway), pass the literal name with `value:`.

- references: KubernetesGateway (`status.outputs.gateway_name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesGateway, name: <that resource's name>, fieldPath: status.outputs.gateway_name}} -- a bare string does not parse

### spec.parentRefs[].sectionName

`string` · required · optional (explicit presence)

Name of a section within the target resource (e.g., a Gateway Listener
name). When unspecified, references the entire resource.

SectionName pattern: 1-253 chars, RFC 1123 subdomain
^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.parentRefs[].port

`int32` · optional (explicit presence)

Network port this Route targets on the parent resource.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.hostnames

`[]string`

Hostnames matched against the HTTP Host header to select this route. A
leading wildcard label (for example "*.example.com") is a suffix match.
When empty, the route matches any hostname permitted by its parent
listeners. Upstream support level: Core.

Each value is an RFC 1123 hostname (IPs not allowed). Pattern (wildcard
prefix allowed): ^(\*\.)?[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"repeated":{"maxItems":"16","items":{"string":{"minLen":"1","maxLen":"253","pattern":"^(\\*\\.)?[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}}}

### spec.rules

`[]KubernetesHttpRouteRule` · required

Routing rules: each rule matches requests, optionally applies filters, and
forwards to one or more backends. At least one rule is required. Upstream
allows up to 16 rules (and up to 64 matches per rule).

Note: upstream also carries an aggregate "total matches across all rules
must be < 128" XValidation expressed as a 16-way unrolled CEL sum. It is
intentionally not translated here: it is a controller-enforced aggregate
limit, the per-field rules<=16 / matches<=64 bounds are enforced, and the
unrolled expression is unmaintainable. (KubernetesGrpcRoute makes the same
choice; do not "fix" this by adding the unrolled rule.)

- rule: {"repeated":{"minItems":"1","maxItems":"16"}}
- rule: a rule using a RequestRedirect filter must not also specify backend_refs
- rule: a rule may use either a RequestRedirect or a URLRewrite filter, but not both
- rule: the RequestHeaderModifier filter may be specified at most once per rule
- rule: the ResponseHeaderModifier filter may be specified at most once per rule
- rule: the RequestRedirect filter may be specified at most once per rule
- rule: the URLRewrite filter may be specified at most once per rule
- rule: the CORS filter may be specified at most once per rule
- rule: a RequestRedirect filter using path.replace_prefix_match requires the rule to have exactly one match whose path type is PathPrefix
- rule: a URLRewrite filter using path.replace_prefix_match requires the rule to have exactly one match whose path type is PathPrefix

### spec.rules[].name

`string` · required · optional (explicit presence)

Name of the route rule; must be unique within the route if set. Upstream
support level: Extended.

Upstream SectionName constraints: 1-253 chars, lowercase RFC 1123 subdomain.
Pattern: ^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].matches

`[]KubernetesHttpRouteMatch`

Conditions for matching requests. A request matches the rule if ANY one of
the matches is satisfied (matches are ORed; conditions within a match are
ANDed). When empty, the controller defaults to a PathPrefix match on "/".

- rule: {"repeated":{"maxItems":"64"}}
- rule: each header match name must be unique within a match
- rule: each query parameter match name must be unique within a match

### spec.rules[].matches[].path

`KubernetesHttpRoutePathMatch`

Request path matcher. When unset, the controller defaults to a PathPrefix
match on "/".

- rule: for Exact and PathPrefix matches, value must be an absolute path beginning with '/'
- rule: for Exact and PathPrefix matches, value must not contain '//', '/./', or '/../' segments
- rule: for Exact and PathPrefix matches, value must not contain '#', '%2f', or '%2F'
- rule: for Exact and PathPrefix matches, value must not end with '/.' or '/..'
- rule: for Exact and PathPrefix matches, value may only contain valid path characters (unreserved, sub-delims, ':@/' or percent-encoded octets)

### spec.rules[].matches[].path.type

`string` · optional (explicit presence)

How the path is compared. "Exact" and "PathPrefix" (default) have Core
support; "RegularExpression" is implementation-specific.

Upstream default: "PathPrefix". Closed enum.

- rule: path match type must be one of 'Exact', 'PathPrefix', or 'RegularExpression'

### spec.rules[].matches[].path.value

`string` · optional (explicit presence)

Path value to match against.

Upstream default: "/".

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].matches[].headers

`[]KubernetesHttpRouteHeaderMatch`

Request header matchers. A request must match all listed headers.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].matches[].headers[].type

`string` · optional (explicit presence)

How the header value is compared. "Exact" (default) has Core support;
"RegularExpression" is implementation-specific.

Upstream default: "Exact". Closed enum.

- rule: header match type must be either 'Exact' or 'RegularExpression'

### spec.rules[].matches[].headers[].name

`string` · required

Name of the header to match (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].matches[].headers[].value

`string` · required

Value the header is matched against.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].matches[].queryParams

`[]KubernetesHttpRouteQueryParamMatch`

Request query-parameter matchers. A request must match all listed params.
Upstream support level: Extended.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].matches[].queryParams[].type

`string` · optional (explicit presence)

How the query-param value is compared. "Exact" (default) has Extended
support; "RegularExpression" is implementation-specific.

Upstream default: "Exact". Closed enum.

- rule: query parameter match type must be either 'Exact' or 'RegularExpression'

### spec.rules[].matches[].queryParams[].name

`string` · required

Name of the query parameter to match (exact string match).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].matches[].queryParams[].value

`string` · required

Value the query parameter is matched against.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"1024"}}

### spec.rules[].matches[].method

`string` · optional (explicit presence)

HTTP method matcher. When set, the request method must equal this value.
Upstream support level: Extended.

Closed enum (external standard exception -- matches the HTTP method tokens).

- rule: method must be one of GET, HEAD, POST, PUT, DELETE, CONNECT, OPTIONS, TRACE, or PATCH

### spec.rules[].filters

`[]KubernetesHttpRouteFilter`

Filters applied to requests matching this rule (header modification,
redirect, rewrite, mirror, CORS, or an implementation-specific extension).

- rule: {"repeated":{"maxItems":"16"}}
- rule: request_header_modifier must be set when type is 'RequestHeaderModifier' and must be unset otherwise
- rule: response_header_modifier must be set when type is 'ResponseHeaderModifier' and must be unset otherwise
- rule: request_mirror must be set when type is 'RequestMirror' and must be unset otherwise
- rule: request_redirect must be set when type is 'RequestRedirect' and must be unset otherwise
- rule: url_rewrite must be set when type is 'URLRewrite' and must be unset otherwise
- rule: cors must be set when type is 'CORS' and must be unset otherwise
- rule: extension_ref must be set when type is 'ExtensionRef' and must be unset otherwise

### spec.rules[].filters[].type

`string` · required

Which filter to apply. Selects exactly one configuration field below.

Closed enum (external standard exception -- matches Gateway API filter type
constants). The experimental "ExternalAuth" type is excluded.

- rule: filter type must be one of 'RequestHeaderModifier', 'ResponseHeaderModifier', 'RequestMirror', 'RequestRedirect', 'URLRewrite', 'CORS', or 'ExtensionRef'
- rule: {"required":true}

### spec.rules[].filters[].requestHeaderModifier

`KubernetesHttpRouteHeaderFilter`

Modifies request headers before forwarding. Set when type is
"RequestHeaderModifier". Upstream support level: Core.

- rule: each header name in 'set' must be unique
- rule: each header name in 'add' must be unique

### spec.rules[].filters[].requestHeaderModifier.set

`[]KubernetesHttpRouteHeader`

Headers to overwrite (set) on the message, replacing any existing values.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].filters[].requestHeaderModifier.set[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].filters[].requestHeaderModifier.set[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].filters[].requestHeaderModifier.add

`[]KubernetesHttpRouteHeader`

Headers to append, adding to any existing values for the same name.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].filters[].requestHeaderModifier.add[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].filters[].requestHeaderModifier.add[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].filters[].requestHeaderModifier.remove

`[]string`

Header names to remove from the message.

- rule: {"repeated":{"maxItems":"16","unique":true}}

### spec.rules[].filters[].responseHeaderModifier

`KubernetesHttpRouteHeaderFilter`

Modifies response headers before returning to the client. Set when type is
"ResponseHeaderModifier". Upstream support level: Extended.

- rule: each header name in 'set' must be unique
- rule: each header name in 'add' must be unique

### spec.rules[].filters[].responseHeaderModifier.set

`[]KubernetesHttpRouteHeader`

Headers to overwrite (set) on the message, replacing any existing values.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].filters[].responseHeaderModifier.set[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].filters[].responseHeaderModifier.set[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].filters[].responseHeaderModifier.add

`[]KubernetesHttpRouteHeader`

Headers to append, adding to any existing values for the same name.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].filters[].responseHeaderModifier.add[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].filters[].responseHeaderModifier.add[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].filters[].responseHeaderModifier.remove

`[]string`

Header names to remove from the message.

- rule: {"repeated":{"maxItems":"16","unique":true}}

### spec.rules[].filters[].requestMirror

`KubernetesHttpRouteRequestMirrorFilter`

Mirrors the request to an additional backend (responses ignored). Set when
type is "RequestMirror". Upstream support level: Extended.

- rule: specify either percent or fraction for the mirror sampling rate, but not both

### spec.rules[].filters[].requestMirror.backendRef

`KubernetesGatewayApiBackendObjectReference` · required

Backend that mirrored requests are sent to. Required. Upstream support
level: Extended for Kubernetes Service.

- rule: {"required":true}
- rule: port must be specified when referencing a core API Service (group is empty and kind is 'Service')

### spec.rules[].filters[].requestMirror.backendRef.group

`string` · optional (explicit presence)

Group of the referent. Empty string infers the core API group.

Upstream default: "" (core API group)
Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].filters[].requestMirror.backendRef.kind

`string` · required · optional (explicit presence)

Kind of the referent.

Upstream default: "Service"
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.rules[].filters[].requestMirror.backendRef.name

`string | valueFrom` · required

Name of the referent. Defaults to a KubernetesService foreign key: in an
infra chart, wire it with valueFrom against the backend Service and the
route deploys after its backend. When the backend is not a
Planton-managed Service (a custom backend kind, or a Service created
outside Planton), pass the literal name with `value:`.

- references: KubernetesService (`status.outputs.service_name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesService, name: <that resource's name>, fieldPath: status.outputs.service_name}} -- a bare string does not parse

### spec.rules[].filters[].requestMirror.backendRef.namespace

`string` · optional (explicit presence)

Namespace of the backend. When unspecified, the local namespace is inferred.
Cross-namespace references require a ReferenceGrant.

### spec.rules[].filters[].requestMirror.backendRef.port

`int32` · optional (explicit presence)

Destination port number. Required when the referent is a core Service.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.rules[].filters[].requestMirror.percent

`int32` · optional (explicit presence)

Percentage of requests to mirror (0-100). When neither percent nor fraction
is set, 100% are mirrored.

- rule: {"int32":{"lte":100,"gte":0}}

### spec.rules[].filters[].requestMirror.fraction

`KubernetesGatewayApiFraction`

Fraction of requests to mirror. When neither percent nor fraction is set,
100% are mirrored.

- rule: numerator must be less than or equal to denominator

### spec.rules[].filters[].requestMirror.fraction.numerator

`int32`

Numerator of the fraction. Must be non-negative.

- rule: {"int32":{"gte":0}}

### spec.rules[].filters[].requestMirror.fraction.denominator

`int32` · optional (explicit presence)

Denominator of the fraction.

Upstream default: 100

- rule: {"int32":{"gte":1}}

### spec.rules[].filters[].requestRedirect

`KubernetesHttpRouteRequestRedirectFilter`

Responds with an HTTP redirect. Set when type is "RequestRedirect". Cannot
be combined with backends or a URLRewrite filter. Upstream support: Core.

### spec.rules[].filters[].requestRedirect.scheme

`string` · optional (explicit presence)

Scheme of the redirect Location. When empty, the request scheme is used.
Upstream support level: Extended.

Closed enum: http | https.

- rule: redirect scheme must be either 'http' or 'https'

### spec.rules[].filters[].requestRedirect.hostname

`string` · required · optional (explicit presence)

Hostname of the redirect Location. When empty, the request Host is used.
Upstream support level: Core.

Upstream PreciseHostname pattern (no wildcard):
^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].filters[].requestRedirect.path

`KubernetesHttpRoutePathModifier`

Path modification applied to construct the redirect Location. Upstream
support level: Extended.

- rule: replace_full_path must be set when type is 'ReplaceFullPath' and must be unset otherwise
- rule: replace_prefix_match must be set when type is 'ReplacePrefixMatch' and must be unset otherwise

### spec.rules[].filters[].requestRedirect.path.type

`string` · required

Kind of path modification.

Closed enum: ReplaceFullPath | ReplacePrefixMatch.

- rule: path modifier type must be either 'ReplaceFullPath' or 'ReplacePrefixMatch'
- rule: {"required":true}

### spec.rules[].filters[].requestRedirect.path.replaceFullPath

`string` · optional (explicit presence)

Value that replaces the entire path. Valid only with ReplaceFullPath.

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].filters[].requestRedirect.path.replacePrefixMatch

`string` · optional (explicit presence)

Value that replaces the matched path prefix. Valid only with
ReplacePrefixMatch (and a PathPrefix match on the same rule).

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].filters[].requestRedirect.port

`int32` · optional (explicit presence)

Port of the redirect Location. When unset, it is derived from the scheme or
the listener port. Upstream support level: Extended.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.rules[].filters[].requestRedirect.statusCode

`int32` · optional (explicit presence)

HTTP status code of the redirect response. Upstream support level: Core.

Upstream default: 302. Closed enum: 301 | 302 | 303 | 307 | 308.

- rule: redirect status_code must be one of 301, 302, 303, 307, or 308

### spec.rules[].filters[].urlRewrite

`KubernetesHttpRouteUrlRewriteFilter`

Rewrites the request line during forwarding. Set when type is "URLRewrite".
Cannot be combined with a RequestRedirect filter. Upstream support:
Extended.

### spec.rules[].filters[].urlRewrite.hostname

`string` · required · optional (explicit presence)

Hostname that replaces the Host header during forwarding. Upstream support
level: Extended.

Upstream PreciseHostname pattern (no wildcard):
^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].filters[].urlRewrite.path

`KubernetesHttpRoutePathModifier`

Path rewrite applied during forwarding. Upstream support level: Extended.

- rule: replace_full_path must be set when type is 'ReplaceFullPath' and must be unset otherwise
- rule: replace_prefix_match must be set when type is 'ReplacePrefixMatch' and must be unset otherwise

### spec.rules[].filters[].urlRewrite.path.type

`string` · required

Kind of path modification.

Closed enum: ReplaceFullPath | ReplacePrefixMatch.

- rule: path modifier type must be either 'ReplaceFullPath' or 'ReplacePrefixMatch'
- rule: {"required":true}

### spec.rules[].filters[].urlRewrite.path.replaceFullPath

`string` · optional (explicit presence)

Value that replaces the entire path. Valid only with ReplaceFullPath.

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].filters[].urlRewrite.path.replacePrefixMatch

`string` · optional (explicit presence)

Value that replaces the matched path prefix. Valid only with
ReplacePrefixMatch (and a PathPrefix match on the same rule).

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].filters[].cors

`KubernetesHttpRouteCorsFilter`

Adds CORS headers to the response. Set when type is "CORS". Upstream
support level: Extended.

### spec.rules[].filters[].cors.allowOrigins

`[]string`

Origins permitted to share the response. Each is a scheme + host with an
optional port; the host may use a leading wildcard. A single "*" allows all
origins. Upstream support level: Extended.

Upstream CORSOrigin pattern:
(^\*$)|(^(http(s)?):\/\/(((\*\.)?([a-zA-Z0-9\-]+\.)*[a-zA-Z0-9-]+|\*)(:([0-9]{1,5}))?)$)

- rule: allow_origins cannot contain '*' alongside other origins
- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"253","pattern":"(^\\*$)|(^(http(s)?):\\/\\/(((\\*\\.)?([a-zA-Z0-9\\-]+\\.)*[a-zA-Z0-9-]+|\\*)(:([0-9]{1,5}))?)$)"}}}}

### spec.rules[].filters[].cors.allowCredentials

`bool` · optional (explicit presence)

Whether cross-origin requests may include credentials. Upstream support
level: Extended.

### spec.rules[].filters[].cors.allowMethods

`[]string`

HTTP methods permitted for cross-origin requests. "*" allows all methods.
Upstream support level: Extended.

- rule: allow_methods may only contain HTTP method tokens or '*'
- rule: allow_methods cannot contain '*' alongside other methods
- rule: {"repeated":{"maxItems":"9","unique":true}}

### spec.rules[].filters[].cors.allowHeaders

`[]string`

Request headers permitted for cross-origin requests. "*" allows all
headers. Upstream support level: Extended.

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$

- rule: allow_headers cannot contain '*' alongside other headers
- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}}}

### spec.rules[].filters[].cors.exposeHeaders

`[]string`

Response headers exposed to client-side scripts. Upstream support level:
Extended.

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$

- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}}}

### spec.rules[].filters[].cors.maxAge

`int32` · optional (explicit presence)

How long (in seconds) clients may cache preflight results.

Upstream default: 5.

- rule: {"int32":{"gte":1}}

### spec.rules[].filters[].extensionRef

`KubernetesGatewayApiLocalObjectReference`

Implementation-specific extension filter. Set when type is "ExtensionRef".
Must not be used for core or extended filters. Upstream support:
Implementation-specific.

### spec.rules[].filters[].extensionRef.group

`string` · required · optional (explicit presence)

Group of the referent (e.g., "gateway.networking.k8s.io").
Empty string infers the core API group.

Upstream requires the KEY to be present, but its Group type explicitly
allows the empty value -- so this is a proto3 `optional` string with a
presence `required` rule: it must be SET (and is therefore emitted to
the rendered CR, whose CRD rejects a missing key) but may be empty.
The `optional` is what keeps the projection faithful: protojson omits
unset proto3 scalars, so a non-optional empty-string group would be
dropped from the manifest and rejected by the API server.

Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"required":true,"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].filters[].extensionRef.kind

`string` · required

Kind of the referent (e.g., "HTTPRoute" or "Service").

Upstream models Kind as a required value type.
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"required":true,"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.rules[].filters[].extensionRef.name

`string` · required

Name of the referent.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"253"}}

### spec.rules[].backendRefs

`[]KubernetesHttpRouteBackendRef`

Backends matching requests are forwarded to. When multiple backends are
given, traffic is split by weight. Upstream support level: Core for
Kubernetes Service.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].backendRefs[].group

`string` · optional (explicit presence)

Group of the referent. Empty string infers the core API group.

Upstream default: "" (core API group).
Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].backendRefs[].kind

`string` · required · optional (explicit presence)

Kind of the referent.

Upstream default: "Service".
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.rules[].backendRefs[].name

`string | valueFrom` · required

Name of the referent. Defaults to a KubernetesService foreign key: in an
infra chart, wire it with valueFrom against the backend Service and the
route deploys after its backend. When the backend is not a
Planton-managed Service, pass the literal name with `value:`.

- references: KubernetesService (`status.outputs.service_name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesService, name: <that resource's name>, fieldPath: status.outputs.service_name}} -- a bare string does not parse

### spec.rules[].backendRefs[].namespace

`string` · optional (explicit presence)

Namespace of the backend. When unspecified, the route's namespace is
inferred. Cross-namespace references require a ReferenceGrant.

### spec.rules[].backendRefs[].port

`int32` · optional (explicit presence)

Destination port number. Required when the referent is a core Service.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.rules[].backendRefs[].weight

`int32` · optional (explicit presence)

Relative weight for traffic splitting, computed as weight/(sum of weights).

Upstream default: 1.

- rule: {"int32":{"lte":1000000,"gte":0}}

### spec.rules[].backendRefs[].filters

`[]KubernetesHttpRouteFilter`

Filters applied only when forwarding to this backend. Upstream support
level: Implementation-specific (prefer rule-level filters for portability).

- rule: {"repeated":{"maxItems":"16"}}
- rule: request_header_modifier must be set when type is 'RequestHeaderModifier' and must be unset otherwise
- rule: response_header_modifier must be set when type is 'ResponseHeaderModifier' and must be unset otherwise
- rule: request_mirror must be set when type is 'RequestMirror' and must be unset otherwise
- rule: request_redirect must be set when type is 'RequestRedirect' and must be unset otherwise
- rule: url_rewrite must be set when type is 'URLRewrite' and must be unset otherwise
- rule: cors must be set when type is 'CORS' and must be unset otherwise
- rule: extension_ref must be set when type is 'ExtensionRef' and must be unset otherwise

### spec.rules[].backendRefs[].filters[].type

`string` · required

Which filter to apply. Selects exactly one configuration field below.

Closed enum (external standard exception -- matches Gateway API filter type
constants). The experimental "ExternalAuth" type is excluded.

- rule: filter type must be one of 'RequestHeaderModifier', 'ResponseHeaderModifier', 'RequestMirror', 'RequestRedirect', 'URLRewrite', 'CORS', or 'ExtensionRef'
- rule: {"required":true}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier

`KubernetesHttpRouteHeaderFilter`

Modifies request headers before forwarding. Set when type is
"RequestHeaderModifier". Upstream support level: Core.

- rule: each header name in 'set' must be unique
- rule: each header name in 'add' must be unique

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.set

`[]KubernetesHttpRouteHeader`

Headers to overwrite (set) on the message, replacing any existing values.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.set[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.set[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.add

`[]KubernetesHttpRouteHeader`

Headers to append, adding to any existing values for the same name.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.add[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.add[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].backendRefs[].filters[].requestHeaderModifier.remove

`[]string`

Header names to remove from the message.

- rule: {"repeated":{"maxItems":"16","unique":true}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier

`KubernetesHttpRouteHeaderFilter`

Modifies response headers before returning to the client. Set when type is
"ResponseHeaderModifier". Upstream support level: Extended.

- rule: each header name in 'set' must be unique
- rule: each header name in 'add' must be unique

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.set

`[]KubernetesHttpRouteHeader`

Headers to overwrite (set) on the message, replacing any existing values.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.set[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.set[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.add

`[]KubernetesHttpRouteHeader`

Headers to append, adding to any existing values for the same name.

- rule: {"repeated":{"maxItems":"16"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.add[].name

`string` · required

Name of the header (case-insensitive).

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$ (1-256 chars).

- rule: {"required":true,"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.add[].value

`string` · required

Value of the header.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"4096"}}

### spec.rules[].backendRefs[].filters[].responseHeaderModifier.remove

`[]string`

Header names to remove from the message.

- rule: {"repeated":{"maxItems":"16","unique":true}}

### spec.rules[].backendRefs[].filters[].requestMirror

`KubernetesHttpRouteRequestMirrorFilter`

Mirrors the request to an additional backend (responses ignored). Set when
type is "RequestMirror". Upstream support level: Extended.

- rule: specify either percent or fraction for the mirror sampling rate, but not both

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef

`KubernetesGatewayApiBackendObjectReference` · required

Backend that mirrored requests are sent to. Required. Upstream support
level: Extended for Kubernetes Service.

- rule: {"required":true}
- rule: port must be specified when referencing a core API Service (group is empty and kind is 'Service')

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef.group

`string` · optional (explicit presence)

Group of the referent. Empty string infers the core API group.

Upstream default: "" (core API group)
Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef.kind

`string` · required · optional (explicit presence)

Kind of the referent.

Upstream default: "Service"
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef.name

`string | valueFrom` · required

Name of the referent. Defaults to a KubernetesService foreign key: in an
infra chart, wire it with valueFrom against the backend Service and the
route deploys after its backend. When the backend is not a
Planton-managed Service (a custom backend kind, or a Service created
outside Planton), pass the literal name with `value:`.

- references: KubernetesService (`status.outputs.service_name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesService, name: <that resource's name>, fieldPath: status.outputs.service_name}} -- a bare string does not parse

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef.namespace

`string` · optional (explicit presence)

Namespace of the backend. When unspecified, the local namespace is inferred.
Cross-namespace references require a ReferenceGrant.

### spec.rules[].backendRefs[].filters[].requestMirror.backendRef.port

`int32` · optional (explicit presence)

Destination port number. Required when the referent is a core Service.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.rules[].backendRefs[].filters[].requestMirror.percent

`int32` · optional (explicit presence)

Percentage of requests to mirror (0-100). When neither percent nor fraction
is set, 100% are mirrored.

- rule: {"int32":{"lte":100,"gte":0}}

### spec.rules[].backendRefs[].filters[].requestMirror.fraction

`KubernetesGatewayApiFraction`

Fraction of requests to mirror. When neither percent nor fraction is set,
100% are mirrored.

- rule: numerator must be less than or equal to denominator

### spec.rules[].backendRefs[].filters[].requestMirror.fraction.numerator

`int32`

Numerator of the fraction. Must be non-negative.

- rule: {"int32":{"gte":0}}

### spec.rules[].backendRefs[].filters[].requestMirror.fraction.denominator

`int32` · optional (explicit presence)

Denominator of the fraction.

Upstream default: 100

- rule: {"int32":{"gte":1}}

### spec.rules[].backendRefs[].filters[].requestRedirect

`KubernetesHttpRouteRequestRedirectFilter`

Responds with an HTTP redirect. Set when type is "RequestRedirect". Cannot
be combined with backends or a URLRewrite filter. Upstream support: Core.

### spec.rules[].backendRefs[].filters[].requestRedirect.scheme

`string` · optional (explicit presence)

Scheme of the redirect Location. When empty, the request scheme is used.
Upstream support level: Extended.

Closed enum: http | https.

- rule: redirect scheme must be either 'http' or 'https'

### spec.rules[].backendRefs[].filters[].requestRedirect.hostname

`string` · required · optional (explicit presence)

Hostname of the redirect Location. When empty, the request Host is used.
Upstream support level: Core.

Upstream PreciseHostname pattern (no wildcard):
^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].backendRefs[].filters[].requestRedirect.path

`KubernetesHttpRoutePathModifier`

Path modification applied to construct the redirect Location. Upstream
support level: Extended.

- rule: replace_full_path must be set when type is 'ReplaceFullPath' and must be unset otherwise
- rule: replace_prefix_match must be set when type is 'ReplacePrefixMatch' and must be unset otherwise

### spec.rules[].backendRefs[].filters[].requestRedirect.path.type

`string` · required

Kind of path modification.

Closed enum: ReplaceFullPath | ReplacePrefixMatch.

- rule: path modifier type must be either 'ReplaceFullPath' or 'ReplacePrefixMatch'
- rule: {"required":true}

### spec.rules[].backendRefs[].filters[].requestRedirect.path.replaceFullPath

`string` · optional (explicit presence)

Value that replaces the entire path. Valid only with ReplaceFullPath.

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].backendRefs[].filters[].requestRedirect.path.replacePrefixMatch

`string` · optional (explicit presence)

Value that replaces the matched path prefix. Valid only with
ReplacePrefixMatch (and a PathPrefix match on the same rule).

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].backendRefs[].filters[].requestRedirect.port

`int32` · optional (explicit presence)

Port of the redirect Location. When unset, it is derived from the scheme or
the listener port. Upstream support level: Extended.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.rules[].backendRefs[].filters[].requestRedirect.statusCode

`int32` · optional (explicit presence)

HTTP status code of the redirect response. Upstream support level: Core.

Upstream default: 302. Closed enum: 301 | 302 | 303 | 307 | 308.

- rule: redirect status_code must be one of 301, 302, 303, 307, or 308

### spec.rules[].backendRefs[].filters[].urlRewrite

`KubernetesHttpRouteUrlRewriteFilter`

Rewrites the request line during forwarding. Set when type is "URLRewrite".
Cannot be combined with a RequestRedirect filter. Upstream support:
Extended.

### spec.rules[].backendRefs[].filters[].urlRewrite.hostname

`string` · required · optional (explicit presence)

Hostname that replaces the Host header during forwarding. Upstream support
level: Extended.

Upstream PreciseHostname pattern (no wildcard):
^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$

- rule: {"string":{"minLen":"1","maxLen":"253","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].backendRefs[].filters[].urlRewrite.path

`KubernetesHttpRoutePathModifier`

Path rewrite applied during forwarding. Upstream support level: Extended.

- rule: replace_full_path must be set when type is 'ReplaceFullPath' and must be unset otherwise
- rule: replace_prefix_match must be set when type is 'ReplacePrefixMatch' and must be unset otherwise

### spec.rules[].backendRefs[].filters[].urlRewrite.path.type

`string` · required

Kind of path modification.

Closed enum: ReplaceFullPath | ReplacePrefixMatch.

- rule: path modifier type must be either 'ReplaceFullPath' or 'ReplacePrefixMatch'
- rule: {"required":true}

### spec.rules[].backendRefs[].filters[].urlRewrite.path.replaceFullPath

`string` · optional (explicit presence)

Value that replaces the entire path. Valid only with ReplaceFullPath.

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].backendRefs[].filters[].urlRewrite.path.replacePrefixMatch

`string` · optional (explicit presence)

Value that replaces the matched path prefix. Valid only with
ReplacePrefixMatch (and a PathPrefix match on the same rule).

- rule: {"string":{"maxLen":"1024"}}

### spec.rules[].backendRefs[].filters[].cors

`KubernetesHttpRouteCorsFilter`

Adds CORS headers to the response. Set when type is "CORS". Upstream
support level: Extended.

### spec.rules[].backendRefs[].filters[].cors.allowOrigins

`[]string`

Origins permitted to share the response. Each is a scheme + host with an
optional port; the host may use a leading wildcard. A single "*" allows all
origins. Upstream support level: Extended.

Upstream CORSOrigin pattern:
(^\*$)|(^(http(s)?):\/\/(((\*\.)?([a-zA-Z0-9\-]+\.)*[a-zA-Z0-9-]+|\*)(:([0-9]{1,5}))?)$)

- rule: allow_origins cannot contain '*' alongside other origins
- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"253","pattern":"(^\\*$)|(^(http(s)?):\\/\\/(((\\*\\.)?([a-zA-Z0-9\\-]+\\.)*[a-zA-Z0-9-]+|\\*)(:([0-9]{1,5}))?)$)"}}}}

### spec.rules[].backendRefs[].filters[].cors.allowCredentials

`bool` · optional (explicit presence)

Whether cross-origin requests may include credentials. Upstream support
level: Extended.

### spec.rules[].backendRefs[].filters[].cors.allowMethods

`[]string`

HTTP methods permitted for cross-origin requests. "*" allows all methods.
Upstream support level: Extended.

- rule: allow_methods may only contain HTTP method tokens or '*'
- rule: allow_methods cannot contain '*' alongside other methods
- rule: {"repeated":{"maxItems":"9","unique":true}}

### spec.rules[].backendRefs[].filters[].cors.allowHeaders

`[]string`

Request headers permitted for cross-origin requests. "*" allows all
headers. Upstream support level: Extended.

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$

- rule: allow_headers cannot contain '*' alongside other headers
- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}}}

### spec.rules[].backendRefs[].filters[].cors.exposeHeaders

`[]string`

Response headers exposed to client-side scripts. Upstream support level:
Extended.

Upstream HeaderName pattern: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$

- rule: {"repeated":{"maxItems":"64","unique":true,"items":{"string":{"minLen":"1","maxLen":"256","pattern":"^[A-Za-z0-9!#$%&'*+\\-.^_`|~]+$"}}}}

### spec.rules[].backendRefs[].filters[].cors.maxAge

`int32` · optional (explicit presence)

How long (in seconds) clients may cache preflight results.

Upstream default: 5.

- rule: {"int32":{"gte":1}}

### spec.rules[].backendRefs[].filters[].extensionRef

`KubernetesGatewayApiLocalObjectReference`

Implementation-specific extension filter. Set when type is "ExtensionRef".
Must not be used for core or extended filters. Upstream support:
Implementation-specific.

### spec.rules[].backendRefs[].filters[].extensionRef.group

`string` · required · optional (explicit presence)

Group of the referent (e.g., "gateway.networking.k8s.io").
Empty string infers the core API group.

Upstream requires the KEY to be present, but its Group type explicitly
allows the empty value -- so this is a proto3 `optional` string with a
presence `required` rule: it must be SET (and is therefore emitted to
the rendered CR, whose CRD rejects a missing key) but may be empty.
The `optional` is what keeps the projection faithful: protojson omits
unset proto3 scalars, so a non-optional empty-string group would be
dropped from the manifest and rejected by the API server.

Group pattern: empty or an RFC 1123 subdomain (max 253).

- rule: {"required":true,"string":{"maxLen":"253","pattern":"^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"}}

### spec.rules[].backendRefs[].filters[].extensionRef.kind

`string` · required

Kind of the referent (e.g., "HTTPRoute" or "Service").

Upstream models Kind as a required value type.
Kind pattern: 1-63 chars, ^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$

- rule: {"required":true,"string":{"minLen":"1","maxLen":"63","pattern":"^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"}}

### spec.rules[].backendRefs[].filters[].extensionRef.name

`string` · required

Name of the referent.

- rule: {"required":true,"string":{"minLen":"1","maxLen":"253"}}

### spec.rules[].timeouts

`KubernetesHttpRouteTimeouts`

Request and per-backend-request timeouts for this rule. Upstream support
level: Extended.

- rule: backend_request timeout cannot be longer than the request timeout

### spec.rules[].timeouts.request

`string` · optional (explicit presence)

Maximum duration for the gateway to respond to a client request. "0s"
disables the timeout. Upstream support level: Extended.

Upstream Duration pattern: ^([0-9]{1,5}(h|m|s|ms)){1,4}$

- rule: {"string":{"pattern":"^([0-9]{1,5}(h|m|s|ms)){1,4}$"}}

### spec.rules[].timeouts.backendRequest

`string` · optional (explicit presence)

Maximum duration for an individual gateway-to-backend request. "0s"
disables the timeout. Must not exceed request. Upstream support level:
Extended.

Upstream Duration pattern: ^([0-9]{1,5}(h|m|s|ms)){1,4}$

- rule: {"string":{"pattern":"^([0-9]{1,5}(h|m|s|ms)){1,4}$"}}

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesHttpRoute, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.route_name` | `string` | Name of the created HTTPRoute (equals metadata.name). In InfraCharts this orders the route after the Gateway and backends it references. |
| `status.outputs.namespace` | `string` | Namespace the HTTPRoute was created in (the resolved spec.namespace). Cross-namespace parent and backend references from this route are subject to ReferenceGrant rules relative to this value. |
| `status.outputs.first_host` | `string` | The first hostname the route matches (spec.hostnames[0]) -- the address a request reaches this route at, exported exactly as the Ingress kind exports its first rule's host, so anything that reads a hostname off an Ingress (a dashboard, a smoke test, a DNS record, a deployment's URL discovery) reads it off a route the same way. Empty when the route declares no hostnames and matches every host its Gateway listener accepts. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |
| `spec.parentRefs[].name` | KubernetesGateway | `status.outputs.gateway_name` |
| `spec.rules[].filters[].requestMirror.backendRef.name` | KubernetesService | `status.outputs.service_name` |
| `spec.rules[].backendRefs[].name` | KubernetesService | `status.outputs.service_name` |
| `spec.rules[].backendRefs[].filters[].requestMirror.backendRef.name` | KubernetesService | `status.outputs.service_name` |

## See Also

- [Overview](../README.md)
