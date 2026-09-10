//go:build !codegen
// +build !codegen

package outputs

import (
	"path/filepath"
	"testing"

	"github.com/plantonhq/planton/shared/cloudresourcekind"
)

// TestStackOutputsConformance is the standing guard against the systemic IaC
// output-drift class: an engine emits output names/shapes that do not flatten
// onto the kind's StackOutputs proto, silently leaving those proto fields empty.
// (The original bug: the Postgres tofu module emitted a flat
// "password_secret_name" output, which flattens to the key "password_secret_name"
// -- with no dot -- and therefore never populated the proto's nested
// password_secret{name,key} field, while the Pulumi module emitted the correct
// "password_secret.name". See the planton-postgres-iac-parity work.)
//
// Why this also enforces tofu<->pulumi parity: both engines feed the SAME generic
// transformer (TransformRaw -> Flatten -> populateMessage). So a single
// conformance bar per kind -- "this representative output set fully populates the
// proto with nothing left unmapped" -- when satisfied by each engine's emitted
// output set, guarantees the two engines produce the same typed StackOutputs.
//
// To extend coverage: add a case with the raw output shape an engine emits (scalars
// as strings; nested objects as map[string]interface{}, exactly how Terraform state
// and the Pulumi automation API surface them) and the proto fields it must populate.
func TestStackOutputsConformance(t *testing.T) {
	// A module dir with no transform override forces the generic reflection path,
	// which is the convention every in-repo module relies on (0 of 364 use an override).
	genericModuleDir := filepath.Join("testdata", "modules", "empty")

	cases := []struct {
		name string
		kind cloudresourcekind.CloudResourceKind
		// rawOutputs mirrors the post-Flatten-input shape both engines emit.
		rawOutputs map[string]interface{}
		// mustPopulate lists StackOutputs proto fields that MUST be set.
		mustPopulate []string
	}{
		{
			// KubernetesValkey: the official-chart naming contract — the
			// write/read/headless Services, the endpoint, and the
			// module-materialized auth Secret handle.
			name: "KubernetesValkey",
			kind: cloudresourcekind.CloudResourceKind_KubernetesValkey,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"service":              "session-cache",
				"read_service":         "session-cache-read",
				"headless_service":     "session-cache-headless",
				"kube_endpoint":        "session-cache.team-alpha.svc.cluster.local:6379",
				"port_forward_command": "kubectl port-forward svc/session-cache -n team-alpha 6379:6379",
				"username":             "default",
				"password_secret": map[string]interface{}{
					"name": "session-cache-auth",
					"key":  "default",
				},
			},
			mustPopulate: []string{
				"namespace", "service", "read_service", "headless_service",
				"kube_endpoint", "port_forward_command", "username",
				"password_secret",
			},
		},
		{
			// KubernetesMysql: the PXC naming contract — the proxy write and
			// read Services, the endpoint, and the operator-managed
			// system-users Secret's root key.
			name: "KubernetesMysql",
			kind: cloudresourcekind.CloudResourceKind_KubernetesMysql,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"cluster_name":         "orders-mysql",
				"primary_service":      "orders-mysql-haproxy",
				"replicas_service":     "orders-mysql-haproxy-replicas",
				"kube_endpoint":        "orders-mysql-haproxy.team-alpha.svc.cluster.local:3306",
				"port_forward_command": "kubectl port-forward svc/orders-mysql-haproxy -n team-alpha 3306:3306",
				"root_password_secret": map[string]interface{}{
					"name": "orders-mysql-secrets",
					"key":  "root",
				},
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "primary_service", "replicas_service",
				"kube_endpoint", "port_forward_command",
				"root_password_secret",
			},
		},
		{
			// KubernetesMongodb: the PSMDB naming contract — the replica-set
			// discovery Service, the endpoint with the driver's replicaSet
			// parameter source, and the system-users Secret's admin key.
			name: "KubernetesMongodb",
			kind: cloudresourcekind.CloudResourceKind_KubernetesMongodb,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"cluster_name":         "orders-mongo",
				"service":              "orders-mongo-rs0",
				"kube_endpoint":        "orders-mongo-rs0.team-alpha.svc.cluster.local:27017",
				"replica_set":          "rs0",
				"port_forward_command": "kubectl port-forward svc/orders-mongo-rs0 -n team-alpha 27017:27017",
				"admin_password_secret": map[string]interface{}{
					"name": "orders-mongo-secrets",
					"key":  "MONGODB_DATABASE_ADMIN_PASSWORD",
				},
				"restore_name": "orders-mongo-restore-9412eb87",
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "service", "kube_endpoint",
				"replica_set", "port_forward_command",
				"admin_password_secret", "restore_name",
			},
		},
		{
			// KubernetesPerconaMongoOperator: installation identity handles.
			name: "KubernetesPerconaMongoOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPerconaMongoOperator,
			rawOutputs: map[string]interface{}{
				"namespace":    "psmdb-operator-system",
				"release_name": "psmdb-op",
			},
			mustPopulate: []string{"namespace", "release_name"},
		},
		{
			// KubernetesPerconaMysqlOperator: installation identity handles.
			name: "KubernetesPerconaMysqlOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPerconaMysqlOperator,
			rawOutputs: map[string]interface{}{
				"namespace":    "pxc-operator-system",
				"release_name": "pxc-op",
			},
			mustPopulate: []string{"namespace", "release_name"},
		},
		{
			// KubernetesStrimziKafkaOperator: installation identity handles.
			name: "KubernetesStrimziKafkaOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesStrimziKafkaOperator,
			rawOutputs: map[string]interface{}{
				"namespace":    "strimzi-system",
				"release_name": "strimzi-op",
			},
			mustPopulate: []string{"namespace", "release_name"},
		},
		{
			// KubernetesKafka: the Strimzi naming contract — the cluster
			// binding name, the bootstrap Service, the in-cluster bootstrap
			// address, and the cluster CA Secret handle.
			name: "KubernetesKafka",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafka,
			rawOutputs: map[string]interface{}{
				"namespace":                   "team-alpha",
				"cluster_name":                "orders-kafka",
				"bootstrap_service_name":      "orders-kafka-kafka-bootstrap",
				"internal_bootstrap_endpoint": "orders-kafka-kafka-bootstrap.team-alpha.svc.cluster.local:9092",
				"cluster_ca_cert_secret_name": "orders-kafka-cluster-ca-cert",
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "bootstrap_service_name",
				"internal_bootstrap_endpoint", "cluster_ca_cert_secret_name",
			},
		},
		{
			// KubernetesOpenSearchOperator: the install handles (release +
			// controller Deployment).
			name: "KubernetesOpenSearchOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesOpenSearchOperator,
			rawOutputs: map[string]interface{}{
				"namespace":       "opensearch-operator-system",
				"release_name":    "os-op",
				"deployment_name": "os-op-controller-manager",
			},
			mustPopulate: []string{"namespace", "release_name", "deployment_name"},
		},
		{
			// KubernetesOpenSearch: the operator naming contract — the
			// cluster Service (= the resource name), the https API
			// endpoint (TLS in every posture), the bootstrapped admin
			// Secret, and the Dashboards handles when enabled.
			name: "KubernetesOpenSearch",
			kind: cloudresourcekind.CloudResourceKind_KubernetesOpenSearch,
			rawOutputs: map[string]interface{}{
				"namespace":                     "search",
				"cluster_name":                  "product-search",
				"service_name":                  "product-search",
				"http_endpoint":                 "https://product-search.search.svc.cluster.local:9200",
				"admin_credentials_secret_name": "product-search-admin-password",
				"dashboards_service_name":       "product-search-dashboards",
				"dashboards_endpoint":           "http://product-search-dashboards.search.svc.cluster.local:5601",
				"port_forward_command":          "kubectl port-forward svc/product-search -n search 9200:9200",
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "service_name", "http_endpoint",
			},
		},
		{
			// KubernetesSolrOperator: the install handles.
			name: "KubernetesSolrOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSolrOperator,
			rawOutputs: map[string]interface{}{
				"namespace":       "solr-operator-system",
				"release_name":    "solr-op",
				"deployment_name": "solr-op-solr-operator",
			},
			mustPopulate: []string{"namespace", "release_name", "deployment_name"},
		},
		{
			// KubernetesSolr: the operator naming contract — the common
			// Service, the in-cluster endpoint, the bootstrapped basic-auth
			// Secret, and the ZooKeeper connection string.
			name: "KubernetesSolr",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSolr,
			rawOutputs: map[string]interface{}{
				"namespace":                   "search",
				"cluster_name":                "product-solr",
				"common_service_name":         "product-solr-solrcloud-common",
				"internal_endpoint":           "http://product-solr-solrcloud-common.search.svc.cluster.local",
				"basic_auth_secret_name":      "product-solr-solrcloud-basic-auth",
				"zookeeper_connection_string": "product-solr-solrcloud-zookeeper-client:2181/",
				"port_forward_command":        "kubectl port-forward svc/product-solr-solrcloud-common -n search 8983:80",
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "common_service_name",
				"internal_endpoint", "zookeeper_connection_string",
			},
		},
		{
			// KubernetesNeo4j: the server handles — the main Service (= the
			// resource name), bolt/http endpoints, and the auth Secret.
			name: "KubernetesNeo4j",
			kind: cloudresourcekind.CloudResourceKind_KubernetesNeo4j,
			rawOutputs: map[string]interface{}{
				"namespace":            "graph",
				"release_name":         "knowledge-graph",
				"service_name":         "knowledge-graph",
				"bolt_endpoint":        "neo4j://knowledge-graph.graph.svc.cluster.local:7687",
				"http_endpoint":        "http://knowledge-graph.graph.svc.cluster.local:7474",
				"auth_secret_name":     "knowledge-graph-auth",
				"port_forward_command": "kubectl port-forward svc/knowledge-graph -n graph 7687:7687",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_name", "bolt_endpoint",
				"http_endpoint",
			},
		},
		{
			// KubernetesKafkaTopic: the topic handle (the KAFKA name, which
			// may differ from the resource name via spec.topic_name).
			name: "KubernetesKafkaTopic",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaTopic,
			rawOutputs: map[string]interface{}{
				"namespace":  "team-alpha",
				"topic_name": "orders.v1_events",
			},
			mustPopulate: []string{"namespace", "topic_name"},
		},
		{
			// KubernetesKafkaUser: the principal name and the
			// operator-generated credentials Secret handle.
			name: "KubernetesKafkaUser",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaUser,
			rawOutputs: map[string]interface{}{
				"namespace":   "team-alpha",
				"username":    "orders-service",
				"secret_name": "orders-service",
			},
			mustPopulate: []string{"namespace", "username", "secret_name"},
		},
		{
			// KubernetesKafkaConnect: the cluster-binding name (what
			// KubernetesKafkaConnector resources bind to) and the Connect
			// REST API handles.
			name: "KubernetesKafkaConnect",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaConnect,
			rawOutputs: map[string]interface{}{
				"namespace":             "team-alpha",
				"connect_name":          "orders-pipes",
				"rest_api_service_name": "orders-pipes-connect-api",
				"rest_api_endpoint":     "http://orders-pipes-connect-api.team-alpha.svc.cluster.local:8083",
			},
			mustPopulate: []string{
				"namespace", "connect_name", "rest_api_service_name",
				"rest_api_endpoint",
			},
		},
		{
			// KubernetesKafkaConnector: the connector's name inside its
			// Connect cluster.
			name: "KubernetesKafkaConnector",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaConnector,
			rawOutputs: map[string]interface{}{
				"namespace":      "team-alpha",
				"connector_name": "orders-cdc",
			},
			mustPopulate: []string{"namespace", "connector_name"},
		},
		{
			// KubernetesKafkaMirrorMaker2: the deployment identity and the
			// engine's read-only REST endpoint.
			name: "KubernetesKafkaMirrorMaker2",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaMirrorMaker2,
			rawOutputs: map[string]interface{}{
				"namespace":         "team-alpha",
				"mirrormaker_name":  "msk-migration",
				"rest_api_endpoint": "http://msk-migration-mirrormaker2-api.team-alpha.svc.cluster.local:8083",
			},
			mustPopulate: []string{"namespace", "mirrormaker_name", "rest_api_endpoint"},
		},
		{
			// KubernetesKarapace: the registry endpoint (the
			// schema.registry.url composition handle), the optional
			// REST-proxy endpoint, and the schemas topic.
			name: "KubernetesKarapace",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKarapace,
			rawOutputs: map[string]interface{}{
				"namespace":           "team-alpha",
				"service_name":        "schemas",
				"endpoint":            "http://schemas.team-alpha.svc.cluster.local:8081",
				"rest_proxy_endpoint": "http://schemas-rest.team-alpha.svc.cluster.local:8082",
				"schemas_topic":       "_schemas",
			},
			mustPopulate: []string{
				"namespace", "service_name", "endpoint",
				"rest_proxy_endpoint", "schemas_topic",
			},
		},
		{
			// KubernetesKafkaUi: the console Service handles exposure kinds
			// attach to, plus the workstation port-forward convenience.
			name: "KubernetesKafkaUi",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKafkaUi,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"service_name":         "console",
				"endpoint":             "http://console.team-alpha.svc.cluster.local:80",
				"port_forward_command": "kubectl port-forward -n team-alpha svc/console 8080:80",
			},
			mustPopulate: []string{
				"namespace", "service_name", "endpoint",
				"port_forward_command",
			},
		},
		{
			// KubernetesPostgres: the CloudNativePG naming contract — the
			// three traffic services, the rw endpoint, and the credential
			// Secret handles (nested objects that flatten to
			// password_secret.name etc.).
			name: "KubernetesPostgres",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPostgres,
			rawOutputs: map[string]interface{}{
				"namespace":             "team-alpha",
				"cluster_name":          "orders-db",
				"rw_service":            "orders-db-rw",
				"ro_service":            "orders-db-ro",
				"r_service":             "orders-db-r",
				"kube_endpoint":         "orders-db-rw.team-alpha.svc.cluster.local:5432",
				"port_forward_command":  "kubectl port-forward svc/orders-db-rw -n team-alpha 5432:5432",
				"superuser_secret_name": "orders-db-superuser",
				"password_secret": map[string]interface{}{
					"name": "orders-db-app",
					"key":  "password",
				},
				"username_secret": map[string]interface{}{
					"name": "orders-db-app",
					"key":  "username",
				},
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "rw_service", "ro_service",
				"r_service", "kube_endpoint", "port_forward_command",
				"superuser_secret_name", "password_secret", "username_secret",
			},
		},
		{
			// KubernetesCloudNativePgOperator: the install identity.
			name: "KubernetesCloudNativePgOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCloudNativePgOperator,
			rawOutputs: map[string]interface{}{
				"namespace":    "cnpg-system",
				"release_name": "cnpg",
			},
			mustPopulate: []string{
				"namespace", "release_name",
			},
		},
		{
			// KubernetesCnpgBarmanCloudPlugin: the install identity plus the
			// CNPG-I plugin name a Cluster's plugins list carries.
			name: "KubernetesCnpgBarmanCloudPlugin",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCnpgBarmanCloudPlugin,
			rawOutputs: map[string]interface{}{
				"namespace":    "cnpg-system",
				"release_name": "plugin-barman-cloud",
				"plugin_name":  "barman-cloud.cloudnative-pg.io",
			},
			mustPopulate: []string{
				"namespace", "release_name", "plugin_name",
			},
		},
		{
			// KubernetesNamespace: flat scalar outputs from both engines describing
			// the namespace and which governance objects were applied.
			name: "KubernetesNamespace",
			kind: cloudresourcekind.CloudResourceKind_KubernetesNamespace,
			rawOutputs: map[string]interface{}{
				"namespace":               "team-alpha",
				"namespace_id":            "team-alpha",
				"resource_quotas_applied": "true",
				"limit_ranges_applied":    "true",
				"pod_security_standard":   "baseline",
			},
			mustPopulate: []string{
				"namespace", "namespace_id", "resource_quotas_applied",
				"limit_ranges_applied", "pod_security_standard",
			},
		},
		{
			// KubernetesConfigMap: flat scalar outputs (name + namespace) from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesConfigMap",
			kind: cloudresourcekind.CloudResourceKind_KubernetesConfigMap,
			rawOutputs: map[string]interface{}{
				"configmap_name": "app-config",
				"namespace":      "team-alpha",
			},
			mustPopulate: []string{"configmap_name", "namespace"},
		},
		{
			// KubernetesSecret: flat scalar outputs (name, namespace, type) from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesSecret",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSecret,
			rawOutputs: map[string]interface{}{
				"secret_name":      "registry-cred",
				"secret_namespace": "team-alpha",
				"secret_type":      "kubernetes.io/dockerconfigjson",
			},
			mustPopulate: []string{"secret_name", "secret_namespace", "secret_type"},
		},
		{
			// KubernetesServiceAccount: identity handles (name, namespace, the
			// assembled RBAC subject, and the bound cloud identity) from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesServiceAccount",
			kind: cloudresourcekind.CloudResourceKind_KubernetesServiceAccount,
			rawOutputs: map[string]interface{}{
				"service_account_name":     "dns-manager",
				"namespace":                "team-alpha",
				"rbac_subject":             "system:serviceaccount:team-alpha:dns-manager",
				"workload_identity_handle": "arn:aws:iam::123456789012:role/dns-manager",
			},
			mustPopulate: []string{
				"service_account_name", "namespace", "rbac_subject",
				"workload_identity_handle",
			},
		},
		{
			// KubernetesRbac: created object names/kinds from both engines must land
			// on the StackOutputs proto.
			name: "KubernetesRbac",
			kind: cloudresourcekind.CloudResourceKind_KubernetesRbac,
			rawOutputs: map[string]interface{}{
				"role_name":    "app-reader",
				"role_kind":    "Role",
				"binding_name": "app-reader-grant",
				"binding_kind": "RoleBinding",
				"namespace":    "team-alpha",
			},
			mustPopulate: []string{
				"role_name", "role_kind", "binding_name", "binding_kind", "namespace",
			},
		},
		{
			// KubernetesDeployment: workload identity + Service handles from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesDeployment",
			kind: cloudresourcekind.CloudResourceKind_KubernetesDeployment,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"deployment_name":      "checkout",
				"service":              "checkout",
				"selector_labels":      "app=checkout,resource_name=checkout",
				"port_forward_command": "kubectl port-forward -n team-alpha service/checkout 8080:8080",
				"kube_endpoint":        "checkout.team-alpha.svc.cluster.local",
			},
			mustPopulate: []string{
				"namespace", "deployment_name", "service", "selector_labels",
				"port_forward_command", "kube_endpoint",
			},
		},
		{
			// KubernetesStatefulSet: identity handles plus the per-replica DNS
			// template from both engines must land on the StackOutputs proto.
			name: "KubernetesStatefulSet",
			kind: cloudresourcekind.CloudResourceKind_KubernetesStatefulSet,
			rawOutputs: map[string]interface{}{
				"namespace":            "team-alpha",
				"stateful_set_name":    "orders-db",
				"service":              "orders-db",
				"selector_labels":      "app=orders-db,resource_name=orders-db",
				"port_forward_command": "kubectl port-forward -n team-alpha service/orders-db 8080:8080",
				"kube_endpoint":        "orders-db.team-alpha.svc.cluster.local",
				"pod_dns_template":     "orders-db-{ordinal}.orders-db.team-alpha.svc.cluster.local",
			},
			mustPopulate: []string{
				"namespace", "stateful_set_name", "service", "selector_labels",
				"port_forward_command", "kube_endpoint", "pod_dns_template",
			},
		},
		{
			// KubernetesDaemonSet: object identity + selector labels from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesDaemonSet",
			kind: cloudresourcekind.CloudResourceKind_KubernetesDaemonSet,
			rawOutputs: map[string]interface{}{
				"namespace":       "kube-system",
				"daemon_set_name": "log-collector",
				"selector_labels": "app=log-collector,resource_name=log-collector",
			},
			mustPopulate: []string{"namespace", "daemon_set_name", "selector_labels"},
		},
		{
			// KubernetesJob: object identity + selector labels from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesJob",
			kind: cloudresourcekind.CloudResourceKind_KubernetesJob,
			rawOutputs: map[string]interface{}{
				"namespace":       "team-alpha",
				"job_name":        "schema-migrate",
				"selector_labels": "app=schema-migrate,resource_name=schema-migrate",
			},
			mustPopulate: []string{"namespace", "job_name", "selector_labels"},
		},
		{
			// KubernetesCronJob: object identity + the effective schedule from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesCronJob",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCronJob,
			rawOutputs: map[string]interface{}{
				"namespace":     "team-alpha",
				"cron_job_name": "nightly-backup",
				"schedule":      "0 3 * * *",
			},
			mustPopulate: []string{"namespace", "cron_job_name", "schedule"},
		},
		{
			// KubernetesService: identity + address handles from both engines must
			// land on the StackOutputs proto. LB handles are exercised with the
			// hostname form (the ip form flattens through the same scalar path).
			name: "KubernetesService",
			kind: cloudresourcekind.CloudResourceKind_KubernetesService,
			rawOutputs: map[string]interface{}{
				"service_name":           "checkout",
				"namespace":              "team-alpha",
				"type":                   "LoadBalancer",
				"cluster_ip":             "10.96.14.7",
				"load_balancer_ip":       "",
				"load_balancer_hostname": "a1b2c3.elb.us-west-2.amazonaws.com",
				"kube_endpoint":          "checkout.team-alpha.svc.cluster.local",
				"port_forward_command":   "kubectl port-forward -n team-alpha service/checkout 80:80",
			},
			mustPopulate: []string{
				"service_name", "namespace", "type", "cluster_ip",
				"load_balancer_hostname", "kube_endpoint", "port_forward_command",
			},
		},
		{
			// KubernetesIngress: identity + load-balancer address handles from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesIngress",
			kind: cloudresourcekind.CloudResourceKind_KubernetesIngress,
			rawOutputs: map[string]interface{}{
				"ingress_name":           "checkout-web",
				"namespace":              "team-alpha",
				"load_balancer_ip":       "203.0.113.10",
				"load_balancer_hostname": "",
				"first_host":             "shop.example.com",
			},
			mustPopulate: []string{
				"ingress_name", "namespace", "load_balancer_ip", "first_host",
			},
		},
		{
			// KubernetesNetworkPolicy: object identity + the governed directions
			// from both engines must land on the StackOutputs proto.
			name: "KubernetesNetworkPolicy",
			kind: cloudresourcekind.CloudResourceKind_KubernetesNetworkPolicy,
			rawOutputs: map[string]interface{}{
				"network_policy_name": "default-deny-all",
				"namespace":           "team-alpha",
				"policy_types":        "Ingress,Egress",
			},
			mustPopulate: []string{
				"network_policy_name", "namespace", "policy_types",
			},
		},
		{
			// KubernetesPersistentVolumeClaim: object identity + the requested
			// size from both engines must land on the StackOutputs proto.
			name: "KubernetesPersistentVolumeClaim",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPersistentVolumeClaim,
			rawOutputs: map[string]interface{}{
				"pvc_name":        "shared-cache",
				"namespace":       "team-alpha",
				"storage_request": "10Gi",
			},
			mustPopulate: []string{
				"pvc_name", "namespace", "storage_request",
			},
		},
		{
			// KubernetesStorageClass: cluster-scoped identity + provisioner +
			// the default-class flag (bool) from both engines must land on the
			// StackOutputs proto.
			name: "KubernetesStorageClass",
			kind: cloudresourcekind.CloudResourceKind_KubernetesStorageClass,
			rawOutputs: map[string]interface{}{
				"storage_class_name": "fast-ssd",
				"provisioner":        "ebs.csi.aws.com",
				"is_default_class":   true,
			},
			mustPopulate: []string{
				"storage_class_name", "provisioner", "is_default_class",
			},
		},
		{
			// KubernetesResourceQuota: the governance pair's identities; the
			// LimitRange name is empty when no limit_defaults were configured,
			// so only the always-present fields are required to populate.
			name: "KubernetesResourceQuota",
			kind: cloudresourcekind.CloudResourceKind_KubernetesResourceQuota,
			rawOutputs: map[string]interface{}{
				"resource_quota_name": "team-quota",
				"namespace":           "team-alpha",
				"limit_range_name":    "team-quota",
			},
			mustPopulate: []string{
				"resource_quota_name", "namespace", "limit_range_name",
			},
		},
		{
			// KubernetesPriorityClass: cluster-scoped identity + the priority
			// integer (int32) from both engines must land on the StackOutputs
			// proto.
			name: "KubernetesPriorityClass",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPriorityClass,
			rawOutputs: map[string]interface{}{
				"priority_class_name": "critical",
				"value":               1000000,
			},
			mustPopulate: []string{
				"priority_class_name", "value",
			},
		},
		{
			// KubernetesPodDisruptionBudget: object identity — a budget has no
			// runtime handles beyond it (the eviction API enforces by selector).
			name: "KubernetesPodDisruptionBudget",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPodDisruptionBudget,
			rawOutputs: map[string]interface{}{
				"pod_disruption_budget_name": "checkout-pdb",
				"namespace":                  "team-alpha",
			},
			mustPopulate: []string{
				"pod_disruption_budget_name", "namespace",
			},
		},
		{
			// KubernetesHorizontalPodAutoscaler: object identity + the scale
			// target handle + the replica bounds (int32s) from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesHorizontalPodAutoscaler",
			kind: cloudresourcekind.CloudResourceKind_KubernetesHorizontalPodAutoscaler,
			rawOutputs: map[string]interface{}{
				"horizontal_pod_autoscaler_name": "checkout-hpa",
				"namespace":                      "team-alpha",
				"scale_target":                   "Deployment/checkout",
				"min_replicas":                   2,
				"max_replicas":                   20,
			},
			mustPopulate: []string{
				"horizontal_pod_autoscaler_name", "namespace", "scale_target",
				"min_replicas", "max_replicas",
			},
		},
		{
			// KubernetesCertManager: install identity + the two composition
			// seams (controller ServiceAccount for cloud identity bindings,
			// cluster-resource namespace where ClusterIssuer credentials
			// live) from both engines must land on the StackOutputs proto.
			name: "KubernetesCertManager",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCertManager,
			rawOutputs: map[string]interface{}{
				"namespace":                  "cert-manager",
				"release_name":               "cert-manager",
				"service_account_name":       "cert-manager",
				"cluster_resource_namespace": "cert-manager",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_account_name", "cluster_resource_namespace",
			},
		},
		{
			// KubernetesClusterIssuer: the issuer handle Certificates and
			// ingress-shim annotations reference, plus the ACME account-key
			// Secret location, from both engines must land on the proto.
			name: "KubernetesClusterIssuer",
			kind: cloudresourcekind.CloudResourceKind_KubernetesClusterIssuer,
			rawOutputs: map[string]interface{}{
				"cluster_issuer_name":          "letsencrypt-production",
				"secrets_namespace":            "cert-manager",
				"acme_account_key_secret_name": "letsencrypt-production-acme-account-key",
			},
			mustPopulate: []string{
				"cluster_issuer_name", "secrets_namespace", "acme_account_key_secret_name",
			},
		},
		{
			// KubernetesIssuer: the namespace-scoped issuer handle
			// same-namespace Certificates reference.
			name: "KubernetesIssuer",
			kind: cloudresourcekind.CloudResourceKind_KubernetesIssuer,
			rawOutputs: map[string]interface{}{
				"namespace":                    "team-a",
				"issuer_name":                  "team-a-ca",
				"acme_account_key_secret_name": "",
			},
			mustPopulate: []string{
				"namespace", "issuer_name",
			},
		},
		{
			// KubernetesCertificate: the TLS Secret handle every consumer
			// (Ingress, Gateway, CA issuer) references.
			name: "KubernetesCertificate",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCertificate,
			rawOutputs: map[string]interface{}{
				"namespace":        "team-a",
				"certificate_name": "api-cert",
				"secret_name":      "api-cert-tls",
			},
			mustPopulate: []string{
				"namespace", "certificate_name", "secret_name",
			},
		},
		{
			// KubernetesExternalDns: the install identity plus the controller
			// ServiceAccount name — the subject cloud-side keyless bindings
			// (IRSA trust policy, GKE WI member, Azure federated credential)
			// reference together with the namespace.
			name: "KubernetesExternalDns",
			kind: cloudresourcekind.CloudResourceKind_KubernetesExternalDns,
			rawOutputs: map[string]interface{}{
				"namespace":            "external-dns",
				"release_name":         "external-dns-cloudflare",
				"service_account_name": "external-dns-cloudflare",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_account_name",
			},
		},
		{
			// KubernetesExternalSecretsOperator: install identity + the two
			// composition seams (controller ServiceAccount for ambient cloud
			// identity, the namespace where ClusterSecretStore credentials
			// live) from both engines must land on the StackOutputs proto.
			name: "KubernetesExternalSecretsOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesExternalSecretsOperator,
			rawOutputs: map[string]interface{}{
				"namespace":                  "external-secrets",
				"release_name":               "external-secrets",
				"controller_service_account": "external-secrets",
			},
			mustPopulate: []string{
				"namespace", "release_name", "controller_service_account",
			},
		},
		{
			// KubernetesClusterSecretStore: the store handle ExternalSecrets
			// reference (kind ClusterSecretStore) plus the credential-secret
			// home namespace.
			name: "KubernetesClusterSecretStore",
			kind: cloudresourcekind.CloudResourceKind_KubernetesClusterSecretStore,
			rawOutputs: map[string]interface{}{
				"store_name":        "aws-prod",
				"secrets_namespace": "external-secrets",
			},
			mustPopulate: []string{
				"store_name", "secrets_namespace",
			},
		},
		{
			// KubernetesSecretStore: the namespaced store handle
			// same-namespace ExternalSecrets reference.
			name: "KubernetesSecretStore",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSecretStore,
			rawOutputs: map[string]interface{}{
				"store_name": "team-a-gcp",
				"namespace":  "team-a",
			},
			mustPopulate: []string{
				"store_name", "namespace",
			},
		},
		{
			// KubernetesExternalSecret: the materialized Secret handle every
			// workload consumer (env valueFrom, volume secretName) wires to.
			name: "KubernetesExternalSecret",
			kind: cloudresourcekind.CloudResourceKind_KubernetesExternalSecret,
			rawOutputs: map[string]interface{}{
				"external_secret_name": "app-db-credentials",
				"namespace":            "team-a",
				"secret_name":          "app-db-credentials",
			},
			mustPopulate: []string{
				"external_secret_name", "namespace", "secret_name",
			},
		},
		{
			// KubernetesIngressNginx: install identity + the composition
			// handles (IngressClass name for KubernetesIngress routing,
			// controller Service names for DNS/traffic wiring, LB address
			// once the host cloud provisions it) from both engines must
			// land on the StackOutputs proto.
			name: "KubernetesIngressNginx",
			kind: cloudresourcekind.CloudResourceKind_KubernetesIngressNginx,
			rawOutputs: map[string]interface{}{
				"namespace":               "ingress-nginx",
				"release_name":            "ingress-nginx",
				"ingress_class_name":      "nginx",
				"controller_service_name": "ingress-nginx-controller",
				"internal_service_name":   "",
				"load_balancer_ip":        "203.0.113.10",
				"load_balancer_hostname":  "",
			},
			mustPopulate: []string{
				"namespace", "release_name", "ingress_class_name",
				"controller_service_name", "load_balancer_ip",
			},
		},
		{
			// KubernetesMetricsServer: install identity + the APIService /
			// Service handles the metrics pipeline registers, from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesMetricsServer",
			kind: cloudresourcekind.CloudResourceKind_KubernetesMetricsServer,
			rawOutputs: map[string]interface{}{
				"namespace":        "kube-system",
				"release_name":     "metrics-server",
				"service_name":     "metrics-server",
				"api_service_name": "v1beta1.metrics.k8s.io",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_name", "api_service_name",
			},
		},
		{
			// KubernetesCilium: install identity + the fixed-name component
			// handles (hubble relay/ui Services, the "cilium" GatewayClass)
			// from both engines must land on the StackOutputs proto.
			name: "KubernetesCilium",
			kind: cloudresourcekind.CloudResourceKind_KubernetesCilium,
			rawOutputs: map[string]interface{}{
				"namespace":                 "kube-system",
				"release_name":              "cilium",
				"cluster_name":              "prod-east",
				"hubble_relay_service_name": "hubble-relay",
				"hubble_ui_service_name":    "hubble-ui",
				"gateway_class_name":        "cilium",
			},
			mustPopulate: []string{
				"namespace", "release_name", "cluster_name",
				"hubble_relay_service_name", "hubble_ui_service_name", "gateway_class_name",
			},
		},
		{
			// KubernetesKeda: install identity + the operator service-account
			// handle keyless cloud bindings are written against, from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesKeda",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKeda,
			rawOutputs: map[string]interface{}{
				"namespace":                     "keda",
				"release_name":                  "keda",
				"operator_service_account_name": "keda-operator",
			},
			mustPopulate: []string{
				"namespace", "release_name", "operator_service_account_name",
			},
		},
		{
			// KubernetesBackendTlsPolicy: the created CR's identity from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesBackendTlsPolicy",
			kind: cloudresourcekind.CloudResourceKind_KubernetesBackendTlsPolicy,
			rawOutputs: map[string]interface{}{
				"policy_name": "payments-backend-tls",
				"namespace":   "payments",
			},
			mustPopulate: []string{
				"policy_name", "namespace",
			},
		},
		{
			// KubernetesKarpenter: install identity (two fixed-name releases)
			// + the controller service-account handle IRSA trust policies are
			// written against, from both engines must land on the
			// StackOutputs proto.
			name: "KubernetesKarpenter",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKarpenter,
			rawOutputs: map[string]interface{}{
				"namespace":            "kube-system",
				"release_name":         "karpenter",
				"crd_release_name":     "karpenter-crd",
				"service_account_name": "karpenter",
			},
			mustPopulate: []string{
				"namespace", "release_name", "crd_release_name", "service_account_name",
			},
		},
		{
			// KubernetesKarpenterNodePool: the created cluster-scoped CR's
			// identity (the karpenter.sh/nodepool join key) from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesKarpenterNodePool",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKarpenterNodePool,
			rawOutputs: map[string]interface{}{
				"node_pool_name": "gp-spot-pool",
			},
			mustPopulate: []string{
				"node_pool_name",
			},
		},
		{
			// KubernetesKarpenterEc2NodeClass: the created cluster-scoped
			// CR's identity (what NodePools reference via node_class_ref)
			// from both engines must land on the StackOutputs proto.
			name: "KubernetesKarpenterEc2NodeClass",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKarpenterEc2NodeClass,
			rawOutputs: map[string]interface{}{
				"node_class_name": "default-al2023",
			},
			mustPopulate: []string{
				"node_class_name",
			},
		},
		{
			// KubernetesClusterAutoscaler: install identity + the
			// service-account handle keyless cloud bindings are written
			// against, from both engines must land on the StackOutputs proto.
			name: "KubernetesClusterAutoscaler",
			kind: cloudresourcekind.CloudResourceKind_KubernetesClusterAutoscaler,
			rawOutputs: map[string]interface{}{
				"namespace":            "kube-system",
				"release_name":         "cluster-autoscaler",
				"service_account_name": "cluster-autoscaler-aws-cluster-autoscaler",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_account_name",
			},
		},
		{
			// KubernetesVelero: install identity + the server
			// service-account handle keyless bindings target + the default
			// BackupStorageLocation name Backups/Schedules reference, from
			// both engines must land on the StackOutputs proto.
			name: "KubernetesVelero",
			kind: cloudresourcekind.CloudResourceKind_KubernetesVelero,
			rawOutputs: map[string]interface{}{
				"namespace":                    "velero",
				"release_name":                 "velero",
				"service_account_name":         "velero-server",
				"backup_storage_location_name": "default",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_account_name", "backup_storage_location_name",
			},
		},
		{
			// KubernetesGatewayApiCrds: install identity (version, channel,
			// manifest URL) from both engines must land on the StackOutputs proto.
			name: "KubernetesGatewayApiCrds",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGatewayApiCrds,
			rawOutputs: map[string]interface{}{
				"installed_version":      "v1.6.1",
				"installed_channel":      "standard",
				"installed_manifest_url": "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.6.1/standard-install.yaml",
			},
			mustPopulate: []string{
				"installed_version", "installed_channel", "installed_manifest_url",
			},
		},
		{
			// KubernetesGatewayClass: class identity (cluster-scoped, no
			// namespace) from both engines must land on the StackOutputs proto.
			name: "KubernetesGatewayClass",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGatewayClass,
			rawOutputs: map[string]interface{}{
				"gateway_class_name": "istio",
				"controller_name":    "istio.io/gateway-controller",
			},
			mustPopulate: []string{"gateway_class_name", "controller_name"},
		},
		{
			// KubernetesGateway: gateway identity + its class handle from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesGateway",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGateway,
			rawOutputs: map[string]interface{}{
				"gateway_name":       "edge-gateway",
				"namespace":          "gateway-system",
				"gateway_class_name": "istio",
			},
			mustPopulate: []string{"gateway_name", "namespace", "gateway_class_name"},
		},
		{
			// KubernetesListenerSet: listener-set identity + the parent Gateway
			// handle from both engines must land on the StackOutputs proto.
			name: "KubernetesListenerSet",
			kind: cloudresourcekind.CloudResourceKind_KubernetesListenerSet,
			rawOutputs: map[string]interface{}{
				"listener_set_name": "team-alpha-listeners",
				"namespace":         "team-alpha",
				"gateway_name":      "edge-gateway",
			},
			mustPopulate: []string{"listener_set_name", "namespace", "gateway_name"},
		},
		{
			// KubernetesHttpRoute: route identity from both engines must land
			// on the StackOutputs proto (the route kinds share this shape).
			name: "KubernetesHttpRoute",
			kind: cloudresourcekind.CloudResourceKind_KubernetesHttpRoute,
			rawOutputs: map[string]interface{}{
				"route_name": "app-route",
				"namespace":  "team-alpha",
				// The route's first hostname is an address, exported as the
				// Ingress exports first_host so one reader serves both.
				"first_host": "api.example.com",
			},
			mustPopulate: []string{"route_name", "namespace", "first_host"},
		},
		{
			name: "KubernetesGrpcRoute",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGrpcRoute,
			rawOutputs: map[string]interface{}{
				"route_name": "grpc-route",
				"namespace":  "team-alpha",
			},
			mustPopulate: []string{"route_name", "namespace"},
		},
		{
			name: "KubernetesTcpRoute",
			kind: cloudresourcekind.CloudResourceKind_KubernetesTcpRoute,
			rawOutputs: map[string]interface{}{
				"route_name": "tcp-route",
				"namespace":  "team-alpha",
			},
			mustPopulate: []string{"route_name", "namespace"},
		},
		{
			name: "KubernetesUdpRoute",
			kind: cloudresourcekind.CloudResourceKind_KubernetesUdpRoute,
			rawOutputs: map[string]interface{}{
				"route_name": "udp-route",
				"namespace":  "team-alpha",
			},
			mustPopulate: []string{"route_name", "namespace"},
		},
		{
			name: "KubernetesTlsRoute",
			kind: cloudresourcekind.CloudResourceKind_KubernetesTlsRoute,
			rawOutputs: map[string]interface{}{
				"route_name": "tls-route",
				"namespace":  "team-alpha",
			},
			mustPopulate: []string{"route_name", "namespace"},
		},
		{
			// KubernetesReferenceGrant: grant identity from both engines must
			// land on the StackOutputs proto.
			name: "KubernetesReferenceGrant",
			kind: cloudresourcekind.CloudResourceKind_KubernetesReferenceGrant,
			rawOutputs: map[string]interface{}{
				"reference_grant_name": "allow-frontend-routes",
				"namespace":            "backend",
			},
			mustPopulate: []string{"reference_grant_name", "namespace"},
		},
		{
			// KubernetesIstio: the control-plane handles downstream resources
			// compose against — the GatewayClass name (KubernetesGateway seam),
			// the trust domain (AuthorizationPolicy principals), the istiod
			// discovery Service, and the data plane mode — from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesIstio",
			kind: cloudresourcekind.CloudResourceKind_KubernetesIstio,
			rawOutputs: map[string]interface{}{
				"namespace":           "istio-system",
				"istiod_service_name": "istiod",
				"revision":            "default",
				"gateway_class_name":  "istio",
				"trust_domain":        "cluster.local",
				"dataplane_mode":      "sidecar",
			},
			mustPopulate: []string{
				"namespace", "istiod_service_name", "revision",
				"gateway_class_name", "trust_domain", "dataplane_mode",
			},
		},
		{
			// KubernetesIstioBaseCrds: the installed-release record from both
			// engines must land on the StackOutputs proto.
			name: "KubernetesIstioBaseCrds",
			kind: cloudresourcekind.CloudResourceKind_KubernetesIstioBaseCrds,
			rawOutputs: map[string]interface{}{
				"installed_release":      "1.30.3",
				"installed_manifest_url": "https://raw.githubusercontent.com/istio/istio/1.30.3/manifests/charts/base/files/crd-all.gen.yaml",
			},
			mustPopulate: []string{"installed_release", "installed_manifest_url"},
		},
		{
			// KubernetesDestinationRule: CR identity from both engines must
			// land on the StackOutputs proto (the typed Istio CR kinds share
			// this shape).
			name: "KubernetesDestinationRule",
			kind: cloudresourcekind.CloudResourceKind_KubernetesDestinationRule,
			rawOutputs: map[string]interface{}{
				"destination_rule_name": "reviews-circuit-breaker",
				"namespace":             "team-alpha",
			},
			mustPopulate: []string{"destination_rule_name", "namespace"},
		},
		{
			name: "KubernetesServiceEntry",
			kind: cloudresourcekind.CloudResourceKind_KubernetesServiceEntry,
			rawOutputs: map[string]interface{}{
				"service_entry_name": "external-api",
				"namespace":          "team-alpha",
			},
			mustPopulate: []string{"service_entry_name", "namespace"},
		},
		{
			name: "KubernetesPeerAuthentication",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPeerAuthentication,
			rawOutputs: map[string]interface{}{
				"peer_authentication_name": "namespace-strict-mtls",
				"namespace":                "team-alpha",
			},
			mustPopulate: []string{"peer_authentication_name", "namespace"},
		},
		{
			name: "KubernetesRequestAuthentication",
			kind: cloudresourcekind.CloudResourceKind_KubernetesRequestAuthentication,
			rawOutputs: map[string]interface{}{
				"request_authentication_name": "jwt-auth",
				"namespace":                   "team-alpha",
			},
			mustPopulate: []string{"request_authentication_name", "namespace"},
		},
		{
			name: "KubernetesAuthorizationPolicy",
			kind: cloudresourcekind.CloudResourceKind_KubernetesAuthorizationPolicy,
			rawOutputs: map[string]interface{}{
				"authorization_policy_name": "require-jwt",
				"namespace":                 "team-alpha",
			},
			mustPopulate: []string{"authorization_policy_name", "namespace"},
		},
		{
			name: "KubernetesTelemetry",
			kind: cloudresourcekind.CloudResourceKind_KubernetesTelemetry,
			rawOutputs: map[string]interface{}{
				"telemetry_name": "mesh-default-tracing",
				"namespace":      "istio-system",
			},
			mustPopulate: []string{"telemetry_name", "namespace"},
		},
		{
			name: "KubernetesEnvoyFilter",
			kind: cloudresourcekind.CloudResourceKind_KubernetesEnvoyFilter,
			rawOutputs: map[string]interface{}{
				"envoy_filter_name": "grpc-web-cors",
				"namespace":         "istio-system",
			},
			mustPopulate: []string{"envoy_filter_name", "namespace"},
		},
		{
			// KubernetesManifest: the anchor namespace + the applied-resource
			// inventory (a repeated string derived from the input YAML) from
			// both engines must land on the StackOutputs proto.
			name: "KubernetesManifest",
			kind: cloudresourcekind.CloudResourceKind_KubernetesManifest,
			rawOutputs: map[string]interface{}{
				"namespace":         "team-alpha",
				"applied_resources": []interface{}{"v1/ConfigMap/app-config", "apps/v1/Deployment/app"},
			},
			mustPopulate: []string{
				"namespace", "applied_resources",
			},
		},
		{
			// KubernetesHelmRelease: release identity + Helm-recorded state
			// (chart/app versions, status, revision int32) from both engines
			// must land on the StackOutputs proto.
			name: "KubernetesHelmRelease",
			kind: cloudresourcekind.CloudResourceKind_KubernetesHelmRelease,
			rawOutputs: map[string]interface{}{
				"namespace":    "podinfo",
				"release_name": "podinfo",
				"version":      "6.9.2",
				"app_version":  "6.9.2",
				"status":       "deployed",
				"revision":     1,
			},
			mustPopulate: []string{
				"namespace", "release_name", "version", "app_version",
				"status", "revision",
			},
		},
		{
			// KubernetesAltinityOperator: the served-chart naming contract —
			// release, Deployment (= chart fullname), the chart-managed
			// operator-credentials Secret, and the metrics-exporter endpoint.
			name: "KubernetesAltinityOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesAltinityOperator,
			rawOutputs: map[string]interface{}{
				"namespace":               "clickhouse-operator-system",
				"release_name":            "altinity-op",
				"deployment_name":         "altinity-op",
				"credentials_secret_name": "altinity-op",
				"metrics_endpoint":        "http://altinity-op-metrics.clickhouse-operator-system.svc.cluster.local:8888/metrics",
			},
			mustPopulate: []string{
				"namespace", "release_name", "deployment_name",
				"credentials_secret_name", "metrics_endpoint",
			},
		},
		{
			// KubernetesClickHouse: the operator naming contract — CHI and
			// cluster identity, the cluster-wide client Service with both
			// protocol endpoints, the module-managed auth Secret, and the
			// managed Keeper handles.
			name: "KubernetesClickHouse",
			kind: cloudresourcekind.CloudResourceKind_KubernetesClickHouse,
			rawOutputs: map[string]interface{}{
				"namespace":            "analytics",
				"chi_name":             "events-ch",
				"cluster_name":         "analytics",
				"service_name":         "clickhouse-events-ch",
				"tcp_endpoint":         "clickhouse-events-ch.analytics.svc.cluster.local:9000",
				"http_endpoint":        "http://clickhouse-events-ch.analytics.svc.cluster.local:8123",
				"auth_secret_name":     "events-ch-clickhouse-auth",
				"keeper_name":          "events-ch-keeper",
				"keeper_service_name":  "keeper-events-ch-keeper",
				"port_forward_command": "kubectl port-forward svc/clickhouse-events-ch -n analytics 8123:8123",
			},
			mustPopulate: []string{
				"namespace", "chi_name", "cluster_name", "service_name",
				"tcp_endpoint", "http_endpoint", "auth_secret_name",
				"keeper_name", "keeper_service_name", "port_forward_command",
			},
		},
		{
			// KubernetesHarbor: the pinned-fullname naming contract — front
			// door + per-component Services, the module-generated admin
			// credential Secret, and the port-forward recipe. Nested
			// admin_password_secret{name,key} is the credential handle.
			name: "KubernetesHarbor",
			kind: cloudresourcekind.CloudResourceKind_KubernetesHarbor,
			rawOutputs: map[string]interface{}{
				"namespace":          "harbor",
				"expose_service":     "registry",
				"kube_endpoint":      "http://registry.harbor.svc.cluster.local:80",
				"external_url":       "https://harbor.example.com",
				"core_service":       "registry-core",
				"portal_service":     "registry-portal",
				"registry_service":   "registry-registry",
				"jobservice_service": "registry-jobservice",
				"trivy_service":      "registry-trivy",
				"database_service":   "registry-database",
				"redis_service":      "registry-redis",
				"admin_username":     "admin",
				"admin_password_secret": map[string]interface{}{
					"name": "registry-admin-auth",
					"key":  "HARBOR_ADMIN_PASSWORD",
				},
				"port_forward_command": "kubectl port-forward svc/registry -n harbor 8080:80",
			},
			mustPopulate: []string{
				"namespace", "expose_service", "kube_endpoint", "external_url",
				"core_service", "portal_service", "registry_service",
				"jobservice_service", "trivy_service", "database_service",
				"redis_service", "admin_username", "admin_password_secret",
				"port_forward_command",
			},
		},
		{
			// KubernetesAirflow: the release-name-is-fullname naming
			// contract — the API server front door, the admin credential
			// handle, and the module-composed connection/key Secret names
			// downstream composition mounts.
			name: "KubernetesAirflow",
			kind: cloudresourcekind.CloudResourceKind_KubernetesAirflow,
			rawOutputs: map[string]interface{}{
				"namespace":           "airflow",
				"api_server_service":  "pipelines-api-server",
				"api_server_endpoint": "http://pipelines-api-server.airflow.svc.cluster.local:8080",
				"admin_password_secret": map[string]interface{}{
					"name": "pipelines-admin-auth",
					"key":  "password",
				},
				"metadata_connection_secret_name": "pipelines-metadata-conn",
				"broker_url_secret_name":          "pipelines-broker-url",
				"fernet_key_secret_name":          "pipelines-fernet-key",
				"port_forward_command":            "kubectl port-forward svc/pipelines-api-server -n airflow 8080:8080",
			},
			mustPopulate: []string{
				"namespace", "api_server_service", "api_server_endpoint",
				"admin_password_secret", "metadata_connection_secret_name",
				"broker_url_secret_name", "fernet_key_secret_name",
				"port_forward_command",
			},
		},
		{
			// KubernetesJupyterHub: the chart-fixed bare-name contract
			// (fullnameOverride "" — hub, proxy-public) and the
			// shared-password credential handle.
			name: "KubernetesJupyterHub",
			kind: cloudresourcekind.CloudResourceKind_KubernetesJupyterHub,
			rawOutputs: map[string]interface{}{
				"namespace":            "notebooks",
				"proxy_public_service": "proxy-public",
				"endpoint":             "http://proxy-public.notebooks.svc.cluster.local:80",
				"hub_service":          "hub",
				"shared_password_secret": map[string]interface{}{
					"name": "notebooks-auth",
					"key":  "password",
				},
				"port_forward_command": "kubectl port-forward svc/proxy-public -n notebooks 8080:80",
			},
			mustPopulate: []string{
				"namespace", "proxy_public_service", "endpoint",
				"hub_service", "shared_password_secret",
				"port_forward_command",
			},
		},
		{
			// KubernetesMlflow: the metadata.name-is-resource-name contract
			// (module-owned manifests), the tracking endpoint ML clients
			// point MLFLOW_TRACKING_URI at, the admin credential handle and
			// the composed backend-URI Secret name.
			name: "KubernetesMlflow",
			kind: cloudresourcekind.CloudResourceKind_KubernetesMlflow,
			rawOutputs: map[string]interface{}{
				"namespace":         "mlflow",
				"service":           "experiments",
				"tracking_endpoint": "http://experiments.mlflow.svc.cluster.local:5000",
				"admin_password_secret": map[string]interface{}{
					"name": "experiments-admin-auth",
					"key":  "password",
				},
				"backend_store_uri_secret_name": "experiments-backend-uri",
				"port_forward_command":          "kubectl port-forward svc/experiments -n mlflow 5000:5000",
			},
			mustPopulate: []string{
				"namespace", "service", "tracking_endpoint",
				"admin_password_secret", "backend_store_uri_secret_name",
				"port_forward_command",
			},
		},
		{
			// KubernetesOtelOperator: the pinned-fullname naming contract —
			// the release, the webhook Service the API server calls for
			// admission and CRD conversion, and the cert-manager-issued
			// serving-cert Secret.
			name: "KubernetesOtelOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesOtelOperator,
			rawOutputs: map[string]interface{}{
				"namespace":                "otel-operator-system",
				"release_name":             "otel-operator",
				"webhook_service":          "otel-operator-webhook",
				"webhook_cert_secret_name": "otel-operator-controller-manager-service-cert",
			},
			mustPopulate: []string{
				"namespace", "release_name", "webhook_service",
				"webhook_cert_secret_name",
			},
		},
		{
			// KubernetesOtelCollector: the operator naming contract — the
			// CR name, the receiver-port Service and its headless/
			// monitoring siblings, and the OTLP ingest endpoints (the
			// composition handles applications push telemetry to).
			name: "KubernetesOtelCollector",
			kind: cloudresourcekind.CloudResourceKind_KubernetesOtelCollector,
			rawOutputs: map[string]interface{}{
				"namespace":          "observability",
				"collector_name":     "otel-gateway",
				"service":            "otel-gateway-collector",
				"otlp_grpc_endpoint": "otel-gateway-collector.observability.svc.cluster.local:4317",
				"otlp_http_endpoint": "http://otel-gateway-collector.observability.svc.cluster.local:4318",
				"headless_service":   "otel-gateway-collector-headless",
				"monitoring_service": "otel-gateway-collector-monitoring",
			},
			mustPopulate: []string{
				"namespace", "collector_name", "service",
				"otlp_grpc_endpoint", "otlp_http_endpoint",
				"headless_service", "monitoring_service",
			},
		},
		{
			// KubernetesSeaweedFs: the chart naming contract — release, the
			// S3/filer/master Service handles, both credential Secrets
			// (chart-owned S3, module-generated admin), and the endpoints.
			name: "KubernetesSeaweedFs",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSeaweedFs,
			rawOutputs: map[string]interface{}{
				"namespace":                  "object-store",
				"release_name":               "artifacts",
				"s3_endpoint":                "http://artifacts-s3.object-store.svc.cluster.local:8333",
				"s3_credentials_secret_name": "artifacts-s3-secret",
				"filer_service_name":         "artifacts-filer",
				"master_service_name":        "artifacts-master",
				"admin_endpoint":             "http://artifacts-admin.object-store.svc.cluster.local:23646",
				"admin_auth_secret_name":     "artifacts-admin-auth",
				"port_forward_command":       "kubectl port-forward svc/artifacts-s3 -n object-store 8333:8333",
			},
			mustPopulate: []string{
				"namespace", "release_name", "s3_endpoint",
				"s3_credentials_secret_name", "filer_service_name",
				"master_service_name", "admin_endpoint",
				"admin_auth_secret_name", "port_forward_command",
			},
		},
		{
			// KubernetesQdrant: the chart naming contract — release, the
			// main Service with REST and gRPC endpoints, and the chart-owned
			// API-key Secret handles (read-write + read-only).
			name: "KubernetesQdrant",
			kind: cloudresourcekind.CloudResourceKind_KubernetesQdrant,
			rawOutputs: map[string]interface{}{
				"namespace":                     "vector-search",
				"release_name":                  "embeddings",
				"service_name":                  "embeddings",
				"http_endpoint":                 "http://embeddings.vector-search.svc.cluster.local:6333",
				"grpc_endpoint":                 "embeddings.vector-search.svc.cluster.local:6334",
				"api_key_secret_name":           "embeddings-apikey",
				"read_only_api_key_secret_name": "embeddings-apikey",
				"port_forward_command":          "kubectl port-forward svc/embeddings -n vector-search 6333:6333",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_name", "http_endpoint",
				"grpc_endpoint", "api_key_secret_name",
				"read_only_api_key_secret_name", "port_forward_command",
			},
		},
		{
			// KubernetesKyverno: the chart naming contract — release, the
			// fullname-derived admission webhook Service and the runtime
			// ConfigMap (the engine's skip-list, the first object to
			// inspect when a resource is unexpectedly skipped or policed).
			name: "KubernetesKyverno",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKyverno,
			rawOutputs: map[string]interface{}{
				"namespace":              "kyverno",
				"release_name":           "kyverno",
				"admission_service_name": "kyverno-svc",
				"config_map_name":        "kyverno",
			},
			mustPopulate: []string{
				"namespace", "release_name", "admission_service_name",
				"config_map_name",
			},
		},
		{
			// KubernetesGatekeeper: the chart-FIXED handles (the chart
			// hardcodes its resource names — no fullname derivation): the
			// webhook Service and the cert Secret the embedded rotator
			// maintains.
			name: "KubernetesGatekeeper",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGatekeeper,
			rawOutputs: map[string]interface{}{
				"namespace":                "gatekeeper-system",
				"release_name":             "gatekeeper",
				"webhook_service_name":     "gatekeeper-webhook-service",
				"webhook_cert_secret_name": "gatekeeper-webhook-server-cert",
			},
			mustPopulate: []string{
				"namespace", "release_name", "webhook_service_name",
				"webhook_cert_secret_name",
			},
		},
		{
			// KubernetesRabbitMqOperator: the release manifest's fixed
			// handles (namespace, Deployment, metrics endpoint, the CRD the
			// manifest installs and deletes with the resource).
			name: "KubernetesRabbitMqOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesRabbitMqOperator,
			rawOutputs: map[string]interface{}{
				"namespace":        "rabbitmq-system",
				"deployment_name":  "rabbitmq-cluster-operator",
				"metrics_endpoint": "http://rabbitmq-cluster-operator-metrics-service.rabbitmq-system.svc.cluster.local:8080/metrics",
				"crd_name":         "rabbitmqclusters.rabbitmq.com",
			},
			mustPopulate: []string{
				"namespace", "deployment_name", "metrics_endpoint", "crd_name",
			},
		},
		{
			// KubernetesRabbitMq: the operator naming contract — the client
			// and headless Services, both client endpoints, and the
			// operator-generated default-user Secret handle.
			name: "KubernetesRabbitMq",
			kind: cloudresourcekind.CloudResourceKind_KubernetesRabbitMq,
			rawOutputs: map[string]interface{}{
				"namespace":                "messaging",
				"cluster_name":             "orders-mq",
				"service_name":             "orders-mq",
				"headless_service_name":    "orders-mq-nodes",
				"amqp_endpoint":            "orders-mq.messaging.svc.cluster.local:5672",
				"management_endpoint":      "http://orders-mq.messaging.svc.cluster.local:15672",
				"default_user_secret_name": "orders-mq-default-user",
				"port_forward_command":     "kubectl port-forward svc/orders-mq -n messaging 15672:15672",
			},
			mustPopulate: []string{
				"namespace", "cluster_name", "service_name",
				"headless_service_name", "amqp_endpoint",
				"management_endpoint", "default_user_secret_name",
				"port_forward_command",
			},
		},
		{
			// KubernetesKubePrometheusStack: the pinned-fullname naming contract —
			// every child service derives from the resource name
			// (`-prometheus`/`-alertmanager`/`-grafana`), the endpoints downstream
			// datasources compose against, and the bundled Grafana's admin-Secret
			// handle.
			name: "KubernetesKubePrometheusStack",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKubePrometheusStack,
			rawOutputs: map[string]interface{}{
				"namespace":                       "observability",
				"release_name":                    "monitoring",
				"prometheus_service":              "monitoring-prometheus",
				"prometheus_endpoint":             "http://monitoring-prometheus.observability.svc.cluster.local:9090",
				"alertmanager_service":            "monitoring-alertmanager",
				"alertmanager_endpoint":           "http://monitoring-alertmanager.observability.svc.cluster.local:9093",
				"grafana_service":                 "monitoring-grafana",
				"grafana_endpoint":                "http://monitoring-grafana.observability.svc.cluster.local",
				"grafana_admin_secret_name":       "monitoring-grafana",
				"prometheus_port_forward_command": "kubectl port-forward svc/monitoring-prometheus -n observability 9090:9090",
				"grafana_port_forward_command":    "kubectl port-forward svc/monitoring-grafana -n observability 3000:80",
			},
			mustPopulate: []string{
				"namespace", "release_name", "prometheus_service",
				"prometheus_endpoint", "alertmanager_service",
				"alertmanager_endpoint", "grafana_service", "grafana_endpoint",
				"grafana_admin_secret_name", "prometheus_port_forward_command",
				"grafana_port_forward_command",
			},
		},
		{
			// KubernetesGrafana: the pinned-fullname naming contract — the Service
			// and the chart-generated admin Secret share the resource name — plus
			// the endpoint and port-forward composition handles.
			name: "KubernetesGrafana",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGrafana,
			rawOutputs: map[string]interface{}{
				"namespace":            "observability",
				"release_name":         "dashboards",
				"service":              "dashboards",
				"endpoint":             "http://dashboards.observability.svc.cluster.local",
				"admin_secret_name":    "dashboards",
				"port_forward_command": "kubectl port-forward svc/dashboards -n observability 3000:80",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service", "endpoint",
				"admin_secret_name", "port_forward_command",
			},
		},
		{
			// KubernetesArgocd: the pinned-fullname naming contract — the server
			// Service is `<name>-server` — plus the APPLICATION-owned initial
			// admin Secret handle (fixed name, created by Argo CD itself at
			// first start) and the endpoint/port-forward composition handles.
			name: "KubernetesArgocd",
			kind: cloudresourcekind.CloudResourceKind_KubernetesArgocd,
			rawOutputs: map[string]interface{}{
				"namespace":                 "gitops",
				"release_name":              "delivery",
				"server_service":            "delivery-server",
				"server_kube_endpoint":      "https://delivery-server.gitops.svc.cluster.local",
				"initial_admin_secret_name": "argocd-initial-admin-secret",
				"port_forward_command":      "kubectl port-forward svc/delivery-server -n gitops 8080:443",
			},
			mustPopulate: []string{
				"namespace", "release_name", "server_service",
				"server_kube_endpoint", "initial_admin_secret_name",
				"port_forward_command",
			},
		},
		{
			// KubernetesTektonOperator: the release manifest's fixed
			// namespace — the one handle the manifest-bundle install exports.
			name: "KubernetesTektonOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesTektonOperator,
			rawOutputs: map[string]interface{}{
				"namespace": "tekton-operator",
			},
			mustPopulate: []string{"namespace"},
		},
		{
			// KubernetesTekton: the resolved installation handles — target
			// namespace, profile, and the dashboard Service handles exposure
			// kinds compose against (populated on profile `all`).
			name: "KubernetesTekton",
			kind: cloudresourcekind.CloudResourceKind_KubernetesTekton,
			rawOutputs: map[string]interface{}{
				"namespace":               "tekton-pipelines",
				"profile":                 "all",
				"dashboard_service":       "tekton-dashboard",
				"dashboard_kube_endpoint": "http://tekton-dashboard.tekton-pipelines.svc.cluster.local:9097",
				"port_forward_command":    "kubectl port-forward -n tekton-pipelines service/tekton-dashboard 9097:9097",
			},
			mustPopulate: []string{
				"namespace", "profile", "dashboard_service",
				"dashboard_kube_endpoint", "port_forward_command",
			},
		},
		{
			// KubernetesPlantonOperator: the installation handles — the
			// namespace and the fixed release name (one operator per
			// cluster, self-enforced at startup).
			name: "KubernetesPlantonOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPlantonOperator,
			rawOutputs: map[string]interface{}{
				"namespace":    "planton-operator",
				"release_name": "planton-operator",
			},
			mustPopulate: []string{"namespace", "release_name"},
		},
		{
			// KubernetesPlantonPlatform: the first-visit handles — the
			// platform's namespace and name (the prefix of every
			// operator-created object), the gateway Service, the
			// setup-code Secret, and the two exact commands a person (or
			// the desktop's connect-existing flow) runs.
			name: "KubernetesPlantonPlatform",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPlantonPlatform,
			rawOutputs: map[string]interface{}{
				"namespace":            "planton",
				"platform_name":        "planton",
				"gateway_service":      "planton-gateway",
				"setup_code_secret":    "planton-identity-setup-code",
				"port_forward_command": "kubectl port-forward -n planton svc/planton-gateway 8080:80",
				"setup_code_command":   "kubectl -n planton get secret planton-identity-setup-code -o jsonpath='{.data.setup-code}' | base64 -d",
			},
			mustPopulate: []string{
				"namespace", "platform_name", "gateway_service",
				"setup_code_secret", "port_forward_command", "setup_code_command",
			},
		},
		{
			// KubernetesGhaRunnerScaleSetController: the pinned-fullname
			// naming contract — the controller ServiceAccount equals the
			// release name — the handle fenced scale sets reference.
			name: "KubernetesGhaRunnerScaleSetController",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGhaRunnerScaleSetController,
			rawOutputs: map[string]interface{}{
				"namespace":            "arc-system",
				"release_name":         "arc",
				"service_account_name": "arc",
			},
			mustPopulate: []string{
				"namespace", "release_name", "service_account_name",
			},
		},
		{
			// KubernetesGhaRunnerScaleSet: the GitHub-visible fleet identity —
			// the exact `runs-on:` value — plus the registration URL.
			name: "KubernetesGhaRunnerScaleSet",
			kind: cloudresourcekind.CloudResourceKind_KubernetesGhaRunnerScaleSet,
			rawOutputs: map[string]interface{}{
				"namespace":             "ci-runners",
				"release_name":          "build-runners",
				"runner_scale_set_name": "build-runners",
				"github_config_url":     "https://github.com/my-org/my-repo",
			},
			mustPopulate: []string{
				"namespace", "release_name", "runner_scale_set_name",
				"github_config_url",
			},
		},
		{
			// KubernetesArgoWorkflows: the pinned-fullname naming contract — the
			// Argo server Service is `<name>-server` — plus the runner
			// ServiceAccount handle (the identity to annotate for
			// IRSA/workload identity) and the endpoint/port-forward handles.
			name: "KubernetesArgoWorkflows",
			kind: cloudresourcekind.CloudResourceKind_KubernetesArgoWorkflows,
			rawOutputs: map[string]interface{}{
				"namespace":                "pipelines",
				"release_name":             "runs",
				"server_service":           "runs-server",
				"server_kube_endpoint":     "http://runs-server.pipelines.svc.cluster.local:2746",
				"workflow_service_account": "argo-workflow",
				"port_forward_command":     "kubectl port-forward svc/runs-server -n pipelines 2746:2746",
			},
			mustPopulate: []string{
				"namespace", "release_name", "server_service",
				"server_kube_endpoint", "workflow_service_account",
				"port_forward_command",
			},
		},
		{
			// KubernetesSignoz: the pinned-fullname naming contract — the server
			// Service (= the release name), the `<name>-otel-collector` ingestion
			// handles, and the composed-ClickHouse connection passthroughs
			// including the password Secret handle (nested name/key — the
			// flat-vs-nested drift class this guard exists for).
			name: "KubernetesSignoz",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSignoz,
			rawOutputs: map[string]interface{}{
				"namespace":              "observability",
				"service":                "observe",
				"kube_endpoint":          "http://observe.observability.svc.cluster.local:8080",
				"port_forward_command":   "kubectl port-forward svc/observe -n observability 8080:8080",
				"otel_collector_service": "observe-otel-collector",
				"otlp_grpc_endpoint":     "observe-otel-collector.observability.svc.cluster.local:4317",
				"otlp_http_endpoint":     "http://observe-otel-collector.observability.svc.cluster.local:4318",
				"clickhouse_endpoint":    "clickhouse-telemetry.observability.svc.cluster.local:9000",
				"clickhouse_username":    "signoz",
				"clickhouse_password_secret": map[string]interface{}{
					"name": "telemetry-clickhouse-auth",
					"key":  "signoz",
				},
			},
			mustPopulate: []string{
				"namespace", "service", "kube_endpoint", "port_forward_command",
				"otel_collector_service", "otlp_grpc_endpoint", "otlp_http_endpoint",
				"clickhouse_endpoint", "clickhouse_username",
				"clickhouse_password_secret",
			},
		},
		{
			// AwsSubnet: flat scalar outputs from both engines (subnet id/arn, AZ,
			// CIDR, route table id, region) must each land on the StackOutputs proto.
			name: "AwsSubnet",
			kind: cloudresourcekind.CloudResourceKind_AwsSubnet,
			rawOutputs: map[string]interface{}{
				"subnet_id":         "subnet-0abc123",
				"subnet_arn":        "arn:aws:ec2:us-west-2:123456789012:subnet/subnet-0abc123",
				"availability_zone": "us-west-2a",
				"cidr_block":        "10.0.1.0/24",
				"route_table_id":    "rtb-0abc123",
				"region":            "us-west-2",
			},
			mustPopulate: []string{
				"subnet_id", "subnet_arn", "availability_zone",
				"cidr_block", "route_table_id", "region",
			},
		},
		{
			// AwsInternetGateway: flat scalar outputs from both engines (gateway
			// id/arn, attached vpc id, region) must each land on the StackOutputs proto.
			name: "AwsInternetGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsInternetGateway,
			rawOutputs: map[string]interface{}{
				"internet_gateway_id":  "igw-0abc123",
				"internet_gateway_arn": "arn:aws:ec2:us-west-2:123456789012:internet-gateway/igw-0abc123",
				"vpc_id":               "vpc-0abc123",
				"region":               "us-west-2",
			},
			mustPopulate: []string{
				"internet_gateway_id", "internet_gateway_arn", "vpc_id", "region",
			},
		},
		{
			// AwsEgressOnlyInternetGateway: flat scalar outputs from both engines
			// (gateway id, attached vpc id, region) must each land on the StackOutputs
			// proto. An egress-only gateway has no ARN, so none is emitted.
			name: "AwsEgressOnlyInternetGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsEgressOnlyInternetGateway,
			rawOutputs: map[string]interface{}{
				"egress_only_internet_gateway_id": "eigw-0abc123",
				"vpc_id":                          "vpc-0abc123",
				"region":                          "us-west-2",
			},
			mustPopulate: []string{
				"egress_only_internet_gateway_id", "vpc_id", "region",
			},
		},
		{
			// GcpServiceAccount: flat scalar outputs from both engines (email, the
			// ready-made IAM member string, stable unique id, fully-qualified name,
			// and the optional key) must each land on the StackOutputs proto.
			name: "GcpServiceAccount",
			kind: cloudresourcekind.CloudResourceKind_GcpServiceAccount,
			rawOutputs: map[string]interface{}{
				"email":      "my-sa@my-project.iam.gserviceaccount.com",
				"member":     "serviceAccount:my-sa@my-project.iam.gserviceaccount.com",
				"unique_id":  "112233445566778899000",
				"name":       "projects/my-project/serviceAccounts/my-sa@my-project.iam.gserviceaccount.com",
				"key_base64": "eyJ0eXBlIjoic2VydmljZV9hY2NvdW50In0=",
			},
			mustPopulate: []string{
				"email", "member", "unique_id", "name", "key_base64",
			},
		},
		{
			// GcpIamCustomRole: flat scalar outputs from both engines (the grantable
			// fully-qualified role name, the bare role id, and the soft-delete flag)
			// must each land on the StackOutputs proto.
			name: "GcpIamCustomRole",
			kind: cloudresourcekind.CloudResourceKind_GcpIamCustomRole,
			rawOutputs: map[string]interface{}{
				"name":    "projects/my-project/roles/logBucketWriter",
				"role_id": "logBucketWriter",
				"deleted": "false",
			},
			mustPopulate: []string{
				"name", "role_id",
			},
		},
		{
			// GcpProjectIamMember: the grant tuple echoed by both engines (project,
			// role, member, policy etag) must each land on the StackOutputs proto.
			name: "GcpProjectIamMember",
			kind: cloudresourcekind.CloudResourceKind_GcpProjectIamMember,
			rawOutputs: map[string]interface{}{
				"project_id": "my-project",
				"role":       "projects/my-project/roles/logBucketWriter",
				"member":     "serviceAccount:my-sa@my-project.iam.gserviceaccount.com",
				"etag":       "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"project_id", "role", "member", "etag",
			},
		},
		{
			// GcpServiceAccountIamMember: the grant tuple echoed by both engines
			// (the account's fully-qualified name, role, member, policy etag)
			// must each land on the StackOutputs proto.
			name: "GcpServiceAccountIamMember",
			kind: cloudresourcekind.CloudResourceKind_GcpServiceAccountIamMember,
			rawOutputs: map[string]interface{}{
				"service_account_id": "projects/my-project/serviceAccounts/deployer@my-project.iam.gserviceaccount.com",
				"role":               "roles/iam.workloadIdentityUser",
				"member":             "principalSet://iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/github/attribute.repository/my-org/my-repo",
				"etag":               "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"service_account_id", "role", "member", "etag",
			},
		},
		{
			// GcpKmsKeyIamMember: the grant tuple echoed by both engines (the
			// key's fully-qualified path, role, member, policy etag) must each
			// land on the StackOutputs proto.
			name: "GcpKmsKeyIamMember",
			kind: cloudresourcekind.CloudResourceKind_GcpKmsKeyIamMember,
			rawOutputs: map[string]interface{}{
				"crypto_key_id": "projects/my-project/locations/us-central1/keyRings/app-ring/cryptoKeys/state-key",
				"role":          "roles/cloudkms.cryptoKeyEncrypterDecrypter",
				"member":        "serviceAccount:service-123456789@gs-project-accounts.iam.gserviceaccount.com",
				"etag":          "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"crypto_key_id", "role", "member", "etag",
			},
		},
		{
			// GcpWorkloadIdentityPool: flat scalar outputs from both engines (the
			// full pool resource name principals embed, the bare pool id providers
			// reference, and the lifecycle state) must each land on the
			// StackOutputs proto.
			name: "GcpWorkloadIdentityPool",
			kind: cloudresourcekind.CloudResourceKind_GcpWorkloadIdentityPool,
			rawOutputs: map[string]interface{}{
				"name":                      "projects/123456789/locations/global/workloadIdentityPools/github-actions",
				"workload_identity_pool_id": "github-actions",
				"state":                     "ACTIVE",
			},
			mustPopulate: []string{
				"name", "workload_identity_pool_id", "state",
			},
		},
		{
			// GcpWorkloadIdentityPoolProvider: flat scalar outputs from both
			// engines (the full provider resource name — the token-exchange
			// audience — the bare provider id, and the lifecycle state) must each
			// land on the StackOutputs proto.
			name: "GcpWorkloadIdentityPoolProvider",
			kind: cloudresourcekind.CloudResourceKind_GcpWorkloadIdentityPoolProvider,
			rawOutputs: map[string]interface{}{
				"name":                               "projects/123456789/locations/global/workloadIdentityPools/github-actions/providers/github-oidc",
				"workload_identity_pool_provider_id": "github-oidc",
				"state":                              "ACTIVE",
			},
			mustPopulate: []string{
				"name", "workload_identity_pool_provider_id", "state",
			},
		},
		{
			// GcpHealthCheck: flat scalar outputs from both engines (the self-link
			// backend services reference, the cloud-side name, the computed probe
			// type, and the scope-marking region — empty for global) must each
			// land on the StackOutputs proto.
			name: "GcpHealthCheck",
			kind: cloudresourcekind.CloudResourceKind_GcpHealthCheck,
			rawOutputs: map[string]interface{}{
				"self_link":         "https://www.googleapis.com/compute/v1/projects/my-project/global/healthChecks/web-probe",
				"health_check_name": "web-probe",
				"type":              "HTTP",
				"region":            "",
			},
			mustPopulate: []string{
				"self_link", "health_check_name", "type",
			},
		},
		{
			// GcpComputeMig: branch-independent outputs from both engines and
			// both location arms (one(concat()) collapsed in Terraform) — the
			// instance-group URL a backend service's group edge consumes, the
			// manager self link, the rotating template link, the group name,
			// and the location.
			name: "GcpComputeMig",
			kind: cloudresourcekind.CloudResourceKind_GcpComputeMig,
			rawOutputs: map[string]interface{}{
				"instance_group":             "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroups/web-tier",
				"self_link":                  "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroupManagers/web-tier",
				"current_template_self_link": "https://www.googleapis.com/compute/v1/projects/my-project/global/instanceTemplates/web-tier-20260811100000000?uniqueId=1234567890",
				"mig_name":                   "web-tier",
				"location":                   "us-central1-a",
			},
			mustPopulate: []string{
				"instance_group", "self_link", "current_template_self_link", "mig_name", "location",
			},
		},
		{
			// GcpArtifactRegistryRepo: the short name, the fully qualified
			// repository path composing resources consume (function
			// docker_repository, virtual/remote upstreams), the registry
			// endpoint clients push to, and the location.
			name: "GcpArtifactRegistryRepo",
			kind: cloudresourcekind.CloudResourceKind_GcpArtifactRegistryRepo,
			rawOutputs: map[string]interface{}{
				"name":            "app-images",
				"repository_path": "projects/prod-project/locations/us-central1/repositories/app-images",
				"registry_uri":    "us-central1-docker.pkg.dev/prod-project/app-images",
				"location":        "us-central1",
			},
			mustPopulate: []string{"name", "repository_path", "registry_uri", "location"},
		},
		{
			// GcpGcsBucket: bucket_id (the name every consumer references),
			// the name alias, the gs:// URL, the API self link, the
			// upper-cased location, and the numeric owning project.
			name: "GcpGcsBucket",
			kind: cloudresourcekind.CloudResourceKind_GcpGcsBucket,
			rawOutputs: map[string]interface{}{
				"bucket_id":      "prod-data-lake",
				"bucket_name":    "prod-data-lake",
				"url":            "gs://prod-data-lake",
				"self_link":      "https://www.googleapis.com/storage/v1/b/prod-data-lake",
				"location":       "US-EAST1",
				"project_number": 123456789012,
			},
			mustPopulate: []string{
				"bucket_id", "bucket_name", "url", "self_link", "location", "project_number",
			},
		},
		{
			// GcpComputeInstance: flat scalar outputs from both engines (the
			// cloud-side name, numeric id, self link the disk/instance-group
			// consumers reference, both IPs — external empty for private
			// VMs — status, zone, machine type, CPU platform) must each land
			// on the StackOutputs proto.
			name: "GcpComputeInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpComputeInstance,
			rawOutputs: map[string]interface{}{
				"instance_name": "pg-primary",
				"instance_id":   "4123456789012345678",
				"self_link":     "https://www.googleapis.com/compute/v1/projects/prod-project/zones/us-central1-a/instances/pg-primary",
				"internal_ip":   "10.10.0.5",
				"external_ip":   "",
				"status":        "RUNNING",
				"zone":          "us-central1-a",
				"machine_type":  "n2-standard-8",
				"cpu_platform":  "Intel Ice Lake",
			},
			mustPopulate: []string{
				"instance_name", "instance_id", "self_link", "internal_ip",
				"status", "zone", "machine_type", "cpu_platform",
			},
		},
		{
			// GcpComputeDisk: flat scalar outputs from both engines (the
			// cloud-side name, numeric id, the self link attached_disks
			// consume, the plain zone, size, and the normalized plain type
			// name) must each land on the StackOutputs proto.
			name: "GcpComputeDisk",
			kind: cloudresourcekind.CloudResourceKind_GcpComputeDisk,
			rawOutputs: map[string]interface{}{
				"name":      "pg-data",
				"disk_id":   "7123456789012345678",
				"self_link": "https://www.googleapis.com/compute/v1/projects/prod-project/zones/us-central1-a/disks/pg-data",
				"zone":      "us-central1-a",
				"size_gb":   500,
				"type":      "pd-ssd",
			},
			mustPopulate: []string{
				"name", "disk_id", "self_link", "zone", "size_gb", "type",
			},
		},
		{
			// GcpFilestoreInstance: the fully qualified instance path
			// (replication peers consume it), the short name, the share's
			// mount addresses, the share name, timestamps, the GCP-resolved
			// reserved range, and the concurrency ETag.
			name: "GcpFilestoreInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpFilestoreInstance,
			rawOutputs: map[string]interface{}{
				"instance_id":       "projects/prod-project/locations/us-central1-a/instances/shared-nfs",
				"instance_name":     "shared-nfs",
				"ip_addresses":      []interface{}{"10.20.0.2"},
				"file_share_name":   "vol1",
				"create_time":       "2026-07-08T10:00:00Z",
				"reserved_ip_range": "10.20.0.0/29",
				"etag":              "abc123",
			},
			mustPopulate: []string{
				"instance_id", "instance_name", "ip_addresses", "file_share_name",
				"create_time", "reserved_ip_range", "etag",
			},
		},
		{
			// GcpBackendBucket: flat scalar outputs from both engines (the
			// self-link URL maps reference, the cloud-side name, and the origin
			// bucket) must each land on the StackOutputs proto.
			name: "GcpBackendBucket",
			kind: cloudresourcekind.CloudResourceKind_GcpBackendBucket,
			rawOutputs: map[string]interface{}{
				"self_link":           "https://www.googleapis.com/compute/v1/projects/my-project/global/backendBuckets/static-assets",
				"backend_bucket_name": "static-assets",
				"bucket_name":         "my-assets-bucket",
			},
			mustPopulate: []string{
				"self_link", "backend_bucket_name", "bucket_name",
			},
		},
		{
			// GcpBackendService: flat scalar outputs from both engines (the
			// self-link URL maps reference, the cloud-side name, the numeric id,
			// and the concurrency fingerprint) must each land on the
			// StackOutputs proto.
			name: "GcpBackendService",
			kind: cloudresourcekind.CloudResourceKind_GcpBackendService,
			rawOutputs: map[string]interface{}{
				"self_link":            "https://www.googleapis.com/compute/v1/projects/my-project/global/backendServices/web-backend",
				"backend_service_name": "web-backend",
				"generated_id":         "1234567890123456789",
				"fingerprint":          "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"self_link", "backend_service_name", "generated_id", "fingerprint",
			},
		},
		{
			// GcpRegionNetworkEndpointGroup: flat scalar outputs from both engines
			// (self-link, name, endpoint type, region) must land on StackOutputs.
			name: "GcpRegionNetworkEndpointGroup",
			kind: cloudresourcekind.CloudResourceKind_GcpRegionNetworkEndpointGroup,
			rawOutputs: map[string]interface{}{
				"self_link":                   "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/networkEndpointGroups/my-neg",
				"network_endpoint_group_name": "my-neg",
				"network_endpoint_type":       "SERVERLESS",
				"region":                      "us-central1",
			},
			mustPopulate: []string{
				"self_link", "network_endpoint_group_name", "network_endpoint_type", "region",
			},
		},
		{
			// GcpUrlMap: flat scalar outputs from both engines (self-link, name,
			// numeric id, fingerprint) must land on StackOutputs.
			name: "GcpUrlMap",
			kind: cloudresourcekind.CloudResourceKind_GcpUrlMap,
			rawOutputs: map[string]interface{}{
				"self_link":    "https://www.googleapis.com/compute/v1/projects/my-project/global/urlMaps/my-map",
				"url_map_name": "my-map",
				"map_id":       "1234567890123456789",
				"fingerprint":  "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"self_link", "url_map_name", "map_id", "fingerprint",
			},
		},
		{
			// GcpManagedSslCertificate: flat scalar outputs from both engines
			// (self-link, name, id, expire_time) must land on StackOutputs.
			name: "GcpManagedSslCertificate",
			kind: cloudresourcekind.CloudResourceKind_GcpManagedSslCertificate,
			rawOutputs: map[string]interface{}{
				"self_link":        "https://www.googleapis.com/compute/v1/projects/my-project/global/sslCertificates/my-cert",
				"certificate_name": "my-cert",
				"certificate_id":   "1234567890123456789",
				"expire_time":      "2027-01-01T00:00:00Z",
			},
			mustPopulate: []string{
				"self_link", "certificate_name", "certificate_id", "expire_time",
			},
		},
		{
			// GcpTargetHttpProxy: flat scalar outputs from both engines
			// (self-link, name, numeric id, fingerprint) must land on StackOutputs.
			name: "GcpTargetHttpProxy",
			kind: cloudresourcekind.CloudResourceKind_GcpTargetHttpProxy,
			rawOutputs: map[string]interface{}{
				"self_link":   "https://www.googleapis.com/compute/v1/projects/my-project/global/targetHttpProxies/my-proxy",
				"proxy_name":  "my-proxy",
				"proxy_id":    "1234567890123456789",
				"fingerprint": "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"self_link", "proxy_name", "proxy_id", "fingerprint",
			},
		},
		{
			// GcpTargetHttpsProxy: flat scalar outputs from both engines
			// (self-link, name, numeric id, fingerprint) must land on StackOutputs.
			name: "GcpTargetHttpsProxy",
			kind: cloudresourcekind.CloudResourceKind_GcpTargetHttpsProxy,
			rawOutputs: map[string]interface{}{
				"self_link":   "https://www.googleapis.com/compute/v1/projects/my-project/global/targetHttpsProxies/my-proxy",
				"proxy_name":  "my-proxy",
				"proxy_id":    "1234567890123456789",
				"fingerprint": "BwYn2FQlJeM=",
			},
			mustPopulate: []string{
				"self_link", "proxy_name", "proxy_id", "fingerprint",
			},
		},
		{
			// GcpGlobalForwardingRule: flat scalar outputs from both engines (the
			// VIP, self-link, name, numeric id, and the PSC connection fields)
			// must land on StackOutputs.
			name: "GcpGlobalForwardingRule",
			kind: cloudresourcekind.CloudResourceKind_GcpGlobalForwardingRule,
			rawOutputs: map[string]interface{}{
				"ip_address":            "34.120.1.2",
				"self_link":             "https://www.googleapis.com/compute/v1/projects/my-project/global/forwardingRules/my-frontend",
				"forwarding_rule_name":  "my-frontend",
				"forwarding_rule_id":    "1234567890123456789",
				"psc_connection_id":     "1111222233334444",
				"psc_connection_status": "ACCEPTED",
			},
			mustPopulate: []string{
				"ip_address", "self_link", "forwarding_rule_name",
				"forwarding_rule_id", "psc_connection_id", "psc_connection_status",
			},
		},
		{
			// GcpSslPolicy: flat scalar outputs plus the repeated enabled_features
			// cipher list both engines emit (per-index keys) must land on
			// StackOutputs.
			name: "GcpSslPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpSslPolicy,
			rawOutputs: map[string]interface{}{
				"self_link":          "https://www.googleapis.com/compute/v1/projects/my-project/global/sslPolicies/my-policy",
				"ssl_policy_name":    "my-policy",
				"enabled_features.0": "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"enabled_features.1": "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
				"region":             "",
			},
			mustPopulate: []string{
				"self_link", "ssl_policy_name", "enabled_features",
			},
		},
		{
			// GcpMonitoringNotificationChannel: the server-assigned channel
			// resource name (the alert-policy composition handle) and the
			// verification state both engines emit must land on StackOutputs.
			name: "GcpMonitoringNotificationChannel",
			kind: cloudresourcekind.CloudResourceKind_GcpMonitoringNotificationChannel,
			rawOutputs: map[string]interface{}{
				"channel_name":        "projects/my-project/notificationChannels/1234567890",
				"verification_status": "VERIFICATION_STATUS_UNSPECIFIED",
			},
			mustPopulate: []string{
				"channel_name", "verification_status",
			},
		},
		{
			// GcpMonitoringAlertPolicy: the policy resource name is the single
			// output both engines emit.
			name: "GcpMonitoringAlertPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpMonitoringAlertPolicy,
			rawOutputs: map[string]interface{}{
				"policy_name": "projects/my-project/alertPolicies/1234567890",
			},
			mustPopulate: []string{
				"policy_name",
			},
		},
		{
			// GcpMonitoringUptimeCheck: the full resource name plus the bare
			// check id (the value alert policies filter on) must land on
			// StackOutputs.
			name: "GcpMonitoringUptimeCheck",
			kind: cloudresourcekind.CloudResourceKind_GcpMonitoringUptimeCheck,
			rawOutputs: map[string]interface{}{
				"uptime_check_name": "projects/my-project/uptimeCheckConfigs/my-check-id",
				"uptime_check_id":   "my-check-id",
			},
			mustPopulate: []string{
				"uptime_check_name", "uptime_check_id",
			},
		},
		{
			// GcpLoggingSink: the sink name and the writer identity (the
			// grant-me-on-the-destination handle) both engines emit must land
			// on StackOutputs.
			name: "GcpLoggingSink",
			kind: cloudresourcekind.CloudResourceKind_GcpLoggingSink,
			rawOutputs: map[string]interface{}{
				"sink_name":       "error-archive",
				"writer_identity": "serviceAccount:service-1234@gcp-sa-logging.iam.gserviceaccount.com",
			},
			mustPopulate: []string{
				"sink_name", "writer_identity",
			},
		},
		{
			// GcpSecretManagerSecret: the full secret name, the short id, and
			// the seeded version handle both engines emit must land on
			// StackOutputs.
			name: "GcpSecretManagerSecret",
			kind: cloudresourcekind.CloudResourceKind_GcpSecretManagerSecret,
			rawOutputs: map[string]interface{}{
				"secret_name":         "projects/my-project/secrets/db-password",
				"secret_id":           "db-password",
				"latest_version_name": "projects/my-project/secrets/db-password/versions/1",
			},
			mustPopulate: []string{
				"secret_name", "secret_id", "latest_version_name",
			},
		},
		{
			// GcpIdentityPlatformConfig: the config's resource name plus the
			// client-SDK bootstrap pair (API key, Firebase subdomain) both
			// engines emit must land on StackOutputs. The api_key is a live
			// credential and rides like any other scalar here (the
			// GcpServiceAccount key grain).
			name: "GcpIdentityPlatformConfig",
			kind: cloudresourcekind.CloudResourceKind_GcpIdentityPlatformConfig,
			rawOutputs: map[string]interface{}{
				"config_name":        "projects/my-project/config",
				"api_key":            "AIzaSyExampleKey123",
				"firebase_subdomain": "my-project",
			},
			mustPopulate: []string{
				"config_name", "api_key", "firebase_subdomain",
			},
		},
		{
			// GcpIdentityPlatformTenant: the server-generated tenant ID (the
			// value client SDKs scope sign-in with) and the full resource
			// name both engines emit must land on StackOutputs.
			name: "GcpIdentityPlatformTenant",
			kind: cloudresourcekind.CloudResourceKind_GcpIdentityPlatformTenant,
			rawOutputs: map[string]interface{}{
				"tenant_id":   "acme-corp-x7k2p",
				"tenant_name": "projects/my-project/tenants/acme-corp-x7k2p",
			},
			mustPopulate: []string{
				"tenant_id", "tenant_name",
			},
		},
		{
			// GcpIamOauthClient: the system-generated client ID, the full
			// resource name, the lifecycle state, and the first credential's
			// secret both engines emit must land on StackOutputs. The secret
			// rides like any other scalar here (the GcpServiceAccount key
			// grain).
			name: "GcpIamOauthClient",
			kind: cloudresourcekind.CloudResourceKind_GcpIamOauthClient,
			rawOutputs: map[string]interface{}{
				"client_id":     "example-client-id",
				"client_name":   "projects/my-project/locations/global/oauthClients/example-client-id",
				"state":         "ACTIVE",
				"client_secret": "generated-secret-value",
			},
			mustPopulate: []string{
				"client_id", "client_name", "state", "client_secret",
			},
		},
		{
			// GcpIamDenyPolicy: the policy identifier (URL-encoded parent +
			// name) and the etag both engines emit must land on StackOutputs.
			name: "GcpIamDenyPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpIamDenyPolicy,
			rawOutputs: map[string]interface{}{
				"policy_name": "cloudresourcemanager.googleapis.com%2Fprojects%2Fmy-project/guard-secrets",
				"etag":        "BwXhCq4YlSo=",
			},
			mustPopulate: []string{
				"policy_name", "etag",
			},
		},
		{
			// GcpCloudRunDomainMapping: scalars plus the server-decided DNS
			// record list. Both engines export resource_records as ONE
			// structured list (the record count is unknowable at program
			// time — a root domain gets A/AAAA sets, a subdomain one CNAME),
			// which the flattener decomposes onto the repeated proto message
			// by dot-indexed keys.
			name: "GcpCloudRunDomainMapping",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudRunDomainMapping,
			rawOutputs: map[string]interface{}{
				"domain":            "app.example.com",
				"region":            "us-central1",
				"mapped_route_name": "my-service",
				"resource_records": []interface{}{
					map[string]interface{}{
						"record_type": "CNAME",
						"record_name": "app",
						"rrdata":      "ghs.googlehosted.com.",
					},
				},
			},
			mustPopulate: []string{
				"domain", "region", "mapped_route_name",
			},
		},
		{
			// GcpMonitoringDashboard: the server-assigned dashboard resource
			// name both engines emit must land on StackOutputs.
			name: "GcpMonitoringDashboard",
			kind: cloudresourcekind.CloudResourceKind_GcpMonitoringDashboard,
			rawOutputs: map[string]interface{}{
				"dashboard_name": "projects/my-project/dashboards/12345678-abcd-4321-9876-fedcba098765",
			},
			mustPopulate: []string{
				"dashboard_name",
			},
		},
		{
			// GcpMonitoringSlo: the SLO's resource name (the burn-rate alert
			// handle) plus the measured service's name — both derived from
			// the SLO's server-assigned name in both engines — must land on
			// StackOutputs.
			name: "GcpMonitoringSlo",
			kind: cloudresourcekind.CloudResourceKind_GcpMonitoringSlo,
			rawOutputs: map[string]interface{}{
				"slo_name":     "projects/my-project/services/checkout/serviceLevelObjectives/availability-slo",
				"service_name": "projects/my-project/services/checkout",
			},
			mustPopulate: []string{
				"slo_name", "service_name",
			},
		},
		{
			// GcpLogBucket: the bucket's full resource name (the sink/metric
			// composition handle) plus the linked BigQuery dataset id (empty
			// when the link is not armed — both engines emit the empty
			// string then).
			name: "GcpLogBucket",
			kind: cloudresourcekind.CloudResourceKind_GcpLogBucket,
			rawOutputs: map[string]interface{}{
				"bucket_name":       "projects/my-project/locations/global/buckets/audit-logs",
				"linked_dataset_id": "audit_logs",
			},
			mustPopulate: []string{
				"bucket_name", "linked_dataset_id",
			},
		},
		{
			// GcpLogMetric: the metric name (addressed from Monitoring as
			// logging.googleapis.com/user/{name}) must land on StackOutputs.
			name: "GcpLogMetric",
			kind: cloudresourcekind.CloudResourceKind_GcpLogMetric,
			rawOutputs: map[string]interface{}{
				"metric_name": "checkout/error-count",
			},
			mustPopulate: []string{
				"metric_name",
			},
		},
		{
			// GcpWorkflow: the full resource name (what Eventarc
			// destinations consume), the short name, the deployed revision,
			// and the state must land on StackOutputs.
			name: "GcpWorkflow",
			kind: cloudresourcekind.CloudResourceKind_GcpWorkflow,
			rawOutputs: map[string]interface{}{
				"workflow_id":   "projects/my-project/locations/us-central1/workflows/order-orchestrator",
				"workflow_name": "order-orchestrator",
				"revision_id":   "000001-a4d",
				"state":         "ACTIVE",
			},
			mustPopulate: []string{
				"workflow_id", "workflow_name", "revision_id", "state",
			},
		},
		{
			// GcpEventarcTrigger: the trigger name and the partner-channel
			// activation token (empty for non-partner triggers — both
			// engines emit the empty string then) must land on
			// StackOutputs.
			name: "GcpEventarcTrigger",
			kind: cloudresourcekind.CloudResourceKind_GcpEventarcTrigger,
			rawOutputs: map[string]interface{}{
				"trigger_name":                     "order-events",
				"trigger_id":                       "projects/my-project/locations/us-central1/triggers/order-events",
				"partner_channel_activation_token": "tok-abc123",
			},
			mustPopulate: []string{
				"trigger_name", "trigger_id", "partner_channel_activation_token",
			},
		},
		{
			// GcpEventarcMessageBus: the full bus resource name (the
			// cross-bus / external-publisher handle) must land on
			// StackOutputs.
			name: "GcpEventarcMessageBus",
			kind: cloudresourcekind.CloudResourceKind_GcpEventarcMessageBus,
			rawOutputs: map[string]interface{}{
				"message_bus_name": "projects/my-project/locations/us-central1/messageBuses/central-bus",
			},
			mustPopulate: []string{
				"message_bus_name",
			},
		},
		{
			// GcpCertificateMap: the full resource name, the
			// //certificatemanager.googleapis.com/ URI (what a
			// GcpTargetHttpsProxy's certificate_map consumes), and the
			// short name must land on StackOutputs.
			name: "GcpCertificateMap",
			kind: cloudresourcekind.CloudResourceKind_GcpCertificateMap,
			rawOutputs: map[string]interface{}{
				"map_id":   "projects/my-project/locations/global/certificateMaps/prod-map",
				"map_uri":  "//certificatemanager.googleapis.com/projects/my-project/locations/global/certificateMaps/prod-map",
				"map_name": "prod-map",
			},
			mustPopulate: []string{
				"map_id", "map_uri", "map_name",
			},
		},
		{
			// GcpSslCertificate: flat scalar outputs from both engines
			// (self-link, name, id, expiry, scope region) must land on
			// StackOutputs. The private key is write-only and never an output.
			name: "GcpSslCertificate",
			kind: cloudresourcekind.CloudResourceKind_GcpSslCertificate,
			rawOutputs: map[string]interface{}{
				"self_link":        "https://www.googleapis.com/compute/v1/projects/my-project/global/sslCertificates/my-cert",
				"certificate_name": "my-cert",
				"certificate_id":   "1234567890123456789",
				"expire_time":      "2036-06-30T12:36:27Z",
				"region":           "",
			},
			mustPopulate: []string{
				"self_link", "certificate_name", "certificate_id", "expire_time",
			},
		},
		{
			// GcpGlobalAddress: name output added for service networking composition.
			name: "GcpGlobalAddress",
			kind: cloudresourcekind.CloudResourceKind_GcpGlobalAddress,
			rawOutputs: map[string]interface{}{
				"address":            "10.100.0.0",
				"self_link":          "https://www.googleapis.com/compute/v1/projects/my-project/global/addresses/vpc-peering-range",
				"creation_timestamp": "2026-01-01T00:00:00Z",
				"name":               "vpc-peering-range",
			},
			mustPopulate: []string{
				"address", "self_link", "creation_timestamp", "name",
			},
		},
		{
			// GcpServiceNetworkingConnection: peering + network outputs from both engines.
			name: "GcpServiceNetworkingConnection",
			kind: cloudresourcekind.CloudResourceKind_GcpServiceNetworkingConnection,
			rawOutputs: map[string]interface{}{
				"peering": "servicenetworking-googleapis-com",
				"network": "projects/my-project/global/networks/app-vpc",
			},
			mustPopulate: []string{"peering", "network"},
		},
		{
			// GcpAddress: regional reservation outputs including plain spec region.
			name: "GcpAddress",
			kind: cloudresourcekind.CloudResourceKind_GcpAddress,
			rawOutputs: map[string]interface{}{
				"address":   "203.0.113.10",
				"self_link": "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/addresses/nat-ip",
				"name":      "nat-ip",
				"region":    "us-central1",
			},
			mustPopulate: []string{"address", "self_link", "name", "region"},
		},
		{
			// GcpCloudSql: instance outputs from both engines, including the
			// service-account identity and the PSC-only fields (empty on
			// non-PSC instances).
			name: "GcpCloudSql",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudSql,
			rawOutputs: map[string]interface{}{
				"instance_name":               "orders-db",
				"connection_name":             "my-project:us-central1:orders-db",
				"private_ip":                  "10.20.0.5",
				"public_ip":                   "",
				"self_link":                   "https://sqladmin.googleapis.com/sql/v1beta4/projects/my-project/instances/orders-db",
				"service_account_email":       "p1234-abcdef@gcp-sa-cloud-sql.iam.gserviceaccount.com",
				"dns_name":                    "",
				"psc_service_attachment_link": "",
			},
			mustPopulate: []string{
				"instance_name", "connection_name", "self_link", "service_account_email",
			},
		},
		{
			// GcpCloudSqlDatabase: database name + self link.
			name: "GcpCloudSqlDatabase",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudSqlDatabase,
			rawOutputs: map[string]interface{}{
				"database_name": "orders",
				"self_link":     "https://sqladmin.googleapis.com/sql/v1beta4/projects/my-project/instances/orders-db/databases/orders",
			},
			mustPopulate: []string{"database_name", "self_link"},
		},
		{
			// GcpCloudSqlUser: the stored user name (IAM users on MySQL come
			// back truncated before the @).
			name: "GcpCloudSqlUser",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudSqlUser,
			rawOutputs: map[string]interface{}{
				"user_name":     "orders-app",
				"instance_name": "orders-db",
			},
			mustPopulate: []string{"user_name", "instance_name"},
		},
		{
			// GcpRedisInstance: endpoint scalars, the secret AUTH string, the
			// repeated CA-cert PEMs (populated when TLS is on), the import/export
			// IAM identity, and the effective reserved range.
			name: "GcpRedisInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpRedisInstance,
			rawOutputs: map[string]interface{}{
				"host":                        "10.118.0.4",
				"port":                        "6379",
				"read_endpoint":               "10.118.0.5",
				"read_endpoint_port":          "6379",
				"current_location_id":         "us-central1-a",
				"auth_string":                 "d1f0e2c3-4b5a-6789-abcd-ef0123456789",
				"server_ca_certs":             []interface{}{"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"},
				"persistence_iam_identity":    "serviceAccount:service-1234@gcp-sa-redis.iam.gserviceaccount.com",
				"effective_reserved_ip_range": "10.118.0.0/29",
				"instance_name":               "session-store-prod",
				"region":                      "us-central1",
			},
			mustPopulate: []string{
				"host", "port", "read_endpoint", "read_endpoint_port",
				"current_location_id", "auth_string", "server_ca_certs",
				"persistence_iam_identity", "effective_reserved_ip_range",
				"instance_name", "region",
			},
		},
		{
			// GcpGkeCluster: control-plane endpoint + CA trust anchor, the
			// Workload Identity pool, the fully qualified cluster ID, and the
			// name/location handles node pools compose against.
			name: "GcpGkeCluster",
			kind: cloudresourcekind.CloudResourceKind_GcpGkeCluster,
			rawOutputs: map[string]interface{}{
				"endpoint":               "34.72.10.11",
				"cluster_ca_certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				"workload_identity_pool": "my-project.svc.id.goog",
				"cluster_id":             "projects/my-project/locations/us-central1/clusters/prod-primary",
				"name":                   "prod-primary",
				"location":               "us-central1",
				"self_link":              "https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/prod-primary",
				"master_version":         "1.31.4-gke.1256000",
			},
			mustPopulate: []string{
				"endpoint", "cluster_ca_certificate", "workload_identity_pool",
				"cluster_id", "name", "location", "self_link", "master_version",
			},
		},
		{
			// GcpGkeNodePool: the pool name/location handles, the backing
			// instance groups, the effective sizing bounds, and the fully
			// qualified pool ID downstream services (e.g. Dataproc on GKE)
			// reference.
			name: "GcpGkeNodePool",
			kind: cloudresourcekind.CloudResourceKind_GcpGkeNodePool,
			rawOutputs: map[string]interface{}{
				"node_pool_name": "general-pool",
				"instance_group_urls": []interface{}{
					"https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroupManagers/gke-prod-primary-general-pool-grp",
				},
				"min_nodes":          "1",
				"max_nodes":          "5",
				"current_node_count": "2",
				"node_pool_id":       "projects/my-project/locations/us-central1/clusters/prod-primary/nodePools/general-pool",
				"location":           "us-central1",
				"version":            "1.31.4-gke.1256000",
			},
			mustPopulate: []string{
				"node_pool_name", "instance_group_urls", "min_nodes",
				"max_nodes", "current_node_count", "node_pool_id",
				"location", "version",
			},
		},
		{
			// GcpCloudRun: the serving URL, the service-name handle serverless
			// NEGs reference, the latest ready revision, and the identifiers
			// API callers use.
			name: "GcpCloudRun",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudRun,
			rawOutputs: map[string]interface{}{
				"url":          "https://my-api-abc123-uc.a.run.app",
				"service_name": "my-api",
				"revision":     "my-api-00042-abc",
				"location":     "us-central1",
				"uid":          "12345678-1234-1234-1234-123456789012",
				"urls": []interface{}{
					"https://my-api-abc123-uc.a.run.app",
				},
			},
			mustPopulate: []string{
				"url", "service_name", "revision", "location", "uid", "urls",
			},
		},
		{
			// GcpRouterNat: NAT name, router self link, and the manual NAT IP
			// self links (empty for auto-allocation).
			name: "GcpRouterNat",
			kind: cloudresourcekind.CloudResourceKind_GcpRouterNat,
			rawOutputs: map[string]interface{}{
				"name":             "prod-nat",
				"router_self_link": "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/routers/prod-router",
				"nat_ips":          []interface{}{"https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/addresses/egress-ip-a"},
			},
			mustPopulate: []string{"name", "router_self_link", "nat_ips"},
		},
		{
			// GcpVpcNetwork: deep-rebuilt outputs — PSA fields removed, gateway + ULA added.
			name: "GcpVpcNetwork",
			kind: cloudresourcekind.CloudResourceKind_GcpVpcNetwork,
			rawOutputs: map[string]interface{}{
				"network_self_link":   "https://www.googleapis.com/compute/v1/projects/my-project/global/networks/app-vpc",
				"network_name":        "app-vpc",
				"network_id":          "projects/my-project/global/networks/app-vpc",
				"gateway_ipv4":        "10.128.0.1",
				"internal_ipv6_range": "fd20:1234:5678::/48",
			},
			mustPopulate: []string{
				"network_self_link", "network_name", "network_id",
			},
		},
		{
			// GcpVertexAiEndpoint: the fully qualified endpoint path, the
			// display name, the dedicated-endpoint DNS (when enabled), the
			// create timestamp, and the numeric endpoint_name both engines
			// derive identically from the resource identity.
			name: "GcpVertexAiEndpoint",
			kind: cloudresourcekind.CloudResourceKind_GcpVertexAiEndpoint,
			rawOutputs: map[string]interface{}{
				"endpoint_id":            "projects/prod-project/locations/us-central1/endpoints/1853927074",
				"display_name":           "inference-api",
				"dedicated_endpoint_dns": "1853927074.us-central1-123456789012.prediction.vertexai.goog",
				"create_time":            "2026-07-05T10:00:00Z",
				"endpoint_name":          "1853927074",
			},
			mustPopulate: []string{
				"endpoint_id", "display_name", "dedicated_endpoint_dns",
				"create_time", "endpoint_name",
			},
		},
		{
			// GcpVertexAiIndex: the fully qualified index path (the deployed
			// index's composition key), the GCP-assigned numeric ID, the
			// metadata schema URI, and both lifecycle timestamps.
			name: "GcpVertexAiIndex",
			kind: cloudresourcekind.CloudResourceKind_GcpVertexAiIndex,
			rawOutputs: map[string]interface{}{
				"index_id":            "projects/prod-project/locations/us-central1/indexes/5022997925215600640",
				"index_name":          "5022997925215600640",
				"metadata_schema_uri": "gs://google-cloud-aiplatform/schema/matchingengine/metadata/nearest_neighbor_search_1.0.0.yaml",
				"create_time":         "2026-07-05T10:00:00Z",
				"update_time":         "2026-07-05T11:00:00Z",
			},
			mustPopulate: []string{
				"index_id", "index_name", "metadata_schema_uri",
				"create_time", "update_time",
			},
		},
		{
			// GcpVertexAiIndexEndpoint: the fully qualified endpoint path (the
			// deployed index's other composition key), the GCP-assigned numeric
			// ID, the public query domain, and both lifecycle timestamps.
			name: "GcpVertexAiIndexEndpoint",
			kind: cloudresourcekind.CloudResourceKind_GcpVertexAiIndexEndpoint,
			rawOutputs: map[string]interface{}{
				"index_endpoint_id":           "projects/prod-project/locations/us-central1/indexEndpoints/7997049335000858624",
				"index_endpoint_name":         "7997049335000858624",
				"public_endpoint_domain_name": "1252330891.us-central1-123456789012.vdb.vertexai.goog",
				"create_time":                 "2026-07-05T10:00:00Z",
				"update_time":                 "2026-07-05T10:00:01Z",
			},
			mustPopulate: []string{
				"index_endpoint_id", "index_endpoint_name",
				"public_endpoint_domain_name", "create_time", "update_time",
			},
		},
		{
			// GcpVertexAiDeployedIndex: the deployment handle pair (parent
			// endpoint path + deployed_index_id), the provider-reported name,
			// the sync/create timestamps, and the private-endpoint addresses
			// both engines export as empty strings on public endpoints.
			name: "GcpVertexAiDeployedIndex",
			kind: cloudresourcekind.CloudResourceKind_GcpVertexAiDeployedIndex,
			rawOutputs: map[string]interface{}{
				"name":               "products_v1",
				"deployed_index_id":  "products_v1",
				"create_time":        "2026-07-05T10:00:00Z",
				"index_sync_time":    "2026-07-05T10:30:00Z",
				"match_grpc_address": "10.128.0.5",
				"service_attachment": "projects/p1/regions/us-central1/serviceAttachments/sa1",
				"index_endpoint":     "projects/prod-project/locations/us-central1/indexEndpoints/7997049335000858624",
			},
			mustPopulate: []string{
				"name", "deployed_index_id", "create_time", "index_sync_time",
				"match_grpc_address", "service_attachment", "index_endpoint",
			},
		},
		{
			// GcpVertexAiNotebook: the fully qualified instance path, the short
			// name, the JupyterLab proxy URI, lifecycle state, creator, and the
			// health/update timestamps the deep rebuild added.
			name: "GcpVertexAiNotebook",
			kind: cloudresourcekind.CloudResourceKind_GcpVertexAiNotebook,
			rawOutputs: map[string]interface{}{
				"instance_id":   "projects/prod-project/locations/us-central1-a/instances/data-exploration",
				"instance_name": "data-exploration",
				"proxy_uri":     "https://abc123-dot-us-central1-a.notebooks.googleusercontent.com",
				"state":         "ACTIVE",
				"creator":       "admin@prod-project.iam.gserviceaccount.com",
				"create_time":   "2026-07-05T10:00:00Z",
				"health_state":  "HEALTHY",
				"update_time":   "2026-07-05T11:00:00Z",
			},
			mustPopulate: []string{
				"instance_id", "instance_name", "proxy_uri", "state",
				"creator", "create_time", "health_state", "update_time",
			},
		},
		{
			// GcpSubnetwork: scalar outputs plus the per-index secondary-range
			// exports both engines emit must land on the StackOutputs proto,
			// including the repeated secondary_ranges message.
			name: "GcpSubnetwork",
			kind: cloudresourcekind.CloudResourceKind_GcpSubnetwork,
			rawOutputs: map[string]interface{}{
				"subnetwork_self_link":             "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/subnetworks/app-subnet",
				"subnetwork_name":                  "app-subnet",
				"region":                           "us-central1",
				"ip_cidr_range":                    "10.10.0.0/20",
				"gateway_address":                  "10.10.0.1",
				"subnetwork_id":                    "1234567890123456789",
				"internal_ipv6_prefix":             "",
				"external_ipv6_prefix":             "",
				"secondary_ranges.0.range_name":    "pods",
				"secondary_ranges.0.ip_cidr_range": "10.16.0.0/14",
				"secondary_ranges.1.range_name":    "services",
				"secondary_ranges.1.ip_cidr_range": "10.20.0.0/20",
			},
			mustPopulate: []string{
				"subnetwork_self_link", "subnetwork_name", "region",
				"ip_cidr_range", "gateway_address", "subnetwork_id",
			},
		},
		{
			// GcpAlloydbCluster: the fully qualified cluster path (the FK target
			// instance and user kinds parent by), the short name, and the bundled
			// primary instance's connection endpoint.
			name: "GcpAlloydbCluster",
			kind: cloudresourcekind.CloudResourceKind_GcpAlloydbCluster,
			rawOutputs: map[string]interface{}{
				"cluster_id":            "projects/my-project/locations/us-central1/clusters/orders-alloydb",
				"cluster_name":          "orders-alloydb",
				"primary_instance_ip":   "10.30.0.5",
				"primary_instance_name": "projects/my-project/locations/us-central1/clusters/orders-alloydb/instances/primary",
				"database_version":      "POSTGRES_16",
				"state":                 "READY",
			},
			mustPopulate: []string{
				"cluster_id", "cluster_name", "primary_instance_ip",
				"primary_instance_name", "database_version", "state",
			},
		},
		{
			// GcpAlloydbInstance: the fully qualified instance path, its private
			// connection endpoint, and lifecycle state.
			name: "GcpAlloydbInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpAlloydbInstance,
			rawOutputs: map[string]interface{}{
				"instance_name": "projects/my-project/locations/us-central1/clusters/orders-alloydb/instances/read-pool",
				"ip_address":    "10.30.0.7",
				"state":         "READY",
			},
			mustPopulate: []string{"instance_name", "ip_address", "state"},
		},
		{
			// GcpAlloydbUser: the fully qualified user path plus the id and
			// cluster handles.
			name: "GcpAlloydbUser",
			kind: cloudresourcekind.CloudResourceKind_GcpAlloydbUser,
			rawOutputs: map[string]interface{}{
				"name":       "projects/my-project/locations/us-central1/clusters/orders-alloydb/users/orders-app",
				"user_id":    "orders-app",
				"cluster_id": "projects/my-project/locations/us-central1/clusters/orders-alloydb",
			},
			mustPopulate: []string{"name", "user_id", "cluster_id"},
		},
		{
			// GcpDnsZone: the numeric zone id, the zone-name handle GcpDnsRecord
			// composes against, and the delegated nameserver set.
			name: "GcpDnsZone",
			kind: cloudresourcekind.CloudResourceKind_GcpDnsZone,
			rawOutputs: map[string]interface{}{
				"zone_id":   "1234567890123456789",
				"zone_name": "example-com",
				"nameservers": []interface{}{
					"ns-cloud-a1.googledomains.com.",
					"ns-cloud-a2.googledomains.com.",
				},
			},
			mustPopulate: []string{"zone_id", "zone_name", "nameservers"},
		},
		{
			// GcpDnsRecord: FQDN, type, zone handle, project, and TTL echoed by
			// both engines after creating a record set.
			name: "GcpDnsRecord",
			kind: cloudresourcekind.CloudResourceKind_GcpDnsRecord,
			rawOutputs: map[string]interface{}{
				"fqdn":         "www.example.com.",
				"record_type":  "A",
				"managed_zone": "example-com",
				"project_id":   "my-project",
				"ttl_seconds":  300,
			},
			mustPopulate: []string{"fqdn", "record_type", "managed_zone", "project_id", "ttl_seconds"},
		},
		{
			// GcpGkeWorkloadIdentityBinding: the IAM member string and bound GSA
			// email echoed by both engines after the grant is applied.
			name: "GcpGkeWorkloadIdentityBinding",
			kind: cloudresourcekind.CloudResourceKind_GcpGkeWorkloadIdentityBinding,
			rawOutputs: map[string]interface{}{
				"member":                "serviceAccount:my-project.svc.id.goog[cert-manager/cert-manager]",
				"service_account_email": "my-sa@my-project.iam.gserviceaccount.com",
			},
			mustPopulate: []string{"member", "service_account_email"},
		},
		{
			// GcpCloudArmorPolicy: policy id/name/self-link/fingerprint — the
			// self-link is the frozen composition key for backend attachments.
			name: "GcpCloudArmorPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudArmorPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id":        "projects/my-project/global/securityPolicies/corp-allowlist",
				"policy_name":      "corp-allowlist",
				"policy_self_link": "https://www.googleapis.com/compute/v1/projects/my-project/global/securityPolicies/corp-allowlist",
				"fingerprint":      "abc123==",
			},
			mustPopulate: []string{"policy_id", "policy_name", "policy_self_link", "fingerprint"},
		},
		{
			// GcpCertManagerDnsAuthorization: authorization id/name/domain and
			// the validation record tuple GcpDnsRecord composes via valueFrom.
			name: "GcpCertManagerDnsAuthorization",
			kind: cloudresourcekind.CloudResourceKind_GcpCertManagerDnsAuthorization,
			rawOutputs: map[string]interface{}{
				"authorization_id":   "projects/my-project/locations/global/dnsAuthorizations/example-auth",
				"authorization_name": "example-auth",
				"domain":             "example.com",
				"dns_record_name":    "_acme-challenge.example.com.",
				"dns_record_type":    "CNAME",
				"dns_record_data":    "abcdef.auth.goog.",
			},
			mustPopulate: []string{
				"authorization_id", "authorization_name", "domain",
				"dns_record_name", "dns_record_type", "dns_record_data",
			},
		},
		{
			// GcpCertManagerCert: certificate id/name/SANs/location/managed state
			// — certificate_name is the frozen key for GcpTargetHttpsProxy.
			name: "GcpCertManagerCert",
			kind: cloudresourcekind.CloudResourceKind_GcpCertManagerCert,
			rawOutputs: map[string]interface{}{
				"certificate_id":   "projects/my-project/locations/global/certificates/example-cert",
				"certificate_name": "example-cert",
				"san_dnsnames":     []interface{}{"example.com", "*.example.com"},
				"location":         "global",
				"managed_state":    "PROVISIONING",
			},
			mustPopulate: []string{"certificate_id", "certificate_name", "location", "managed_state"},
		},
		{
			// GcpProject: display name, immutable project id, and numeric
			// project number — project_id is the frozen Layer-0 composition key.
			name: "GcpProject",
			kind: cloudresourcekind.CloudResourceKind_GcpProject,
			rawOutputs: map[string]interface{}{
				"name":           "My Production Project",
				"project_id":     "my-prod-project",
				"project_number": "123456789012",
			},
			mustPopulate: []string{"name", "project_id", "project_number"},
		},
		{
			// GcpCloudRunJob: the job-name handle gcloud/Scheduler trigger, the
			// region, the server-assigned uid, and the latest execution (empty
			// until first run).
			name: "GcpCloudRunJob",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudRunJob,
			rawOutputs: map[string]interface{}{
				"job_name":                 "nightly-etl",
				"location":                 "us-central1",
				"uid":                      "12345678-1234-1234-1234-123456789012",
				"latest_created_execution": "",
			},
			mustPopulate: []string{"job_name", "location", "uid"},
		},
		{
			// GcpSpannerInstance: the fully qualified instance path (the IAM/API
			// handle), the short name downstream databases and backup schedules
			// reference, the lifecycle state, and the geographic config.
			name: "GcpSpannerInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpSpannerInstance,
			rawOutputs: map[string]interface{}{
				"instance_id":   "projects/prod-project/instances/orders-spanner",
				"instance_name": "orders-spanner",
				"state":         "READY",
				"config":        "regional-us-central1",
			},
			mustPopulate: []string{"instance_id", "instance_name", "state", "config"},
		},
		{
			// GcpSpannerDatabase: the fully qualified database path (the IAM/API
			// handle), the short name backup schedules reference, and the
			// lifecycle state.
			name: "GcpSpannerDatabase",
			kind: cloudresourcekind.CloudResourceKind_GcpSpannerDatabase,
			rawOutputs: map[string]interface{}{
				"database_id":   "projects/prod-project/instances/orders-spanner/databases/orders",
				"database_name": "orders",
				"state":         "READY",
			},
			mustPopulate: []string{"database_id", "database_name", "state"},
		},
		{
			// GcpSpannerBackupSchedule: the fully qualified schedule path (the
			// API handle) and the short name within the database.
			name: "GcpSpannerBackupSchedule",
			kind: cloudresourcekind.CloudResourceKind_GcpSpannerBackupSchedule,
			rawOutputs: map[string]interface{}{
				"schedule_id":   "projects/prod-project/instances/orders-spanner/databases/orders/backupSchedules/daily-backups",
				"schedule_name": "daily-backups",
			},
			mustPopulate: []string{"schedule_id", "schedule_name"},
		},
		{
			// GcpBigQueryDataset: the short dataset id SQL queries reference,
			// the self link, resolved project, creation time, location, and etag.
			name: "GcpBigQueryDataset",
			kind: cloudresourcekind.CloudResourceKind_GcpBigQueryDataset,
			rawOutputs: map[string]interface{}{
				"dataset_id":    "analytics_prod",
				"self_link":     "https://bigquery.googleapis.com/bigquery/v2/projects/prod-project/datasets/analytics_prod",
				"project":       "prod-project",
				"creation_time": int64(1700000000000),
				"location":      "US",
				"etag":          "abc123",
			},
			mustPopulate: []string{"dataset_id", "self_link", "project", "creation_time", "location", "etag"},
		},
		{
			// GcpBigQueryTable: the short table id, self link, resolved project,
			// parent dataset, table type, location, creation time, and the
			// pre-assembled dotted handle Pub/Sub BigQuery delivery consumes.
			name: "GcpBigQueryTable",
			kind: cloudresourcekind.CloudResourceKind_GcpBigQueryTable,
			rawOutputs: map[string]interface{}{
				"table_id":       "events_raw",
				"self_link":      "https://bigquery.googleapis.com/bigquery/v2/projects/prod-project/datasets/analytics_prod/tables/events_raw",
				"project":        "prod-project",
				"dataset_id":     "analytics_prod",
				"type":           "TABLE",
				"location":       "US",
				"creation_time":  int64(1700000000000),
				"qualified_name": "prod-project.analytics_prod.events_raw",
			},
			mustPopulate: []string{"table_id", "self_link", "project", "dataset_id", "type", "location", "creation_time", "qualified_name"},
		},
		{
			// GcpPubSubSchema: the fully qualified schema path a topic's
			// schema_settings.schema reference consumes, and the short name.
			name: "GcpPubSubSchema",
			kind: cloudresourcekind.CloudResourceKind_GcpPubSubSchema,
			rawOutputs: map[string]interface{}{
				"schema_id":   "projects/prod-project/schemas/order-events",
				"schema_name": "order-events",
			},
			mustPopulate: []string{"schema_id", "schema_name"},
		},
		{
			// GcpPubSubTopic: the fully qualified topic path subscriptions
			// and event triggers consume, and the short name.
			name: "GcpPubSubTopic",
			kind: cloudresourcekind.CloudResourceKind_GcpPubSubTopic,
			rawOutputs: map[string]interface{}{
				"topic_id":   "projects/prod-project/topics/order-events",
				"topic_name": "order-events",
			},
			mustPopulate: []string{"topic_id", "topic_name"},
		},
		{
			// GcpPubSubSubscription: the fully qualified subscription path
			// consumers and monitoring reference, and the short name.
			name: "GcpPubSubSubscription",
			kind: cloudresourcekind.CloudResourceKind_GcpPubSubSubscription,
			rawOutputs: map[string]interface{}{
				"subscription_id":   "projects/prod-project/subscriptions/order-events-worker",
				"subscription_name": "order-events-worker",
			},
			mustPopulate: []string{"subscription_id", "subscription_name"},
		},
		{
			// GcpCloudTasksQueue: the fully qualified queue path task
			// producers enqueue against, the short name, and the
			// GCP-computed effective burst size.
			name: "GcpCloudTasksQueue",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudTasksQueue,
			rawOutputs: map[string]interface{}{
				"queue_id":       "projects/prod-project/locations/us-central1/queues/order-processing",
				"queue_name":     "order-processing",
				"max_burst_size": 100,
			},
			mustPopulate: []string{"queue_id", "queue_name", "max_burst_size"},
		},
		{
			// GcpCloudSchedulerJob: the fully qualified job path, the short
			// name, and the reconciled state (ENABLED unless created paused).
			name: "GcpCloudSchedulerJob",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudSchedulerJob,
			rawOutputs: map[string]interface{}{
				"job_id":   "projects/prod-project/locations/us-central1/jobs/daily-report-trigger",
				"job_name": "daily-report-trigger",
				"state":    "ENABLED",
			},
			mustPopulate: []string{"job_id", "job_name", "state"},
		},
		{
			// GcpKmsKeyRing: the fully qualified ring path a GcpKmsKey's
			// key_ring_id reference consumes, the short name for bare-name
			// consumers, and the location they pair it with.
			name: "GcpKmsKeyRing",
			kind: cloudresourcekind.CloudResourceKind_GcpKmsKeyRing,
			rawOutputs: map[string]interface{}{
				"key_ring_id":   "projects/prod-project/locations/us-central1/keyRings/prod-encryption",
				"key_ring_name": "prod-encryption",
				"location":      "us-central1",
			},
			mustPopulate: []string{"key_ring_id", "key_ring_name", "location"},
		},
		{
			// GcpKmsKey: the fully qualified key path every CMEK consumer
			// takes, the short name for bare-name consumers, and the primary
			// version handle + state (populated for ENCRYPT_DECRYPT keys).
			name: "GcpKmsKey",
			kind: cloudresourcekind.CloudResourceKind_GcpKmsKey,
			rawOutputs: map[string]interface{}{
				"key_id":               "projects/prod-project/locations/us-central1/keyRings/prod-encryption/cryptoKeys/cmek-data-key",
				"key_name":             "cmek-data-key",
				"primary_version_name": "projects/prod-project/locations/us-central1/keyRings/prod-encryption/cryptoKeys/cmek-data-key/cryptoKeyVersions/1",
				"primary_state":        "ENABLED",
			},
			mustPopulate: []string{"key_id", "key_name", "primary_version_name", "primary_state"},
		},
		{
			// GcpServerlessVpcConnector: the short connector name, the fully
			// qualified path serverless workloads attach to, the reconciled
			// state, and the plain region name.
			name: "GcpServerlessVpcConnector",
			kind: cloudresourcekind.CloudResourceKind_GcpServerlessVpcConnector,
			rawOutputs: map[string]interface{}{
				"name":      "svc-egress",
				"self_link": "projects/prod-project/locations/us-central1/connectors/svc-egress",
				"state":     "READY",
				"region":    "us-central1",
			},
			mustPopulate: []string{"name", "self_link", "state", "region"},
		},
		{
			// GcpCloudFunction: the fully qualified function path, the bare
			// name serverless NEGs reference, both serving URLs, the
			// underlying Cloud Run service, runtime identity, state,
			// environment, and update time (eventarc trigger empty for HTTP
			// functions).
			name: "GcpCloudFunction",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudFunction,
			rawOutputs: map[string]interface{}{
				"function_id":           "projects/prod-project/locations/us-central1/functions/hello-api",
				"function_url":          "https://us-central1-prod-project.cloudfunctions.net/hello-api",
				"service_account_email": "fn-runtime@prod-project.iam.gserviceaccount.com",
				"state":                 "ACTIVE",
				"cloud_run_service_id":  "projects/prod-project/locations/us-central1/services/hello-api",
				"eventarc_trigger_id":   "",
				"name":                  "hello-api",
				"uri":                   "https://hello-api-abc123-uc.a.run.app",
				"environment":           "GEN_2",
				"update_time":           "2026-07-05T12:00:00Z",
			},
			mustPopulate: []string{
				"function_id", "function_url", "service_account_email", "state",
				"cloud_run_service_id", "name", "uri", "environment", "update_time",
			},
		},
		{
			// GcpServiceConnectionPolicy: the fully qualified policy path, the
			// short name, the connectivity mechanism the automation reports,
			// and the change-detection etag.
			name: "GcpServiceConnectionPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpServiceConnectionPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id":      "projects/prod-project/locations/us-central1/serviceConnectionPolicies/memorystore-policy",
				"name":           "memorystore-policy",
				"infrastructure": "PSC",
				"etag":           "abc123etag",
			},
			mustPopulate: []string{"policy_id", "name", "infrastructure", "etag"},
		},
		{
			// GcpMemorystoreInstance: the PSC discovery endpoint (address +
			// numeric port), the server uid, the node memory a node_type
			// implies (float), the full resource path DR secondaries
			// reference, and the backup collection automated backups land in.
			name: "GcpMemorystoreInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpMemorystoreInstance,
			rawOutputs: map[string]interface{}{
				"discovery_address": "10.9.0.5",
				"discovery_port":    6379,
				"instance_uid":      "a1b2c3d4-uid",
				"node_size_gb":      1.4,
				"name":              "projects/prod-project/locations/us-central1/instances/prod-cache",
				"backup_collection": "projects/prod-project/locations/us-central1/backupCollections/col-1",
			},
			mustPopulate: []string{
				"discovery_address", "discovery_port", "instance_uid",
				"node_size_gb", "name", "backup_collection",
			},
		},
		{
			// GcpBigtableInstance: the fully qualified instance path and the
			// short name client libraries connect with.
			name: "GcpBigtableInstance",
			kind: cloudresourcekind.CloudResourceKind_GcpBigtableInstance,
			rawOutputs: map[string]interface{}{
				"instance_id":   "projects/prod-project/instances/prod-bigtable",
				"instance_name": "prod-bigtable",
			},
			mustPopulate: []string{"instance_id", "instance_name"},
		},
		{
			// GcpBigtableTable: the fully qualified table path, the short
			// name clients open, and the parent instance.
			name: "GcpBigtableTable",
			kind: cloudresourcekind.CloudResourceKind_GcpBigtableTable,
			rawOutputs: map[string]interface{}{
				"table_id":      "projects/prod-project/instances/prod-bigtable/tables/events",
				"table_name":    "events",
				"instance_name": "prod-bigtable",
			},
			mustPopulate: []string{"table_id", "table_name", "instance_name"},
		},
		{
			// GcpFirestoreDatabase: the fully qualified database path, the
			// name clients connect with, the server-generated uid, and the
			// PITR/version-retention posture timestamps.
			name: "GcpFirestoreDatabase",
			kind: cloudresourcekind.CloudResourceKind_GcpFirestoreDatabase,
			rawOutputs: map[string]interface{}{
				"database_id":              "projects/prod-project/databases/orders-db",
				"database_name":            "orders-db",
				"uid":                      "8d68546e-3c88-4244-8722-0a4b0a4b0a4b",
				"create_time":              "2026-07-05T10:00:00Z",
				"earliest_version_time":    "2026-07-05T10:00:00Z",
				"version_retention_period": "3600s",
				"key_prefix":               "",
				"update_time":              "2026-07-05T10:00:00Z",
			},
			mustPopulate: []string{
				"database_id", "database_name", "uid",
				"create_time", "earliest_version_time",
				"version_retention_period", "update_time",
			},
		},
		{
			// GcpFirestoreBackupSchedule: the server-assigned schedule id and
			// the parent database name the verifier reassembles the resource
			// path from.
			name: "GcpFirestoreBackupSchedule",
			kind: cloudresourcekind.CloudResourceKind_GcpFirestoreBackupSchedule,
			rawOutputs: map[string]interface{}{
				"schedule_id": "8d68546e-3c88-4244-8722-0a4b0a4b0a4b",
				"database":    "orders-db",
			},
			mustPopulate: []string{"schedule_id", "database"},
		},
		{
			// GcpFirestoreIndex: the server-defined index resource path and
			// the collection group it serves.
			name: "GcpFirestoreIndex",
			kind: cloudresourcekind.CloudResourceKind_GcpFirestoreIndex,
			rawOutputs: map[string]interface{}{
				"index_id":   "projects/prod-project/databases/orders-db/collectionGroups/orders/indexes/CICAgJjF6JEK",
				"collection": "orders",
			},
			mustPopulate: []string{"index_id", "collection"},
		},
		{
			// GcpDataprocCluster: the fully qualified cluster path (the
			// composition handle downstream spark-history-server references
			// consume), the short name, and the staging bucket in use.
			name: "GcpDataprocCluster",
			kind: cloudresourcekind.CloudResourceKind_GcpDataprocCluster,
			rawOutputs: map[string]interface{}{
				"cluster_id":     "projects/prod-project/regions/us-central1/clusters/etl-cluster",
				"cluster_name":   "etl-cluster",
				"staging_bucket": "dataproc-staging-us-central1-123456789012-abcdef",
			},
			mustPopulate: []string{"cluster_id", "cluster_name", "staging_bucket"},
		},
		{
			// GcpDataprocAutoscalingPolicy: the fully qualified policy path
			// (what a cluster's autoscaling_policy_uri reference resolves to)
			// plus the plain id and region.
			name: "GcpDataprocAutoscalingPolicy",
			kind: cloudresourcekind.CloudResourceKind_GcpDataprocAutoscalingPolicy,
			rawOutputs: map[string]interface{}{
				"name":      "projects/prod-project/locations/us-central1/autoscalingPolicies/batch-scaling",
				"policy_id": "batch-scaling",
				"location":  "us-central1",
			},
			mustPopulate: []string{"name", "policy_id", "location"},
		},
		{
			// GcpCloudComposerEnvironment: the fully qualified environment
			// path, the short name, and the assembled-stack handles (Airflow
			// UI, DAG bucket prefix, underlying GKE cluster).
			name: "GcpCloudComposerEnvironment",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudComposerEnvironment,
			rawOutputs: map[string]interface{}{
				"environment_id":   "projects/prod-project/locations/us-central1/environments/data-pipelines",
				"environment_name": "data-pipelines",
				"airflow_uri":      "https://12345678-dot-us-central1.composer.googleusercontent.com",
				"dag_gcs_prefix":   "gs://us-central1-data-pipelines-abcdef-bucket/dags",
				"gke_cluster":      "projects/prod-project/locations/us-central1/clusters/us-central1-data-pipelines-abcdef-gke",
			},
			mustPopulate: []string{
				"environment_id", "environment_name", "airflow_uri",
				"dag_gcs_prefix", "gke_cluster",
			},
		},
		{
			// GcpCloudComposerUserWorkloadsSecret: the fully qualified secret
			// path and the Kubernetes Secret name DAGs reference. The secret
			// data is deliberately never an output.
			name: "GcpCloudComposerUserWorkloadsSecret",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudComposerUserWorkloadsSecret,
			rawOutputs: map[string]interface{}{
				"name":        "projects/prod-project/locations/us-central1/environments/data-pipelines/userWorkloadsSecrets/airflow-connections",
				"secret_name": "airflow-connections",
			},
			mustPopulate: []string{"name", "secret_name"},
		},
		{
			// GcpCloudComposerUserWorkloadsConfigMap: the fully qualified
			// config-map path and the Kubernetes ConfigMap name DAGs reference.
			name: "GcpCloudComposerUserWorkloadsConfigMap",
			kind: cloudresourcekind.CloudResourceKind_GcpCloudComposerUserWorkloadsConfigMap,
			rawOutputs: map[string]interface{}{
				"name":            "projects/prod-project/locations/us-central1/environments/data-pipelines/userWorkloadsConfigMaps/dag-configuration",
				"config_map_name": "dag-configuration",
			},
			mustPopulate: []string{"name", "config_map_name"},
		},
		{
			// AwsNatGateway: flat scalar outputs from both engines (gateway id,
			// public/private ip, ENI id, subnet id, region) must each land on the
			// StackOutputs proto. A NAT gateway has no ARN, so none is emitted.
			name: "AwsNatGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsNatGateway,
			rawOutputs: map[string]interface{}{
				"nat_gateway_id":       "nat-0abc123",
				"public_ip":            "52.10.20.30",
				"private_ip":           "10.0.0.10",
				"network_interface_id": "eni-0abc123",
				"subnet_id":            "subnet-0abc123",
				"region":               "us-west-2",
			},
			mustPopulate: []string{
				"nat_gateway_id", "public_ip", "private_ip",
				"network_interface_id", "subnet_id", "region",
			},
		},
		{
			// AwsVpc: flat scalar outputs from both engines (vpc id/arn, primary and
			// IPv6 CIDR, owner, the route-table/default-resource ids, region) must
			// each land on the thin StackOutputs proto.
			name: "AwsVpc",
			kind: cloudresourcekind.CloudResourceKind_AwsVpc,
			rawOutputs: map[string]interface{}{
				"vpc_id":                    "vpc-0abc123",
				"vpc_arn":                   "arn:aws:ec2:us-west-2:123456789012:vpc/vpc-0abc123",
				"cidr_block":                "10.0.0.0/16",
				"ipv6_cidr_block":           "2600:1f18:abcd:1200::/56",
				"owner_id":                  "123456789012",
				"main_route_table_id":       "rtb-0abc123",
				"default_security_group_id": "sg-0abc123",
				"default_network_acl_id":    "acl-0abc123",
				"default_route_table_id":    "rtb-0abc123",
				"region":                    "us-west-2",
			},
			mustPopulate: []string{
				"vpc_id", "vpc_arn", "cidr_block", "ipv6_cidr_block", "owner_id",
				"main_route_table_id", "default_security_group_id",
				"default_network_acl_id", "default_route_table_id", "region",
			},
		},
		{
			// AwsIamPolicy: flat scalar outputs from both engines (policy arn/id/name)
			// must each land on the StackOutputs proto -- policy_arn is what role/user
			// attachments and permissions boundaries reference.
			name: "AwsIamPolicy",
			kind: cloudresourcekind.CloudResourceKind_AwsIamPolicy,
			rawOutputs: map[string]interface{}{
				"policy_arn":  "arn:aws:iam::123456789012:policy/s3-read-only",
				"policy_id":   "ANPAEXAMPLEID12345678",
				"policy_name": "s3-read-only",
			},
			mustPopulate: []string{"policy_arn", "policy_id", "policy_name"},
		},
		{
			// AwsIamInstanceProfile: flat scalar outputs from both engines (profile
			// arn/name/id and the carried role's name) must each land on the
			// StackOutputs proto -- instance_profile_arn is what EC2-shaped resources
			// reference.
			name: "AwsIamInstanceProfile",
			kind: cloudresourcekind.CloudResourceKind_AwsIamInstanceProfile,
			rawOutputs: map[string]interface{}{
				"instance_profile_arn":  "arn:aws:iam::123456789012:instance-profile/web-server",
				"instance_profile_name": "web-server",
				"instance_profile_id":   "AIPAEXAMPLEID12345678",
				"role_name":             "web-server-role",
			},
			mustPopulate: []string{
				"instance_profile_arn", "instance_profile_name",
				"instance_profile_id", "role_name",
			},
		},
		{
			// AwsIamRole: flat scalar outputs from both engines (role arn/name/id)
			// must each land on the StackOutputs proto. Guards the removal of the
			// role's former instance-profile outputs: EC2 delivery now composes
			// through AwsIamInstanceProfile, so the role emits only role-shaped
			// outputs.
			name: "AwsIamRole",
			kind: cloudresourcekind.CloudResourceKind_AwsIamRole,
			rawOutputs: map[string]interface{}{
				"role_arn":  "arn:aws:iam::123456789012:role/lambda-exec",
				"role_name": "lambda-exec",
				"role_id":   "AROAEXAMPLEID12345678",
			},
			mustPopulate: []string{"role_arn", "role_name", "role_id"},
		},
		{
			// AwsIamUser: flat scalar outputs from both engines (user arn/name/id,
			// access key id + base64 secret, console url) must each land on the
			// StackOutputs proto. The secret is base64-encoded by BOTH engines so the
			// emitted values are byte-identical.
			name: "AwsIamUser",
			kind: cloudresourcekind.CloudResourceKind_AwsIamUser,
			rawOutputs: map[string]interface{}{
				"user_arn":          "arn:aws:iam::123456789012:user/ci-deploy",
				"user_name":         "ci-deploy",
				"user_id":           "AIDAEXAMPLEID12345678",
				"access_key_id":     "AKIAEXAMPLEID1234567",
				"secret_access_key": "c2VjcmV0LWtleS1tYXRlcmlhbA==",
				"console_url":       "https://signin.aws.amazon.com/console",
			},
			mustPopulate: []string{
				"user_arn", "user_name", "user_id",
				"access_key_id", "secret_access_key", "console_url",
			},
		},
		{
			// AwsAlb: flat scalar outputs from both engines (arn/name/dns
			// name/hosted zone id/arn suffix) must each land on the StackOutputs
			// proto -- load_balancer_arn is what listeners attach through, the DNS
			// pair is what Route53 alias records consume, and arn_suffix is the
			// CloudWatch LoadBalancer dimension request-count autoscaling scopes on.
			name: "AwsAlb",
			kind: cloudresourcekind.CloudResourceKind_AwsAlb,
			rawOutputs: map[string]interface{}{
				"load_balancer_arn":            "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/demo/50dc6c495c0c9188",
				"load_balancer_name":           "demo",
				"load_balancer_dns_name":       "demo-1234567890.us-west-2.elb.amazonaws.com",
				"load_balancer_hosted_zone_id": "Z1H1FL5HABSF5",
				"arn_suffix":                   "app/demo/50dc6c495c0c9188",
			},
			mustPopulate: []string{
				"load_balancer_arn", "load_balancer_name",
				"load_balancer_dns_name", "load_balancer_hosted_zone_id",
				"arn_suffix",
			},
		},
		{
			// AwsNlb: the same four load-balancer scalars as AwsAlb. Guards the
			// load-balancer-only output shape: the NLB emits no listener or target
			// group outputs because those are first-class kinds with their own
			// outputs.
			name: "AwsNlb",
			kind: cloudresourcekind.CloudResourceKind_AwsNlb,
			rawOutputs: map[string]interface{}{
				"load_balancer_arn":            "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/demo/50dc6c495c0c9188",
				"load_balancer_name":           "demo",
				"load_balancer_dns_name":       "demo-1234567890.elb.us-west-2.amazonaws.com",
				"load_balancer_hosted_zone_id": "Z18D5FSROUN65G",
			},
			mustPopulate: []string{
				"load_balancer_arn", "load_balancer_name",
				"load_balancer_dns_name", "load_balancer_hosted_zone_id",
			},
		},
		{
			// AwsLbTargetGroup: flat scalar outputs from both engines (arn, the
			// possibly-truncated name, and the CloudWatch arn_suffix) must each land
			// on the StackOutputs proto -- target_group_arn is what listener forward
			// actions, ECS services, and ASG attachments reference.
			name: "AwsLbTargetGroup",
			kind: cloudresourcekind.CloudResourceKind_AwsLbTargetGroup,
			rawOutputs: map[string]interface{}{
				"target_group_arn":  "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/api/943f017f100becff",
				"target_group_name": "api",
				"arn_suffix":        "targetgroup/api/943f017f100becff",
			},
			mustPopulate: []string{"target_group_arn", "target_group_name", "arn_suffix"},
		},
		{
			// AwsLbListener: a single flat output -- listener_arn is what listener
			// rules attach through.
			name: "AwsLbListener",
			kind: cloudresourcekind.CloudResourceKind_AwsLbListener,
			rawOutputs: map[string]interface{}{
				"listener_arn": "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/demo/50dc6c495c0c9188/f2f7dc8efc522ab2",
			},
			mustPopulate: []string{"listener_arn"},
		},
		{
			// AwsLbListenerRule: the rule ARN plus the AWS-assigned priority.
			// Priority is emitted as a STRING by both engines (Terraform's tostring
			// and the Pulumi module's strconv conversion) so the shapes stay
			// byte-identical -- this case guards that contract.
			name: "AwsLbListenerRule",
			kind: cloudresourcekind.CloudResourceKind_AwsLbListenerRule,
			rawOutputs: map[string]interface{}{
				"rule_arn": "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener-rule/app/demo/50dc6c495c0c9188/f2f7dc8efc522ab2/9683b2d02a6cabee",
				"priority": "10",
			},
			mustPopulate: []string{"rule_arn", "priority"},
		},
		{
			// AwsEcsTaskDefinition: the revision-carrying ARN is the handle ECS
			// services reference (each new revision changes it and rolls the
			// service); revision is an int64 proto field fed from numeric engine
			// outputs, guarding the numeric-to-int64 flattening.
			name: "AwsEcsTaskDefinition",
			kind: cloudresourcekind.CloudResourceKind_AwsEcsTaskDefinition,
			rawOutputs: map[string]interface{}{
				"task_definition_arn":  "arn:aws:ecs:us-west-2:123456789012:task-definition/api:7",
				"arn_without_revision": "arn:aws:ecs:us-west-2:123456789012:task-definition/api",
				"family":               "api",
				"revision":             float64(7),
				"log_group_name":       "/ecs/api",
				"log_group_arn":        "arn:aws:logs:us-west-2:123456789012:log-group:/ecs/api",
			},
			mustPopulate: []string{
				"task_definition_arn", "arn_without_revision", "family",
				"revision", "log_group_name", "log_group_arn",
			},
		},
		{
			// AwsEcsService: flat scalar outputs -- the service ARN encodes both
			// the cluster and service names (the E2E verifier's key), and the
			// cluster/task-definition ARNs are republished resolved references.
			name: "AwsEcsService",
			kind: cloudresourcekind.CloudResourceKind_AwsEcsService,
			rawOutputs: map[string]interface{}{
				"service_arn":         "arn:aws:ecs:us-west-2:123456789012:service/prod/api",
				"service_name":        "api",
				"cluster_arn":         "arn:aws:ecs:us-west-2:123456789012:cluster/prod",
				"task_definition_arn": "arn:aws:ecs:us-west-2:123456789012:task-definition/api:7",
			},
			mustPopulate: []string{
				"service_arn", "service_name", "cluster_arn", "task_definition_arn",
			},
		},
		{
			// AwsPlantonRunner: flat scalar outputs -- the compute handles
			// (service/cluster/task-definition ARNs, the E2E verifier keys on
			// service_arn), the security-group id private targets trust, the
			// two IAM identities, the token secret, the log group carrying
			// the runner's operation audit trail, and the name the runner
			// registers itself under.
			name: "AwsPlantonRunner",
			kind: cloudresourcekind.CloudResourceKind_AwsPlantonRunner,
			rawOutputs: map[string]interface{}{
				"service_arn":         "arn:aws:ecs:us-west-2:123456789012:service/vpc-runner/vpc-runner",
				"service_name":        "vpc-runner",
				"cluster_arn":         "arn:aws:ecs:us-west-2:123456789012:cluster/vpc-runner",
				"task_definition_arn": "arn:aws:ecs:us-west-2:123456789012:task-definition/vpc-runner:1",
				"log_group_name":      "/ecs/vpc-runner",
				"security_group_id":   "sg-0abc123",
				"execution_role_arn":  "arn:aws:iam::123456789012:role/vpc-runner-exec",
				"task_role_arn":       "arn:aws:iam::123456789012:role/vpc-runner-runtime",
				"token_secret_arn":    "arn:aws:secretsmanager:us-west-2:123456789012:secret:vpc-runner-token-AbCdEf",
				"region":              "us-west-2",
				"runner_name":         "prod-vpc-runner",
			},
			mustPopulate: []string{
				"service_arn", "service_name", "cluster_arn", "task_definition_arn",
				"log_group_name", "security_group_id", "execution_role_arn",
				"task_role_arn", "token_secret_arn", "region", "runner_name",
			},
		},
		{
			// GcpPlantonRunner: flat scalar outputs -- the Cloud Run service
			// handles (the E2E verifier keys on service_short_name plus
			// region/project_id), the runtime service account, the token
			// secret, and the name the runner registers itself under.
			name: "GcpPlantonRunner",
			kind: cloudresourcekind.CloudResourceKind_GcpPlantonRunner,
			rawOutputs: map[string]interface{}{
				"service_name":          "projects/my-project/locations/us-central1/services/vpc-runner",
				"service_short_name":    "vpc-runner",
				"service_account_email": "vpc-runner@my-project.iam.gserviceaccount.com",
				"token_secret_id":       "projects/my-project/secrets/vpc-runner-token",
				"runner_name":           "prod-vpc-runner",
				"project_id":            "my-project",
				"region":                "us-central1",
			},
			mustPopulate: []string{
				"service_name", "service_short_name", "service_account_email",
				"token_secret_id", "runner_name", "project_id", "region",
			},
		},
		{
			// AzurePlantonRunner: flat scalar outputs -- the Container App
			// handles (the E2E verifier keys on container_app_id), the app's
			// token secret name, the registration name, and the resource
			// group.
			name: "AzurePlantonRunner",
			kind: cloudresourcekind.CloudResourceKind_AzurePlantonRunner,
			rawOutputs: map[string]interface{}{
				"container_app_id":    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/my-rg/providers/Microsoft.App/containerApps/vnet-runner",
				"container_app_name":  "vnet-runner",
				"token_secret_name":   "runner-token",
				"runner_name":         "prod-vnet-runner",
				"resource_group_name": "my-rg",
			},
			mustPopulate: []string{
				"container_app_id", "container_app_name", "token_secret_name",
				"runner_name", "resource_group_name",
			},
		},
		{
			// KubernetesPlantonRunner: flat scalar outputs -- the release
			// handles (namespace + release name), the module-created token
			// Secret, and the name the runner registers itself under.
			name: "KubernetesPlantonRunner",
			kind: cloudresourcekind.CloudResourceKind_KubernetesPlantonRunner,
			rawOutputs: map[string]interface{}{
				"namespace":         "planton-runner",
				"release_name":      "cluster-runner",
				"token_secret_name": "cluster-runner-token",
				"runner_name":       "prod-cluster-runner",
			},
			mustPopulate: []string{
				"namespace", "release_name", "token_secret_name", "runner_name",
			},
		},
		{
			// AwsLaunchTemplate: the template id/arn plus the two version numbers.
			// latest_version and default_version are int64 proto fields fed from
			// numeric engine outputs (Terraform's number, Pulumi's IntOutput) --
			// this case guards that numeric outputs flatten onto int64 fields.
			name: "AwsLaunchTemplate",
			kind: cloudresourcekind.CloudResourceKind_AwsLaunchTemplate,
			rawOutputs: map[string]interface{}{
				"launch_template_id":  "lt-0123456789abcdef0",
				"launch_template_arn": "arn:aws:ec2:us-west-2:123456789012:launch-template/lt-0123456789abcdef0",
				"latest_version":      float64(3),
				"default_version":     float64(3),
			},
			mustPopulate: []string{"launch_template_id", "launch_template_arn", "latest_version", "default_version"},
		},
		{
			// AwsAutoScalingGroup: flat scalar outputs -- the group name is the
			// CloudWatch dimension and ECS capacity-provider handle; the ARN scopes
			// IAM policies and EventBridge rules.
			name: "AwsAutoScalingGroup",
			kind: cloudresourcekind.CloudResourceKind_AwsAutoScalingGroup,
			rawOutputs: map[string]interface{}{
				"autoscaling_group_name": "web",
				"autoscaling_group_arn":  "arn:aws:autoscaling:us-west-2:123456789012:autoScalingGroup:uuid:autoScalingGroupName/web",
			},
			mustPopulate: []string{"autoscaling_group_name", "autoscaling_group_arn"},
		},
		{
			// AwsEksAddon: flat scalar outputs -- the ARN keys the E2E verifier
			// (it encodes cluster and add-on names); addon_version reports the
			// resolved AWS default when the spec pinned nothing.
			name: "AwsEksAddon",
			kind: cloudresourcekind.CloudResourceKind_AwsEksAddon,
			rawOutputs: map[string]interface{}{
				"addon_arn":     "arn:aws:eks:us-west-2:123456789012:addon/platform/vpc-cni/9ac7ab21-1a2b",
				"addon_name":    "vpc-cni",
				"addon_version": "v1.18.1-eksbuild.3",
			},
			mustPopulate: []string{"addon_arn", "addon_name", "addon_version"},
		},
		{
			// AwsEksFargateProfile: flat scalar outputs -- the ARN keys the E2E
			// verifier (it encodes cluster and profile names); status is ACTIVE
			// after a successful create.
			name: "AwsEksFargateProfile",
			kind: cloudresourcekind.CloudResourceKind_AwsEksFargateProfile,
			rawOutputs: map[string]interface{}{
				"fargate_profile_arn":  "arn:aws:eks:us-west-2:123456789012:fargateprofile/platform/serverless/9ac7ab21-1a2b",
				"fargate_profile_name": "serverless",
				"status":               "ACTIVE",
			},
			mustPopulate: []string{"fargate_profile_arn", "fargate_profile_name", "status"},
		},
		{
			// AwsEksAccessEntry: flat scalar outputs -- the entry ARN keys the E2E
			// verifier (it encodes the cluster and the principal identity), and the
			// resolved principal ARN is what downstream references consume.
			name: "AwsEksAccessEntry",
			kind: cloudresourcekind.CloudResourceKind_AwsEksAccessEntry,
			rawOutputs: map[string]interface{}{
				"access_entry_arn": "arn:aws:eks:us-west-2:123456789012:access-entry/platform/role/123456789012/TeamViewerRole/9ac7ab21-1a2b",
				"principal_arn":    "arn:aws:iam::123456789012:role/TeamViewerRole",
			},
			mustPopulate: []string{"access_entry_arn", "principal_arn"},
		},
		{
			// AwsVpcEndpoint: the endpoint id keys the E2E verifier; prefix_list_id
			// is the gateway-endpoint route/security-group handle; dns_name +
			// hosted_zone_id compose Route53 aliases to interface endpoints; and
			// network_interface_ids guards list outputs flattening onto a repeated
			// string field.
			name: "AwsVpcEndpoint",
			kind: cloudresourcekind.CloudResourceKind_AwsVpcEndpoint,
			rawOutputs: map[string]interface{}{
				"vpc_endpoint_id":       "vpce-0123456789abcdef0",
				"arn":                   "arn:aws:ec2:us-west-2:123456789012:vpc-endpoint/vpce-0123456789abcdef0",
				"state":                 "available",
				"prefix_list_id":        "pl-68a54001",
				"dns_name":              "vpce-0123456789abcdef0-abcd1234.sts.us-west-2.vpce.amazonaws.com",
				"hosted_zone_id":        "Z1K56Z6FNPJRR",
				"network_interface_ids": []interface{}{"eni-0123456789abcdef0", "eni-0f9e8d7c6b5a43210"},
			},
			mustPopulate: []string{
				"vpc_endpoint_id", "arn", "state", "prefix_list_id",
				"dns_name", "hosted_zone_id", "network_interface_ids",
			},
		},
		{
			// AwsRdsCluster: the identifier keys the E2E verifier; endpoint +
			// reader_endpoint are the connection handles downstream references
			// consume; master_user_secret_arn carries the AWS-managed credential
			// handle; and instance_endpoints guards list outputs flattening onto a
			// repeated string field (the folded per-name cluster instances).
			name: "AwsRdsCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsRdsCluster,
			rawOutputs: map[string]interface{}{
				"cluster_identifier":              "orders-db",
				"arn":                             "arn:aws:rds:us-west-2:123456789012:cluster:orders-db",
				"cluster_resource_id":             "cluster-ABCDEFGHIJKL01234",
				"endpoint":                        "orders-db.cluster-abc123.us-west-2.rds.amazonaws.com",
				"reader_endpoint":                 "orders-db.cluster-ro-abc123.us-west-2.rds.amazonaws.com",
				"port":                            5432,
				"hosted_zone_id":                  "Z1PVIF0B656C1W",
				"engine_version_actual":           "16.4",
				"master_user_secret_arn":          "arn:aws:secretsmanager:us-west-2:123456789012:secret:rds!cluster-abc-def",
				"db_subnet_group_name":            "orders-db",
				"db_cluster_parameter_group_name": "default.aurora-postgresql16",
				"instance_endpoints":              []interface{}{"orders-db-writer.abc123.us-west-2.rds.amazonaws.com"},
			},
			mustPopulate: []string{
				"cluster_identifier", "arn", "cluster_resource_id", "endpoint",
				"reader_endpoint", "port", "hosted_zone_id", "engine_version_actual",
				"master_user_secret_arn", "db_subnet_group_name",
				"db_cluster_parameter_group_name", "instance_endpoints",
			},
		},
		{
			// AwsRdsInstance: the identifier keys the E2E verifier; endpoint is
			// address:port while address is the bare hostname (both are real AWS
			// attributes downstream references consume differently); resource_id is
			// the durable handle for IAM auth policies and point-in-time restores.
			name: "AwsRdsInstance",
			kind: cloudresourcekind.CloudResourceKind_AwsRdsInstance,
			rawOutputs: map[string]interface{}{
				"instance_identifier":    "billing-db",
				"arn":                    "arn:aws:rds:us-west-2:123456789012:db:billing-db",
				"resource_id":            "db-ABCDEFGHIJKL01234",
				"endpoint":               "billing-db.abc123.us-west-2.rds.amazonaws.com:5432",
				"address":                "billing-db.abc123.us-west-2.rds.amazonaws.com",
				"port":                   5432,
				"hosted_zone_id":         "Z1PVIF0B656C1W",
				"engine_version_actual":  "16.4",
				"master_user_secret_arn": "arn:aws:secretsmanager:us-west-2:123456789012:secret:rds!db-abc-def",
				"db_subnet_group_name":   "billing-db",
			},
			mustPopulate: []string{
				"instance_identifier", "arn", "resource_id", "endpoint", "address",
				"port", "hosted_zone_id", "engine_version_actual",
				"master_user_secret_arn", "db_subnet_group_name",
			},
		},
		{
			name: "AwsElasticacheUser",
			kind: cloudresourcekind.CloudResourceKind_AwsElasticacheUser,
			rawOutputs: map[string]interface{}{
				"user_id":   "app-cache-user",
				"arn":       "arn:aws:elasticache:us-west-2:123456789012:user:app-cache-user",
				"user_name": "app-cache-user",
			},
			mustPopulate: []string{"user_id", "arn", "user_name"},
		},
		{
			name: "AwsElasticacheUserGroup",
			kind: cloudresourcekind.CloudResourceKind_AwsElasticacheUserGroup,
			rawOutputs: map[string]interface{}{
				"user_group_id": "app-cache-group",
				"arn":           "arn:aws:elasticache:us-west-2:123456789012:usergroup:app-cache-group",
			},
			mustPopulate: []string{"user_group_id", "arn"},
		},
		{
			name: "AwsRedisElasticache",
			kind: cloudresourcekind.CloudResourceKind_AwsRedisElasticache,
			rawOutputs: map[string]interface{}{
				"replication_group_id":           "orders-cache",
				"primary_endpoint_address":       "orders-cache.abc123.usw2.cache.amazonaws.com",
				"reader_endpoint_address":        "orders-cache-ro.abc123.usw2.cache.amazonaws.com",
				"configuration_endpoint_address": "",
				"arn":                            "arn:aws:elasticache:us-west-2:123456789012:replicationgroup:orders-cache",
				"port":                           6379,
				"subnet_group_name":              "orders-cache",
				"parameter_group_name":           "orders-cache-custom",
				"engine_version_actual":          "7.1.0",
			},
			mustPopulate: []string{
				"replication_group_id", "primary_endpoint_address", "reader_endpoint_address",
				"arn", "port", "subnet_group_name", "parameter_group_name", "engine_version_actual",
			},
		},
		{
			name: "AwsMemcachedElasticache",
			kind: cloudresourcekind.CloudResourceKind_AwsMemcachedElasticache,
			rawOutputs: map[string]interface{}{
				"cluster_id":             "session-cache",
				"cluster_address":        "session-cache.abc123.cfg.usw2.cache.amazonaws.com",
				"configuration_endpoint": "session-cache.abc123.cfg.usw2.cache.amazonaws.com:11211",
				"arn":                    "arn:aws:elasticache:us-west-2:123456789012:cluster:session-cache",
				"port":                   11211,
				"subnet_group_name":      "session-cache",
				"parameter_group_name":   "session-cache-custom",
			},
			mustPopulate: []string{
				"cluster_id", "cluster_address", "configuration_endpoint",
				"arn", "port", "subnet_group_name", "parameter_group_name",
			},
		},
		{
			name: "AwsServerlessElasticache",
			kind: cloudresourcekind.CloudResourceKind_AwsServerlessElasticache,
			rawOutputs: map[string]interface{}{
				"arn":                     "arn:aws:elasticache:us-west-2:123456789012:serverlesscache:orders-srvless",
				"endpoint_address":        "orders-srvless-abc123.serverless.usw2.cache.amazonaws.com",
				"endpoint_port":           6379,
				"reader_endpoint_address": "orders-srvless-abc123-ro.serverless.usw2.cache.amazonaws.com",
				"reader_endpoint_port":    6380,
				"full_engine_version":     "7.1.0",
				"name":                    "orders-srvless",
			},
			mustPopulate: []string{
				"arn", "endpoint_address", "endpoint_port", "reader_endpoint_address",
				"reader_endpoint_port", "full_engine_version", "name",
			},
		},
		{
			// AwsDocumentDb: the identifier keys the E2E verifier; endpoint +
			// reader_endpoint are the connection handles downstream references
			// consume; master_user_secret_arn carries the AWS-managed credential
			// handle; and instance_endpoints guards list outputs flattening onto a
			// repeated string field (the folded per-name cluster instances).
			name: "AwsDocumentDb",
			kind: cloudresourcekind.CloudResourceKind_AwsDocumentDb,
			rawOutputs: map[string]interface{}{
				"cluster_identifier":              "catalog-docdb",
				"arn":                             "arn:aws:rds:us-west-2:123456789012:cluster:catalog-docdb",
				"cluster_resource_id":             "cluster-ABCDEFGHIJKL01234",
				"endpoint":                        "catalog-docdb.cluster-abc123.us-west-2.docdb.amazonaws.com",
				"reader_endpoint":                 "catalog-docdb.cluster-ro-abc123.us-west-2.docdb.amazonaws.com",
				"port":                            27017,
				"hosted_zone_id":                  "ZNKXH85TT8WVW",
				"engine_version_actual":           "5.0.0",
				"master_user_secret_arn":          "arn:aws:secretsmanager:us-west-2:123456789012:secret:rds!cluster-abc-def",
				"db_subnet_group_name":            "catalog-docdb",
				"db_cluster_parameter_group_name": "default.docdb5.0",
				"instance_endpoints":              []interface{}{"catalog-docdb-writer.abc123.us-west-2.docdb.amazonaws.com"},
			},
			mustPopulate: []string{
				"cluster_identifier", "arn", "cluster_resource_id", "endpoint",
				"reader_endpoint", "port", "hosted_zone_id", "engine_version_actual",
				"master_user_secret_arn", "db_subnet_group_name",
				"db_cluster_parameter_group_name", "instance_endpoints",
			},
		},
		{
			// AwsNeptuneCluster: the identifier keys the E2E verifier; the
			// cluster_resource_id is the durable handle IAM database-auth policies
			// scope to; and instance_endpoints guards list outputs flattening onto
			// a repeated string field. This case also guards the Terraform module's
			// first-ever outputs.tf (its absence was a live cross-engine parity bug).
			name: "AwsNeptuneCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsNeptuneCluster,
			rawOutputs: map[string]interface{}{
				"cluster_identifier":                   "knowledge-graph",
				"arn":                                  "arn:aws:rds:us-west-2:123456789012:cluster:knowledge-graph",
				"cluster_resource_id":                  "cluster-ABCDEFGHIJKL01234",
				"endpoint":                             "knowledge-graph.cluster-abc123.us-west-2.neptune.amazonaws.com",
				"reader_endpoint":                      "knowledge-graph.cluster-ro-abc123.us-west-2.neptune.amazonaws.com",
				"port":                                 8182,
				"hosted_zone_id":                       "Z2T2AVZR3PGPQK",
				"engine_version_actual":                "1.4.5.1",
				"neptune_subnet_group_name":            "knowledge-graph",
				"neptune_cluster_parameter_group_name": "default.neptune1.4",
				"instance_endpoints":                   []interface{}{"knowledge-graph-writer.abc123.us-west-2.neptune.amazonaws.com"},
			},
			mustPopulate: []string{
				"cluster_identifier", "arn", "cluster_resource_id", "endpoint",
				"reader_endpoint", "port", "hosted_zone_id", "engine_version_actual",
				"neptune_subnet_group_name", "neptune_cluster_parameter_group_name",
				"instance_endpoints",
			},
		},
		{
			// AwsRedshiftCluster: the identifier keys the E2E verifier; endpoint +
			// dns_name are the connection handles downstream references consume;
			// cluster_namespace_arn is the data-sharing/Data-API handle; and
			// master_password_secret_arn carries the AWS-managed credential handle.
			name: "AwsRedshiftCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsRedshiftCluster,
			rawOutputs: map[string]interface{}{
				"cluster_identifier":         "analytics-warehouse",
				"cluster_arn":                "arn:aws:redshift:us-west-2:123456789012:cluster:analytics-warehouse",
				"cluster_namespace_arn":      "arn:aws:redshift:us-west-2:123456789012:namespace:abc12345-6789-0abc-def1-234567890abc",
				"endpoint":                   "analytics-warehouse.abc123.us-west-2.redshift.amazonaws.com:5439",
				"dns_name":                   "analytics-warehouse.abc123.us-west-2.redshift.amazonaws.com",
				"database_name":              "analytics",
				"port":                       5439,
				"subnet_group_name":          "analytics-warehouse",
				"parameter_group_name":       "analytics-warehouse",
				"master_password_secret_arn": "arn:aws:secretsmanager:us-west-2:123456789012:secret:redshift!analytics-abc",
			},
			mustPopulate: []string{
				"cluster_identifier", "cluster_arn", "cluster_namespace_arn",
				"endpoint", "dns_name", "database_name", "port",
				"subnet_group_name", "parameter_group_name",
				"master_password_secret_arn",
			},
		},
		{
			// AwsRedshiftServerlessNamespace: namespace_name is the join key
			// workgroups attach with (downstream references resolve against
			// stack outputs, never metadata); admin_password_secret_arn
			// carries the AWS-managed credential handle.
			name: "AwsRedshiftServerlessNamespace",
			kind: cloudresourcekind.CloudResourceKind_AwsRedshiftServerlessNamespace,
			rawOutputs: map[string]interface{}{
				"namespace_name":            "analytics-data",
				"namespace_id":              "abc12345-6789-0abc-def1-234567890abc",
				"arn":                       "arn:aws:redshift-serverless:us-west-2:123456789012:namespace/abc12345-6789-0abc-def1-234567890abc",
				"db_name":                   "analytics",
				"admin_password_secret_arn": "arn:aws:secretsmanager:us-west-2:123456789012:secret:redshift!analytics-data-admin-abc",
			},
			mustPopulate: []string{
				"namespace_name", "namespace_id", "arn", "db_name",
				"admin_password_secret_arn",
			},
		},
		{
			// AwsRedshiftServerlessWorkgroup: workgroup_name keys the E2E
			// verifier and the credentials API; endpoint_address + port are
			// the connection handles downstream references consume.
			name: "AwsRedshiftServerlessWorkgroup",
			kind: cloudresourcekind.CloudResourceKind_AwsRedshiftServerlessWorkgroup,
			rawOutputs: map[string]interface{}{
				"workgroup_name":   "analytics-compute",
				"workgroup_id":     "def67890-1234-5abc-def6-789012345def",
				"arn":              "arn:aws:redshift-serverless:us-west-2:123456789012:workgroup/def67890-1234-5abc-def6-789012345def",
				"endpoint_address": "analytics-compute.123456789012.us-west-2.redshift-serverless.amazonaws.com",
				"port":             5439,
			},
			mustPopulate: []string{
				"workgroup_name", "workgroup_id", "arn", "endpoint_address",
				"port",
			},
		},
		{
			// AwsDynamodb: table_name/table_arn are the join keys IAM policies
			// and application config consume; stream_arn is what Lambda
			// event-source mappings attach to when streams are enabled.
			name: "AwsDynamodb",
			kind: cloudresourcekind.CloudResourceKind_AwsDynamodb,
			rawOutputs: map[string]interface{}{
				"table_name":   "orders",
				"table_arn":    "arn:aws:dynamodb:us-west-2:123456789012:table/orders",
				"table_id":     "orders",
				"stream_arn":   "arn:aws:dynamodb:us-west-2:123456789012:table/orders/stream/2026-07-04T00:00:00.000",
				"stream_label": "2026-07-04T00:00:00.000",
			},
			mustPopulate: []string{
				"table_name", "table_arn", "table_id", "stream_arn",
				"stream_label",
			},
		},
		{
			// AwsMskCluster: cluster_arn keys the E2E verifier and IAM
			// policies; the bootstrap_brokers_* family carries the
			// per-listener connection strings clients consume (each engine
			// emits every variant, empty when the listener is off); and
			// configuration_arn surfaces the module-managed configuration
			// folded from server_properties.
			name: "AwsMskCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsMskCluster,
			rawOutputs: map[string]interface{}{
				"cluster_arn":                                   "arn:aws:kafka:us-west-2:123456789012:cluster/orders-streaming/abc12345-6789-0abc-def1-234567890abc-2",
				"cluster_name":                                  "orders-streaming",
				"cluster_uuid":                                  "abc12345-6789-0abc-def1-234567890abc-2",
				"current_version":                               "K3AEGXETSR30VB",
				"bootstrap_brokers":                             "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9092",
				"bootstrap_brokers_tls":                         "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9094",
				"bootstrap_brokers_sasl_iam":                    "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9098",
				"bootstrap_brokers_sasl_scram":                  "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9096",
				"bootstrap_brokers_public_tls":                  "b-1-public.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9194",
				"bootstrap_brokers_public_sasl_iam":             "b-1-public.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9198",
				"bootstrap_brokers_public_sasl_scram":           "b-1-public.orders.abc123.c2.kafka.us-west-2.amazonaws.com:9196",
				"bootstrap_brokers_vpc_connectivity_tls":        "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:14001",
				"bootstrap_brokers_vpc_connectivity_sasl_iam":   "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:14003",
				"bootstrap_brokers_vpc_connectivity_sasl_scram": "b-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:14002",
				"zookeeper_connect_string":                      "z-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:2181",
				"zookeeper_connect_string_tls":                  "z-1.orders.abc123.c2.kafka.us-west-2.amazonaws.com:2182",
				"configuration_arn":                             "arn:aws:kafka:us-west-2:123456789012:configuration/orders-streaming/def67890-1234-5abc-def6-789012345def-3",
			},
			mustPopulate: []string{
				"cluster_arn", "cluster_name", "cluster_uuid", "current_version",
				"bootstrap_brokers", "bootstrap_brokers_tls",
				"bootstrap_brokers_sasl_iam", "bootstrap_brokers_sasl_scram",
				"bootstrap_brokers_public_tls", "bootstrap_brokers_public_sasl_iam",
				"bootstrap_brokers_public_sasl_scram",
				"bootstrap_brokers_vpc_connectivity_tls",
				"bootstrap_brokers_vpc_connectivity_sasl_iam",
				"bootstrap_brokers_vpc_connectivity_sasl_scram",
				"zookeeper_connect_string", "zookeeper_connect_string_tls",
				"configuration_arn",
			},
		},
		{
			// AwsMskServerlessCluster: cluster_arn keys the E2E verifier and
			// the kafka-cluster:* IAM policies clients need;
			// bootstrap_brokers_sasl_iam is the only connection string
			// serverless MSK exposes (SASL/IAM is its sole auth scheme).
			name: "AwsMskServerlessCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsMskServerlessCluster,
			rawOutputs: map[string]interface{}{
				"cluster_arn":                "arn:aws:kafka:us-west-2:123456789012:cluster/events-kafka/abc12345-6789-0abc-def1-234567890abc-s1",
				"cluster_name":               "events-kafka",
				"cluster_uuid":               "abc12345-6789-0abc-def1-234567890abc-s1",
				"bootstrap_brokers_sasl_iam": "boot-abc123.c1.kafka-serverless.us-west-2.amazonaws.com:9098",
			},
			mustPopulate: []string{
				"cluster_arn", "cluster_name", "cluster_uuid",
				"bootstrap_brokers_sasl_iam",
			},
		},
		{
			// AwsSecurityGroup: security_group_id is the join key every
			// attach-shaped kind references; security_group_arn is the form
			// IAM policy conditions expect; owner_id enables cross-account
			// rule references (<owner_id>/<group_id>).
			name: "AwsSecurityGroup",
			kind: cloudresourcekind.CloudResourceKind_AwsSecurityGroup,
			rawOutputs: map[string]interface{}{
				"security_group_id":  "sg-0123456789abcdef0",
				"security_group_arn": "arn:aws:ec2:us-west-2:123456789012:security-group/sg-0123456789abcdef0",
				"owner_id":           "123456789012",
			},
			mustPopulate: []string{
				"security_group_id", "security_group_arn", "owner_id",
			},
		},
		{
			// AwsLambda: function_name keys the E2E verifier; function_arn is
			// the join key for event-source mappings and IAM policies; invoke_arn
			// is what API Gateway integrations consume.
			name: "AwsLambda",
			kind: cloudresourcekind.CloudResourceKind_AwsLambda,
			rawOutputs: map[string]interface{}{
				"function_arn":   "arn:aws:lambda:us-west-2:123456789012:function:planton-oss-e2e-lambda-smoke",
				"function_name":  "planton-oss-e2e-lambda-smoke",
				"invoke_arn":     "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:123456789012:function:planton-oss-e2e-lambda-smoke/invocations",
				"qualified_arn":  "",
				"version":        "",
				"function_url":   "",
				"alias_arns":     map[string]interface{}{},
				"log_group_name": "/aws/lambda/planton-oss-e2e-lambda-smoke",
			},
			mustPopulate: []string{
				"function_arn", "function_name", "invoke_arn", "log_group_name",
			},
		},
		{
			// AwsKmsKey: key_id keys the E2E verifier; key_arn is the join key
			// encryption-at-rest fields reference; alias_names carries the human-
			// friendly addresses SDK callers may use instead of the key ID.
			name: "AwsKmsKey",
			kind: cloudresourcekind.CloudResourceKind_AwsKmsKey,
			rawOutputs: map[string]interface{}{
				"key_id":      "12345678-1234-1234-1234-123456789012",
				"key_arn":     "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012",
				"alias_names": []interface{}{"alias/planton-oss-e2e-kms-smoke"},
			},
			mustPopulate: []string{
				"key_id", "key_arn", "alias_names",
			},
		},
		{
			// AwsSqsQueue: queue_url is the SQS API handle; queue_arn is the
			// IAM/cross-service join key (DLQ targets, SNS subscriptions,
			// Lambda event source mappings); queue_name keys the E2E verifier.
			name: "AwsSqsQueue",
			kind: cloudresourcekind.CloudResourceKind_AwsSqsQueue,
			rawOutputs: map[string]interface{}{
				"queue_url":  "https://sqs.us-west-2.amazonaws.com/123456789012/planton-oss-e2e-sqs-smoke",
				"queue_arn":  "arn:aws:sqs:us-west-2:123456789012:planton-oss-e2e-sqs-smoke",
				"queue_name": "planton-oss-e2e-sqs-smoke",
			},
			mustPopulate: []string{
				"queue_url", "queue_arn", "queue_name",
			},
		},
		{
			// AwsSnsTopic: topic_arn is the subscription/EventBridge join key;
			// topic_name keys the E2E verifier; owner and beginning_archive_time
			// surface FIFO archive metadata when enabled.
			name: "AwsSnsTopic",
			kind: cloudresourcekind.CloudResourceKind_AwsSnsTopic,
			rawOutputs: map[string]interface{}{
				"topic_arn":              "arn:aws:sns:us-west-2:123456789012:planton-oss-e2e-sns-smoke",
				"topic_name":             "planton-oss-e2e-sns-smoke",
				"owner":                  "123456789012",
				"beginning_archive_time": "2026-07-04T12:00:00Z",
			},
			mustPopulate: []string{
				"topic_arn", "topic_name", "owner", "beginning_archive_time",
			},
		},
		{
			// AwsSnsSubscription: subscription_arn is the AWS identity and
			// unsubscribe handle; owner_id supports cross-account wiring;
			// pending_confirmation and confirmation_was_authenticated surface
			// the HTTP/email handshake lifecycle.
			name: "AwsSnsSubscription",
			kind: cloudresourcekind.CloudResourceKind_AwsSnsSubscription,
			rawOutputs: map[string]interface{}{
				"subscription_arn":               "arn:aws:sns:us-west-2:123456789012:planton-oss-e2e-sns-smoke:01234567-89ab-cdef-0123-456789abcdef",
				"owner_id":                       "123456789012",
				"pending_confirmation":           true,
				"confirmation_was_authenticated": true,
			},
			mustPopulate: []string{
				"subscription_arn", "owner_id",
				"pending_confirmation", "confirmation_was_authenticated",
			},
		},
		{
			// AwsEventBridgeBus: bus_name keys the E2E verifier and rule
			// event_bus_name references; bus_arn is the IAM/cross-account
			// join key.
			name: "AwsEventBridgeBus",
			kind: cloudresourcekind.CloudResourceKind_AwsEventBridgeBus,
			rawOutputs: map[string]interface{}{
				"bus_name": "planton-oss-e2e-eventbridge-bus-smoke",
				"bus_arn":  "arn:aws:events:us-west-2:123456789012:event-bus/planton-oss-e2e-eventbridge-bus-smoke",
			},
			mustPopulate: []string{
				"bus_name", "bus_arn",
			},
		},
		{
			// AwsEventBridgeRule: rule_arn is the IAM/monitoring join key;
			// rule_name keys the E2E verifier and EventBridge API calls.
			name: "AwsEventBridgeRule",
			kind: cloudresourcekind.CloudResourceKind_AwsEventBridgeRule,
			rawOutputs: map[string]interface{}{
				"rule_arn":  "arn:aws:events:us-west-2:123456789012:rule/planton-oss-e2e-eventbridge-bus-smoke/planton-oss-e2e-rule-smoke",
				"rule_name": "planton-oss-e2e-rule-smoke",
			},
			mustPopulate: []string{
				"rule_arn", "rule_name",
			},
		},
		{
			// AwsLambdaEventSourceMapping: uuid keys the E2E verifier;
			// mapping_arn and function_arn are the join keys downstream
			// automation consumes; state surfaces the last observed lifecycle.
			name: "AwsLambdaEventSourceMapping",
			kind: cloudresourcekind.CloudResourceKind_AwsLambdaEventSourceMapping,
			rawOutputs: map[string]interface{}{
				"uuid":         "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"mapping_arn":  "arn:aws:lambda:us-west-2:123456789012:event-source-mapping:a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"function_arn": "arn:aws:lambda:us-west-2:123456789012:function:planton-oss-e2e-lambda-smoke",
				"state":        "Enabled",
			},
			mustPopulate: []string{
				"uuid", "mapping_arn", "function_arn", "state",
			},
		},
		{
			// AwsMwaaEnvironment: environment_name keys the E2E verifier;
			// webserver_url is the operator's handle on the Airflow UI; the
			// two *_vpc_endpoint_service outputs are what CUSTOMER endpoint
			// management composes AwsVpcEndpoint nodes against.
			name: "AwsMwaaEnvironment",
			kind: cloudresourcekind.CloudResourceKind_AwsMwaaEnvironment,
			rawOutputs: map[string]interface{}{
				"environment_arn":                "arn:aws:airflow:us-west-2:123456789012:environment/prod-airflow",
				"environment_name":               "prod-airflow",
				"webserver_url":                  "abc123de-f456-7890-abcd-ef1234567890.c2.us-west-2.airflow.amazonaws.com",
				"airflow_version":                "2.10.1",
				"service_role_arn":               "arn:aws:iam::123456789012:role/aws-service-role/airflow.amazonaws.com/AWSServiceRoleForAmazonMWAA",
				"environment_class":              "mw1.medium",
				"status":                         "AVAILABLE",
				"created_at":                     "2026-07-04T12:00:00Z",
				"database_vpc_endpoint_service":  "com.amazonaws.vpce.us-west-2.vpce-svc-0123456789abcdef0",
				"webserver_vpc_endpoint_service": "com.amazonaws.vpce.us-west-2.vpce-svc-0fedcba9876543210",
			},
			mustPopulate: []string{
				"environment_arn", "environment_name", "webserver_url",
				"airflow_version", "service_role_arn", "environment_class",
				"status", "created_at", "database_vpc_endpoint_service",
				"webserver_vpc_endpoint_service",
			},
		},
		{
			// AwsOpenSearchDomain: domain_name keys the E2E verifier;
			// endpoint + dashboard_endpoint are the connection handles
			// downstream references consume; the *_v2 trio carries the
			// dual-stack endpoint surface added this session.
			name: "AwsOpenSearchDomain",
			kind: cloudresourcekind.CloudResourceKind_AwsOpenSearchDomain,
			rawOutputs: map[string]interface{}{
				"domain_id":                         "123456789012/search-logs",
				"domain_name":                       "search-logs",
				"domain_arn":                        "arn:aws:es:us-west-2:123456789012:domain/search-logs",
				"endpoint":                          "search-search-logs-abc123.us-west-2.es.amazonaws.com",
				"dashboard_endpoint":                "search-search-logs-abc123.us-west-2.es.amazonaws.com/_dashboards",
				"endpoint_v2":                       "search-logs-abc123.us-west-2.aos.amazonaws.com",
				"dashboard_endpoint_v2":             "search-logs-abc123.us-west-2.aos.amazonaws.com/_dashboards",
				"domain_endpoint_v2_hosted_zone_id": "Z1H1FL5HABSF5",
			},
			mustPopulate: []string{
				"domain_id", "domain_name", "domain_arn", "endpoint",
				"dashboard_endpoint", "endpoint_v2", "dashboard_endpoint_v2",
				"domain_endpoint_v2_hosted_zone_id",
			},
		},
		{
			// AwsEc2Instance: instance_id is the join key target groups
			// register; the address quartet carries the connection surface
			// (public values empty for private-only instances -- both
			// engines emit them regardless).
			name: "AwsEc2Instance",
			kind: cloudresourcekind.CloudResourceKind_AwsEc2Instance,
			rawOutputs: map[string]interface{}{
				"instance_id":                  "i-0123456789abcdef0",
				"arn":                          "arn:aws:ec2:us-west-2:123456789012:instance/i-0123456789abcdef0",
				"instance_state":               "running",
				"availability_zone":            "us-west-2a",
				"private_ip":                   "10.0.1.15",
				"private_dns":                  "ip-10-0-1-15.us-west-2.compute.internal",
				"public_ip":                    "",
				"public_dns":                   "",
				"primary_network_interface_id": "eni-0123456789abcdef0",
			},
			mustPopulate: []string{
				"instance_id", "arn", "instance_state", "availability_zone",
				"private_ip", "private_dns", "primary_network_interface_id",
			},
		},
		{
			// AwsEcsCluster: cluster_arn is the join key AwsEcsService
			// references; capacity_provider_names is the strategy
			// vocabulary (built-ins plus folded EC2 providers) and
			// capacity_provider_arns the folded providers' identities --
			// both list outputs, guarding list flattening.
			name: "AwsEcsCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsEcsCluster,
			rawOutputs: map[string]interface{}{
				"cluster_name":            "prod-apps",
				"cluster_arn":             "arn:aws:ecs:us-west-2:123456789012:cluster/prod-apps",
				"capacity_provider_names": []interface{}{"FARGATE", "FARGATE_SPOT", "general-purpose"},
				"capacity_provider_arns":  []interface{}{"arn:aws:ecs:us-west-2:123456789012:capacity-provider/general-purpose"},
			},
			mustPopulate: []string{
				"cluster_name", "cluster_arn", "capacity_provider_names",
				"capacity_provider_arns",
			},
		},
		{
			// AwsS3Bucket: bucket_id (name) and bucket_arn are the join keys the
			// catalog's 12 consumer fields reference; the regional domain doubles
			// as the CloudFront origin domain; the website pair is exported as
			// empty strings when hosting is off so the output contract stays
			// shape-stable across both engines.
			name: "AwsS3Bucket",
			kind: cloudresourcekind.CloudResourceKind_AwsS3Bucket,
			rawOutputs: map[string]interface{}{
				"bucket_id":                   "planton-oss-e2e-awss3bucket-smoke",
				"bucket_arn":                  "arn:aws:s3:::planton-oss-e2e-awss3bucket-smoke",
				"region":                      "us-west-2",
				"bucket_regional_domain_name": "planton-oss-e2e-awss3bucket-smoke.s3.us-west-2.amazonaws.com",
				"bucket_domain_name":          "planton-oss-e2e-awss3bucket-smoke.s3.amazonaws.com",
				"hosted_zone_id":              "Z3BJ6K6RIION7M",
				"website_endpoint":            "planton-oss-e2e-awss3bucket-smoke.s3-website-us-west-2.amazonaws.com",
				"website_domain":              "s3-website-us-west-2.amazonaws.com",
			},
			mustPopulate: []string{
				"bucket_id", "bucket_arn", "region",
				"bucket_regional_domain_name", "bucket_domain_name",
				"hosted_zone_id", "website_endpoint", "website_domain",
			},
		},
		{
			// AwsS3ObjectSet: a map-output kind — the three per-key maps are
			// dot-flattened by both engines (object_etags.config/app.json = ...)
			// and must route back into the proto maps with keys VERBATIM
			// (object keys contain slashes and dots). bucket_id keys the E2E
			// verifier's HeadObject loop; object_arns composes into IAM policy
			// Resource lists.
			name: "AwsS3ObjectSet",
			kind: cloudresourcekind.CloudResourceKind_AwsS3ObjectSet,
			rawOutputs: map[string]interface{}{
				"bucket_id": "planton-oss-e2e-awss3bucket-prereq",
				"object_arns": map[string]interface{}{
					"config/app.json": "arn:aws:s3:::planton-oss-e2e-awss3bucket-prereq/config/app.json",
				},
				"object_etags": map[string]interface{}{
					"config/app.json": "9a0364b9e99bb480dd25e1f0284c8555",
				},
				"object_version_ids": map[string]interface{}{
					"config/app.json": "3sL4kqtJlcpXroDTDmJ+rmSpXd3dIbrHY+MTRCxf3vjVBH40Nr8X8gdRQBpUMLUo",
				},
			},
			mustPopulate: []string{
				"bucket_id", "object_arns", "object_etags", "object_version_ids",
			},
		},
		{
			// AwsKinesisStream: stream_arn is the join key consumers, Lambda
			// event source mappings, DynamoDB streaming destinations, and
			// Firehose sources reference; stream_name keys the E2E verifier.
			name: "AwsKinesisStream",
			kind: cloudresourcekind.CloudResourceKind_AwsKinesisStream,
			rawOutputs: map[string]interface{}{
				"stream_arn":  "arn:aws:kinesis:us-west-2:123456789012:stream/planton-oss-e2e-kinesis-smoke",
				"stream_name": "planton-oss-e2e-kinesis-smoke",
			},
			mustPopulate: []string{
				"stream_arn", "stream_name",
			},
		},
		{
			// AwsKinesisStreamConsumer: consumer_arn is the enhanced-fan-out
			// identity SubscribeToShard callers use; consumer_name keys the
			// E2E verifier; stream_arn echoes the parent join key.
			name: "AwsKinesisStreamConsumer",
			kind: cloudresourcekind.CloudResourceKind_AwsKinesisStreamConsumer,
			rawOutputs: map[string]interface{}{
				"consumer_arn":       "arn:aws:kinesis:us-west-2:123456789012:stream/planton-oss-e2e-kinesis-smoke/consumer/planton-oss-e2e-consumer-smoke:1751700000",
				"consumer_name":      "planton-oss-e2e-consumer-smoke",
				"stream_arn":         "arn:aws:kinesis:us-west-2:123456789012:stream/planton-oss-e2e-kinesis-smoke",
				"creation_timestamp": "2026-07-05T12:00:00Z",
			},
			mustPopulate: []string{
				"consumer_arn", "consumer_name", "stream_arn", "creation_timestamp",
			},
		},
		{
			// AwsKinesisFirehose: delivery_stream_arn is the IAM/EventBridge
			// join key; delivery_stream_name keys the E2E verifier and the
			// MSK broker-log delivery reference; destination_id + version_id
			// are the UpdateDestination coordinates AWS assigns at creation.
			name: "AwsKinesisFirehose",
			kind: cloudresourcekind.CloudResourceKind_AwsKinesisFirehose,
			rawOutputs: map[string]interface{}{
				"delivery_stream_arn":  "arn:aws:firehose:us-west-2:123456789012:deliverystream/planton-oss-e2e-firehose-smoke",
				"delivery_stream_name": "planton-oss-e2e-firehose-smoke",
				"destination_id":       "destinationId-000000000001",
				"version_id":           "1",
			},
			mustPopulate: []string{
				"delivery_stream_arn", "delivery_stream_name", "destination_id", "version_id",
			},
		},
		{
			// AwsEcrRepo: repository_url is what docker push/pull targets;
			// repository_arn scopes IAM policies; repository_name keys the
			// E2E verifier; registry_id is the owning account.
			name: "AwsEcrRepo",
			kind: cloudresourcekind.CloudResourceKind_AwsEcrRepo,
			rawOutputs: map[string]interface{}{
				"repository_name": "planton-oss-e2e/full-surface",
				"repository_url":  "123456789012.dkr.ecr.us-west-2.amazonaws.com/planton-oss-e2e/full-surface",
				"repository_arn":  "arn:aws:ecr:us-west-2:123456789012:repository/planton-oss-e2e/full-surface",
				"registry_id":     "123456789012",
			},
			mustPopulate: []string{
				"repository_name", "repository_url", "repository_arn", "registry_id",
			},
		},
		{
			// AwsRoute53Zone: zone_id is the join key every DNS-composing
			// resource references (records, ACM validation, ALB/NLB alias
			// registration) and keys the E2E verifier; nameservers carry the
			// registrar delegation values.
			name: "AwsRoute53Zone",
			kind: cloudresourcekind.CloudResourceKind_AwsRoute53Zone,
			rawOutputs: map[string]interface{}{
				"zone_id":             "Z1D633PJN98FT9",
				"zone_name":           "example.com",
				"nameservers":         []interface{}{"ns-1.awsdns-01.org", "ns-2.awsdns-02.com"},
				"primary_name_server": "ns-1.awsdns-01.org",
				"zone_arn":            "arn:aws:route53:::hostedzone/Z1D633PJN98FT9",
			},
			mustPopulate: []string{
				"zone_id", "zone_name", "nameservers", "primary_name_server", "zone_arn",
			},
		},
		{
			// AwsRoute53DnsRecord: fqdn + record_type + zone_id together key
			// the E2E verifier (a record has no standalone describe API);
			// is_alias and set_identifier echo the record's shape.
			name: "AwsRoute53DnsRecord",
			kind: cloudresourcekind.CloudResourceKind_AwsRoute53DnsRecord,
			rawOutputs: map[string]interface{}{
				"fqdn":           "canary.example.com",
				"record_type":    "A",
				"zone_id":        "Z1D633PJN98FT9",
				"is_alias":       false,
				"set_identifier": "canary",
			},
			mustPopulate: []string{
				"fqdn", "record_type", "zone_id", "set_identifier",
			},
		},
		{
			// AwsRoute53HealthCheck: health_check_id is what DNS records
			// reference (health_check_id) and calculated parents aggregate;
			// it also keys the E2E verifier.
			name: "AwsRoute53HealthCheck",
			kind: cloudresourcekind.CloudResourceKind_AwsRoute53HealthCheck,
			rawOutputs: map[string]interface{}{
				"health_check_id":  "abcdef11-2222-3333-4444-555555fedcba",
				"health_check_arn": "arn:aws:route53:::healthcheck/abcdef11-2222-3333-4444-555555fedcba",
			},
			mustPopulate: []string{
				"health_check_id", "health_check_arn",
			},
		},
		{
			// AwsWafWebAcl: web_acl_arn is the FK target for every protected
			// resource (CloudFront's web_acl_arn, the ALB association);
			// capacity reports the deployed WCU total and
			// application_integration_url carries the CAPTCHA/Challenge JS
			// integration endpoint.
			name: "AwsWafWebAcl",
			kind: cloudresourcekind.CloudResourceKind_AwsWafWebAcl,
			rawOutputs: map[string]interface{}{
				"web_acl_arn":                 "arn:aws:wafv2:us-west-2:123456789012:regional/webacl/edge-acl/11111111-2222-3333-4444-555555555555",
				"web_acl_id":                  "11111111-2222-3333-4444-555555555555",
				"web_acl_name":                "edge-acl",
				"capacity":                    125,
				"application_integration_url": "https://11111111.us-west-2.captcha.awswaf.com/11111111/",
			},
			mustPopulate: []string{
				"web_acl_arn", "web_acl_id", "web_acl_name", "capacity", "application_integration_url",
			},
		},
		{
			// AwsWafIpSet: ip_set_arn is what web ACL ip_set_reference
			// statements point at; id + name address the set through the
			// WAFv2 API (and key the E2E verifier).
			name: "AwsWafIpSet",
			kind: cloudresourcekind.CloudResourceKind_AwsWafIpSet,
			rawOutputs: map[string]interface{}{
				"ip_set_arn":  "arn:aws:wafv2:us-west-2:123456789012:regional/ipset/office-allowlist/66666666-7777-8888-9999-000000000000",
				"ip_set_id":   "66666666-7777-8888-9999-000000000000",
				"ip_set_name": "office-allowlist",
			},
			mustPopulate: []string{
				"ip_set_arn", "ip_set_id", "ip_set_name",
			},
		},
		{
			// AwsWafRegexPatternSet: regex_pattern_set_arn is what web ACL
			// regex_pattern_set_reference statements point at; id + name
			// address the set through the WAFv2 API (and key the E2E
			// verifier).
			name: "AwsWafRegexPatternSet",
			kind: cloudresourcekind.CloudResourceKind_AwsWafRegexPatternSet,
			rawOutputs: map[string]interface{}{
				"regex_pattern_set_arn":  "arn:aws:wafv2:us-west-2:123456789012:regional/regexpatternset/blocked-paths/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				"regex_pattern_set_id":   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				"regex_pattern_set_name": "blocked-paths",
			},
			mustPopulate: []string{
				"regex_pattern_set_arn", "regex_pattern_set_id", "regex_pattern_set_name",
			},
		},
		{
			// AwsBatchComputeEnvironment: compute_environment_arn is what job
			// queues reference in their compute_environment_order; the name
			// keys the E2E verifier and ecs_cluster_arn exposes the ECS
			// cluster Batch runs tasks on.
			name: "AwsBatchComputeEnvironment",
			kind: cloudresourcekind.CloudResourceKind_AwsBatchComputeEnvironment,
			rawOutputs: map[string]interface{}{
				"compute_environment_arn":  "arn:aws:batch:us-west-2:123456789012:compute-environment/etl-fargate",
				"compute_environment_name": "etl-fargate",
				"ecs_cluster_arn":          "arn:aws:ecs:us-west-2:123456789012:cluster/AWSBatch-etl-fargate-11111111-2222-3333-4444-555555555555",
				"status":                   "VALID",
			},
			mustPopulate: []string{
				"compute_environment_arn", "compute_environment_name", "ecs_cluster_arn", "status",
			},
		},
		{
			// AwsBatchJobQueue: job_queue_arn is the submission handle and
			// the target EventBridge Batch targets point at; the name keys
			// the E2E verifier and name-addressed SubmitJob calls.
			name: "AwsBatchJobQueue",
			kind: cloudresourcekind.CloudResourceKind_AwsBatchJobQueue,
			rawOutputs: map[string]interface{}{
				"job_queue_arn":  "arn:aws:batch:us-west-2:123456789012:job-queue/etl-queue",
				"job_queue_name": "etl-queue",
			},
			mustPopulate: []string{
				"job_queue_arn", "job_queue_name",
			},
		},
		{
			// AwsBatchSchedulingPolicy: scheduling_policy_arn is what job
			// queues reference through their scheduling_policy field.
			name: "AwsBatchSchedulingPolicy",
			kind: cloudresourcekind.CloudResourceKind_AwsBatchSchedulingPolicy,
			rawOutputs: map[string]interface{}{
				"scheduling_policy_arn":  "arn:aws:batch:us-west-2:123456789012:scheduling-policy/fair-share",
				"scheduling_policy_name": "fair-share",
			},
			mustPopulate: []string{
				"scheduling_policy_arn", "scheduling_policy_name",
			},
		},
		{
			// AwsBatchJobDefinition: the revision-carrying job_definition_arn
			// is what EventBridge Batch targets reference (a new revision
			// rolls the rule); arn_without_revision serves latest-ACTIVE
			// consumers and revision is int64-typed.
			name: "AwsBatchJobDefinition",
			kind: cloudresourcekind.CloudResourceKind_AwsBatchJobDefinition,
			rawOutputs: map[string]interface{}{
				"job_definition_arn":   "arn:aws:batch:us-west-2:123456789012:job-definition/etl:7",
				"arn_without_revision": "arn:aws:batch:us-west-2:123456789012:job-definition/etl",
				"job_definition_name":  "etl",
				"revision":             7,
			},
			mustPopulate: []string{
				"job_definition_arn", "arn_without_revision", "job_definition_name", "revision",
			},
		},
		{
			// AwsCloudwatchLogGroup: log_group_arn is the FK target for Step
			// Functions logging, Route 53 query logging, API Gateway access
			// logs, and OpenSearch log publishing; log_group_name is the join
			// key for name-addressed consumers (ECS awslogs, ElastiCache) and
			// the E2E verifier.
			name: "AwsCloudwatchLogGroup",
			kind: cloudresourcekind.CloudResourceKind_AwsCloudwatchLogGroup,
			rawOutputs: map[string]interface{}{
				"log_group_arn":  "arn:aws:logs:us-west-2:123456789012:log-group:app-logs",
				"log_group_name": "app-logs",
			},
			mustPopulate: []string{
				"log_group_arn", "log_group_name",
			},
		},
		{
			// AwsCloudwatchAlarm: alarm_arn is referenced by ECS service
			// rollback alarms and ASG instance-refresh alarms; alarm_name is
			// the join key composite alarm rules and actions suppressors use,
			// and keys the E2E verifier.
			name: "AwsCloudwatchAlarm",
			kind: cloudresourcekind.CloudResourceKind_AwsCloudwatchAlarm,
			rawOutputs: map[string]interface{}{
				"alarm_arn":  "arn:aws:cloudwatch:us-west-2:123456789012:alarm:cpu-high",
				"alarm_name": "cpu-high",
			},
			mustPopulate: []string{
				"alarm_arn", "alarm_name",
			},
		},
		{
			// AwsCloudwatchCompositeAlarm: alarm_name is how parent composite
			// alarms reference this one inside their own rule expressions;
			// it also keys the E2E verifier.
			name: "AwsCloudwatchCompositeAlarm",
			kind: cloudresourcekind.CloudResourceKind_AwsCloudwatchCompositeAlarm,
			rawOutputs: map[string]interface{}{
				"alarm_arn":  "arn:aws:cloudwatch:us-west-2:123456789012:alarm:shared-cause",
				"alarm_name": "shared-cause",
			},
			mustPopulate: []string{
				"alarm_arn", "alarm_name",
			},
		},
		{
			// AwsStepFunction: state_machine_arn is the FK target for
			// EventBridge targets and API Gateway service integrations;
			// state_machine_version_arn pins consumers to a published
			// snapshot when spec.publish is set. The name keys the E2E
			// verifier.
			name: "AwsStepFunction",
			kind: cloudresourcekind.CloudResourceKind_AwsStepFunction,
			rawOutputs: map[string]interface{}{
				"state_machine_arn":         "arn:aws:states:us-west-2:123456789012:stateMachine:orders",
				"state_machine_name":        "orders",
				"state_machine_version_arn": "arn:aws:states:us-west-2:123456789012:stateMachine:orders:1",
				"revision_id":               "aaaa1111-bbbb-2222-cccc-333344445555",
				"status":                    "ACTIVE",
				"creation_date":             "2026-07-07T00:00:00Z",
			},
			mustPopulate: []string{
				"state_machine_arn", "state_machine_name",
				"state_machine_version_arn", "revision_id", "status", "creation_date",
			},
		},
		{
			// AwsHttpApiGateway: api_id is the join key domain mappings
			// reference; execution_arn feeds Lambda resource policies;
			// stage_name composes into domain mappings.
			name: "AwsHttpApiGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsHttpApiGateway,
			rawOutputs: map[string]interface{}{
				"api_id":           "a1b2c3d4",
				"api_endpoint":     "https://a1b2c3d4.execute-api.us-west-2.amazonaws.com",
				"api_arn":          "arn:aws:apigateway:us-west-2::/apis/a1b2c3d4",
				"execution_arn":    "arn:aws:execute-api:us-west-2:123456789012:a1b2c3d4",
				"stage_invoke_url": "https://a1b2c3d4.execute-api.us-west-2.amazonaws.com",
				"stage_name":       "$default",
			},
			mustPopulate: []string{
				"api_id", "api_endpoint", "api_arn",
				"execution_arn", "stage_invoke_url", "stage_name",
			},
		},
		{
			// AwsHttpApiVpcLink: vpc_link_id is what private integrations set
			// as connection_id; it also keys the E2E verifier.
			name: "AwsHttpApiVpcLink",
			kind: cloudresourcekind.CloudResourceKind_AwsHttpApiVpcLink,
			rawOutputs: map[string]interface{}{
				"vpc_link_id":  "abc123",
				"vpc_link_arn": "arn:aws:apigateway:us-west-2::/vpclinks/abc123",
			},
			mustPopulate: []string{"vpc_link_id", "vpc_link_arn"},
		},
		{
			// AwsHttpApiDomain: target_domain_name + hosted_zone_id are the
			// DNS composition surface (a Route 53 alias record targets them);
			// domain_name is the domain's join key and keys the E2E verifier.
			name: "AwsHttpApiDomain",
			kind: cloudresourcekind.CloudResourceKind_AwsHttpApiDomain,
			rawOutputs: map[string]interface{}{
				"domain_name":        "api.example.com",
				"domain_name_arn":    "arn:aws:apigateway:us-west-2::/domainnames/api.example.com",
				"target_domain_name": "d-abc123.execute-api.us-west-2.amazonaws.com",
				"hosted_zone_id":     "Z2OJLYMUO9EFXC",
			},
			mustPopulate: []string{
				"domain_name", "domain_name_arn", "target_domain_name", "hosted_zone_id",
			},
		},
		{
			// AwsCognitoUserPool: issuer is the JWT-authorizer join key (the
			// scheme-carrying spelling of user_pool_endpoint); user_pool_domain
			// is the RAW domain string ALB authenticate-cognito actions take;
			// the CloudFront trio composes a custom domain's DNS alias record.
			name: "AwsCognitoUserPool",
			kind: cloudresourcekind.CloudResourceKind_AwsCognitoUserPool,
			rawOutputs: map[string]interface{}{
				"user_pool_id":                "us-west-2_Ab1Cd2EfG",
				"user_pool_arn":               "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_Ab1Cd2EfG",
				"user_pool_endpoint":          "cognito-idp.us-west-2.amazonaws.com/us-west-2_Ab1Cd2EfG",
				"issuer":                      "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Ab1Cd2EfG",
				"user_pool_domain":            "myapp-auth",
				"hosted_ui_url":               "https://myapp-auth.auth.us-west-2.amazoncognito.com",
				"cloudfront_distribution":     "d111abcdef8.cloudfront.net",
				"cloudfront_distribution_arn": "arn:aws:cloudfront::123456789012:distribution/E1ABCDEF",
				"cloudfront_hosted_zone_id":   "Z2FDTNDATAQYW2",
			},
			mustPopulate: []string{
				"user_pool_id", "user_pool_arn", "user_pool_endpoint", "issuer",
				"user_pool_domain", "hosted_ui_url",
				"cloudfront_distribution", "cloudfront_distribution_arn", "cloudfront_hosted_zone_id",
			},
		},
		{
			// AwsCognitoIdentityProvider: provider_name is the sole
			// integration identifier (IdPs have no ARN) -- app clients list it
			// in supported_identity_providers.
			name: "AwsCognitoIdentityProvider",
			kind: cloudresourcekind.CloudResourceKind_AwsCognitoIdentityProvider,
			rawOutputs: map[string]interface{}{
				"provider_name": "Google",
				"provider_type": "Google",
				"user_pool_id":  "us-west-2_Ab1Cd2EfG",
			},
			mustPopulate: []string{"provider_name", "provider_type", "user_pool_id"},
		},
		{
			// AwsCognitoUserPoolClient: client_id is the join key JWT
			// authorizers list as an audience and ALB authenticate-cognito
			// actions take as user_pool_client_id; the secret only exists for
			// confidential clients.
			name: "AwsCognitoUserPoolClient",
			kind: cloudresourcekind.CloudResourceKind_AwsCognitoUserPoolClient,
			rawOutputs: map[string]interface{}{
				"client_id":     "1a2b3c4d5e6f7g8h9i0j",
				"client_secret": "shhh-not-a-real-secret",
				"user_pool_id":  "us-west-2_Ab1Cd2EfG",
			},
			mustPopulate: []string{"client_id", "client_secret", "user_pool_id"},
		},
		{
			// AwsCognitoResourceServer: scope_identifiers are the exact
			// strings app clients list in allowed_oauth_scopes; the identifier
			// keys the E2E verifier within its pool.
			name: "AwsCognitoResourceServer",
			kind: cloudresourcekind.CloudResourceKind_AwsCognitoResourceServer,
			rawOutputs: map[string]interface{}{
				"resource_server_identifier": "https://api.example.com",
				"scope_identifiers":          []interface{}{"https://api.example.com/read", "https://api.example.com/orders:write"},
				"user_pool_id":               "us-west-2_Ab1Cd2EfG",
			},
			mustPopulate: []string{"resource_server_identifier", "scope_identifiers", "user_pool_id"},
		},
		{
			// AwsElasticFileSystem: both engines emit the file system identity,
			// the regional DNS name, the four per-subnet mount-target maps
			// (keyed by resolved subnet ID), and the replication destination
			// (empty string when replication is not configured — the shape must
			// stay stable across the arms).
			name: "AwsElasticFileSystem",
			kind: cloudresourcekind.CloudResourceKind_AwsElasticFileSystem,
			rawOutputs: map[string]interface{}{
				"file_system_id":  "fs-0123456789abcdef0",
				"file_system_arn": "arn:aws:elasticfilesystem:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
				"dns_name":        "fs-0123456789abcdef0.efs.us-west-2.amazonaws.com",
				"mount_target_ids": map[string]interface{}{
					"subnet-0aaa": "fsmt-0123456789abcdef0",
					"subnet-0bbb": "fsmt-0123456789abcdef1",
				},
				"mount_target_ips": map[string]interface{}{
					"subnet-0aaa": "10.0.1.50",
					"subnet-0bbb": "10.0.2.51",
				},
				"mount_target_ipv6_addresses": map[string]interface{}{
					"subnet-0aaa": "",
					"subnet-0bbb": "",
				},
				"mount_target_dns_names": map[string]interface{}{
					"subnet-0aaa": "us-west-2a.fs-0123456789abcdef0.efs.us-west-2.amazonaws.com",
					"subnet-0bbb": "us-west-2b.fs-0123456789abcdef0.efs.us-west-2.amazonaws.com",
				},
				"replication_destination_file_system_id": "",
			},
			mustPopulate: []string{"file_system_id", "file_system_arn", "dns_name", "mount_target_ids", "mount_target_ips", "mount_target_dns_names"},
		},
		{
			// AwsEfsAccessPoint: both engines emit the access point identity
			// (Lambda takes the ARN, ECS volume authorization the ID) plus the
			// file system it enters, so consumers can wire everything from
			// this one node.
			name: "AwsEfsAccessPoint",
			kind: cloudresourcekind.CloudResourceKind_AwsEfsAccessPoint,
			rawOutputs: map[string]interface{}{
				"access_point_id":  "fsap-0123456789abcdef0",
				"access_point_arn": "arn:aws:elasticfilesystem:us-west-2:123456789012:access-point/fsap-0123456789abcdef0",
				"file_system_id":   "fs-0123456789abcdef0",
				"file_system_arn":  "arn:aws:elasticfilesystem:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
			},
			mustPopulate: []string{"access_point_id", "access_point_arn", "file_system_id", "file_system_arn"},
		},
		{
			// AwsSesConfigurationSet: configuration_set_name is the join key
			// email identities reference through their configuration_set field
			// (and what SendEmail calls name); the ARN scopes IAM sending
			// policies. The name also keys the E2E verifier.
			name: "AwsSesConfigurationSet",
			kind: cloudresourcekind.CloudResourceKind_AwsSesConfigurationSet,
			rawOutputs: map[string]interface{}{
				"configuration_set_arn":  "arn:aws:ses:us-west-2:123456789012:configuration-set/transactional-set",
				"configuration_set_name": "transactional-set",
			},
			mustPopulate: []string{"configuration_set_arn", "configuration_set_name"},
		},
		{
			// AwsSesEmailIdentity: the identity string keys the E2E verifier
			// and composes DNS record names; dkim_tokens is the repeated
			// output downstream Route53 CNAME records are built from -- it
			// must land on the proto's repeated field, not silently drop.
			name: "AwsSesEmailIdentity",
			kind: cloudresourcekind.CloudResourceKind_AwsSesEmailIdentity,
			rawOutputs: map[string]interface{}{
				"identity_arn":        "arn:aws:ses:us-west-2:123456789012:identity/example.com",
				"email_identity":      "example.com",
				"identity_type":       "DOMAIN",
				"verification_status": "PENDING",
				"dkim_tokens": []interface{}{
					"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					"cccccccccccccccccccccccccccccccc",
				},
			},
			mustPopulate: []string{"identity_arn", "email_identity", "identity_type", "verification_status", "dkim_tokens"},
		},
		{
			// AwsAppRunnerService: service_arn is the join key (VPC ingress
			// connections, deployment triggers, WAF association); service_url
			// is the endpoint consumers point DNS at; custom_domains guards
			// the two-level repeated-message shape (per-domain DNS target +
			// certificate-validation records) Route53 compositions consume.
			name: "AwsAppRunnerService",
			kind: cloudresourcekind.CloudResourceKind_AwsAppRunnerService,
			rawOutputs: map[string]interface{}{
				"service_arn":    "arn:aws:apprunner:us-west-2:123456789012:service/my-api/abc123",
				"service_id":     "abc123",
				"service_url":    "abc123.us-west-2.awsapprunner.com",
				"service_name":   "my-api",
				"service_status": "RUNNING",
				"custom_domains": []interface{}{
					map[string]interface{}{
						"domain_name": "app.example.com",
						"dns_target":  "abc123.us-west-2.awsapprunner.com",
						"status":      "pending_certificate_dns_validation",
						"certificate_validation_records": []interface{}{
							map[string]interface{}{
								"record_name":  "_abc123.app.example.com.",
								"record_type":  "CNAME",
								"record_value": "_def456.acm-validations.aws.",
							},
						},
					},
				},
			},
			mustPopulate: []string{
				"service_arn", "service_id", "service_url", "service_name",
				"service_status", "custom_domains",
			},
		},
		{
			// AwsAppRunnerAutoScalingConfiguration: the revision-carrying ARN
			// is what services reference (and what rolls them when a new
			// revision registers); it also keys the E2E verifier.
			name: "AwsAppRunnerAutoScalingConfiguration",
			kind: cloudresourcekind.CloudResourceKind_AwsAppRunnerAutoScalingConfiguration,
			rawOutputs: map[string]interface{}{
				"configuration_arn":      "arn:aws:apprunner:us-west-2:123456789012:autoscalingconfiguration/my-asc/3/abc123",
				"configuration_revision": 3,
				"latest":                 true,
			},
			mustPopulate: []string{"configuration_arn", "configuration_revision", "latest"},
		},
		{
			// AwsAppRunnerVpcConnector: the ARN is the egress join key App
			// Runner services reference; it also keys the E2E verifier.
			name: "AwsAppRunnerVpcConnector",
			kind: cloudresourcekind.CloudResourceKind_AwsAppRunnerVpcConnector,
			rawOutputs: map[string]interface{}{
				"vpc_connector_arn":      "arn:aws:apprunner:us-west-2:123456789012:vpcconnector/my-vc/1/abc123",
				"vpc_connector_revision": 1,
				"status":                 "ACTIVE",
			},
			mustPopulate: []string{"vpc_connector_arn", "vpc_connector_revision", "status"},
		},
		{
			// AwsAppRunnerObservabilityConfiguration: the revision-carrying
			// ARN is what services reference to enable tracing; it also keys
			// the E2E verifier.
			name: "AwsAppRunnerObservabilityConfiguration",
			kind: cloudresourcekind.CloudResourceKind_AwsAppRunnerObservabilityConfiguration,
			rawOutputs: map[string]interface{}{
				"configuration_arn":      "arn:aws:apprunner:us-west-2:123456789012:observabilityconfiguration/my-oc/2/abc123",
				"configuration_revision": 2,
				"latest":                 true,
			},
			mustPopulate: []string{"configuration_arn", "configuration_revision", "latest"},
		},
		{
			// AwsTransitGateway: the gateway ID is the join key attachments,
			// route tables, and subnet routes reference; the default route
			// table pair lets tooling address the built-in tables.
			name: "AwsTransitGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsTransitGateway,
			rawOutputs: map[string]interface{}{
				"transit_gateway_id":                 "tgw-0123456789abcdef0",
				"transit_gateway_arn":                "arn:aws:ec2:us-west-2:123456789012:transit-gateway/tgw-0123456789abcdef0",
				"owner_id":                           "123456789012",
				"association_default_route_table_id": "tgw-rtb-0123456789abcdef0",
				"propagation_default_route_table_id": "tgw-rtb-0123456789abcdef0",
			},
			mustPopulate: []string{"transit_gateway_id", "transit_gateway_arn", "owner_id", "association_default_route_table_id", "propagation_default_route_table_id"},
		},
		{
			// AwsTransitGatewayVpcAttachment: the attachment ID is the join
			// key route tables associate, propagate, and route against.
			name: "AwsTransitGatewayVpcAttachment",
			kind: cloudresourcekind.CloudResourceKind_AwsTransitGatewayVpcAttachment,
			rawOutputs: map[string]interface{}{
				"attachment_id":  "tgw-attach-0123456789abcdef0",
				"attachment_arn": "arn:aws:ec2:us-west-2:123456789012:transit-gateway-attachment/tgw-attach-0123456789abcdef0",
				"vpc_owner_id":   "123456789012",
			},
			mustPopulate: []string{"attachment_id", "attachment_arn", "vpc_owner_id"},
		},
		{
			// AwsTransitGatewayRouteTable: the table ID keys the E2E verifier
			// and any route-management tooling.
			name: "AwsTransitGatewayRouteTable",
			kind: cloudresourcekind.CloudResourceKind_AwsTransitGatewayRouteTable,
			rawOutputs: map[string]interface{}{
				"route_table_id":  "tgw-rtb-0123456789abcdef0",
				"route_table_arn": "arn:aws:ec2:us-west-2:123456789012:transit-gateway-route-table/tgw-rtb-0123456789abcdef0",
			},
			mustPopulate: []string{"route_table_id", "route_table_arn"},
		},
		{
			// AwsAthenaWorkgroup: workgroup_name keys the E2E verifier and every
			// StartQueryExecution call; effective_engine_version reflects what AWS
			// resolved for an AUTO engine selection.
			name: "AwsAthenaWorkgroup",
			kind: cloudresourcekind.CloudResourceKind_AwsAthenaWorkgroup,
			rawOutputs: map[string]interface{}{
				"workgroup_arn":            "arn:aws:athena:us-west-2:123456789012:workgroup/analytics",
				"workgroup_name":           "analytics",
				"effective_engine_version": "Athena engine version 3",
			},
			mustPopulate: []string{"workgroup_arn", "workgroup_name", "effective_engine_version"},
		},
		{
			// AwsGlueCatalogDatabase: database_name keys the E2E verifier and is
			// the join key Athena queries, Glue crawlers, and Redshift Spectrum
			// external schemas reference.
			name: "AwsGlueCatalogDatabase",
			kind: cloudresourcekind.CloudResourceKind_AwsGlueCatalogDatabase,
			rawOutputs: map[string]interface{}{
				"database_name": "sales_lake",
				"database_arn":  "arn:aws:glue:us-west-2:123456789012:database/sales_lake",
				"catalog_id":    "123456789012",
			},
			mustPopulate: []string{"database_name", "database_arn", "catalog_id"},
		},
		{
			// AwsCodeBuildProject: project_name keys the E2E verifier and
			// CodePipeline Build actions; the webhook trio (incl. the
			// provider-minted secret) is exported empty when no webhook exists,
			// so the output SHAPE is engine- and configuration-invariant.
			name: "AwsCodeBuildProject",
			kind: cloudresourcekind.CloudResourceKind_AwsCodeBuildProject,
			rawOutputs: map[string]interface{}{
				"project_arn":          "arn:aws:codebuild:us-west-2:123456789012:project/api-ci",
				"project_name":         "api-ci",
				"service_role_arn":     "arn:aws:iam::123456789012:role/codebuild-service-role",
				"badge_url":            "https://codebuild.us-west-2.amazonaws.com/badges?uuid=abc",
				"public_project_alias": "api-ci-public",
				"webhook_url":          "https://github.com/example/repo/settings/hooks/1",
				"webhook_payload_url":  "https://codebuild.us-west-2.amazonaws.com/webhooks?t=abc",
				"webhook_secret":       "0123456789abcdef",
			},
			mustPopulate: []string{
				"project_arn", "project_name", "service_role_arn", "badge_url",
				"public_project_alias", "webhook_url", "webhook_payload_url", "webhook_secret",
			},
		},
		{
			// AwsCodePipeline: pipeline_name keys the E2E verifier and CLI
			// operations; pipeline_arn is what IAM policies and EventBridge
			// targets reference.
			name: "AwsCodePipeline",
			kind: cloudresourcekind.CloudResourceKind_AwsCodePipeline,
			rawOutputs: map[string]interface{}{
				"pipeline_arn":  "arn:aws:codepipeline:us-west-2:123456789012:release-pipeline",
				"pipeline_name": "release-pipeline",
			},
			mustPopulate: []string{"pipeline_arn", "pipeline_name"},
		},
		{
			// AwsMemorydbCluster: cluster_name keys the E2E verifier;
			// cluster_endpoint_address/port are what applications connect to;
			// the group-name pair is exported in EVERY arm (module-managed,
			// bring-your-own, or empty when a default applies) so the output
			// shape is engine- and configuration-invariant.
			name: "AwsMemorydbCluster",
			kind: cloudresourcekind.CloudResourceKind_AwsMemorydbCluster,
			rawOutputs: map[string]interface{}{
				"cluster_endpoint_address": "clustercfg.sessions.abc123.memorydb.us-west-2.amazonaws.com",
				"cluster_endpoint_port":    6379,
				"cluster_arn":              "arn:aws:memorydb:us-west-2:123456789012:cluster/sessions",
				"cluster_name":             "sessions",
				"engine_patch_version":     "7.2.6",
				"subnet_group_name":        "sessions",
				"parameter_group_name":     "",
			},
			mustPopulate: []string{
				"cluster_endpoint_address", "cluster_endpoint_port", "cluster_arn",
				"cluster_name", "engine_patch_version", "subnet_group_name",
			},
		},
		{
			// AwsMemorydbUser: user_name is the join key ACL membership lists
			// reference; user_arn is what IAM memorydb:Connect policies need.
			name: "AwsMemorydbUser",
			kind: cloudresourcekind.CloudResourceKind_AwsMemorydbUser,
			rawOutputs: map[string]interface{}{
				"user_name":              "orders-service",
				"user_arn":               "arn:aws:memorydb:us-west-2:123456789012:user/orders-service",
				"minimum_engine_version": "7.2",
			},
			mustPopulate: []string{"user_name", "user_arn", "minimum_engine_version"},
		},
		{
			// AwsMemorydbAcl: acl_name is the join key the cluster's acl_name
			// reference resolves against.
			name: "AwsMemorydbAcl",
			kind: cloudresourcekind.CloudResourceKind_AwsMemorydbAcl,
			rawOutputs: map[string]interface{}{
				"acl_name":               "payments-env-acl",
				"acl_arn":                "arn:aws:memorydb:us-west-2:123456789012:acl/payments-env-acl",
				"minimum_engine_version": "7.2",
			},
			mustPopulate: []string{"acl_name", "acl_arn", "minimum_engine_version"},
		},
		{
			// AwsClientVpn: the endpoint ID keys the E2E verifier and the
			// client-configuration export; subnet_association_ids proves the
			// dot-flattened map route into the proto map field; the TGW
			// attachment ID is exported empty for VPC-attached endpoints so
			// the output shape is configuration-invariant.
			name: "AwsClientVpn",
			kind: cloudresourcekind.CloudResourceKind_AwsClientVpn,
			rawOutputs: map[string]interface{}{
				"client_vpn_endpoint_id":               "cvpn-endpoint-0123456789abcdef0",
				"client_vpn_endpoint_arn":              "arn:aws:ec2:us-west-2:123456789012:client-vpn-endpoint/cvpn-endpoint-0123456789abcdef0",
				"endpoint_dns_name":                    "cvpn-endpoint-0123456789abcdef0.prod.clientvpn.us-west-2.amazonaws.com",
				"self_service_portal_url":              "",
				"subnet_association_ids.subnet-aaa111": "cvpn-assoc-0aaa",
				"subnet_association_ids.subnet-bbb222": "cvpn-assoc-0bbb",
				"transit_gateway_attachment_id":        "",
			},
			mustPopulate: []string{
				"client_vpn_endpoint_id", "client_vpn_endpoint_arn", "endpoint_dns_name",
				"subnet_association_ids",
			},
		},
		{
			// AwsGlobalAccelerator: the accelerator ARN keys the E2E verifier;
			// the repeated accelerator_ip_addresses guards the static-anycast
			// export both engines flatten from ip_sets (an engine that exports
			// an empty list fails here); the dot-flattened listener/endpoint
			// group maps prove the name-keyed ARN routes into the proto map
			// fields, including the "listener/group" composite keys surviving
			// per-segment field lookup. dual_stack_dns_name is exported empty
			// for IPV4 accelerators so the output shape is
			// configuration-invariant.
			name: "AwsGlobalAccelerator",
			kind: cloudresourcekind.CloudResourceKind_AwsGlobalAccelerator,
			rawOutputs: map[string]interface{}{
				"accelerator_arn":                 "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				"accelerator_dns_name":            "a1234567890abcdef.awsglobalaccelerator.com",
				"accelerator_dual_stack_dns_name": "",
				"accelerator_hosted_zone_id":      "Z2BJ6XQ5FK7U4H",
				"accelerator_ip_addresses":        []interface{}{"75.2.0.1", "99.83.0.1"},
				"listener_arns.web":               "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/0123vxyz",
				"endpoint_group_arns.web/primary": "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/0123vxyz/endpoint-group/098765zyxwvu",
			},
			mustPopulate: []string{
				"accelerator_arn", "accelerator_dns_name", "accelerator_hosted_zone_id",
				"accelerator_ip_addresses", "listener_arns", "endpoint_group_arns",
			},
		},
		{
			// AwsFsxLustreFileSystem: file_system_id keys the E2E verifier;
			// dns_name + mount_name compose the Lustre mount command; the
			// repeated network_interface_ids guard the ENI list export both
			// engines emit.
			name: "AwsFsxLustreFileSystem",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxLustreFileSystem,
			rawOutputs: map[string]interface{}{
				"file_system_id":           "fs-0123456789abcdef0",
				"file_system_arn":          "arn:aws:fsx:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
				"dns_name":                 "fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"mount_name":               "2p5wpbwj",
				"network_interface_ids":    []interface{}{"eni-0abc123", "eni-0def456"},
				"vpc_id":                   "vpc-0abc123",
				"file_system_type_version": "2.15",
				"owner_id":                 "123456789012",
			},
			mustPopulate: []string{
				"file_system_id", "file_system_arn", "dns_name", "mount_name",
				"network_interface_ids", "vpc_id", "file_system_type_version", "owner_id",
			},
		},
		{
			// AwsFsxOpenzfsFileSystem: file_system_id keys the E2E verifier;
			// root_volume_id is the join key for child volumes;
			// endpoint_ip_address carries the (floating, for MULTI_AZ) NFS
			// endpoint.
			name: "AwsFsxOpenzfsFileSystem",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxOpenzfsFileSystem,
			rawOutputs: map[string]interface{}{
				"file_system_id":        "fs-0123456789abcdef0",
				"file_system_arn":       "arn:aws:fsx:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
				"dns_name":              "fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"endpoint_ip_address":   "10.0.1.25",
				"root_volume_id":        "fsvol-0123456789abcdef0",
				"network_interface_ids": []interface{}{"eni-0abc123"},
				"vpc_id":                "vpc-0abc123",
				"owner_id":              "123456789012",
			},
			mustPopulate: []string{
				"file_system_id", "file_system_arn", "dns_name", "endpoint_ip_address",
				"root_volume_id", "network_interface_ids", "vpc_id", "owner_id",
			},
		},
		{
			// AwsFsxWindowsFileSystem: file_system_id keys the E2E verifier;
			// preferred_file_server_ip + remote_administration_endpoint carry
			// the SMB/PowerShell endpoints.
			name: "AwsFsxWindowsFileSystem",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxWindowsFileSystem,
			rawOutputs: map[string]interface{}{
				"file_system_id":                 "fs-0123456789abcdef0",
				"file_system_arn":                "arn:aws:fsx:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
				"dns_name":                       "fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"preferred_file_server_ip":       "10.0.1.30",
				"remote_administration_endpoint": "fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"network_interface_ids":          []interface{}{"eni-0abc123"},
				"vpc_id":                         "vpc-0abc123",
				"owner_id":                       "123456789012",
			},
			mustPopulate: []string{
				"file_system_id", "file_system_arn", "dns_name", "preferred_file_server_ip",
				"remote_administration_endpoint", "network_interface_ids", "vpc_id", "owner_id",
			},
		},
		{
			// AwsFsxDataRepositoryAssociation: association_id keys the E2E
			// verifier and FSx data repository tasks; file_system_id is
			// echoed for composition.
			name: "AwsFsxDataRepositoryAssociation",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxDataRepositoryAssociation,
			rawOutputs: map[string]interface{}{
				"association_id":  "dra-0123456789abcdef0",
				"association_arn": "arn:aws:fsx:us-west-2:123456789012:association/fs-0123456789abcdef0/dra-0123456789abcdef0",
				"file_system_id":  "fs-0123456789abcdef0",
			},
			mustPopulate: []string{
				"association_id", "association_arn", "file_system_id",
			},
		},
		{
			// AwsFsxOntapFileSystem: file_system_id keys the E2E verifier and
			// is the SVM join key; the management/intercluster endpoint pairs
			// carry the ONTAP CLI and SnapMirror endpoints (there is NO
			// file-system-level data dns_name for ONTAP — data access is via
			// SVM endpoints); the repeated lists guard the multi-value
			// exports both engines emit.
			name: "AwsFsxOntapFileSystem",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxOntapFileSystem,
			rawOutputs: map[string]interface{}{
				"file_system_id":            "fs-0123456789abcdef0",
				"file_system_arn":           "arn:aws:fsx:us-west-2:123456789012:file-system/fs-0123456789abcdef0",
				"management_dns_name":       "management.fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"management_ip_addresses":   []interface{}{"198.19.255.10"},
				"intercluster_dns_name":     "intercluster.fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"intercluster_ip_addresses": []interface{}{"198.19.255.11", "198.19.255.12"},
				"network_interface_ids":     []interface{}{"eni-0abc123", "eni-0def456"},
				"vpc_id":                    "vpc-0abc123",
				"owner_id":                  "123456789012",
			},
			mustPopulate: []string{
				"file_system_id", "file_system_arn", "management_dns_name",
				"management_ip_addresses", "intercluster_dns_name",
				"intercluster_ip_addresses", "network_interface_ids", "vpc_id", "owner_id",
			},
		},
		{
			// AwsFsxOntapStorageVirtualMachine: svm_id keys the E2E verifier
			// and is the volume join key; the per-protocol endpoint pairs
			// (iscsi/management/nfs/smb) carry the data-access surface — smb
			// is exported empty when no Active Directory is configured so the
			// output shape stays configuration-invariant.
			name: "AwsFsxOntapStorageVirtualMachine",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxOntapStorageVirtualMachine,
			rawOutputs: map[string]interface{}{
				"svm_id":                  "svm-0123456789abcdef0",
				"arn":                     "arn:aws:fsx:us-west-2:123456789012:storage-virtual-machine/fs-0123456789abcdef0/svm-0123456789abcdef0",
				"uuid":                    "abcdef01-2345-6789-abcd-ef0123456789",
				"subtype":                 "DEFAULT",
				"iscsi_dns_name":          "iscsi.svm-0123456789abcdef0.fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"iscsi_ip_addresses":      []interface{}{"10.0.1.40"},
				"management_dns_name":     "svm-0123456789abcdef0.fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"management_ip_addresses": []interface{}{"10.0.1.41"},
				"nfs_dns_name":            "svm-0123456789abcdef0.fs-0123456789abcdef0.fsx.us-west-2.amazonaws.com",
				"nfs_ip_addresses":        []interface{}{"10.0.1.41"},
				"smb_dns_name":            "",
				"smb_ip_addresses":        []interface{}{},
			},
			mustPopulate: []string{
				"svm_id", "arn", "uuid", "subtype", "iscsi_dns_name",
				"iscsi_ip_addresses", "management_dns_name", "management_ip_addresses",
				"nfs_dns_name", "nfs_ip_addresses",
			},
		},
		{
			// AwsFsxOntapVolume: volume_id keys the E2E verifier; uuid and
			// the echoed file_system_id support ONTAP CLI operations and
			// composition; flexcache_endpoint_type and ontap_volume_type are
			// AWS-computed classifications.
			name: "AwsFsxOntapVolume",
			kind: cloudresourcekind.CloudResourceKind_AwsFsxOntapVolume,
			rawOutputs: map[string]interface{}{
				"volume_id":               "fsvol-0123456789abcdef0",
				"arn":                     "arn:aws:fsx:us-west-2:123456789012:volume/fs-0123456789abcdef0/fsvol-0123456789abcdef0",
				"uuid":                    "01234567-89ab-cdef-0123-456789abcdef",
				"file_system_id":          "fs-0123456789abcdef0",
				"flexcache_endpoint_type": "NONE",
				"ontap_volume_type":       "RW",
			},
			mustPopulate: []string{
				"volume_id", "arn", "uuid", "file_system_id",
				"flexcache_endpoint_type", "ontap_volume_type",
			},
		},
		{
			// AwsSagemakerDomain: domain_id keys the E2E verifier and is the
			// join key user profiles and spaces reference; the two SSO outputs
			// are empty strings under IAM auth so the output SHAPE is
			// auth-mode-invariant.
			name: "AwsSagemakerDomain",
			kind: cloudresourcekind.CloudResourceKind_AwsSagemakerDomain,
			rawOutputs: map[string]interface{}{
				"domain_id":                             "d-0123456789ab",
				"domain_arn":                            "arn:aws:sagemaker:us-west-2:123456789012:domain/d-0123456789ab",
				"domain_url":                            "https://d-0123456789ab.studio.us-west-2.sagemaker.aws",
				"home_efs_file_system_id":               "fs-0123456789abcdef0",
				"security_group_id_for_domain_boundary": "sg-0123456789abcdef0",
				"single_sign_on_application_arn":        "arn:aws:sso::123456789012:application/ssoins-abc/apl-def",
				"single_sign_on_managed_application_instance_id": "app-ins-0123456789abcdef",
			},
			mustPopulate: []string{
				"domain_id", "domain_arn", "domain_url", "home_efs_file_system_id",
				"security_group_id_for_domain_boundary", "single_sign_on_application_arn",
				"single_sign_on_managed_application_instance_id",
			},
		},
		{
			// CloudflareR2Bucket: both engines emit the same outputs -- bucket name,
			// path-style S3 URL, the list of custom-domain URLs, and the managed
			// r2.dev public URL -- each of which must land on the StackOutputs proto.
			name: "CloudflareR2Bucket",
			kind: cloudresourcekind.CloudResourceKind_CloudflareR2Bucket,
			rawOutputs: map[string]interface{}{
				"bucket_name":        "media-assets",
				"bucket_url":         "https://00000000000000000000000000000000.r2.cloudflarestorage.com/media-assets",
				"custom_domain_urls": []interface{}{"https://media.example.com", "https://cdn.example.com"},
				"public_url":         "https://pub-0123456789abcdef.r2.dev",
			},
			mustPopulate: []string{"bucket_name", "bucket_url", "custom_domain_urls", "public_url"},
		},
		{
			// CloudflareD1Database: both engines emit the database id and name as
			// flat scalars (a Worker reaches D1 through its binding; no DSN exists).
			name: "CloudflareD1Database",
			kind: cloudresourcekind.CloudResourceKind_CloudflareD1Database,
			rawOutputs: map[string]interface{}{
				"database_id":   "9a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d",
				"database_name": "app-prod-db",
				"version":       "production",
			},
			mustPopulate: []string{"database_id", "database_name", "version"},
		},
		{
			// CloudflareKvNamespace: both engines emit the namespace id and the
			// url-encoding support flag.
			name: "CloudflareKvNamespace",
			kind: cloudresourcekind.CloudResourceKind_CloudflareKvNamespace,
			rawOutputs: map[string]interface{}{
				"namespace_id":          "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
				"supports_url_encoding": true,
			},
			mustPopulate: []string{"namespace_id", "supports_url_encoding"},
		},
		{
			// CloudflareWorkersKvPair: both engines emit the entry key and the
			// namespace it was written to.
			name: "CloudflareWorkersKvPair",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWorkersKvPair,
			rawOutputs: map[string]interface{}{
				"key_name":     "feature.new-dashboard",
				"namespace_id": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
			},
			mustPopulate: []string{"key_name", "namespace_id"},
		},
		{
			// CloudflareHyperdriveConfig: both engines emit the config id and name.
			name: "CloudflareHyperdriveConfig",
			kind: cloudresourcekind.CloudResourceKind_CloudflareHyperdriveConfig,
			rawOutputs: map[string]interface{}{
				"hyperdrive_id": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
				"name":          "app-prod-pg",
			},
			mustPopulate: []string{"hyperdrive_id", "name"},
		},
		{
			// CloudflareQueue: both engines emit the queue id and name (referenced by
			// consumers, worker producer bindings, and R2 event notifications).
			name: "CloudflareQueue",
			kind: cloudresourcekind.CloudResourceKind_CloudflareQueue,
			rawOutputs: map[string]interface{}{
				"queue_id":    "a1b2c3d4e5f60718293a4b5c6d7e8f90",
				"queue_name":  "orders-queue",
				"created_on":  "2026-06-25T00:00:00Z",
				"modified_on": "2026-06-25T00:00:00Z",
			},
			mustPopulate: []string{"queue_id", "queue_name"},
		},
		{
			// CloudflarePagesProject: both engines emit the project name, its
			// pages.dev subdomain, attached custom domains, and creation time.
			name: "CloudflarePagesProject",
			kind: cloudresourcekind.CloudResourceKind_CloudflarePagesProject,
			rawOutputs: map[string]interface{}{
				"project_name": "marketing-site",
				"subdomain":    "marketing-site.pages.dev",
				"domains":      []interface{}{"www.example.com"},
				"created_on":   "2026-06-25T00:00:00Z",
			},
			mustPopulate: []string{"project_name", "subdomain"},
		},
		{
			// CloudflareDnsRecord: both engines emit the record id, name, type and
			// proxied flag as flat scalars onto the StackOutputs proto.
			name: "CloudflareDnsRecord",
			kind: cloudresourcekind.CloudResourceKind_CloudflareDnsRecord,
			rawOutputs: map[string]interface{}{
				"record_id":   "372e67954025e0ba6aaa6d586b9e0b59",
				"record_name": "www",
				"record_type": "A",
				"proxied":     true,
			},
			mustPopulate: []string{"record_id", "record_name", "record_type", "proxied"},
		},
		{
			// CloudflareDnsZone: both engines emit the zone id (scalar) and the
			// assigned nameservers (repeated string) onto the StackOutputs proto.
			name: "CloudflareDnsZone",
			kind: cloudresourcekind.CloudResourceKind_CloudflareDnsZone,
			rawOutputs: map[string]interface{}{
				"zone_id":                 "023e105f4ecef8ad9ca31a8372d0c353",
				"nameservers":             []interface{}{"ns1.cloudflare.com", "ns2.cloudflare.com"},
				"status":                  "active",
				"dnssec_status":           "active",
				"dnssec_ds":               "example.com. 3600 IN DS 2371 13 2 ABCDEF",
				"dnssec_digest":           "abcdef0123456789",
				"dnssec_digest_type":      "2",
				"dnssec_digest_algorithm": "SHA256",
				"dnssec_algorithm":        "13",
				"dnssec_key_tag":          "2371",
				"dnssec_public_key":       "mdsswUyr3DPW132mOi8V9xESWE8jTo0d",
				"dnssec_flags":            "257",
			},
			mustPopulate: []string{"zone_id", "nameservers", "status", "dnssec_ds", "dnssec_key_tag"},
		},
		{
			// CloudflareRuleset: both engines emit ruleset id, version, and the
			// zone_id/phase pass-throughs as flat scalars onto the proto.
			name: "CloudflareRuleset",
			kind: cloudresourcekind.CloudResourceKind_CloudflareRuleset,
			rawOutputs: map[string]interface{}{
				"ruleset_id": "2f2feab2026849078ba485f918791bdc",
				"version":    "3",
				"zone_id":    "023e105f4ecef8ad9ca31a8372d0c353",
				"phase":      "http_request_origin",
			},
			mustPopulate: []string{"ruleset_id", "version", "zone_id", "phase"},
		},
		{
			// CloudflareLoadBalancer: both engines emit the load balancer id,
			// hostname, and cname target as flat scalars onto the proto.
			name: "CloudflareLoadBalancer",
			kind: cloudresourcekind.CloudResourceKind_CloudflareLoadBalancer,
			rawOutputs: map[string]interface{}{
				"load_balancer_id":              "699d98642c564d2e855e9661899b7252",
				"load_balancer_dns_record_name": "lb.example.com",
				"load_balancer_cname_target":    "699d98642c564d2e855e9661899b7252",
			},
			mustPopulate: []string{"load_balancer_id", "load_balancer_dns_record_name", "load_balancer_cname_target"},
		},
		{
			// CloudflareLoadBalancerPool: both engines emit the pool id and name
			// (account-scoped pool referenced by load balancers).
			name: "CloudflareLoadBalancerPool",
			kind: cloudresourcekind.CloudResourceKind_CloudflareLoadBalancerPool,
			rawOutputs: map[string]interface{}{
				"pool_id":   "17b5962d775c646f3f9725cbc7a53df4",
				"pool_name": "web-pool",
			},
			mustPopulate: []string{"pool_id", "pool_name"},
		},
		{
			// CloudflareLoadBalancerMonitor: both engines emit the monitor id and
			// its protocol (account-scoped health check referenced by pools).
			name: "CloudflareLoadBalancerMonitor",
			kind: cloudresourcekind.CloudResourceKind_CloudflareLoadBalancerMonitor,
			rawOutputs: map[string]interface{}{
				"monitor_id":   "f1aba936b94213e5b8dca0c0dbf1f9cc",
				"monitor_type": "https",
			},
			mustPopulate: []string{"monitor_id", "monitor_type"},
		},
		{
			// CloudflareWorker: both engines emit the script id and name (scalars),
			// the custom-domain hostnames / route patterns (repeated strings), and
			// the keyed maps import needs for custom domains and routes.
			name: "CloudflareWorker",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWorker,
			rawOutputs: map[string]interface{}{
				"script_id":               "my-worker",
				"script_name":             "my-worker",
				"custom_domain_hostnames": []interface{}{"api.example.com"},
				"route_patterns":          []interface{}{"api.example.com/*"},
				"custom_domain_ids":       map[string]interface{}{"api.example.com": "dom-1"},
				"route_ids":               map[string]interface{}{"0": "route-1"},
				"route_zone_ids":          map[string]interface{}{"0": "023e105f4ecef8ad9ca31a8372d0c353"},
			},
			mustPopulate: []string{"script_id", "script_name", "custom_domain_hostnames", "route_patterns", "custom_domain_ids", "route_ids", "route_zone_ids"},
		},
		{
			// CloudflareZeroTrustAccessApplication: both engines emit the
			// application id, audience tag, protected domain, and SaaS material.
			name: "CloudflareZeroTrustAccessApplication",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessApplication,
			rawOutputs: map[string]interface{}{
				"application_id":     "f174e90a-fafe-4643-bbbc-4a0ed4fc8415",
				"aud":                "8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b",
				"domain":             "dashboard.example.com",
				"saas_client_id":     "client-abc",
				"saas_client_secret": "secret-xyz",
				"saas_public_key":    "MIIBIjANBgkqh...",
				"saas_sso_endpoint":  "https://example.cloudflareaccess.com/cdn-cgi/access/sso/saml/abc",
				"saas_idp_entity_id": "https://example.cloudflareaccess.com",
			},
			mustPopulate: []string{
				"application_id", "aud", "domain", "saas_client_id", "saas_client_secret",
				"saas_public_key", "saas_sso_endpoint", "saas_idp_entity_id",
			},
		},
		{
			// CloudflareZeroTrustAccessPolicy: both engines emit the policy id.
			name: "CloudflareZeroTrustAccessPolicy",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id": "699d98642c564d2e855e9661899b7252",
			},
			mustPopulate: []string{"policy_id"},
		},
		{
			// CloudflareZeroTrustAccessGroup: both engines emit the group id.
			name: "CloudflareZeroTrustAccessGroup",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessGroup,
			rawOutputs: map[string]interface{}{
				"group_id": "aa9d98642c564d2e855e9661899b7252",
			},
			mustPopulate: []string{"group_id"},
		},
		{
			// CloudflareZeroTrustTunnel: both engines emit flat scalar outputs --
			// tunnel id, CNAME target, the (sensitive) connector token, status, the
			// account tag, and the creation timestamp.
			name: "CloudflareZeroTrustTunnel",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustTunnel,
			rawOutputs: map[string]interface{}{
				"tunnel_id":     "f70ff985-a4ef-4643-bbbc-4a0ed4fc8415",
				"tunnel_cname":  "f70ff985-a4ef-4643-bbbc-4a0ed4fc8415.cfargotunnel.com",
				"tunnel_token":  "eyJhIjoiMDc0NzU1YTc4ZDhlIn0=",
				"tunnel_status": "healthy",
				"account_tag":   "074755a78d8e8f77c119a90a125e8a06",
				"created_on":    "2026-06-25T12:00:00Z",
			},
			mustPopulate: []string{
				"tunnel_id", "tunnel_cname", "tunnel_token",
				"tunnel_status", "account_tag", "created_on",
			},
		},
		{
			// CloudflareZeroTrustTunnelVirtualNetwork: both engines emit the virtual
			// network id and name.
			name: "CloudflareZeroTrustTunnelVirtualNetwork",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustTunnelVirtualNetwork,
			rawOutputs: map[string]interface{}{
				"virtual_network_id":   "aaaa1111-bbbb-2222-cccc-333344445555",
				"virtual_network_name": "prod-vnet",
			},
			mustPopulate: []string{"virtual_network_id", "virtual_network_name"},
		},
		{
			// CloudflareZeroTrustTunnelRoute: both engines emit the route id and the
			// advertised CIDR.
			name: "CloudflareZeroTrustTunnelRoute",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustTunnelRoute,
			rawOutputs: map[string]interface{}{
				"route_id": "b8f2e1c0-1111-2222-3333-444455556666",
				"network":  "10.0.0.0/24",
			},
			mustPopulate: []string{"route_id", "network"},
		},
		{
			// CloudflareList: both engines emit the list id, name, and kind.
			name: "CloudflareList",
			kind: cloudresourcekind.CloudResourceKind_CloudflareList,
			rawOutputs: map[string]interface{}{
				"list_id": "2c0fc9fa937b11eaa1b71c4d701ab86e",
				"name":    "office_allowlist",
				"kind":    "ip",
			},
			mustPopulate: []string{"list_id", "name", "kind"},
		},
		{
			// CloudflareListItem: both engines emit the item id and parent list id.
			name: "CloudflareListItem",
			kind: cloudresourcekind.CloudResourceKind_CloudflareListItem,
			rawOutputs: map[string]interface{}{
				"item_id": "70c4e0c9b0e34f1a9b6f2d3c4a5b6c7d",
				"list_id": "2c0fc9fa937b11eaa1b71c4d701ab86e",
			},
			mustPopulate: []string{"item_id", "list_id"},
		},
		{
			// CloudflareTurnstileWidget: both engines emit the site key, the
			// (sensitive) secret, and timestamps.
			name: "CloudflareTurnstileWidget",
			kind: cloudresourcekind.CloudResourceKind_CloudflareTurnstileWidget,
			rawOutputs: map[string]interface{}{
				"sitekey":     "0x4AAAAAAA_examplesitekey",
				"secret":      "0x4AAAAAAA_examplesecretkey",
				"created_on":  "2026-06-25T00:00:00Z",
				"modified_on": "2026-06-25T00:00:00Z",
			},
			mustPopulate: []string{"sitekey", "secret"},
		},
		{
			// CloudflareEmailRoutingZone: both engines emit the zone id, enabled
			// flag, status, and name.
			name: "CloudflareEmailRoutingZone",
			kind: cloudresourcekind.CloudResourceKind_CloudflareEmailRoutingZone,
			rawOutputs: map[string]interface{}{
				"zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
				"enabled": "true",
				"status":  "ready",
				"name":    "example.com",
			},
			mustPopulate: []string{"zone_id", "status", "name"},
		},
		{
			// CloudflareEmailRoutingRule: both engines emit the rule id and zone id.
			name: "CloudflareEmailRoutingRule",
			kind: cloudresourcekind.CloudResourceKind_CloudflareEmailRoutingRule,
			rawOutputs: map[string]interface{}{
				"rule_id": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
				"zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"rule_id", "zone_id"},
		},
		{
			// CloudflareEmailRoutingAddress: both engines emit the address id,
			// email, and timestamps.
			name: "CloudflareEmailRoutingAddress",
			kind: cloudresourcekind.CloudResourceKind_CloudflareEmailRoutingAddress,
			rawOutputs: map[string]interface{}{
				"address_id": "b8f2e1c0a1b2c3d4e5f60718293a4b5c",
				"email":      "ops@example.com",
				"created":    "2026-06-25T00:00:00Z",
			},
			mustPopulate: []string{"address_id", "email"},
		},
		{
			// CloudflareOriginCaCertificate: both engines emit the certificate id,
			// the certificate PEM, the (sensitive) generated private key, and expiry.
			name: "CloudflareOriginCaCertificate",
			kind: cloudresourcekind.CloudResourceKind_CloudflareOriginCaCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id": "b8f2e1c0a1b2c3d4e5f60718293a4b5c",
				"certificate":    "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
				"private_key":    "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----",
				"expires_on":     "2041-06-25T00:00:00Z",
			},
			mustPopulate: []string{"certificate_id", "certificate"},
		},
		{
			// CloudflareCertificatePack: both engines emit the pack id and the
			// zone the pack was ordered in (a pack's API identity is
			// zone_id + certificate_pack_id). The pack's issuance status and
			// primary certificate id are server-driven async values that move
			// without a config change, so they are not stack outputs.
			name: "CloudflareCertificatePack",
			kind: cloudresourcekind.CloudResourceKind_CloudflareCertificatePack,
			rawOutputs: map[string]interface{}{
				"certificate_pack_id": "3822ff90e3534420ac41fc7e4a1f4b07",
				"zone_id":             "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"certificate_pack_id", "zone_id"},
		},
		{
			// CloudflareCustomHostname: both engines emit the hostname id,
			// the ownership-verification records, the creation timestamp, and
			// the zone the hostname was onboarded onto (API identity is
			// zone_id + custom_hostname_id). Validation status and the
			// server-appended verification errors are async values that move
			// without a config change, so they are not stack outputs.
			name: "CloudflareCustomHostname",
			kind: cloudresourcekind.CloudResourceKind_CloudflareCustomHostname,
			rawOutputs: map[string]interface{}{
				"custom_hostname_id":               "0d89c70f8d4f4b1aa1b5d2e3f4a5b6c7",
				"ownership_verification_name":      "_cf-custom-hostname.support.acme.com",
				"ownership_verification_type":      "txt",
				"ownership_verification_value":     "1f2e3d4c5b6a7988",
				"ownership_verification_http_url":  "http://support.acme.com/.well-known/cf-custom-hostname-challenge/0d89",
				"ownership_verification_http_body": "1f2e3d4c5b6a7988",
				"created_at":                       "2026-06-25T00:00:00Z",
				"zone_id":                          "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"custom_hostname_id", "zone_id"},
		},
		{
			// CloudflareCustomHostnameFallbackOrigin: both engines emit
			// timestamps and the zone this singleton belongs to (the fallback
			// origin has no resource id; its API identity IS the zone).
			// Deployment status and the server-appended errors list are async
			// values that move without a config change, so they are not stack
			// outputs.
			name: "CloudflareCustomHostnameFallbackOrigin",
			kind: cloudresourcekind.CloudResourceKind_CloudflareCustomHostnameFallbackOrigin,
			rawOutputs: map[string]interface{}{
				"created_at": "2026-06-25T00:00:00Z",
				"updated_at": "2026-06-25T00:00:00Z",
				"zone_id":    "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"created_at", "zone_id"},
		},
		{
			// CloudflareZoneSettings: a zone singleton with no resource id of its
			// own -- both engines emit the zone id (the settings' API identity).
			name: "CloudflareZoneSettings",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZoneSettings,
			rawOutputs: map[string]interface{}{
				"zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			// CloudflareCacheSettings: a zone singleton with no resource id of
			// its own -- both engines emit the zone id.
			name: "CloudflareCacheSettings",
			kind: cloudresourcekind.CloudResourceKind_CloudflareCacheSettings,
			rawOutputs: map[string]interface{}{
				"zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			// CloudflareZoneTlsSettings: a zone singleton with no resource id of
			// its own -- both engines emit the zone id.
			name: "CloudflareZoneTlsSettings",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZoneTlsSettings,
			rawOutputs: map[string]interface{}{
				"zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			// CloudflareZeroTrustAccessIdentityProvider: both engines emit the
			// provider id plus the SCIM material (base URL and the create-only
			// bearer secret, present when SCIM is enabled).
			name: "CloudflareZeroTrustAccessIdentityProvider",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessIdentityProvider,
			rawOutputs: map[string]interface{}{
				"identity_provider_id": "f174e90a-fafe-4643-bbbc-4a0ed4fc8415",
				"scim_base_url":        "https://scim.cloudflareaccess.com/v2/orgs/example",
				"scim_secret":          "scim-bearer-secret",
			},
			mustPopulate: []string{"identity_provider_id", "scim_base_url", "scim_secret"},
		},
		{
			// CloudflareZeroTrustAccessServiceToken: both engines emit the token
			// id, the client id, the create-only client secret, and the expiry.
			name: "CloudflareZeroTrustAccessServiceToken",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessServiceToken,
			rawOutputs: map[string]interface{}{
				"service_token_id": "699d98642c564d2e855e9661899b7252",
				"client_id":        "88bf3b6d86161464f6509f7219099e57.access",
				"client_secret":    "bdd31cbc4dec990953e39163fbbb194c93313ca9f0a6e420346af9d326b1d2a5",
				"expires_at":       "2027-08-16T00:00:00Z",
			},
			mustPopulate: []string{"service_token_id", "client_id", "client_secret", "expires_at"},
		},
		{
			// CloudflareZeroTrustGatewayPolicy: both engines emit the policy id
			// and the (possibly Cloudflare-assigned) precedence.
			name: "CloudflareZeroTrustGatewayPolicy",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustGatewayPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id":  "f174e90a-fafe-4643-bbbc-4a0ed4fc8415",
				"precedence": "1000",
			},
			mustPopulate: []string{"policy_id", "precedence"},
		},
		{
			// CloudflareZeroTrustList: both engines emit the list id.
			name: "CloudflareZeroTrustList",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustList,
			rawOutputs: map[string]interface{}{
				"list_id": "aa9d98642c564d2e855e9661899b7252",
			},
			mustPopulate: []string{"list_id"},
		},
		{
			name: "CloudflareIpAccessRule",
			kind: cloudresourcekind.CloudResourceKind_CloudflareIpAccessRule,
			rawOutputs: map[string]interface{}{
				"rule_id":    "2661fcac0be5a0bb64583f83b6709d4d",
				"zone_id":    "0da42c8d2132a9ddaf714f9e7c920711",
				"account_id": "023e105f4ecef8ad9ca31a8372d0c353",
			},
			mustPopulate: []string{"rule_id"},
		},
		{
			name: "CloudflareBotManagement",
			kind: cloudresourcekind.CloudResourceKind_CloudflareBotManagement,
			rawOutputs: map[string]interface{}{
				"zone_id": "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			name: "CloudflareSnippet",
			kind: cloudresourcekind.CloudResourceKind_CloudflareSnippet,
			rawOutputs: map[string]interface{}{
				"snippet_name": "redirect_legacy_urls",
				"zone_id":      "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"snippet_name", "zone_id"},
		},
		{
			name: "CloudflareSnippetRules",
			kind: cloudresourcekind.CloudResourceKind_CloudflareSnippetRules,
			rawOutputs: map[string]interface{}{
				"zone_id": "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			name: "CloudflareHealthcheck",
			kind: cloudresourcekind.CloudResourceKind_CloudflareHealthcheck,
			rawOutputs: map[string]interface{}{
				"healthcheck_id": "f174e90a-fafe-4643-bbbc-4a0ed4fc8415",
				"zone_id":        "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"healthcheck_id", "zone_id"},
		},
		{
			name: "CloudflareWaitingRoom",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWaitingRoom,
			rawOutputs: map[string]interface{}{
				"waiting_room_id": "4ae2018d-4a8e-4a1d-88e3-2a3a56d67057",
				"zone_id":         "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"waiting_room_id", "zone_id"},
		},
		{
			name: "CloudflareWaitingRoomEvent",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWaitingRoomEvent,
			rawOutputs: map[string]interface{}{
				"event_id":        "25756c26-6616-4ea9-bbb5-f06974fdaa39",
				"waiting_room_id": "4ae2018d-4a8e-4a1d-88e3-2a3a56d67057",
				"zone_id":         "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"event_id", "waiting_room_id", "zone_id"},
		},
		{
			name: "CloudflareCustomSslCertificate",
			kind: cloudresourcekind.CloudResourceKind_CloudflareCustomSslCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id": "7e7b8deba8538af625850b7b2530034c",
				"zone_id":        "0da42c8d2132a9ddaf714f9e7c920711",
				"expires_on":     "2027-02-01T05:20:00Z",
			},
			// status is deliberately not an output: deployment is
			// asynchronous, so the phase flips on the first refresh after
			// the transition and re-plans forever (measured live 2026-08-28
			// on the sibling AOP certificate).
			mustPopulate: []string{"certificate_id", "zone_id", "expires_on"},
		},
		{
			name: "CloudflareMtlsCertificate",
			kind: cloudresourcekind.CloudResourceKind_CloudflareMtlsCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id": "2458ce5a-0c35-4c7f-82c7-8e9487d3ff60",
				"expires_on":     "2027-01-01T00:00:00Z",
				"serial_number":  "6743787633689793699141714808227354901927759474",
			},
			mustPopulate: []string{"certificate_id", "expires_on", "serial_number"},
		},
		{
			name: "CloudflareAuthenticatedOriginPulls",
			kind: cloudresourcekind.CloudResourceKind_CloudflareAuthenticatedOriginPulls,
			rawOutputs: map[string]interface{}{
				"zone_id": "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"zone_id"},
		},
		{
			name: "CloudflareAuthenticatedOriginPullsCertificate",
			kind: cloudresourcekind.CloudResourceKind_CloudflareAuthenticatedOriginPullsCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id": "9e13e848-3aa1-4a4e-b222-e5e79e15fc1a",
				"zone_id":        "0da42c8d2132a9ddaf714f9e7c920711",
				"expires_on":     "2027-01-01T00:00:00Z",
			},
			// status is deliberately not an output: deployment is
			// asynchronous (pending_deployment -> active seconds after
			// create), so the phase flips on the first refresh after the
			// transition and re-plans forever (measured live 2026-08-28).
			mustPopulate: []string{"certificate_id", "zone_id", "expires_on"},
		},
		{
			name: "CloudflareWorkflow",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWorkflow,
			rawOutputs: map[string]interface{}{
				"workflow_name": "order-fulfillment",
				"version_id":    "b71b0b3f-13a8-4a90-9c56-8f2b6e6f3c5e",
			},
			mustPopulate: []string{"workflow_name", "version_id"},
		},
		{
			name: "CloudflareSecretsStore",
			kind: cloudresourcekind.CloudResourceKind_CloudflareSecretsStore,
			rawOutputs: map[string]interface{}{
				"store_id": "7b0a3d5c1e9f42c68d1a2b3c4d5e6f70",
			},
			mustPopulate: []string{"store_id"},
		},
		{
			name: "CloudflareSecretsStoreSecret",
			kind: cloudresourcekind.CloudResourceKind_CloudflareSecretsStoreSecret,
			rawOutputs: map[string]interface{}{
				"secret_id": "3a5c1e9f42c68d1a2b3c4d5e6f707b0a",
				"store_id":  "7b0a3d5c1e9f42c68d1a2b3c4d5e6f70",
			},
			mustPopulate: []string{"secret_id", "store_id"},
		},
		{
			// CloudflareAiGateway: the gateway slug plus the keyed map of
			// managed dynamic-route ids (route name -> id) must both land on
			// the StackOutputs proto -- the per-route import identity derives
			// from the map.
			name: "CloudflareAiGateway",
			kind: cloudresourcekind.CloudResourceKind_CloudflareAiGateway,
			rawOutputs: map[string]interface{}{
				"gateway_id":        "prod-llm-gateway",
				"dynamic_route_ids": map[string]interface{}{"cheap-first": "route-1"},
			},
			mustPopulate: []string{"gateway_id", "dynamic_route_ids"},
		},
		{
			// CloudflareZeroTrustOrganization: a settings singleton -- the
			// account id is the identity the harness and import recipes key
			// on; auth_domain is the semantic output consumers reference.
			name: "CloudflareZeroTrustOrganization",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustOrganization,
			rawOutputs: map[string]interface{}{
				"auth_domain": "acme",
				"account_id":  "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"auth_domain", "account_id"},
		},
		{
			name: "CloudflareZeroTrustAccessInfrastructureTarget",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustAccessInfrastructureTarget,
			rawOutputs: map[string]interface{}{
				"target_id": "f70ff985-a4ef-4643-bbbc-4a0ed4fc8415",
			},
			mustPopulate: []string{"target_id"},
		},
		{
			name: "CloudflareZeroTrustMcpPortal",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustMcpPortal,
			rawOutputs: map[string]interface{}{
				"portal_id": "eng-tools",
				"hostname":  "mcp.example.com",
			},
			mustPopulate: []string{"portal_id", "hostname"},
		},
		{
			name: "CloudflareZeroTrustMcpServer",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustMcpServer,
			rawOutputs: map[string]interface{}{
				"server_id": "docs-search",
			},
			mustPopulate: []string{"server_id"},
		},
		{
			// CloudflareZeroTrustGatewaySettings: a settings singleton --
			// the account id is the only identity (the fold has no resource
			// id of its own).
			name: "CloudflareZeroTrustGatewaySettings",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustGatewaySettings,
			rawOutputs: map[string]interface{}{
				"account_id": "0da42c8d2132a9ddaf714f9e7c920711",
			},
			mustPopulate: []string{"account_id"},
		},
		{
			name: "CloudflareZeroTrustDnsLocation",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustDnsLocation,
			rawOutputs: map[string]interface{}{
				"location_id":   "5f9c1e2a3b4d5e6f708192a3b4c5d6e7",
				"doh_subdomain": "q7x2p9r4m1",
				"ip":            "172.64.36.5",
			},
			mustPopulate: []string{"location_id", "doh_subdomain", "ip"},
		},
		{
			// CloudflareZeroTrustDeviceDefaultProfile: a settings singleton
			// -- the account id is the identity the harness and import
			// recipes key on; the Gateway-side id and policy id are the
			// semantic outputs consumers reference.
			name: "CloudflareZeroTrustDeviceDefaultProfile",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustDeviceDefaultProfile,
			rawOutputs: map[string]interface{}{
				"account_id":        "0da42c8d2132a9ddaf714f9e7c920711",
				"gateway_unique_id": "e9f42c68d1a2b3c4d5e6f707b0a3d5c1",
				"policy_id":         "default",
			},
			mustPopulate: []string{"account_id", "gateway_unique_id", "policy_id"},
		},
		{
			name: "CloudflareZeroTrustDeviceCustomProfile",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustDeviceCustomProfile,
			rawOutputs: map[string]interface{}{
				"policy_id":         "f70ff985-a4ef-4643-bbbc-4a0ed4fc8415",
				"gateway_unique_id": "e9f42c68d1a2b3c4d5e6f707b0a3d5c1",
			},
			mustPopulate: []string{"policy_id", "gateway_unique_id"},
		},
		{
			name: "CloudflareZeroTrustDevicePostureRule",
			kind: cloudresourcekind.CloudResourceKind_CloudflareZeroTrustDevicePostureRule,
			rawOutputs: map[string]interface{}{
				"rule_id": "f70ff985-a4ef-4643-bbbc-4a0ed4fc8415",
			},
			mustPopulate: []string{"rule_id"},
		},
		{
			// CloudflareLogpushJob: the numeric job id travels in string form,
			// the scope ids key the verifier's path, and the challenge trio is
			// populated only when the issuing arm ran (empty here -- the
			// common shape).
			name: "CloudflareLogpushJob",
			kind: cloudresourcekind.CloudResourceKind_CloudflareLogpushJob,
			rawOutputs: map[string]interface{}{
				"job_id":                       "146678",
				"account_id":                   "",
				"zone_id":                      "023e105f4ecef8ad9ca31a8372d0c353",
				"ownership_challenge_filename": "",
				"ownership_challenge_message":  "",
				"ownership_challenge_valid":    false,
			},
			mustPopulate: []string{"job_id", "zone_id"},
		},
		{
			name: "CloudflareNotificationPolicy",
			kind: cloudresourcekind.CloudResourceKind_CloudflareNotificationPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id": "0da2b59e-f118-42de-95bd-14fdd8ff4d7a",
			},
			mustPopulate: []string{"policy_id"},
		},
		{
			// CloudflareNotificationWebhook: the destination UUID policies
			// reference, plus the type Cloudflare inferred from the URL.
			name: "CloudflareNotificationWebhook",
			kind: cloudresourcekind.CloudResourceKind_CloudflareNotificationWebhook,
			rawOutputs: map[string]interface{}{
				"webhook_id": "9430334d-cf60-4147-8b76-8d6cbea1b099",
				"type":       "generic",
			},
			mustPopulate: []string{"webhook_id", "type"},
		},
		{
			// CloudflareWebAnalyticsSite: the site tag keys every RUM API
			// path, the token and snippet are the beacon credential (secret-
			// marked in both engines), and the ruleset id is the parent the
			// folded rules live under.
			name: "CloudflareWebAnalyticsSite",
			kind: cloudresourcekind.CloudResourceKind_CloudflareWebAnalyticsSite,
			rawOutputs: map[string]interface{}{
				"site_tag":   "0b7b0f1a08a54c6db26a6f1b193a2c85",
				"site_token": "e64a2265d1a04f1cbb073b7d3a4a6b8d",
				"snippet":    "<script defer src='https://static.cloudflareinsights.com/beacon.min.js'></script>",
				"ruleset_id": "b4c1e7f2a3d94e6f8a9b0c1d2e3f4a5b",
			},
			mustPopulate: []string{"site_tag", "site_token", "ruleset_id"},
		},
		{
			// CloudflareAccountApiToken: the management id and the
			// create-only secret value (returned by Cloudflare exactly once;
			// secret-marked in both engines).
			name: "CloudflareAccountApiToken",
			kind: cloudresourcekind.CloudResourceKind_CloudflareAccountApiToken,
			rawOutputs: map[string]interface{}{
				"token_id": "ed17574386854bf78a67040be0a770b0",
				"value":    "8M7wS6hCpXVc-DoRnPPY_UCWPgy8aea4Wy6kCe5T",
			},
			mustPopulate: []string{"token_id", "value"},
		},
		{
			// AzureResourceGroup: flat scalar outputs from both engines (ARM id,
			// name, region) must each land on the StackOutputs proto --
			// resource_group_name is the FK target every other Azure kind
			// references, and resource_group_id is the default scope for role
			// assignments.
			name: "AzureResourceGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureResourceGroup,
			rawOutputs: map[string]interface{}{
				"resource_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg",
				"resource_group_name": "platform-rg",
				"region":              "eastus",
			},
			mustPopulate: []string{"resource_group_id", "resource_group_name", "region"},
		},
		{
			// AzureRoleAssignment: flat scalar outputs from both engines (the
			// fully-scoped assignment id, GUID name, scope, resolved role
			// definition id, principal id/type) must each land on the StackOutputs
			// proto -- role_assignment_id is what the authorization API and the
			// E2E verifier key on.
			name: "AzureRoleAssignment",
			kind: cloudresourcekind.CloudResourceKind_AzureRoleAssignment,
			rawOutputs: map[string]interface{}{
				"role_assignment_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.Authorization/roleAssignments/a67e1183-4b2d-4b6e-93f1-2b2b8d2e1c11",
				"name":               "a67e1183-4b2d-4b6e-93f1-2b2b8d2e1c11",
				"scope":              "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg",
				"role_definition_id": "/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Authorization/roleDefinitions/acdd72a7-3385-48ef-bd42-f606fba81ae7",
				"principal_id":       "11111111-1111-1111-1111-111111111111",
				"principal_type":     "ServicePrincipal",
			},
			mustPopulate: []string{
				"role_assignment_id", "name", "scope",
				"role_definition_id", "principal_id", "principal_type",
			},
		},
		{
			// AzureRoleDefinition: scalar outputs plus a repeated string
			// (assignable_scopes) from both engines must land on the
			// StackOutputs proto -- role_definition_id carries the fully-scoped
			// ARM id (what an AzureRoleAssignment binds and what the E2E
			// verifier keys on), not the bare GUID (that is
			// role_definition_guid).
			name: "AzureRoleDefinition",
			kind: cloudresourcekind.CloudResourceKind_AzureRoleDefinition,
			rawOutputs: map[string]interface{}{
				"role_definition_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c",
				"role_definition_guid": "b24988ac-6180-42a0-ab88-20f7382dd24c",
				"role_name":            "acme-vm-operator",
				"scope":                "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg",
				"assignable_scopes":    []interface{}{"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg"},
			},
			mustPopulate: []string{
				"role_definition_id", "role_definition_guid", "role_name",
				"scope", "assignable_scopes",
			},
		},
		{
			// AzureUserAssignedIdentity: the identity's three identifiers plus
			// its ARM id must land on the StackOutputs proto -- principal_id is
			// what role assignments grant to, client_id is what workloads
			// present to authenticate, identity_id is what consuming resources
			// and federated credentials attach to.
			name: "AzureUserAssignedIdentity",
			kind: cloudresourcekind.CloudResourceKind_AzureUserAssignedIdentity,
			rawOutputs: map[string]interface{}{
				"identity_id":  "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/payments-api",
				"principal_id": "11111111-1111-1111-1111-111111111111",
				"client_id":    "22222222-2222-2222-2222-222222222222",
				"tenant_id":    "33333333-3333-3333-3333-333333333333",
			},
			mustPopulate: []string{
				"identity_id", "principal_id", "client_id", "tenant_id",
			},
		},
		{
			// AzureFederatedIdentityCredential: both engines export the
			// credential's ARM id plus the trust coordinates (issuer /
			// subject / audience) as deployed. audience is a single string on
			// the proto even though ARM's wire shape is a one-element list --
			// the Terraform module exports the sole element, matching the
			// Pulumi provider's flattened attribute.
			name: "AzureFederatedIdentityCredential",
			kind: cloudresourcekind.CloudResourceKind_AzureFederatedIdentityCredential,
			rawOutputs: map[string]interface{}{
				"federated_identity_credential_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/ci-deployer/federatedIdentityCredentials/github-main-branch",
				"name":                             "github-main-branch",
				"user_assigned_identity_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/ci-deployer",
				"issuer":                           "https://token.actions.githubusercontent.com",
				"subject":                          "repo:acme/platform:ref:refs/heads/main",
				"audience":                         "api://AzureADTokenExchange",
			},
			mustPopulate: []string{
				"federated_identity_credential_id", "name",
				"user_assigned_identity_id", "issuer", "subject", "audience",
			},
		},
		{
			// AzureVirtualNetwork: scalar identifiers plus a repeated string
			// (address_spaces) from both engines must land on the StackOutputs
			// proto -- virtual_network_id is the join key subnets, peerings,
			// and DNS links attach through, and address_spaces reflects the
			// ACTUAL ranges (IPAM-provisioned when pools delegate allocation).
			name: "AzureVirtualNetwork",
			kind: cloudresourcekind.CloudResourceKind_AzureVirtualNetwork,
			rawOutputs: map[string]interface{}{
				"virtual_network_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/virtualNetworks/prod-vnet",
				"virtual_network_name": "prod-vnet",
				"guid":                 "44444444-4444-4444-4444-444444444444",
				"address_spaces":       []interface{}{"10.0.0.0/16", "10.1.0.0/16"},
			},
			mustPopulate: []string{
				"virtual_network_id", "virtual_network_name", "guid",
				"address_spaces",
			},
		},
		{
			// AzureRouteTable: both engines export the table's ARM id (the
			// join key subnets use to attach the table's routing policy) and
			// its name.
			name: "AzureRouteTable",
			kind: cloudresourcekind.CloudResourceKind_AzureRouteTable,
			rawOutputs: map[string]interface{}{
				"route_table_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/routeTables/egress-via-firewall",
				"route_table_name": "egress-via-firewall",
			},
			mustPopulate: []string{
				"route_table_id", "route_table_name",
			},
		},
		{
			// AzurePrivateDnsZone: the zone's ARM id (the join key links,
			// private endpoints, and databases attach through), its DNS name,
			// and its resource group (echoed for tooling that joins on
			// name+RG rather than parsing ARM ids).
			name: "AzurePrivateDnsZone",
			kind: cloudresourcekind.CloudResourceKind_AzurePrivateDnsZone,
			rawOutputs: map[string]interface{}{
				"zone_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/privateDnsZones/privatelink.postgres.database.azure.com",
				"zone_name":           "privatelink.postgres.database.azure.com",
				"resource_group_name": "network-rg",
			},
			mustPopulate: []string{
				"zone_id", "zone_name", "resource_group_name",
			},
		},
		{
			// AzurePrivateDnsZoneVirtualNetworkLink: both engines export the
			// link's ARM id (a child of the zone:
			// {zone-id}/virtualNetworkLinks/{name}) and its name.
			name: "AzurePrivateDnsZoneVirtualNetworkLink",
			kind: cloudresourcekind.CloudResourceKind_AzurePrivateDnsZoneVirtualNetworkLink,
			rawOutputs: map[string]interface{}{
				"link_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/privateDnsZones/privatelink.postgres.database.azure.com/virtualNetworkLinks/hub-vnet",
				"link_name": "hub-vnet",
			},
			mustPopulate: []string{
				"link_id", "link_name",
			},
		},
		{
			// AzureSubnet: the catalog's most-referenced join key (subnet_id)
			// plus a repeated string (address_prefixes reflects the ACTUAL
			// ranges, IPAM-provisioned when a pool delegates allocation) and
			// the parent coordinates derived from the referenced network's
			// ARM id.
			name: "AzureSubnet",
			kind: cloudresourcekind.CloudResourceKind_AzureSubnet,
			rawOutputs: map[string]interface{}{
				"subnet_id":            "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/virtualNetworks/prod-vnet/subnets/app",
				"subnet_name":          "app",
				"address_prefixes":     []interface{}{"10.0.1.0/24"},
				"virtual_network_name": "prod-vnet",
				"resource_group_name":  "network-rg",
			},
			mustPopulate: []string{
				"subnet_id", "subnet_name", "address_prefixes",
				"virtual_network_name", "resource_group_name",
			},
		},
		{
			// AzureNetworkSecurityGroup: both engines export the group's ARM
			// id (the join key subnets attach through) and its name.
			name: "AzureNetworkSecurityGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureNetworkSecurityGroup,
			rawOutputs: map[string]interface{}{
				"network_security_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/networkSecurityGroups/web-tier",
				"network_security_group_name": "web-tier",
			},
			mustPopulate: []string{
				"network_security_group_id", "network_security_group_name",
			},
		},
		{
			// AzurePublicIp: the address's ARM id (the join key gateways and
			// load balancers attach), the allocated address itself, and the
			// Azure-managed FQDN when a DNS label is set.
			name: "AzurePublicIp",
			kind: cloudresourcekind.CloudResourceKind_AzurePublicIp,
			rawOutputs: map[string]interface{}{
				"public_ip_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/publicIPAddresses/prod-frontend",
				"ip_address":     "20.42.1.1",
				"fqdn":           "prod-gateway.eastus.cloudapp.azure.com",
				"public_ip_name": "prod-frontend",
			},
			mustPopulate: []string{
				"public_ip_id", "ip_address", "fqdn", "public_ip_name",
			},
		},
		{
			// AzurePublicIpPrefix: the prefix's ARM id (referenced by public
			// IPs and NAT gateway associations) and the ACTUAL reserved CIDR
			// -- known only after creation, the value partners allowlist.
			name: "AzurePublicIpPrefix",
			kind: cloudresourcekind.CloudResourceKind_AzurePublicIpPrefix,
			rawOutputs: map[string]interface{}{
				"public_ip_prefix_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/publicIPPrefixes/prod-egress",
				"ip_prefix":             "20.42.0.16/28",
				"public_ip_prefix_name": "prod-egress",
			},
			mustPopulate: []string{
				"public_ip_prefix_id", "ip_prefix", "public_ip_prefix_name",
			},
		},
		{
			// AzureApplicationSecurityGroup: the group's ARM id is the
			// composition seam -- NIC ip configurations, scale-set network
			// profiles, and NSG rules reference it to declare membership or
			// target the group.
			name: "AzureApplicationSecurityGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureApplicationSecurityGroup,
			rawOutputs: map[string]interface{}{
				"application_security_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/applicationSecurityGroups/web-tier",
				"application_security_group_name": "web-tier",
			},
			mustPopulate: []string{
				"application_security_group_id", "application_security_group_name",
			},
		},
		{
			// AzurePrivateEndpoint: the endpoint's ARM id, its private IP
			// (the address the service FQDN resolves to inside the VNet),
			// and the auto-created NIC's id.
			name: "AzurePrivateEndpoint",
			kind: cloudresourcekind.CloudResourceKind_AzurePrivateEndpoint,
			rawOutputs: map[string]interface{}{
				"private_endpoint_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/privateEndpoints/pg-pe",
				"private_endpoint_name": "pg-pe",
				"private_ip_address":    "10.0.1.10",
				"network_interface_id":  "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/networkInterfaces/pg-pe-nic",
			},
			mustPopulate: []string{
				"private_endpoint_id", "private_endpoint_name", "private_ip_address", "network_interface_id",
			},
		},
		{
			// AzureDiskEncryptionSet: the set's ARM id (referenced by disks,
			// VMs, and scale sets) plus the system-assigned identity's
			// principal/tenant (the grant target for Key Vault crypto access).
			name: "AzureDiskEncryptionSet",
			kind: cloudresourcekind.CloudResourceKind_AzureDiskEncryptionSet,
			rawOutputs: map[string]interface{}{
				"disk_encryption_set_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.Compute/diskEncryptionSets/prod-des",
				"disk_encryption_set_name": "prod-des",
				"identity_principal_id":    "11111111-2222-3333-4444-555555555555",
				"identity_tenant_id":       "99999999-8888-7777-6666-555555555555",
			},
			mustPopulate: []string{
				"disk_encryption_set_id", "disk_encryption_set_name",
			},
		},
		{
			// AzureMssqlFailoverGroup: the group's ARM id plus the
			// DNS-composed listener endpoints -- the read-write listener is
			// the failover-following connection target downstream apps use.
			name: "AzureMssqlFailoverGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureMssqlFailoverGroup,
			rawOutputs: map[string]interface{}{
				"failover_group_id":            "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Sql/servers/primary/failoverGroups/prod-sql-fog",
				"failover_group_name":          "prod-sql-fog",
				"read_write_listener_endpoint": "prod-sql-fog.database.windows.net",
				"read_only_listener_endpoint":  "prod-sql-fog.secondary.database.windows.net",
			},
			mustPopulate: []string{
				"failover_group_id", "failover_group_name", "read_write_listener_endpoint", "read_only_listener_endpoint",
			},
		},
		{
			// AzureMonitorActivityLogAlert: the alert's ARM id and name.
			name: "AzureMonitorActivityLogAlert",
			kind: cloudresourcekind.CloudResourceKind_AzureMonitorActivityLogAlert,
			rawOutputs: map[string]interface{}{
				"activity_log_alert_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Insights/activityLogAlerts/vm-delete-alert",
				"activity_log_alert_name": "vm-delete-alert",
			},
			mustPopulate: []string{
				"activity_log_alert_id", "activity_log_alert_name",
			},
		},
		{
			// AzureApplicationInsightsStandardWebTest: the test's ARM id
			// (referenced by a metric alert's web-test criteria), its name,
			// and the synthetic monitor id.
			name: "AzureApplicationInsightsStandardWebTest",
			kind: cloudresourcekind.CloudResourceKind_AzureApplicationInsightsStandardWebTest,
			rawOutputs: map[string]interface{}{
				"web_test_id":          "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Insights/webTests/homepage-health",
				"web_test_name":        "homepage-health",
				"synthetic_monitor_id": "homepage-health",
			},
			mustPopulate: []string{
				"web_test_id", "web_test_name", "synthetic_monitor_id",
			},
		},
		{
			// AzureLoadBalancer: the name-keyed maps are the composition
			// seams -- backend_pool_ids is what NIC ip_configurations and
			// scale-set network profiles join, nat_rule_ids is what a NIC's
			// NAT-rule association completes, probe_ids is what a scale
			// set's rolling-upgrade health probe references.
			name: "AzureLoadBalancer",
			kind: cloudresourcekind.CloudResourceKind_AzureLoadBalancer,
			rawOutputs: map[string]interface{}{
				"load_balancer_id":     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/loadBalancers/app-lb",
				"load_balancer_name":   "app-lb",
				"private_ip_address":   "10.0.1.6",
				"private_ip_addresses": []interface{}{"10.0.1.6"},
				"frontend_ip_configuration_ids": map[string]interface{}{
					"internal": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/loadBalancers/app-lb/frontendIPConfigurations/internal",
				},
				"backend_pool_ids": map[string]interface{}{
					"web": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/loadBalancers/app-lb/backendAddressPools/web",
				},
				"probe_ids": map[string]interface{}{
					"http-health": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/loadBalancers/app-lb/probes/http-health",
				},
				"nat_rule_ids": map[string]interface{}{
					"ssh-admin": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/loadBalancers/app-lb/inboundNatRules/ssh-admin",
				},
			},
			mustPopulate: []string{
				"load_balancer_id", "load_balancer_name",
				"private_ip_address", "private_ip_addresses",
				"frontend_ip_configuration_ids", "backend_pool_ids",
				"probe_ids", "nat_rule_ids",
			},
		},
		{
			// AzureNatGateway: the gateway's ARM id (the join key subnets
			// attach through), its name, and the ARM-assigned GUID.
			name: "AzureNatGateway",
			kind: cloudresourcekind.CloudResourceKind_AzureNatGateway,
			rawOutputs: map[string]interface{}{
				"nat_gateway_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/natGateways/prod-egress",
				"nat_gateway_name": "prod-egress",
				"resource_guid":    "55555555-5555-5555-5555-555555555555",
			},
			mustPopulate: []string{
				"nat_gateway_id", "nat_gateway_name", "resource_guid",
			},
		},
		{
			// AzureVirtualNetworkPeering: one direction's ARM id (a child of
			// the local network: {vnet-id}/virtualNetworkPeerings/{name}),
			// its name, and the local coordinates derived from the referenced
			// network's ARM id.
			name: "AzureVirtualNetworkPeering",
			kind: cloudresourcekind.CloudResourceKind_AzureVirtualNetworkPeering,
			rawOutputs: map[string]interface{}{
				"peering_id":           "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/hub-rg/providers/Microsoft.Network/virtualNetworks/hub-vnet/virtualNetworkPeerings/hub-to-spoke1",
				"peering_name":         "hub-to-spoke1",
				"virtual_network_name": "hub-vnet",
				"resource_group_name":  "hub-rg",
			},
			mustPopulate: []string{
				"peering_id", "peering_name", "virtual_network_name",
				"resource_group_name",
			},
		},
		{
			// AzureAksCluster: cluster_id is the parent seam every standalone
			// AzureAksNodePool consumes; oidc_issuer_url is the trust anchor
			// AzureFederatedIdentityCredential binds to.
			name: "AzureAksCluster",
			kind: cloudresourcekind.CloudResourceKind_AzureAksCluster,
			rawOutputs: map[string]interface{}{
				"cluster_id":                    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/aks-rg/providers/Microsoft.ContainerService/managedClusters/prod-aks",
				"cluster_name":                  "prod-aks",
				"fqdn":                          "prod-aks-abc123.hcp.eastus.azmk8s.io",
				"private_fqdn":                  "",
				"portal_fqdn":                   "prod-aks-abc123.privatelink.eastus.azmk8s.io",
				"oidc_issuer_url":               "https://eastus.oic.prod-aks.abc123.azmk8s.io/00000000-0000-0000-0000-000000000000/",
				"node_resource_group":           "MC_aks-rg_prod-aks_eastus",
				"node_resource_group_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/MC_aks-rg_prod-aks_eastus",
				"cluster_kubeconfig":            "YXBpVmVyc2lvbjogdjE=",
				"cluster_identity_principal_id": "11111111-1111-1111-1111-111111111111",
				"kubelet_identity_object_id":    "22222222-2222-2222-2222-222222222222",
				"kubelet_identity_client_id":    "33333333-3333-3333-3333-333333333333",
				"current_kubernetes_version":    "1.35.2",
			},
			mustPopulate: []string{
				"cluster_id", "cluster_name", "fqdn", "oidc_issuer_url",
				"node_resource_group", "node_resource_group_id",
				"cluster_identity_principal_id", "current_kubernetes_version",
			},
		},
		{
			// AzureAksNodePool: the pool's ARM id (node_pool_id) is the
			// verification key; node_image_version reflects the OS patch
			// level actually rolled out.
			name: "AzureAksNodePool",
			kind: cloudresourcekind.CloudResourceKind_AzureAksNodePool,
			rawOutputs: map[string]interface{}{
				"node_pool_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/aks-rg/providers/Microsoft.ContainerService/managedClusters/prod-aks/agentPools/general",
				"node_pool_name":     "general",
				"node_image_version": "AKSUbuntu-2204gen2containerd-202502.03.0",
			},
			mustPopulate: []string{
				"node_pool_id", "node_pool_name", "node_image_version",
			},
		},
		{
			// AzureContainerRegistry: the registry's ARM id is the seam AKS
			// clusters and AcrPull/AcrPush role assignments scope to;
			// login_server is what images are tagged with; the admin
			// credentials and data-endpoint hostnames are feature-gated
			// outputs (empty/absent when their features are off).
			name: "AzureContainerRegistry",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerRegistry,
			rawOutputs: map[string]interface{}{
				"container_registry_id":                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/platform-rg/providers/Microsoft.ContainerRegistry/registries/prodimages",
				"container_registry_name":               "prodimages",
				"login_server":                          "prodimages.azurecr.io",
				"admin_username":                        "prodimages",
				"admin_password":                        "s3cr3t-rotatable",
				"system_assigned_identity_principal_id": "44444444-4444-4444-4444-444444444444",
				"data_endpoint_host_names":              []interface{}{"prodimages.eastus.data.azurecr.io"},
			},
			mustPopulate: []string{
				"container_registry_id", "container_registry_name",
				"login_server", "admin_username", "admin_password",
				"system_assigned_identity_principal_id",
				"data_endpoint_host_names",
			},
		},
		{
			// AzureNetworkInterface: the NIC's ARM id is the seam
			// AzureVirtualMachine.network_interface_ids consumes; the
			// private address is what backends and DNS records key on; the
			// MAC populates only once attached to a running VM.
			name: "AzureNetworkInterface",
			kind: cloudresourcekind.CloudResourceKind_AzureNetworkInterface,
			rawOutputs: map[string]interface{}{
				"network_interface_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/networkInterfaces/app-nic",
				"network_interface_name":      "app-nic",
				"private_ip_address":          "10.0.1.4",
				"private_ip_addresses":        []interface{}{"10.0.1.4"},
				"mac_address":                 "00-0D-3A-1B-2C-3D",
				"internal_domain_name_suffix": "abc123.bx.internal.cloudapp.net",
			},
			mustPopulate: []string{
				"network_interface_id", "network_interface_name",
				"private_ip_address", "private_ip_addresses",
				"mac_address", "internal_domain_name_suffix",
			},
		},
		{
			// AzureManagedDisk: the disk's ARM id is the seam
			// AzureVirtualMachine.data_disk_attachments consumes; the
			// actual size matters for COPY/FROM_IMAGE disks that inherited
			// the source's size.
			name: "AzureManagedDisk",
			kind: cloudresourcekind.CloudResourceKind_AzureManagedDisk,
			rawOutputs: map[string]interface{}{
				"disk_id":      "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Compute/disks/orders-db-data",
				"disk_name":    "orders-db-data",
				"disk_size_gb": 256,
			},
			mustPopulate: []string{
				"disk_id", "disk_name", "disk_size_gb",
			},
		},
		{
			// AzureVirtualMachineScaleSet: the scale set's ARM id is the
			// seam a standalone VM's Flexible-attach consumes and what
			// autoscale/monitoring scope to; the system-assigned principal
			// is the AzureRoleAssignment seam (UNIFORM sets).
			name: "AzureVirtualMachineScaleSet",
			kind: cloudresourcekind.CloudResourceKind_AzureVirtualMachineScaleSet,
			rawOutputs: map[string]interface{}{
				"scale_set_id":                          "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Compute/virtualMachineScaleSets/web-fleet",
				"scale_set_name":                        "web-fleet",
				"unique_id":                             "88888888-8888-8888-8888-888888888888",
				"system_assigned_identity_principal_id": "99999999-9999-9999-9999-999999999999",
			},
			mustPopulate: []string{
				"scale_set_id", "scale_set_name", "unique_id",
				"system_assigned_identity_principal_id",
			},
		},
		{
			// AzureVirtualMachine: vm_id is what grants and policies scope
			// to; the identity principal is the AzureRoleAssignment seam;
			// the IP conveniences aggregate from the referenced NICs.
			name: "AzureVirtualMachine",
			kind: cloudresourcekind.CloudResourceKind_AzureVirtualMachine,
			rawOutputs: map[string]interface{}{
				"vm_id":                                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Compute/virtualMachines/app-vm",
				"vm_name":                               "app-vm",
				"virtual_machine_guid":                  "66666666-6666-6666-6666-666666666666",
				"private_ip_address":                    "10.0.1.4",
				"public_ip_address":                     "",
				"computer_name":                         "app-vm",
				"system_assigned_identity_principal_id": "77777777-7777-7777-7777-777777777777",
			},
			mustPopulate: []string{
				"vm_id", "vm_name", "virtual_machine_guid",
				"private_ip_address", "computer_name",
				"system_assigned_identity_principal_id",
			},
		},
		{
			// AzureKeyVault: the vault's ARM id is the seam keys,
			// certificates, VM/VMSS secret blocks, and vault-scoped role
			// assignments reference; vault_uri is the data-plane endpoint
			// applications call.
			name: "AzureKeyVault",
			kind: cloudresourcekind.CloudResourceKind_AzureKeyVault,
			rawOutputs: map[string]interface{}{
				"key_vault_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/security-rg/providers/Microsoft.KeyVault/vaults/platform-kv",
				"key_vault_name":      "platform-kv",
				"vault_uri":           "https://platform-kv.vault.azure.net/",
				"tenant_id":           "255aadad-0000-0000-0000-000000000000",
				"resource_group_name": "security-rg",
			},
			mustPopulate: []string{
				"key_vault_id", "key_vault_name", "vault_uri",
				"tenant_id", "resource_group_name",
			},
		},
		{
			// AzureKeyVaultKey: versionless_id is the CMK seam consumers
			// (ACR encryption) reference so rotation propagates; key_id
			// pins a version (the AKS KMS grain); the ARM proxy ids serve
			// control-plane integrations.
			name: "AzureKeyVaultKey",
			kind: cloudresourcekind.CloudResourceKind_AzureKeyVaultKey,
			rawOutputs: map[string]interface{}{
				"key_id":                  "https://platform-kv.vault.azure.net/keys/storage-cmk/abc123def456",
				"versionless_id":          "https://platform-kv.vault.azure.net/keys/storage-cmk",
				"key_name":                "storage-cmk",
				"version":                 "abc123def456",
				"resource_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/security-rg/providers/Microsoft.KeyVault/vaults/platform-kv/keys/storage-cmk/versions/abc123def456",
				"resource_versionless_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/security-rg/providers/Microsoft.KeyVault/vaults/platform-kv/keys/storage-cmk",
				"public_key_pem":          "-----BEGIN PUBLIC KEY-----\nMIIBIjAN...\n-----END PUBLIC KEY-----",
				"public_key_openssh":      "ssh-rsa AAAAB3NzaC1yc2E...",
			},
			mustPopulate: []string{
				"key_id", "versionless_id", "key_name", "version",
				"resource_id", "resource_versionless_id",
				"public_key_pem", "public_key_openssh",
			},
		},
		{
			// AzureApplicationGateway: the name-keyed maps are the
			// composition seams -- backend_address_pool_ids is what NIC
			// ip_configurations and scale-set network profiles join;
			// frontend_ip_configuration_ids chains frontends; a private
			// frontend's address is what internal DNS records point at.
			name: "AzureApplicationGateway",
			kind: cloudresourcekind.CloudResourceKind_AzureApplicationGateway,
			rawOutputs: map[string]interface{}{
				"application_gateway_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/applicationGateways/web-gateway",
				"application_gateway_name": "web-gateway",
				"backend_address_pool_ids": map[string]interface{}{
					"web": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/applicationGateways/web-gateway/backendAddressPools/web",
				},
				"frontend_ip_configuration_ids": map[string]interface{}{
					"public": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/applicationGateways/web-gateway/frontendIPConfigurations/public",
				},
				"private_ip_address":   "10.0.2.10",
				"private_ip_addresses": []interface{}{"10.0.2.10"},
			},
			mustPopulate: []string{
				"application_gateway_id", "application_gateway_name",
				"backend_address_pool_ids", "frontend_ip_configuration_ids",
				"private_ip_address", "private_ip_addresses",
			},
		},
		{
			// AzureWebApplicationFirewallPolicy: policy_id is the seam
			// Application Gateways attach the policy through -- gateway-wide,
			// per listener, and per URL path rule.
			name: "AzureWebApplicationFirewallPolicy",
			kind: cloudresourcekind.CloudResourceKind_AzureWebApplicationFirewallPolicy,
			rawOutputs: map[string]interface{}{
				"policy_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/applicationGatewayWebApplicationFirewallPolicies/org-waf-baseline",
				"policy_name": "org-waf-baseline",
			},
			mustPopulate: []string{
				"policy_id", "policy_name",
			},
		},
		{
			// AzurePostgresqlFlexibleServer: fqdn + administrator_login are
			// what applications build connection strings from; server_id is
			// the seam private endpoints and replica/restore servers
			// (source_server_id) reference; database_ids is the name-keyed
			// map seam for per-database references.
			name: "AzurePostgresqlFlexibleServer",
			kind: cloudresourcekind.CloudResourceKind_AzurePostgresqlFlexibleServer,
			rawOutputs: map[string]interface{}{
				"server_id":           "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.DBforPostgreSQL/flexibleServers/orders-pg",
				"server_name":         "orders-pg",
				"fqdn":                "orders-pg.postgres.database.azure.com",
				"administrator_login": "pgadmin",
				"database_ids": map[string]interface{}{
					"orders": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.DBforPostgreSQL/flexibleServers/orders-pg/databases/orders",
				},
				"identity_principal_id": "44444444-4444-4444-4444-444444444444",
			},
			mustPopulate: []string{
				"server_id", "server_name", "fqdn", "administrator_login",
				"database_ids", "identity_principal_id",
			},
		},
		{
			// AzureMysqlFlexibleServer: fqdn + administrator_login are what
			// applications build connection strings from; server_id is the
			// seam private endpoints and replica/restore servers
			// (source_server_id) reference; database_ids is the name-keyed
			// map seam; replica_capacity sizes replica topologies.
			name: "AzureMysqlFlexibleServer",
			kind: cloudresourcekind.CloudResourceKind_AzureMysqlFlexibleServer,
			rawOutputs: map[string]interface{}{
				"server_id":           "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.DBforMySQL/flexibleServers/orders-mysql",
				"server_name":         "orders-mysql",
				"fqdn":                "orders-mysql.mysql.database.azure.com",
				"administrator_login": "mysqladmin",
				"database_ids": map[string]interface{}{
					"orders": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.DBforMySQL/flexibleServers/orders-mysql/databases/orders",
				},
				"replica_capacity": 10,
			},
			mustPopulate: []string{
				"server_id", "server_name", "fqdn", "administrator_login",
				"database_ids", "replica_capacity",
			},
		},
		{
			// AzureMssqlServer: server_id is the parent seam
			// AzureMssqlDatabase and AzureMssqlElasticPool reference (and
			// AzurePrivateEndpoint's connection target); fqdn +
			// administrator_login build connection strings.
			name: "AzureMssqlServer",
			kind: cloudresourcekind.CloudResourceKind_AzureMssqlServer,
			rawOutputs: map[string]interface{}{
				"server_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.Sql/servers/orders-sql",
				"server_name":           "orders-sql",
				"fqdn":                  "orders-sql.database.windows.net",
				"administrator_login":   "sqladmin",
				"identity_principal_id": "44444444-4444-4444-4444-444444444444",
			},
			mustPopulate: []string{
				"server_id", "server_name", "fqdn", "administrator_login",
				"identity_principal_id",
			},
		},
		{
			// AzureMssqlDatabase: database_id is the seam
			// copy/secondary/restore databases reference
			// (creation_source_database_id).
			name: "AzureMssqlDatabase",
			kind: cloudresourcekind.CloudResourceKind_AzureMssqlDatabase,
			rawOutputs: map[string]interface{}{
				"database_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.Sql/servers/orders-sql/databases/orders",
				"database_name": "orders",
			},
			mustPopulate: []string{
				"database_id", "database_name",
			},
		},
		{
			// AzureMssqlElasticPool: elastic_pool_id is the seam pooled
			// databases attach through (AzureMssqlDatabase.elastic_pool_id).
			name: "AzureMssqlElasticPool",
			kind: cloudresourcekind.CloudResourceKind_AzureMssqlElasticPool,
			rawOutputs: map[string]interface{}{
				"elastic_pool_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/data-rg/providers/Microsoft.Sql/servers/orders-sql/elasticPools/tenant-pool",
				"elastic_pool_name": "tenant-pool",
			},
			mustPopulate: []string{
				"elastic_pool_id", "elastic_pool_name",
			},
		},
		{
			// AzureStorageAccount: storage_account_id is the parent seam
			// AzureStorageContainer references (and data-plane role
			// assignments scope to); the name + primary_access_key pair is
			// what Function App / Linux Web App storage bindings consume;
			// the endpoints are what applications and CDN origins connect
			// to.
			name: "AzureStorageAccount",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageAccount,
			rawOutputs: map[string]interface{}{
				"storage_account_id":               "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage",
				"storage_account_name":             "plantonappstorage",
				"resource_group_name":              "app-rg",
				"primary_blob_endpoint":            "https://plantonappstorage.blob.core.windows.net/",
				"primary_blob_host":                "plantonappstorage.blob.core.windows.net",
				"primary_queue_endpoint":           "https://plantonappstorage.queue.core.windows.net/",
				"primary_table_endpoint":           "https://plantonappstorage.table.core.windows.net/",
				"primary_file_endpoint":            "https://plantonappstorage.file.core.windows.net/",
				"primary_dfs_endpoint":             "https://plantonappstorage.dfs.core.windows.net/",
				"primary_web_endpoint":             "https://plantonappstorage.z13.web.core.windows.net/",
				"primary_web_host":                 "plantonappstorage.z13.web.core.windows.net",
				"secondary_blob_endpoint":          "https://plantonappstorage-secondary.blob.core.windows.net/",
				"primary_access_key":               "base64keymaterial==",
				"secondary_access_key":             "base64keymaterial2==",
				"primary_connection_string":        "DefaultEndpointsProtocol=https;AccountName=plantonappstorage;AccountKey=base64keymaterial==;EndpointSuffix=core.windows.net",
				"secondary_connection_string":      "DefaultEndpointsProtocol=https;AccountName=plantonappstorage;AccountKey=base64keymaterial2==;EndpointSuffix=core.windows.net",
				"primary_blob_connection_string":   "DefaultEndpointsProtocol=https;BlobEndpoint=https://plantonappstorage.blob.core.windows.net/;AccountName=plantonappstorage;AccountKey=base64keymaterial==",
				"secondary_blob_connection_string": "DefaultEndpointsProtocol=https;BlobEndpoint=https://plantonappstorage-secondary.blob.core.windows.net/;AccountName=plantonappstorage;AccountKey=base64keymaterial2==",
				"identity_principal_id":            "44444444-4444-4444-4444-444444444444",
				"blob_service_id":                  "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/blobServices/default",
				"file_service_id":                  "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/fileServices/default",
				"queue_service_id":                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/queueServices/default",
				"table_service_id":                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/tableServices/default",
			},
			mustPopulate: []string{
				"storage_account_id", "storage_account_name", "resource_group_name",
				"primary_blob_endpoint", "primary_blob_host", "primary_queue_endpoint",
				"primary_table_endpoint", "primary_file_endpoint", "primary_dfs_endpoint",
				"primary_web_endpoint", "primary_web_host", "secondary_blob_endpoint",
				"primary_access_key", "secondary_access_key", "primary_connection_string",
				"secondary_connection_string", "primary_blob_connection_string",
				"secondary_blob_connection_string", "identity_principal_id",
				"blob_service_id", "file_service_id", "queue_service_id",
				"table_service_id",
			},
		},
		{
			// AzureStorageContainer: container_id is the scope data-plane
			// role assignments target for container-level access; the
			// account/container name pair is what SDK clients and function
			// bindings consume.
			name: "AzureStorageContainer",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageContainer,
			rawOutputs: map[string]interface{}{
				"container_id":         "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/blobServices/default/containers/uploads",
				"container_name":       "uploads",
				"storage_account_name": "plantonappstorage",
			},
			mustPopulate: []string{
				"container_id", "container_name", "storage_account_name",
			},
		},
		{
			// AzureStorageShare: share_id is the management identity;
			// rbac_scope_id is the DIFFERENT segment Azure Files data-plane
			// role assignments scope to; the account/share name pair is
			// what mount commands and CSI volume definitions consume.
			name: "AzureStorageShare",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageShare,
			rawOutputs: map[string]interface{}{
				"share_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/fileServices/default/shares/team-files",
				"rbac_scope_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/fileServices/default/fileshares/team-files",
				"share_name":           "team-files",
				"storage_account_name": "plantonappstorage",
			},
			mustPopulate: []string{
				"share_id", "rbac_scope_id", "share_name", "storage_account_name",
			},
		},
		{
			// AzureStorageQueue: queue_id is the scope data-plane role
			// assignments target for queue-level access; the account/queue
			// name pair is what SDK clients and Functions queue triggers
			// consume.
			name: "AzureStorageQueue",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageQueue,
			rawOutputs: map[string]interface{}{
				"queue_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/queueServices/default/queues/work-items",
				"queue_name":           "work-items",
				"storage_account_name": "plantonappstorage",
			},
			mustPopulate: []string{
				"queue_id", "queue_name", "storage_account_name",
			},
		},
		{
			// AzureStorageTable: table_id carries the resource-manager id
			// from BOTH engines (the addressing parity exception never
			// touches outputs); the account/table name pair is what SDK
			// clients and Functions table bindings consume.
			name: "AzureStorageTable",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageTable,
			rawOutputs: map[string]interface{}{
				"table_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/tableServices/default/tables/AppEntities",
				"table_name":           "AppEntities",
				"storage_account_name": "plantonappstorage",
			},
			mustPopulate: []string{
				"table_id", "table_name", "storage_account_name",
			},
		},
		{
			// AzureStorageEncryptionScope: encryption_scope_name is the
			// seam containers (default_encryption_scope) and ADLS
			// filesystems reference for sub-account key isolation.
			name: "AzureStorageEncryptionScope",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageEncryptionScope,
			rawOutputs: map[string]interface{}{
				"encryption_scope_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Storage/storageAccounts/plantonappstorage/encryptionScopes/tenant42scope",
				"encryption_scope_name": "tenant42scope",
				"storage_account_name":  "plantonappstorage",
			},
			mustPopulate: []string{
				"encryption_scope_id", "encryption_scope_name", "storage_account_name",
			},
		},
		{
			// AzureStorageDataLakeGen2Filesystem: filesystem_id is the ARM
			// container-proxy ID data-plane role assignments scope to
			// (ADLS filesystems surface in ARM as blob containers).
			name: "AzureStorageDataLakeGen2Filesystem",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageDataLakeGen2Filesystem,
			rawOutputs: map[string]interface{}{
				"filesystem_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/lake-rg/providers/Microsoft.Storage/storageAccounts/plantonlake/blobServices/default/containers/raw-zone",
				"filesystem_name":      "raw-zone",
				"storage_account_name": "plantonlake",
			},
			mustPopulate: []string{
				"filesystem_id", "filesystem_name", "storage_account_name",
			},
		},
		{
			// AzureStorageLocalUser: sftp_username is the composed login
			// clients connect with; sid and password are the
			// secret-bearing credential outputs.
			name: "AzureStorageLocalUser",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageLocalUser,
			rawOutputs: map[string]interface{}{
				"local_user_id":        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/exchange-rg/providers/Microsoft.Storage/storageAccounts/plantonsftp/localUsers/partner01",
				"user_name":            "partner01",
				"sftp_username":        "plantonsftp.partner01",
				"sid":                  "S-1-2-0-3895023191-1105595861-2277418014-1116",
				"password":             "generated-once-by-azure",
				"storage_account_name": "plantonsftp",
			},
			mustPopulate: []string{
				"local_user_id", "user_name", "sftp_username", "sid", "password", "storage_account_name",
			},
		},
		{
			// AzureStorageObjectReplication: one logical policy
			// materialized on BOTH accounts under one GUID -- two ARM IDs
			// plus the shared policy_id monitoring keys on.
			name: "AzureStorageObjectReplication",
			kind: cloudresourcekind.CloudResourceKind_AzureStorageObjectReplication,
			rawOutputs: map[string]interface{}{
				"source_object_replication_id":      "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/dr-rg/providers/Microsoft.Storage/storageAccounts/plantonorsrc/objectReplicationPolicies/6a2f5b7e-1c3d-4e5f-8a9b-0c1d2e3f4a5b",
				"destination_object_replication_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/dr-rg/providers/Microsoft.Storage/storageAccounts/plantonordst/objectReplicationPolicies/6a2f5b7e-1c3d-4e5f-8a9b-0c1d2e3f4a5b",
				"policy_id":                         "6a2f5b7e-1c3d-4e5f-8a9b-0c1d2e3f4a5b",
			},
			mustPopulate: []string{
				"source_object_replication_id", "destination_object_replication_id", "policy_id",
			},
		},
		{
			// AzureKeyVaultCertificate: the secret face
			// (versionless_secret_id) is the seam TLS terminators
			// (Application Gateway) consume so renewals propagate; the
			// thumbprint serves fingerprint-pinning integrations.
			name: "AzureKeyVaultCertificate",
			kind: cloudresourcekind.CloudResourceKind_AzureKeyVaultCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id":                  "https://platform-kv.vault.azure.net/certificates/internal-tls/fed321cba654",
				"versionless_id":                  "https://platform-kv.vault.azure.net/certificates/internal-tls",
				"secret_id":                       "https://platform-kv.vault.azure.net/secrets/internal-tls/fed321cba654",
				"versionless_secret_id":           "https://platform-kv.vault.azure.net/secrets/internal-tls",
				"certificate_name":                "internal-tls",
				"version":                         "fed321cba654",
				"thumbprint":                      "9F3C4E2A1B0D8765F4E3D2C1B0A99887E6D5C4B3",
				"resource_manager_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/security-rg/providers/Microsoft.KeyVault/vaults/platform-kv/certificates/internal-tls/versions/fed321cba654",
				"resource_manager_versionless_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/security-rg/providers/Microsoft.KeyVault/vaults/platform-kv/certificates/internal-tls",
			},
			mustPopulate: []string{
				"certificate_id", "versionless_id", "secret_id",
				"versionless_secret_id", "certificate_name", "version",
				"thumbprint", "resource_manager_id",
				"resource_manager_versionless_id",
			},
		},
		{
			// AzureRedisCache: redis_cache_id is what the linked-server,
			// access-policy, and private-endpoint kinds reference; region
			// is the linked-server location seam; both key faces stay live
			// so clients rotate with zero downtime.
			name: "AzureRedisCache",
			kind: cloudresourcekind.CloudResourceKind_AzureRedisCache,
			rawOutputs: map[string]interface{}{
				"redis_cache_id":              "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redis/app-cache",
				"redis_cache_name":            "app-cache",
				"region":                      "eastus",
				"resource_group_name":         "app-rg",
				"hostname":                    "app-cache.redis.cache.windows.net",
				"port":                        6379,
				"ssl_port":                    6380,
				"primary_access_key":          "primary-key-value",
				"secondary_access_key":        "secondary-key-value",
				"primary_connection_string":   "app-cache.redis.cache.windows.net:6380,password=primary-key-value,ssl=True,abortConnect=False",
				"secondary_connection_string": "app-cache.redis.cache.windows.net:6380,password=secondary-key-value,ssl=True,abortConnect=False",
				"identity_principal_id":       "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"redis_cache_id", "redis_cache_name", "region",
				"resource_group_name", "hostname", "port", "ssl_port",
				"primary_access_key", "secondary_access_key",
				"primary_connection_string", "secondary_connection_string",
				"identity_principal_id",
			},
		},
		{
			// AzureRedisLinkedServer: the geo hostname follows the CURRENT
			// primary across failovers -- the stable endpoint applications
			// point at instead of either cache's own hostname.
			name: "AzureRedisLinkedServer",
			kind: cloudresourcekind.CloudResourceKind_AzureRedisLinkedServer,
			rawOutputs: map[string]interface{}{
				"linked_server_id":                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-east/providers/Microsoft.Cache/redis/app-cache-east/linkedServers/app-cache-west",
				"linked_server_name":               "app-cache-west",
				"geo_replicated_primary_host_name": "app-cache-east.geo.redis.cache.windows.net",
			},
			mustPopulate: []string{
				"linked_server_id", "linked_server_name",
				"geo_replicated_primary_host_name",
			},
		},
		{
			// AzureRedisCacheAccessPolicy: access_policy_name is the seam
			// assignments reference to grant the policy to an identity.
			name: "AzureRedisCacheAccessPolicy",
			kind: cloudresourcekind.CloudResourceKind_AzureRedisCacheAccessPolicy,
			rawOutputs: map[string]interface{}{
				"access_policy_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redis/app-cache/accessPolicies/app-read-only",
				"access_policy_name": "app-read-only",
			},
			mustPopulate: []string{
				"access_policy_id", "access_policy_name",
			},
		},
		{
			// AzureManagedRedis: managed_redis_id is what the
			// geo-replication and access-policy-assignment kinds
			// reference; database_id is the grant/link scope; the keys
			// populate only while access-keys authentication is enabled
			// (keyless is the default).
			name: "AzureManagedRedis",
			kind: cloudresourcekind.CloudResourceKind_AzureManagedRedis,
			rawOutputs: map[string]interface{}{
				"managed_redis_id":      "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redisEnterprise/app-cache",
				"managed_redis_name":    "app-cache",
				"region":                "eastus",
				"resource_group_name":   "app-rg",
				"hostname":              "app-cache.eastus.redis.azure.net",
				"database_id":           "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redisEnterprise/app-cache/databases/default",
				"port":                  10000,
				"primary_access_key":    "primary-key-value",
				"secondary_access_key":  "secondary-key-value",
				"identity_principal_id": "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"managed_redis_id", "managed_redis_name", "region",
				"resource_group_name", "hostname", "database_id", "port",
				"primary_access_key", "secondary_access_key",
				"identity_principal_id",
			},
		},
		{
			// AzureServicePlan: service_plan_id is what the web/function
			// app kinds reference; kind and reserved are Azure-computed
			// attributes read back after creation.
			name: "AzureServicePlan",
			kind: cloudresourcekind.CloudResourceKind_AzureServicePlan,
			rawOutputs: map[string]interface{}{
				"service_plan_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Web/serverFarms/app-plan",
				"service_plan_name": "app-plan",
				"os_type":           "Linux",
				"sku_name":          "P1v3",
				"kind":              "linux",
				"reserved":          true,
			},
			mustPopulate: []string{
				"service_plan_id", "service_plan_name", "os_type",
				"sku_name", "kind", "reserved",
			},
		},
		{
			// AzureLinuxWebApp: default_hostname is the app's endpoint;
			// the outbound IP sets arrive as real lists from both
			// engines; the site credential populates while basic-auth
			// publishing is enabled.
			name: "AzureLinuxWebApp",
			kind: cloudresourcekind.CloudResourceKind_AzureLinuxWebApp,
			rawOutputs: map[string]interface{}{
				"web_app_id":                     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Web/sites/app-web",
				"default_hostname":               "app-web.azurewebsites.net",
				"outbound_ip_addresses":          []interface{}{"20.1.2.3", "20.1.2.4"},
				"possible_outbound_ip_addresses": []interface{}{"20.1.2.3", "20.1.2.4", "20.1.2.5"},
				"identity_principal_id":          "11111111-2222-3333-4444-555555555555",
				"identity_tenant_id":             "99999999-8888-7777-6666-555555555555",
				"custom_domain_verification_id":  "ABCD1234",
				"kind":                           "app,linux",
				"hosting_environment_id":         "",
				"site_credential_name":           "$app-web",
				"site_credential_password":       "publish-password",
			},
			mustPopulate: []string{
				"web_app_id", "default_hostname", "outbound_ip_addresses",
				"possible_outbound_ip_addresses", "identity_principal_id",
				"identity_tenant_id", "custom_domain_verification_id",
				"kind", "site_credential_name", "site_credential_password",
			},
		},
		{
			// AzureFunctionApp: default_hostname serves HTTP triggers;
			// the outbound IP sets arrive as real lists from both
			// engines; the site credential populates while basic-auth
			// publishing is enabled.
			name: "AzureFunctionApp",
			kind: cloudresourcekind.CloudResourceKind_AzureFunctionApp,
			rawOutputs: map[string]interface{}{
				"function_app_id":                "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Web/sites/app-fn",
				"default_hostname":               "app-fn.azurewebsites.net",
				"outbound_ip_addresses":          []interface{}{"20.1.2.3", "20.1.2.4"},
				"possible_outbound_ip_addresses": []interface{}{"20.1.2.3", "20.1.2.4", "20.1.2.5"},
				"identity_principal_id":          "11111111-2222-3333-4444-555555555555",
				"identity_tenant_id":             "99999999-8888-7777-6666-555555555555",
				"custom_domain_verification_id":  "ABCD1234",
				"kind":                           "functionapp,linux",
				"hosting_environment_id":         "",
				"site_credential_name":           "$app-fn",
				"site_credential_password":       "publish-password",
			},
			mustPopulate: []string{
				"function_app_id", "default_hostname",
				"outbound_ip_addresses", "possible_outbound_ip_addresses",
				"identity_principal_id", "identity_tenant_id",
				"custom_domain_verification_id", "kind",
				"site_credential_name", "site_credential_password",
			},
		},
		{
			// AzureManagedRedisGeoReplication: the group has no ARM
			// object of its own -- its resource ID is the managing
			// cluster's ARM ID.
			name: "AzureManagedRedisGeoReplication",
			kind: cloudresourcekind.CloudResourceKind_AzureManagedRedisGeoReplication,
			rawOutputs: map[string]interface{}{
				"geo_replication_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-east/providers/Microsoft.Cache/redisEnterprise/app-cache-east",
			},
			mustPopulate: []string{
				"geo_replication_id",
			},
		},
		{
			// AzureManagedRedisAccessPolicyAssignment: Azure names the
			// assignment after the granted object ID, so the name equals
			// the principal's GUID.
			name: "AzureManagedRedisAccessPolicyAssignment",
			kind: cloudresourcekind.CloudResourceKind_AzureManagedRedisAccessPolicyAssignment,
			rawOutputs: map[string]interface{}{
				"access_policy_assignment_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redisEnterprise/app-cache/databases/default/accessPolicyAssignments/11111111-2222-3333-4444-555555555555",
				"access_policy_assignment_name": "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"access_policy_assignment_id", "access_policy_assignment_name",
			},
		},
		{
			// AzureRedisCacheAccessPolicyAssignment: the grant half of the
			// keyless cache story -- id and name identify the grant for
			// audits and teardown.
			name: "AzureRedisCacheAccessPolicyAssignment",
			kind: cloudresourcekind.CloudResourceKind_AzureRedisCacheAccessPolicyAssignment,
			rawOutputs: map[string]interface{}{
				"access_policy_assignment_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cache/redis/app-cache/accessPolicyAssignments/app-identity-data-reader",
				"access_policy_assignment_name": "app-identity-data-reader",
			},
			mustPopulate: []string{
				"access_policy_assignment_id", "access_policy_assignment_name",
			},
		},
		{
			// AzureCosmosdbAccount: cosmosdb_account_id is what the SQL/Mongo
			// database kinds and private endpoints reference; the keys and
			// ready-made connection strings are the credential surface; the
			// per-region endpoint lists are repeated string outputs.
			name: "AzureCosmosdbAccount",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbAccount,
			rawOutputs: map[string]interface{}{
				"cosmosdb_account_id":                          "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos",
				"cosmosdb_account_name":                        "app-cosmos",
				"endpoint":                                     "https://app-cosmos.documents.azure.com:443/",
				"read_endpoints":                               []interface{}{"https://app-cosmos-eastus.documents.azure.com:443/", "https://app-cosmos-westus2.documents.azure.com:443/"},
				"write_endpoints":                              []interface{}{"https://app-cosmos-eastus.documents.azure.com:443/"},
				"primary_key":                                  "primary-key-value",
				"secondary_key":                                "secondary-key-value",
				"primary_readonly_key":                         "primary-readonly-key-value",
				"secondary_readonly_key":                       "secondary-readonly-key-value",
				"primary_sql_connection_string":                "AccountEndpoint=https://app-cosmos.documents.azure.com:443/;AccountKey=primary-key-value;",
				"secondary_sql_connection_string":              "AccountEndpoint=https://app-cosmos.documents.azure.com:443/;AccountKey=secondary-key-value;",
				"primary_readonly_sql_connection_string":       "AccountEndpoint=https://app-cosmos.documents.azure.com:443/;AccountKey=primary-readonly-key-value;",
				"secondary_readonly_sql_connection_string":     "AccountEndpoint=https://app-cosmos.documents.azure.com:443/;AccountKey=secondary-readonly-key-value;",
				"primary_mongodb_connection_string":            "mongodb://app-cosmos:primary-key-value@app-cosmos.mongo.cosmos.azure.com:10255/?ssl=true",
				"secondary_mongodb_connection_string":          "mongodb://app-cosmos:secondary-key-value@app-cosmos.mongo.cosmos.azure.com:10255/?ssl=true",
				"primary_readonly_mongodb_connection_string":   "mongodb://app-cosmos:primary-readonly-key-value@app-cosmos.mongo.cosmos.azure.com:10255/?ssl=true",
				"secondary_readonly_mongodb_connection_string": "mongodb://app-cosmos:secondary-readonly-key-value@app-cosmos.mongo.cosmos.azure.com:10255/?ssl=true",
				"identity_principal_id":                        "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"cosmosdb_account_id", "cosmosdb_account_name", "endpoint",
				"read_endpoints", "write_endpoints",
				"primary_key", "secondary_key",
				"primary_readonly_key", "secondary_readonly_key",
				"primary_sql_connection_string", "secondary_sql_connection_string",
				"primary_readonly_sql_connection_string", "secondary_readonly_sql_connection_string",
				"primary_mongodb_connection_string", "secondary_mongodb_connection_string",
				"primary_readonly_mongodb_connection_string", "secondary_readonly_mongodb_connection_string",
				"identity_principal_id",
			},
		},
		{
			// AzureCosmosdbSqlDatabase: sql_database_id is the seam
			// containers (AzureCosmosdbSqlContainer.sql_database_id)
			// reference; the account/database name pair is what SDK calls
			// consume inside the account's connection.
			name: "AzureCosmosdbSqlDatabase",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbSqlDatabase,
			rawOutputs: map[string]interface{}{
				"sql_database_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos/sqlDatabases/app-data",
				"sql_database_name":     "app-data",
				"cosmosdb_account_name": "app-cosmos",
			},
			mustPopulate: []string{
				"sql_database_id", "sql_database_name", "cosmosdb_account_name",
			},
		},
		{
			// AzureCosmosdbSqlContainer: sql_container_id is the
			// management identity and the container-level data-plane RBAC
			// scope; the name triple addresses the container inside the
			// account's connection.
			name: "AzureCosmosdbSqlContainer",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbSqlContainer,
			rawOutputs: map[string]interface{}{
				"sql_container_id":      "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos/sqlDatabases/app-data/containers/orders",
				"sql_container_name":    "orders",
				"sql_database_name":     "app-data",
				"cosmosdb_account_name": "app-cosmos",
			},
			mustPopulate: []string{
				"sql_container_id", "sql_container_name",
				"sql_database_name", "cosmosdb_account_name",
			},
		},
		{
			// AzureCosmosdbMongoDatabase: mongo_database_id is the seam
			// collections (AzureCosmosdbMongoCollection.mongo_database_id)
			// reference.
			name: "AzureCosmosdbMongoDatabase",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbMongoDatabase,
			rawOutputs: map[string]interface{}{
				"mongo_database_id":     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos-mongo/mongodbDatabases/app-data",
				"mongo_database_name":   "app-data",
				"cosmosdb_account_name": "app-cosmos-mongo",
			},
			mustPopulate: []string{
				"mongo_database_id", "mongo_database_name", "cosmosdb_account_name",
			},
		},
		{
			// AzureCosmosdbMongoCollection: the name triple addresses the
			// collection inside the account's Mongo connection string.
			name: "AzureCosmosdbMongoCollection",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbMongoCollection,
			rawOutputs: map[string]interface{}{
				"mongo_collection_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos-mongo/mongodbDatabases/app-data/collections/events",
				"mongo_collection_name": "events",
				"mongo_database_name":   "app-data",
				"cosmosdb_account_name": "app-cosmos-mongo",
			},
			mustPopulate: []string{
				"mongo_collection_id", "mongo_collection_name",
				"mongo_database_name", "cosmosdb_account_name",
			},
		},
		{
			// AzureCosmosdbSqlRoleDefinition: role_definition_id is the
			// fully-scoped ARM id an AzureCosmosdbSqlRoleAssignment's
			// role_definition_id field consumes with zero translation.
			name: "AzureCosmosdbSqlRoleDefinition",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbSqlRoleDefinition,
			rawOutputs: map[string]interface{}{
				"role_definition_id":    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos/sqlRoleDefinitions/9b7f3f6a-2f0e-4b9a-8f0d-2f6a8f0d2f6a",
				"role_definition_guid":  "9b7f3f6a-2f0e-4b9a-8f0d-2f6a8f0d2f6a",
				"role_name":             "app-reader",
				"cosmosdb_account_name": "app-cosmos",
			},
			mustPopulate: []string{
				"role_definition_id", "role_definition_guid",
				"role_name", "cosmosdb_account_name",
			},
		},
		{
			// AzureCosmosdbSqlRoleAssignment: the grant record's ARM
			// identity, exported for audit trails and cross-references.
			name: "AzureCosmosdbSqlRoleAssignment",
			kind: cloudresourcekind.CloudResourceKind_AzureCosmosdbSqlRoleAssignment,
			rawOutputs: map[string]interface{}{
				"role_assignment_id":    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.DocumentDB/databaseAccounts/app-cosmos/sqlRoleAssignments/7c1de3f8-5a4b-4c2d-9e8f-1a2b3c4d5e6f",
				"role_assignment_guid":  "7c1de3f8-5a4b-4c2d-9e8f-1a2b3c4d5e6f",
				"cosmosdb_account_name": "app-cosmos",
			},
			mustPopulate: []string{
				"role_assignment_id", "role_assignment_guid",
				"cosmosdb_account_name",
			},
		},
		{
			// AzureFrontDoorProfile: profile_id is the parent seam every
			// Front Door delivery kind (endpoint, origin group) references;
			// identity_principal_id is the Key Vault grant target for
			// bring-your-own TLS certificates.
			name: "AzureFrontDoorProfile",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorProfile,
			rawOutputs: map[string]interface{}{
				"profile_id":            "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd",
				"profile_name":          "app-fd",
				"resource_guid":         "11111111-2222-3333-4444-555555555555",
				"identity_principal_id": "66666666-7777-8888-9999-000000000000",
			},
			mustPopulate: []string{
				"profile_id", "profile_name", "resource_guid", "identity_principal_id",
			},
		},
		{
			// AzureFrontDoorEndpoint: endpoint_id is the route's parent
			// seam; host_name is the generated *.azurefd.net hostname DNS
			// records CNAME onto.
			name: "AzureFrontDoorEndpoint",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorEndpoint,
			rawOutputs: map[string]interface{}{
				"endpoint_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/afdEndpoints/web",
				"endpoint_name": "web",
				"host_name":     "web-abc123.z01.azurefd.net",
			},
			mustPopulate: []string{
				"endpoint_id", "endpoint_name", "host_name",
			},
		},
		{
			// AzureFrontDoorOriginGroup: origin_group_id is what origins
			// reference as parent and routes reference as destination.
			name: "AzureFrontDoorOriginGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorOriginGroup,
			rawOutputs: map[string]interface{}{
				"origin_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/originGroups/api-backends",
				"origin_group_name": "api-backends",
			},
			mustPopulate: []string{
				"origin_group_id", "origin_group_name",
			},
		},
		{
			// AzureFrontDoorOrigin: origin_id is what routes list in
			// origin_ids to sequence deployment after the backends exist.
			name: "AzureFrontDoorOrigin",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorOrigin,
			rawOutputs: map[string]interface{}{
				"origin_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/originGroups/api-backends/origins/primary",
				"origin_name": "primary",
			},
			mustPopulate: []string{
				"origin_id", "origin_name",
			},
		},
		{
			// AzureFrontDoorRoute: the traffic-serving edge of the Front
			// Door graph; no hostname output on purpose (it lives on the
			// endpoint).
			name: "AzureFrontDoorRoute",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorRoute,
			rawOutputs: map[string]interface{}{
				"route_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/afdEndpoints/web/routes/default",
				"route_name": "default",
			},
			mustPopulate: []string{
				"route_id", "route_name",
			},
		},
		{
			// AzureFrontDoorRuleSet: rule_set_id is what routes reference
			// in rule_set_ids to attach the delivery policy; the folded
			// rules export no ids on purpose (nothing references a rule).
			name: "AzureFrontDoorRuleSet",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorRuleSet,
			rawOutputs: map[string]interface{}{
				"rule_set_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/ruleSets/deliverypolicy",
				"rule_set_name": "deliverypolicy",
			},
			mustPopulate: []string{
				"rule_set_id", "rule_set_name",
			},
		},
		{
			// AzureFrontDoorCustomDomain: custom_domain_id is the route
			// attach seam; validation_token is the DNS TXT challenge the
			// operator publishes at _dnsauth.<host_name>.
			name: "AzureFrontDoorCustomDomain",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorCustomDomain,
			rawOutputs: map[string]interface{}{
				"custom_domain_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/customDomains/www-example-com",
				"host_name":        "www.example.com",
				"validation_token": "_zy4mhfvswrqzeqmnyv6gjr26xk1mbrv",
				"expiration_date":  "2026-07-16T00:00:00.000Z",
			},
			mustPopulate: []string{
				"custom_domain_id", "host_name", "validation_token", "expiration_date",
			},
		},
		{
			// AzureFrontDoorSecret: secret_id is the custom domain's
			// tls.secret_id seam; the SANs are read back from the wrapped
			// Key Vault certificate.
			name: "AzureFrontDoorSecret",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorSecret,
			rawOutputs: map[string]interface{}{
				"secret_id":                 "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/secrets/wildcard-example-com",
				"secret_name":               "wildcard-example-com",
				"subject_alternative_names": []interface{}{"*.example.com", "example.com"},
			},
			mustPopulate: []string{
				"secret_id", "secret_name", "subject_alternative_names",
			},
		},
		{
			// AzureFrontDoorFirewallPolicy: firewall_policy_id is what the
			// security policy references in firewall_policy_id to attach
			// the WAF to a profile's domains.
			name: "AzureFrontDoorFirewallPolicy",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorFirewallPolicy,
			rawOutputs: map[string]interface{}{
				"firewall_policy_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Network/frontDoorWebApplicationFirewallPolicies/edgewaf",
				"firewall_policy_name": "edgewaf",
			},
			mustPopulate: []string{
				"firewall_policy_id", "firewall_policy_name",
			},
		},
		{
			// AzureFrontDoorSecurityPolicy: the association itself --
			// nothing composes on it; the id serves operational
			// addressing.
			name: "AzureFrontDoorSecurityPolicy",
			kind: cloudresourcekind.CloudResourceKind_AzureFrontDoorSecurityPolicy,
			rawOutputs: map[string]interface{}{
				"security_policy_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.Cdn/profiles/app-fd/securityPolicies/edge-waf-attach",
				"security_policy_name": "edge-waf-attach",
			},
			mustPopulate: []string{
				"security_policy_id", "security_policy_name",
			},
		},
		{
			// AzureContainerAppEnvironment: environment_id is what every
			// kind living inside the environment references; the
			// platform-reserved values only populate for VNet-injected
			// environments but the output shape stays constant.
			name: "AzureContainerAppEnvironment",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppEnvironment,
			rawOutputs: map[string]interface{}{
				"environment_id":                   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env",
				"environment_name":                 "app-env",
				"default_domain":                   "app-env.eastus.azurecontainerapps.io",
				"static_ip_address":                "20.1.2.3",
				"platform_reserved_cidr":           "10.0.0.0/24",
				"platform_reserved_dns_ip_address": "10.0.0.2",
				"docker_bridge_cidr":               "172.17.0.1/16",
				"custom_domain_verification_id":    "ABCD1234",
				"identity_principal_id":            "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"environment_id", "environment_name", "default_domain",
				"static_ip_address", "platform_reserved_cidr",
				"platform_reserved_dns_ip_address", "docker_bridge_cidr",
				"custom_domain_verification_id", "identity_principal_id",
			},
		},
		{
			// AzureContainerApp: ingress_fqdn is the user-facing endpoint;
			// the outbound IPs arrive as a real list from both engines;
			// custom_domain_verification_id is provider-Sensitive (the TF
			// output carries sensitive = true).
			name: "AzureContainerApp",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerApp,
			rawOutputs: map[string]interface{}{
				"container_app_id":              "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/containerApps/app-web",
				"container_app_name":            "app-web",
				"latest_revision_name":          "app-web--abc123",
				"latest_revision_fqdn":          "app-web--abc123.app-env.eastus.azurecontainerapps.io",
				"outbound_ip_addresses":         []interface{}{"20.1.2.3", "20.1.2.4"},
				"ingress_fqdn":                  "app-web.app-env.eastus.azurecontainerapps.io",
				"custom_domain_verification_id": "ABCD1234",
				"identity_principal_id":         "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"container_app_id", "container_app_name",
				"latest_revision_name", "latest_revision_fqdn",
				"outbound_ip_addresses", "ingress_fqdn",
				"custom_domain_verification_id", "identity_principal_id",
			},
		},
		{
			// AzureContainerAppJob: job_id is the handle for starting
			// manual executions; event_stream_endpoint feeds execution
			// monitoring.
			name: "AzureContainerAppJob",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppJob,
			rawOutputs: map[string]interface{}{
				"job_id":                "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/jobs/nightly-report",
				"job_name":              "nightly-report",
				"event_stream_endpoint": "https://eastus.azurecontainerapps.dev/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/containerAppJobs/nightly-report/eventstream",
				"outbound_ip_addresses": []interface{}{"20.1.2.3", "20.1.2.4"},
				"identity_principal_id": "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"job_id", "job_name", "event_stream_endpoint",
				"outbound_ip_addresses", "identity_principal_id",
			},
		},
		{
			// AzureContainerAppEnvironmentStorage: storage_name is the
			// seam app and job volumes reference in storage_name.
			name: "AzureContainerAppEnvironmentStorage",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppEnvironmentStorage,
			rawOutputs: map[string]interface{}{
				"storage_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env/storages/app-data",
				"storage_name": "app-data",
			},
			mustPopulate: []string{
				"storage_id", "storage_name",
			},
		},
		{
			// AzureContainerAppEnvironmentDaprComponent: component_name is
			// what application code passes to the Dapr API.
			name: "AzureContainerAppEnvironmentDaprComponent",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppEnvironmentDaprComponent,
			rawOutputs: map[string]interface{}{
				"dapr_component_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env/daprComponents/statestore",
				"component_name":    "statestore",
			},
			mustPopulate: []string{
				"dapr_component_id", "component_name",
			},
		},
		{
			// AzureContainerAppEnvironmentCertificate: certificate_id is the
			// binding seam AzureContainerAppCustomDomain consumes; the
			// certificate facts feed expiry monitoring.
			name: "AzureContainerAppEnvironmentCertificate",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppEnvironmentCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id":  "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env/certificates/app.example.com",
				"subject_name":    "CN=app.example.com",
				"issuer":          "CN=R11, O=Let's Encrypt, C=US",
				"issue_date":      "2026-07-01T00:00:00+00:00",
				"expiration_date": "2026-09-29T00:00:00+00:00",
				"thumbprint":      "A1B2C3D4E5F60718293A4B5C6D7E8F9012345678",
			},
			mustPopulate: []string{
				"certificate_id", "subject_name", "issuer",
				"issue_date", "expiration_date", "thumbprint",
			},
		},
		{
			// AzureContainerAppEnvironmentManagedCertificate:
			// certificate_id identifies the Azure-issued certificate;
			// validation_token is informational once issuance completes.
			name: "AzureContainerAppEnvironmentManagedCertificate",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppEnvironmentManagedCertificate,
			rawOutputs: map[string]interface{}{
				"certificate_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env/managedCertificates/app-example-com",
				"validation_token": "0123456789abcdef0123456789abcdef",
			},
			mustPopulate: []string{
				"certificate_id", "validation_token",
			},
		},
		{
			// AzureContainerAppCustomDomain: custom_domain_id is the
			// providers' synthetic binding identifier (the binding lives
			// inside the app's ingress configuration, not as its own ARM
			// resource). managed_certificate_id is legitimately empty for
			// bring-your-own bindings and until Azure attaches the managed
			// certificate, so it is not asserted.
			name: "AzureContainerAppCustomDomain",
			kind: cloudresourcekind.CloudResourceKind_AzureContainerAppCustomDomain,
			rawOutputs: map[string]interface{}{
				"custom_domain_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/containerApps/web-app/customDomainName/app.example.com",
				"managed_certificate_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/app-rg/providers/Microsoft.App/managedEnvironments/app-env/managedCertificates/app-example-com",
			},
			mustPopulate: []string{
				"custom_domain_id", "managed_certificate_id",
			},
		},
		{
			// AzureDnsZone: zone_name (with resource_group_name) is the join
			// key AzureDnsRecord addresses record sets through; zone_id is
			// the ARM seam for kinds watching the zone (Front Door custom
			// domains, AKS web-app routing); name_servers is the registrar
			// delegation handoff.
			name: "AzureDnsZone",
			kind: cloudresourcekind.CloudResourceKind_AzureDnsZone,
			rawOutputs: map[string]interface{}{
				"zone_id":                   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/dns-rg/providers/Microsoft.Network/dnsZones/example.com",
				"zone_name":                 "example.com",
				"resource_group_name":       "dns-rg",
				"name_servers":              []interface{}{"ns1-05.azure-dns.com.", "ns2-05.azure-dns.net.", "ns3-05.azure-dns.org.", "ns4-05.azure-dns.info."},
				"max_number_of_record_sets": 10000,
			},
			mustPopulate: []string{
				"zone_id", "zone_name", "resource_group_name",
				"name_servers", "max_number_of_record_sets",
			},
		},
		{
			// AzureDnsRecord: record_id embeds the record type as its own
			// ARM path segment; fqdn is DNS's own trailing-dot spelling.
			name: "AzureDnsRecord",
			kind: cloudresourcekind.CloudResourceKind_AzureDnsRecord,
			rawOutputs: map[string]interface{}{
				"record_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/dns-rg/providers/Microsoft.Network/dnsZones/example.com/MX/@",
				"fqdn":      "example.com.",
			},
			mustPopulate: []string{
				"record_id", "fqdn",
			},
		},
		{
			// AzureLogAnalyticsWorkspace: workspace_id (the ARM id) is the FK
			// seam App Insights / AKS / Container Apps / diagnostic settings
			// reference; workspace_customer_id is the agent-facing GUID the
			// provider confusingly calls workspace_id.
			name: "AzureLogAnalyticsWorkspace",
			kind: cloudresourcekind.CloudResourceKind_AzureLogAnalyticsWorkspace,
			rawOutputs: map[string]interface{}{
				"workspace_id":          "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.OperationalInsights/workspaces/platform-law",
				"workspace_name":        "platform-law",
				"workspace_customer_id": "11111111-2222-3333-4444-555555555555",
				"resource_group_name":   "obs-rg",
				"primary_shared_key":    "cHJpbWFyeS1rZXk=",
				"secondary_shared_key":  "c2Vjb25kYXJ5LWtleQ==",
				"identity_principal_id": "99999999-8888-7777-6666-555555555555",
			},
			mustPopulate: []string{
				"workspace_id", "workspace_name", "workspace_customer_id",
				"resource_group_name", "primary_shared_key",
				"secondary_shared_key", "identity_principal_id",
			},
		},
		{
			// AzureApplicationInsights: connection_string is the seam the
			// app-hosting kinds reference.
			name: "AzureApplicationInsights",
			kind: cloudresourcekind.CloudResourceKind_AzureApplicationInsights,
			rawOutputs: map[string]interface{}{
				"application_insights_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.Insights/components/platform-appinsights",
				"application_insights_name": "platform-appinsights",
				"instrumentation_key":       "22222222-3333-4444-5555-666666666666",
				"connection_string":         "InstrumentationKey=22222222-3333-4444-5555-666666666666;IngestionEndpoint=https://eastus-8.in.applicationinsights.azure.com/",
				"app_id":                    "77777777-8888-9999-aaaa-bbbbbbbbbbbb",
			},
			mustPopulate: []string{
				"application_insights_id", "application_insights_name",
				"instrumentation_key", "connection_string", "app_id",
			},
		},
		{
			// AzureMonitorDiagnosticSetting: the id is the CONSTRUCTED ARM
			// extension-resource id (the provider's own state id is a
			// "{target}|{name}" composite no API consumes).
			name: "AzureMonitorDiagnosticSetting",
			kind: cloudresourcekind.CloudResourceKind_AzureMonitorDiagnosticSetting,
			rawOutputs: map[string]interface{}{
				"diagnostic_setting_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.KeyVault/vaults/app-vault/providers/Microsoft.Insights/diagnosticSettings/route-to-law",
				"diagnostic_setting_name": "route-to-law",
				"target_resource_id":      "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.KeyVault/vaults/app-vault",
			},
			mustPopulate: []string{
				"diagnostic_setting_id", "diagnostic_setting_name", "target_resource_id",
			},
		},
		{
			// AzureMonitorActionGroup: action_group_id is the seam alert
			// rules reference.
			name: "AzureMonitorActionGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureMonitorActionGroup,
			rawOutputs: map[string]interface{}{
				"action_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.Insights/actionGroups/platform-oncall",
				"action_group_name": "platform-oncall",
			},
			mustPopulate: []string{
				"action_group_id", "action_group_name",
			},
		},
		{
			name: "AzureMonitorMetricAlert",
			kind: cloudresourcekind.CloudResourceKind_AzureMonitorMetricAlert,
			rawOutputs: map[string]interface{}{
				"metric_alert_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.Insights/metricAlerts/storage-availability",
				"metric_alert_name": "storage-availability",
			},
			mustPopulate: []string{
				"metric_alert_id", "metric_alert_name",
			},
		},
		{
			name: "AzureMonitorScheduledQueryAlert",
			kind: cloudresourcekind.CloudResourceKind_AzureMonitorScheduledQueryAlert,
			rawOutputs: map[string]interface{}{
				"scheduled_query_alert_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/obs-rg/providers/Microsoft.Insights/scheduledQueryRules/error-spike",
				"scheduled_query_alert_name": "error-spike",
				"identity_principal_id":      "33333333-4444-5555-6666-777777777777",
			},
			mustPopulate: []string{
				"scheduled_query_alert_id", "scheduled_query_alert_name",
				"identity_principal_id",
			},
		},
		{
			// AzureServiceBusNamespace: namespace_id is the parent seam every
			// Service Bus child kind references (queue, topic, authorization
			// rule, geo-DR pairing); the root SAS rule's four credential
			// faces are the quick-start connection surface.
			name: "AzureServiceBusNamespace",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusNamespace,
			rawOutputs: map[string]interface{}{
				"namespace_id":                        "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus",
				"namespace_name":                      "orders-bus",
				"endpoint":                            "https://orders-bus.servicebus.windows.net:443/",
				"identity_principal_id":               "55555555-6666-7777-8888-999999999999",
				"default_primary_connection_string":   "Endpoint=sb://orders-bus.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key==",
				"default_secondary_connection_string": "Endpoint=sb://orders-bus.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key2==",
				"default_primary_key":                 "base64key==",
				"default_secondary_key":               "base64key2==",
			},
			mustPopulate: []string{
				"namespace_id", "namespace_name", "endpoint",
				"identity_principal_id", "default_primary_connection_string",
				"default_secondary_connection_string", "default_primary_key",
				"default_secondary_key",
			},
		},
		{
			// AzureServiceBusQueue: queue_id is the data-plane RBAC scope and
			// the parent seam queue-scoped SAS rules reference; the
			// namespace/queue name pair is what SDK clients and function
			// bindings consume.
			name: "AzureServiceBusQueue",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusQueue,
			rawOutputs: map[string]interface{}{
				"queue_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus/queues/orders",
				"queue_name":     "orders",
				"namespace_name": "orders-bus",
			},
			mustPopulate: []string{
				"queue_id", "queue_name", "namespace_name",
			},
		},
		{
			// AzureServiceBusTopic: topic_id is the parent seam subscriptions
			// and topic-scoped SAS rules reference.
			name: "AzureServiceBusTopic",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusTopic,
			rawOutputs: map[string]interface{}{
				"topic_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus/topics/events",
				"topic_name":     "events",
				"namespace_name": "orders-bus",
			},
			mustPopulate: []string{
				"topic_id", "topic_name", "namespace_name",
			},
		},
		{
			// AzureServiceBusSubscription: consumers receive by the
			// namespace/topic/subscription triple.
			name: "AzureServiceBusSubscription",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusSubscription,
			rawOutputs: map[string]interface{}{
				"subscription_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus/topics/events/subscriptions/audit",
				"subscription_name": "audit",
				"topic_name":        "events",
				"namespace_name":    "orders-bus",
			},
			mustPopulate: []string{
				"subscription_id", "subscription_name", "topic_name",
				"namespace_name",
			},
		},
		{
			// AzureServiceBusAuthorizationRule: authorization_rule_id is the
			// seam the geo-DR pairing's alias_authorization_rule_id consumes;
			// the six key/connection-string faces are the least-privilege
			// credential surface applications hold.
			name: "AzureServiceBusAuthorizationRule",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusAuthorizationRule,
			rawOutputs: map[string]interface{}{
				"authorization_rule_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus/queues/orders/authorizationRules/orders-sender",
				"rule_name":                         "orders-sender",
				"primary_key":                       "base64key==",
				"secondary_key":                     "base64key2==",
				"primary_connection_string":         "Endpoint=sb://orders-bus.servicebus.windows.net/;SharedAccessKeyName=orders-sender;SharedAccessKey=base64key==;EntityPath=orders",
				"secondary_connection_string":       "Endpoint=sb://orders-bus.servicebus.windows.net/;SharedAccessKeyName=orders-sender;SharedAccessKey=base64key2==;EntityPath=orders",
				"primary_connection_string_alias":   "",
				"secondary_connection_string_alias": "",
			},
			mustPopulate: []string{
				"authorization_rule_id", "rule_name", "primary_key",
				"secondary_key", "primary_connection_string",
				"secondary_connection_string",
			},
		},
		{
			// AzureServiceBusDisasterRecoveryConfig: the alias connection
			// strings are what DR-aware clients hold -- they survive a
			// failover without reconfiguration.
			name: "AzureServiceBusDisasterRecoveryConfig",
			kind: cloudresourcekind.CloudResourceKind_AzureServiceBusDisasterRecoveryConfig,
			rawOutputs: map[string]interface{}{
				"disaster_recovery_config_id":       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/msg-rg/providers/Microsoft.ServiceBus/namespaces/orders-bus-eastus/disasterRecoveryConfigs/orders-bus-alias",
				"alias_name":                        "orders-bus-alias",
				"primary_connection_string_alias":   "Endpoint=sb://orders-bus-alias.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key==",
				"secondary_connection_string_alias": "Endpoint=sb://orders-bus-alias.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key2==",
				"default_primary_key":               "base64key==",
				"default_secondary_key":             "base64key2==",
			},
			mustPopulate: []string{
				"disaster_recovery_config_id", "alias_name",
				"primary_connection_string_alias",
				"secondary_connection_string_alias", "default_primary_key",
				"default_secondary_key",
			},
		},
		{
			// AzureEventHubNamespace: namespace_id is the parent seam every
			// Event Hubs child kind references (hub, authorization rule,
			// schema group, geo-DR pairing, CMK); the root SAS rule's
			// credential faces (incl. the geo-DR alias pair) are the
			// quick-start connection surface.
			name: "AzureEventHubNamespace",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubNamespace,
			rawOutputs: map[string]interface{}{
				"namespace_id":                              "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs",
				"namespace_name":                            "telemetry-hubs",
				"identity_principal_id":                     "55555555-6666-7777-8888-999999999999",
				"default_primary_connection_string":         "Endpoint=sb://telemetry-hubs.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key==",
				"default_secondary_connection_string":       "Endpoint=sb://telemetry-hubs.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=base64key2==",
				"default_primary_key":                       "base64key==",
				"default_secondary_key":                     "base64key2==",
				"default_primary_connection_string_alias":   "",
				"default_secondary_connection_string_alias": "",
			},
			mustPopulate: []string{
				"namespace_id", "namespace_name", "identity_principal_id",
				"default_primary_connection_string",
				"default_secondary_connection_string", "default_primary_key",
				"default_secondary_key",
			},
		},
		{
			// AzureEventHub: event_hub_id is the parent seam consumer groups
			// and hub-scoped SAS rules reference; partition_ids is the
			// repeated output partition-aware consumers enumerate.
			name: "AzureEventHub",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHub,
			rawOutputs: map[string]interface{}{
				"event_hub_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs/eventhubs/telemetry",
				"event_hub_name": "telemetry",
				"partition_ids":  []interface{}{"0", "1", "2", "3"},
			},
			mustPopulate: []string{
				"event_hub_id", "event_hub_name", "partition_ids",
			},
		},
		{
			// AzureEventHubConsumerGroup: the group name is what consumer
			// applications pass to their SDK client alongside the hub name.
			name: "AzureEventHubConsumerGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubConsumerGroup,
			rawOutputs: map[string]interface{}{
				"consumer_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs/eventhubs/telemetry/consumergroups/analytics",
				"consumer_group_name": "analytics",
			},
			mustPopulate: []string{
				"consumer_group_id", "consumer_group_name",
			},
		},
		{
			// AzureEventHubAuthorizationRule: identical credential faces
			// regardless of scope; the alias pair is only populated when a
			// geo-DR pairing exists.
			name: "AzureEventHubAuthorizationRule",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubAuthorizationRule,
			rawOutputs: map[string]interface{}{
				"authorization_rule_id":             "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs/eventhubs/telemetry/authorizationRules/producer-send",
				"rule_name":                         "producer-send",
				"primary_key":                       "base64key==",
				"secondary_key":                     "base64key2==",
				"primary_connection_string":         "Endpoint=sb://telemetry-hubs.servicebus.windows.net/;SharedAccessKeyName=producer-send;SharedAccessKey=base64key==;EntityPath=telemetry",
				"secondary_connection_string":       "Endpoint=sb://telemetry-hubs.servicebus.windows.net/;SharedAccessKeyName=producer-send;SharedAccessKey=base64key2==;EntityPath=telemetry",
				"primary_connection_string_alias":   "",
				"secondary_connection_string_alias": "",
			},
			mustPopulate: []string{
				"authorization_rule_id", "rule_name", "primary_key",
				"secondary_key", "primary_connection_string",
				"secondary_connection_string",
			},
		},
		{
			// AzureEventHubDisasterRecoveryConfig: alias credentials
			// deliberately live on the namespace/authorization-rule kinds
			// (Azure's own surface) -- this kind exports the pairing
			// identity only.
			name: "AzureEventHubDisasterRecoveryConfig",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubDisasterRecoveryConfig,
			rawOutputs: map[string]interface{}{
				"disaster_recovery_config_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs/disasterRecoveryConfigs/telemetry-alias",
				"alias_name":                  "telemetry-alias",
			},
			mustPopulate: []string{
				"disaster_recovery_config_id", "alias_name",
			},
		},
		{
			// AzureEventHubSchemaGroup: the group name is what
			// schema-registry serializers address at runtime.
			name: "AzureEventHubSchemaGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubSchemaGroup,
			rawOutputs: map[string]interface{}{
				"schema_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs/schemagroups/telemetry-schemas",
				"schema_group_name": "telemetry-schemas",
			},
			mustPopulate: []string{
				"schema_group_id", "schema_group_name",
			},
		},
		{
			// AzureEventHubCluster: cluster_id is the seam a namespace's
			// dedicated_cluster_id references for single-tenant placement.
			name: "AzureEventHubCluster",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubCluster,
			rawOutputs: map[string]interface{}{
				"cluster_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/clusters/streaming-dedicated",
				"cluster_name": "streaming-dedicated",
			},
			mustPopulate: []string{
				"cluster_id", "cluster_name",
			},
		},
		{
			// AzureEventHubNamespaceCustomerManagedKey: the configuration is
			// a property of the namespace (no ARM object of its own), so its
			// identity output is the namespace's ARM id.
			name: "AzureEventHubNamespaceCustomerManagedKey",
			kind: cloudresourcekind.CloudResourceKind_AzureEventHubNamespaceCustomerManagedKey,
			rawOutputs: map[string]interface{}{
				"customer_managed_key_id": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/stream-rg/providers/Microsoft.EventHub/namespaces/telemetry-hubs",
			},
			mustPopulate: []string{
				"customer_managed_key_id",
			},
		},
		{
			// AzureFirewallPolicy: the policy's ARM id is the composition
			// seam (rule collection groups nest under it, firewalls attach
			// it, child policies inherit from it) plus the system identity's
			// principal for Key Vault grants.
			name: "AzureFirewallPolicy",
			kind: cloudresourcekind.CloudResourceKind_AzureFirewallPolicy,
			rawOutputs: map[string]interface{}{
				"firewall_policy_id":    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/firewallPolicies/egress-baseline",
				"firewall_policy_name":  "egress-baseline",
				"identity_principal_id": "11111111-2222-3333-4444-555555555555",
			},
			mustPopulate: []string{
				"firewall_policy_id", "firewall_policy_name", "identity_principal_id",
			},
		},
		{
			// AzureFirewallPolicyRuleCollectionGroup: the group's ARM id
			// (nested under its parent policy) and name.
			name: "AzureFirewallPolicyRuleCollectionGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureFirewallPolicyRuleCollectionGroup,
			rawOutputs: map[string]interface{}{
				"rule_collection_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/firewallPolicies/egress-baseline/ruleCollectionGroups/platform-baseline",
				"rule_collection_group_name": "platform-baseline",
			},
			mustPopulate: []string{
				"rule_collection_group_id", "rule_collection_group_name",
			},
		},
		{
			// AzureFirewall: the firewall's ARM id plus its data-path
			// private IP -- THE hub-spoke seam route tables send egress to
			// via a VIRTUAL_APPLIANCE next hop. Hub-deployment outputs stay
			// empty on a VNet firewall.
			name: "AzureFirewall",
			kind: cloudresourcekind.CloudResourceKind_AzureFirewall,
			rawOutputs: map[string]interface{}{
				"firewall_id":                     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/azureFirewalls/hub-egress-fw",
				"firewall_name":                   "hub-egress-fw",
				"private_ip_address":              "10.0.255.4",
				"management_private_ip_address":   "10.0.254.4",
				"virtual_hub_public_ip_addresses": []interface{}{},
				"virtual_hub_private_ip_address":  "",
			},
			mustPopulate: []string{
				"firewall_id", "firewall_name", "private_ip_address",
			},
		},
		{
			// AzureIpGroup: the group's ARM id is the composition seam --
			// firewall policy rules and IDPS bypasses reference it to
			// target the address set.
			name: "AzureIpGroup",
			kind: cloudresourcekind.CloudResourceKind_AzureIpGroup,
			rawOutputs: map[string]interface{}{
				"ip_group_id":   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/network-rg/providers/Microsoft.Network/ipGroups/branch-offices",
				"ip_group_name": "branch-offices",
			},
			mustPopulate: []string{
				"ip_group_id", "ip_group_name",
			},
		},
		{
			// AwsCertManagerCert: cert_arn is the join key every TLS consumer
			// references (listeners, CloudFront, Cognito, OpenSearch, Client
			// VPN); domain_validation_records guards the repeated-message
			// shape external-DNS users consume to create their validation
			// CNAMEs; status is what the E2E verifier keys on (a no-zone cert
			// rests in PENDING_VALIDATION).
			name: "AwsCertManagerCert",
			kind: cloudresourcekind.CloudResourceKind_AwsCertManagerCert,
			rawOutputs: map[string]interface{}{
				"cert_arn": "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
				"status":   "PENDING_VALIDATION",
				"domain_validation_records": []interface{}{
					map[string]interface{}{
						"domain_name":  "example.com",
						"record_name":  "_3839f23e624907e70b9e.example.com.",
						"record_type":  "CNAME",
						"record_value": "_632077f7a35f9d.mhbtsbpdnt.acm-validations.aws.",
					},
				},
				"not_before":       "",
				"not_after":        "",
				"certificate_type": "AMAZON_ISSUED",
			},
			mustPopulate: []string{
				"cert_arn", "status", "certificate_type", "domain_validation_records",
			},
		},
		{
			// AwsCloudFront: distribution_id keys the E2E verifier and
			// invalidation requests; domain_name + hosted_zone_id are what
			// Route53 alias records compose against; distribution_arn is the
			// WAF-association join key.
			name: "AwsCloudFront",
			kind: cloudresourcekind.CloudResourceKind_AwsCloudFront,
			rawOutputs: map[string]interface{}{
				"distribution_id":  "E2ABCDEF123456",
				"distribution_arn": "arn:aws:cloudfront::123456789012:distribution/E2ABCDEF123456",
				"domain_name":      "d123abc456def.cloudfront.net",
				"hosted_zone_id":   "Z2FDTNDATAQYW2",
				"status":           "Deployed",
			},
			mustPopulate: []string{
				"distribution_id", "distribution_arn", "domain_name",
				"hosted_zone_id", "status",
			},
		},
		{
			// KubernetesSparkOperator: the release identity plus the
			// workload contract — the service account SparkApplications
			// reference and the watch fence (a repeated string, exercising
			// the list flattening path).
			name: "KubernetesSparkOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesSparkOperator,
			rawOutputs: map[string]interface{}{
				"namespace":                "spark-operator",
				"release_name":             "spark-operator",
				"workload_service_account": "spark",
				"watched_namespaces":       []interface{}{"data-pipelines", "ml-jobs"},
			},
			mustPopulate: []string{
				"namespace", "release_name", "workload_service_account",
				"watched_namespaces",
			},
		},
		{
			// KubernetesKubeRayOperator: the release identity plus the
			// watch fence.
			name: "KubernetesKubeRayOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesKubeRayOperator,
			rawOutputs: map[string]interface{}{
				"namespace":          "ray-system",
				"release_name":       "kuberay-operator",
				"watched_namespaces": []interface{}{"ml-team-a"},
			},
			mustPopulate: []string{
				"namespace", "release_name", "watched_namespaces",
			},
		},
		{
			// KubernetesRayCluster: the head-service endpoint trio and the
			// nested bearer-token Secret handle — the nested-message
			// flattening class this guard exists for (both engines must
			// emit "auth_token_secret.name"/".key", never a flat
			// "auth_token_secret_name").
			name: "KubernetesRayCluster",
			kind: cloudresourcekind.CloudResourceKind_KubernetesRayCluster,
			rawOutputs: map[string]interface{}{
				"namespace":          "ml-platform",
				"head_service":       "ml-ray-head-svc",
				"client_endpoint":    "ml-ray-head-svc.ml-platform.svc.cluster.local:10001",
				"dashboard_endpoint": "ml-ray-head-svc.ml-platform.svc.cluster.local:8265",
				"gcs_endpoint":       "ml-ray-head-svc.ml-platform.svc.cluster.local:6379",
				"auth_token_secret": map[string]interface{}{
					"name": "ml-ray-auth-token",
					"key":  "auth_token",
				},
				"port_forward_command": "kubectl port-forward -n ml-platform service/ml-ray-head-svc 8265:8265",
			},
			mustPopulate: []string{
				"namespace", "head_service", "client_endpoint",
				"dashboard_endpoint", "gcs_endpoint", "auth_token_secret",
				"port_forward_command",
			},
		},
		{
			// KubernetesFlinkOperator: the release identity, the runner
			// identity contract, the watch fence, and the chart-fixed
			// webhook Service handle.
			name: "KubernetesFlinkOperator",
			kind: cloudresourcekind.CloudResourceKind_KubernetesFlinkOperator,
			rawOutputs: map[string]interface{}{
				"namespace":           "flink-system",
				"release_name":        "flink-operator",
				"job_service_account": "flink",
				"watched_namespaces":  []interface{}{"stream-team-a"},
				"webhook_service":     "flink-operator-webhook-service",
			},
			mustPopulate: []string{
				"namespace", "release_name", "job_service_account",
				"watched_namespaces", "webhook_service",
			},
		},
		{
			// KubernetesFlinkDeployment: the REST service naming contract
			// (`<name>-rest`) every exposure and job submission composes
			// against.
			name: "KubernetesFlinkDeployment",
			kind: cloudresourcekind.CloudResourceKind_KubernetesFlinkDeployment,
			rawOutputs: map[string]interface{}{
				"namespace":            "stream-processing",
				"rest_service":         "orders-pipeline-rest",
				"rest_endpoint":        "orders-pipeline-rest.stream-processing.svc.cluster.local:8081",
				"port_forward_command": "kubectl port-forward -n stream-processing service/orders-pipeline-rest 8081:8081",
			},
			mustPopulate: []string{
				"namespace", "rest_service", "rest_endpoint",
				"port_forward_command",
			},
		},
		{
			// KubernetesLocust: the master Service handles (web +
			// worker-connect endpoints) and the module-generated web-UI
			// login credential handle.
			name: "KubernetesLocust",
			kind: cloudresourcekind.CloudResourceKind_KubernetesLocust,
			rawOutputs: map[string]interface{}{
				"namespace":            "load-test",
				"master_service":       "load-test",
				"web_endpoint":         "http://load-test.load-test.svc.cluster.local:8089",
				"master_bind_endpoint": "load-test.load-test.svc.cluster.local:5557",
				"web_ui_username":      "locust",
				"web_ui_password_secret": map[string]interface{}{
					"name": "load-test-auth",
					"key":  "password",
				},
				"port_forward_command": "kubectl port-forward svc/load-test -n load-test 8089:8089",
			},
			mustPopulate: []string{
				"namespace", "master_service", "web_endpoint",
				"master_bind_endpoint", "web_ui_username",
				"web_ui_password_secret", "port_forward_command",
			},
		},
		{
			// AwsSecretsManagerSecret: secret_arn keys the E2E verifier and
			// is the canonical join key (AWS appends a random suffix, so the
			// ARN is never derivable from the name); version_id is exported
			// in EVERY arm (empty for a shell secret) so the output shape is
			// engine- and configuration-invariant.
			name: "AwsSecretsManagerSecret",
			kind: cloudresourcekind.CloudResourceKind_AwsSecretsManagerSecret,
			rawOutputs: map[string]interface{}{
				"secret_arn":  "arn:aws:secretsmanager:us-west-2:123456789012:secret:prod/payments/db-AbCdEf",
				"secret_name": "prod/payments/db",
				"version_id":  "12345678-90ab-cdef-1234-567890abcdef",
			},
			mustPopulate: []string{
				"secret_arn", "secret_name", "version_id",
			},
		},
		{
			// AwsOpenSearchServerlessCollection: collection_name keys the
			// E2E verifier; collection_arn is the vector-store join key for
			// Bedrock knowledge bases; the endpoints are what applications
			// and Dashboards users connect to; kms_key_arn echoes the
			// effective key (AWS-owned or customer-managed).
			name: "AwsOpenSearchServerlessCollection",
			kind: cloudresourcekind.CloudResourceKind_AwsOpenSearchServerlessCollection,
			rawOutputs: map[string]interface{}{
				"collection_id":       "a1b2c3d4e5f6g7",
				"collection_arn":      "arn:aws:aoss:us-west-2:123456789012:collection/a1b2c3d4e5f6g7",
				"collection_name":     "app-search",
				"collection_endpoint": "https://a1b2c3d4e5f6g7.us-west-2.aoss.amazonaws.com",
				"dashboard_endpoint":  "https://a1b2c3d4e5f6g7.us-west-2.aoss.amazonaws.com/_dashboards",
				"kms_key_arn":         "arn:aws:kms:us-west-2:123456789012:key/aws-owned",
			},
			mustPopulate: []string{
				"collection_id", "collection_arn", "collection_name",
				"collection_endpoint", "dashboard_endpoint", "kms_key_arn",
			},
		},
		{
			// AwsBedrockGuardrail: guardrail_id keys the E2E verifier;
			// consumers pin guardrail_id + a version_numbers entry (the
			// name-keyed published-version map -- AWS assigns the numbers);
			// draft_version is the literal DRAFT.
			name: "AwsBedrockGuardrail",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockGuardrail,
			rawOutputs: map[string]interface{}{
				"guardrail_id":  "gr1a2b3c4d5e",
				"guardrail_arn": "arn:aws:bedrock:us-west-2:123456789012:guardrail/gr1a2b3c4d5e",
				"draft_version": "DRAFT",
				"version_numbers": map[string]interface{}{
					"prod": "1",
				},
			},
			mustPopulate: []string{
				"guardrail_id", "guardrail_arn", "draft_version", "version_numbers",
			},
		},
		{
			// AwsBedrockCustomModel: job_arn keys the E2E verifier (the job
			// is the tracked object); custom_model_arn is what provisioned
			// throughput references; job_status reports the async training
			// outcome.
			name: "AwsBedrockCustomModel",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockCustomModel,
			rawOutputs: map[string]interface{}{
				"custom_model_arn":  "arn:aws:bedrock:us-east-1:123456789012:custom-model/amazon.titan-text-lite-v1/abc123def456",
				"custom_model_name": "support-titan-ft",
				"job_arn":           "arn:aws:bedrock:us-east-1:123456789012:model-customization-job/amazon.titan-text-lite-v1/xyz789",
				"job_status":        "InProgress",
			},
			mustPopulate: []string{
				"custom_model_arn", "custom_model_name", "job_arn", "job_status",
			},
		},
		{
			// AwsBedrockInferenceProfile: inference_profile_id keys the E2E
			// verifier; the ARN is the modelId applications invoke through.
			name: "AwsBedrockInferenceProfile",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockInferenceProfile,
			rawOutputs: map[string]interface{}{
				"inference_profile_arn": "arn:aws:bedrock:us-west-2:123456789012:application-inference-profile/checkout-nova",
				"inference_profile_id":  "checkout-nova",
				"status":                "ACTIVE",
				"type":                  "APPLICATION",
			},
			mustPopulate: []string{
				"inference_profile_arn", "inference_profile_id", "status", "type",
			},
		},
		{
			// AwsBedrockProvisionedThroughput: provisioned_model_arn keys
			// the E2E verifier and is the modelId applications invoke
			// through.
			name: "AwsBedrockProvisionedThroughput",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockProvisionedThroughput,
			rawOutputs: map[string]interface{}{
				"provisioned_model_arn":  "arn:aws:bedrock:us-east-1:123456789012:provisioned-model/1y5n57gh5y2e",
				"provisioned_model_name": "support-model-capacity",
			},
			mustPopulate: []string{
				"provisioned_model_arn", "provisioned_model_name",
			},
		},
		{
			// AwsBedrockModelAccess: model_id keys the E2E verifier and is
			// the chart-ordering join key for model-consuming components.
			name: "AwsBedrockModelAccess",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockModelAccess,
			rawOutputs: map[string]interface{}{
				"model_id": "mistral.mistral-7b-instruct-v0:2",
			},
			mustPopulate: []string{
				"model_id",
			},
		},
		{
			// AwsBedrockAgent: agent_id keys the E2E verifier; the
			// name-keyed satellite maps (alias_arns especially -- the
			// value supervisors, prompts, and flows consume) feed the
			// keyed-by-address import derivations.
			name: "AwsBedrockAgent",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgent,
			rawOutputs: map[string]interface{}{
				"agent_id":      "GGRRAED6JP",
				"agent_arn":     "arn:aws:bedrock:us-west-2:123456789012:agent/GGRRAED6JP",
				"draft_version": "DRAFT",
				"alias_ids": map[string]interface{}{
					"live": "66IVY0GUTF",
				},
				"alias_arns": map[string]interface{}{
					"live": "arn:aws:bedrock:us-west-2:123456789012:agent-alias/GGRRAED6JP/66IVY0GUTF",
				},
				"action_group_ids": map[string]interface{}{
					"orders": "MMAUDBZTH4",
				},
				"collaborator_ids": map[string]interface{}{
					"billing": "AG3TN4RQIY",
				},
				"associated_knowledge_base_ids": map[string]interface{}{
					"docs": "EMDPPAYPZI",
				},
			},
			mustPopulate: []string{
				"agent_id", "agent_arn", "draft_version", "alias_ids",
				"alias_arns", "action_group_ids", "collaborator_ids",
				"associated_knowledge_base_ids",
			},
		},
		{
			// AwsBedrockKnowledgeBase: knowledge_base_id keys the E2E
			// verifier and is what agents and flows consume; the
			// name-keyed data_source_ids map feeds the keyed-by-address
			// import derivations.
			name: "AwsBedrockKnowledgeBase",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockKnowledgeBase,
			rawOutputs: map[string]interface{}{
				"knowledge_base_id":  "EMDPPAYPZI",
				"knowledge_base_arn": "arn:aws:bedrock:us-west-2:123456789012:knowledge-base/EMDPPAYPZI",
				"data_source_ids": map[string]interface{}{
					"docs": "GWCMFMQF6T",
				},
			},
			mustPopulate: []string{
				"knowledge_base_id", "knowledge_base_arn", "data_source_ids",
			},
		},
		{
			// AwsBedrockFlow: flow_id keys the E2E verifier; flow_arn is
			// the invocation key; draft_version is the literal DRAFT.
			name: "AwsBedrockFlow",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockFlow,
			rawOutputs: map[string]interface{}{
				"flow_id":       "FLOWID1234",
				"flow_arn":      "arn:aws:bedrock:us-west-2:123456789012:flow/FLOWID1234",
				"draft_version": "DRAFT",
			},
			mustPopulate: []string{
				"flow_id", "flow_arn", "draft_version",
			},
		},
		{
			// AwsBedrockPrompt: prompt_id keys the E2E verifier;
			// prompt_arn is what a flow's prompt node consumes;
			// draft_version is the literal DRAFT.
			name: "AwsBedrockPrompt",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockPrompt,
			rawOutputs: map[string]interface{}{
				"prompt_id":     "1A2BC3DEFG",
				"prompt_arn":    "arn:aws:bedrock:us-west-2:123456789012:prompt/1A2BC3DEFG",
				"draft_version": "DRAFT",
			},
			mustPopulate: []string{
				"prompt_id", "prompt_arn", "draft_version",
			},
		},
		{
			// AwsBedrockAgentCoreRuntime: agent_runtime_id keys the E2E
			// verifier; agent_runtime_arn is what gateway HTTP targets
			// and the resource policy consume; the name-keyed
			// endpoint_arns map feeds the keyed-by-address import
			// derivations (an endpoint's AWS identity IS its name).
			name: "AwsBedrockAgentCoreRuntime",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgentCoreRuntime,
			rawOutputs: map[string]interface{}{
				"agent_runtime_id":      "support_agent-Ab1Cd2Ef3G",
				"agent_runtime_arn":     "arn:aws:bedrock-agentcore:us-west-2:123456789012:runtime/support_agent-Ab1Cd2Ef3G",
				"agent_runtime_version": "1",
				"workload_identity_arn": "arn:aws:bedrock-agentcore:us-west-2:123456789012:workload-identity-directory/default/workload-identity/support_agent-Ab1Cd2Ef3G",
				"endpoint_arns": map[string]interface{}{
					"live": "arn:aws:bedrock-agentcore:us-west-2:123456789012:runtime/support_agent-Ab1Cd2Ef3G/runtime-endpoint/live",
				},
			},
			mustPopulate: []string{
				"agent_runtime_id", "agent_runtime_arn",
				"agent_runtime_version", "workload_identity_arn",
				"endpoint_arns",
			},
		},
		{
			// AwsBedrockAgentCoreGateway: gateway_id keys the E2E
			// verifier; gateway_url is the MCP URL agents connect to;
			// the name-keyed target_ids map feeds the keyed-by-address
			// import derivations.
			name: "AwsBedrockAgentCoreGateway",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgentCoreGateway,
			rawOutputs: map[string]interface{}{
				"gateway_id":            "support-tools-abc123de45",
				"gateway_arn":           "arn:aws:bedrock-agentcore:us-west-2:123456789012:gateway/support-tools-abc123de45",
				"gateway_url":           "https://support-tools-abc123de45.gateway.bedrock-agentcore.us-west-2.amazonaws.com/mcp",
				"workload_identity_arn": "arn:aws:bedrock-agentcore:us-west-2:123456789012:workload-identity-directory/default/workload-identity/support-tools-abc123de45",
				"target_ids": map[string]interface{}{
					"orders": "TGT123ABC4",
				},
			},
			mustPopulate: []string{
				"gateway_id", "gateway_arn", "gateway_url",
				"workload_identity_arn", "target_ids",
			},
		},
		{
			// AwsBedrockAgentCoreMemory: memory_id keys the E2E verifier;
			// memory_arn is what harnesses and agent code consume; the
			// name-keyed strategy_ids map feeds the keyed-by-address
			// import derivations.
			name: "AwsBedrockAgentCoreMemory",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgentCoreMemory,
			rawOutputs: map[string]interface{}{
				"memory_id":  "support_memory-Ab1Cd2Ef3G",
				"memory_arn": "arn:aws:bedrock-agentcore:us-west-2:123456789012:memory/support_memory-Ab1Cd2Ef3G",
				"strategy_ids": map[string]interface{}{
					"facts": "facts-Zy9Xw8Vu7T",
				},
			},
			mustPopulate: []string{
				"memory_id", "memory_arn", "strategy_ids",
			},
		},
		{
			// AwsBedrockAgentCoreIdentity: every arm is a name-keyed map
			// (the bundle has no single id); the provider ARNs are what
			// gateway target credentials consume, and policy_engine_arn
			// is what a gateway's policy-engine attachment consumes.
			name: "AwsBedrockAgentCoreIdentity",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgentCoreIdentity,
			rawOutputs: map[string]interface{}{
				"workload_identity_arns": map[string]interface{}{
					"support-agent": "arn:aws:bedrock-agentcore:us-west-2:123456789012:workload-identity-directory/default/workload-identity/support-agent",
				},
				"api_key_provider_arns": map[string]interface{}{
					"docs-api": "arn:aws:bedrock-agentcore:us-west-2:123456789012:token-vault/default/apikeycredentialprovider/docs-api",
				},
				"api_key_secret_arns": map[string]interface{}{
					"docs-api": "arn:aws:secretsmanager:us-west-2:123456789012:secret:bedrock-agentcore-identity!default/apikey/docs-api-AbCdEf",
				},
				"oauth2_provider_arns": map[string]interface{}{
					"github": "arn:aws:bedrock-agentcore:us-west-2:123456789012:token-vault/default/oauth2credentialprovider/github",
				},
				"oauth2_client_secret_arns": map[string]interface{}{
					"github": "arn:aws:secretsmanager:us-west-2:123456789012:secret:bedrock-agentcore-identity!default/oauth2/github-AbCdEf",
				},
				"policy_engine_id":  "agent_authz-a1b2c3d4e5",
				"policy_engine_arn": "arn:aws:bedrock-agentcore:us-west-2:123456789012:policy-engine/agent_authz-a1b2c3d4e5",
				"policy_ids": map[string]interface{}{
					"allow_order_reads": "allow_order_reads-f6g7h8i9j0",
				},
			},
			mustPopulate: []string{
				"workload_identity_arns", "api_key_provider_arns",
				"api_key_secret_arns", "oauth2_provider_arns",
				"oauth2_client_secret_arns", "policy_engine_id",
				"policy_engine_arn", "policy_ids",
			},
		},
		{
			// AwsBedrockAgentCoreTools: every arm is a name-keyed map
			// (the bundle has no single id); the ARNs are what harness
			// browser/code-interpreter tools consume, and the id maps
			// feed the keyed-by-address import derivations.
			name: "AwsBedrockAgentCoreTools",
			kind: cloudresourcekind.CloudResourceKind_AwsBedrockAgentCoreTools,
			rawOutputs: map[string]interface{}{
				"browser_ids": map[string]interface{}{
					"research-browser": "research-browser-Ab1Cd2Ef3G",
				},
				"browser_arns": map[string]interface{}{
					"research-browser": "arn:aws:bedrock-agentcore:us-west-2:123456789012:browser/research-browser-Ab1Cd2Ef3G",
				},
				"browser_profile_ids": map[string]interface{}{
					"logged_in_docs": "logged_in_docs-Zy9Xw8Vu7T",
				},
				"browser_profile_arns": map[string]interface{}{
					"logged_in_docs": "arn:aws:bedrock-agentcore:us-west-2:123456789012:browser-profile/logged_in_docs-Zy9Xw8Vu7T",
				},
				"code_interpreter_ids": map[string]interface{}{
					"python_sandbox": "python_sandbox-Qw2Er4Ty6U",
				},
				"code_interpreter_arns": map[string]interface{}{
					"python_sandbox": "arn:aws:bedrock-agentcore:us-west-2:123456789012:code-interpreter/python_sandbox-Qw2Er4Ty6U",
				},
			},
			mustPopulate: []string{
				"browser_ids", "browser_arns", "browser_profile_ids",
				"browser_profile_arns", "code_interpreter_ids",
				"code_interpreter_arns",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ValidateOverride(tc.kind, genericModuleDir, tc.rawOutputs)
			if err != nil {
				t.Fatalf("ValidateOverride failed: %v", err)
			}
			if len(result.SchemaErrors) != 0 {
				t.Fatalf("unexpected schema errors: %v", result.SchemaErrors)
			}
			if result.DryRun == nil {
				t.Fatal("expected a dry-run result")
			}

			// Core invariant: every emitted output lands on a proto field. A
			// regression to a flat/mismatched output name surfaces here.
			if len(result.DryRun.UnmappedOutputs) != 0 {
				t.Errorf("%s: outputs did not map onto the StackOutputs proto: %v",
					tc.kind.String(), result.DryRun.UnmappedOutputs)
			}

			populated := make(map[string]bool, len(result.DryRun.PopulatedFields))
			for _, f := range result.DryRun.PopulatedFields {
				populated[f.ProtoField] = true
			}
			for _, field := range tc.mustPopulate {
				if !populated[field] {
					t.Errorf("%s: expected proto field %q to be populated, but it was not",
						tc.kind.String(), field)
				}
			}
		})
	}
}

// TestStackOutputsConformance_DetectsFlatSecretDrift proves the guard actually
// catches the historical drift: the pre-fix Postgres tofu module emitted flat
// "password_secret_name"/"password_secret_key" outputs, which do NOT flatten onto
// the proto's password_secret{name,key} field. The guard must flag both the
// unmapped output and the unpopulated proto field.
func TestStackOutputsConformance_DetectsFlatSecretDrift(t *testing.T) {
	genericModuleDir := filepath.Join("testdata", "modules", "empty")
	kind := cloudresourcekind.CloudResourceKind_KubernetesPostgres

	flatDriftOutputs := map[string]interface{}{
		"namespace":            "gosilver-prod",
		"password_secret_name": "postgres.db-gosilver-prod-postgres.credentials.postgresql.acid.zalan.do",
		"password_secret_key":  "password",
	}

	result, err := ValidateOverride(kind, genericModuleDir, flatDriftOutputs)
	if err != nil {
		t.Fatalf("ValidateOverride failed: %v", err)
	}
	if result.DryRun == nil {
		t.Fatal("expected a dry-run result")
	}

	if len(result.DryRun.UnmappedOutputs) == 0 {
		t.Error("expected the flat password_secret_name/_key outputs to be reported as unmapped, but none were")
	}
	for _, f := range result.DryRun.PopulatedFields {
		if f.ProtoField == "password_secret" {
			t.Error("flat outputs must NOT populate the nested password_secret proto field")
		}
	}
}
