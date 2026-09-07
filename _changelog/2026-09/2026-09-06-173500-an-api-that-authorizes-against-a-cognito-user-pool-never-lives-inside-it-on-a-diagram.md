# An API that authorizes against a Cognito user pool never lives inside it on a diagram

## What changed

- **`AwsRestApiGatewayAuthorizer.provider_arns` is containment-exempt.** A COGNITO_USER_POOLS authorizer names the pools whose tokens it accepts. The user pool is a container kind (`container_kind: true`) -- its app clients, identity providers, and resource servers are created into it and belong inside its box -- but a REST API that validates tokens against the pool is a caller of the pool, not a resident. Until now the reference was placement by omission, so the moment a diagram drew the pool as a room, a REST API with a Cognito authorizer would have been drawn inside it.
- **`AwsAppSyncCognitoUserPoolAuth.user_pool_id` and `AwsAppSyncEventsCognitoAuth.user_pool_id` are containment-exempt.** The same verdict for both AppSync arms (GraphQL and Events): the API authorizes callers against the pool and stands outside it.
- **`AwsAppSyncDatasourceEventbridge.event_bus_arn` is containment-exempt.** The same shape on AppSync's other room reference: an API with an EventBridge data source PUBLISHES events onto the bus. The bus is a container kind whose rules live inside it; the API stands outside with a line in -- the verdict an SES configuration set's event-bus destination already carries.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves exactly those four lines from `contained` to `exempt`; nothing else in the registry moved. The pool's three true residents (`AwsCognitoUserPoolClient`, `AwsCognitoIdentityProvider`, `AwsCognitoResourceServer`) and the bus's one (`AwsEventBridgeRule`) keep their placement.

## Why

`container_kind` says a kind is a box other resources nest inside, and `containment_exempt` says a reference into such a box is access, not placement. Every other "authorize against this pool" reference in the catalog already carries the exemption -- the HTTP API's JWT issuer, the ALB listener's and listener rule's Cognito actions, the OpenSearch domain's Cognito options -- so these three were the odd ones out, placement by omission on references that only ever meant "check tokens here". On a diagram the pool is now a room its clients and identity providers live in, and an API stands outside with a line in.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the four exempt lines
grep -n containment_exempt catalog/aws/awsrestapigateway/v1alpha1/spec.proto catalog/aws/awsappsyncapi/v1alpha1/spec.proto
```
