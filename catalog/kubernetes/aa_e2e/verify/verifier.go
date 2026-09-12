package verify

import (
	"context"
	"strings"
)

// ResourceVerifier knows how to verify a specific Kubernetes resource type.
type ResourceVerifier interface {
	VerifyExists(ctx context.Context, kubeconfig string) error
	VerifyAbsent(ctx context.Context, kubeconfig string) error
}

// RuntimeCauseVerifier is the optional capability a verifier implements when
// its kind's scenario deploys a workload DESIGNED to fail (the framework's
// expected-runtime-failure lane): it must pin the failure to exactly the
// expected cause with the cluster's own evidence -- pod states and pod logs --
// never merely tolerate "some failure".
type RuntimeCauseVerifier interface {
	VerifyRuntimeFailureCause(ctx context.Context, kubeconfig, cause string) error
}

// DeployFailureVerifier is the optional capability a verifier implements when
// a scenario's deploy (or upgrade) is DESIGNED to be refused (the framework's
// expect-deploy-failure and expect-upgrade-failure lanes): it must pin the
// engine's error to exactly the expected class -- for the CRD lifecycle, to
// the three-part text (observed, meaning, next step) the module promises --
// and may read the cluster to confirm nothing was touched.
type DeployFailureVerifier interface {
	VerifyExpectedDeployFailure(ctx context.Context, kubeconfig, expectation string, deployErr error) error
}

// operatorKinds lists manifest kind values (lowercased) for operator/controller
// components. Operators install CRD controllers that watch resources but typically
// do not expose a Kubernetes Service. Verification checks namespace + running
// pods only (no service requirement).
var operatorKinds = map[string]bool{
	"kubernetesstrimzikafkaoperator": true,
}

// helmTier2Kinds lists manifest kind values (lowercased) for Helm-based
// Kubernetes components that deploy applications with Services.
// These must match the CloudResourceKind enum names from cloud_resource_kind.proto
// (case-insensitive via lowercasing).
var helmTier2Kinds = map[string]bool{
	// Tier 2 Helm applications
	"kubernetesjenkins": true,
	"kubernetesharbor":  true,
	// Helm applications with dedicated behavioral verifiers (the
	// dispatch cases win; these rows are the generic fallback).
	// NOTE kubernetesopenbao is deliberately NOT listed: its pods are
	// NotReady BY DESIGN until initialized/unsealed, so the generic
	// readiness-waiting fallback would hang every lane — only its
	// dedicated seal-lifecycle verifier is honest. kuberneteskeycloak
	// is operator-CR (not Helm) and dispatches to its own verifier.
	"kubernetesopenfga":   true,
	"kubernetesseaweedfs": true,
	"kubernetesqdrant":    true,
	"kubernetestemporal":  true,
	"kubernetesnats":      true,
	"kuberneteslocust":    true,
}

// crdInstallKinds maps manifest kind values (lowercased) to their expected CRD
// names for components that only install cluster-scoped CRDs without deploying
// any pods or services.
var crdInstallKinds = map[string][]string{
	// The standard channel serves all of these from the v1.6 release onward
	// (TCPRoute and UDPRoute graduated to GA; ListenerSet is standard since
	// v1.5) — assert the full set the catalog's projection kinds deploy onto.
	"kubernetesgatewayapicrds": {
		"gatewayclasses.gateway.networking.k8s.io",
		"gateways.gateway.networking.k8s.io",
		"listenersets.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"grpcroutes.gateway.networking.k8s.io",
		"tcproutes.gateway.networking.k8s.io",
		"udproutes.gateway.networking.k8s.io",
		"tlsroutes.gateway.networking.k8s.io",
		"referencegrants.gateway.networking.k8s.io",
		"backendtlspolicies.gateway.networking.k8s.io",
	},
	// KubernetesIstioBaseCrds installs the istio/base CRD bundle (no istiod). Verify the
	// CRDs backing the seven typed Istio components are present.
	"kubernetesistiobasecrds": {
		"destinationrules.networking.istio.io",
		"serviceentries.networking.istio.io",
		"envoyfilters.networking.istio.io",
		"peerauthentications.security.istio.io",
		"requestauthentications.security.istio.io",
		"authorizationpolicies.security.istio.io",
		"telemetries.telemetry.istio.io",
	},
}

// gatewayApiCustomResource describes how to verify a Gateway API custom resource
// created by one of the Gateway API components. These components do
// not run pods; verification confirms the CR itself exists after apply and is
// gone after destroy. The CRDs are installed by the KubernetesGatewayApiCrds
// registry prerequisite before the component applies.
type gatewayApiCustomResource struct {
	// resource is the fully-qualified kubectl resource (plural.group), which is
	// stable across the served apiVersion.
	resource string
	// clusterScoped is true for cluster-scoped kinds (GatewayClass), which must
	// be queried without a namespace.
	clusterScoped bool
}

// gatewayApiKinds maps manifest kind values (lowercased) to their Gateway API
// custom-resource verification descriptor.
var gatewayApiKinds = map[string]gatewayApiCustomResource{
	"kubernetesgatewayclass":     {resource: "gatewayclasses.gateway.networking.k8s.io", clusterScoped: true},
	"kubernetesgateway":          {resource: "gateways.gateway.networking.k8s.io"},
	"kuberneteslistenerset":      {resource: "listenersets.gateway.networking.k8s.io"},
	"kuberneteshttproute":        {resource: "httproutes.gateway.networking.k8s.io"},
	"kubernetesgrpcroute":        {resource: "grpcroutes.gateway.networking.k8s.io"},
	"kubernetestcproute":         {resource: "tcproutes.gateway.networking.k8s.io"},
	"kubernetesudproute":         {resource: "udproutes.gateway.networking.k8s.io"},
	"kubernetestlsroute":         {resource: "tlsroutes.gateway.networking.k8s.io"},
	"kubernetesreferencegrant":   {resource: "referencegrants.gateway.networking.k8s.io"},
	"kubernetesbackendtlspolicy": {resource: "backendtlspolicies.gateway.networking.k8s.io"},
}

// istioApiKinds maps manifest kind values (lowercased) to the fully-qualified
// kubectl resource (plural.group) for the typed Istio API components
// (853-859). Like the Gateway API kinds, these components do not run pods;
// verification confirms the CR itself exists after apply and is gone after
// destroy. The Istio CRDs are installed by the KubernetesIstioBaseCrds registry
// prerequisite before the component applies. All seven Istio kinds are
// namespaced, so no clusterScoped flag is needed.
var istioApiKinds = map[string]string{
	"kubernetespeerauthentication":    "peerauthentications.security.istio.io",
	"kubernetesrequestauthentication": "requestauthentications.security.istio.io",
	"kubernetesauthorizationpolicy":   "authorizationpolicies.security.istio.io",
	"kubernetesserviceentry":          "serviceentries.networking.istio.io",
	"kubernetesdestinationrule":       "destinationrules.networking.istio.io",
	"kubernetesenvoyfilter":           "envoyfilters.networking.istio.io",
	"kubernetestelemetry":             "telemetries.telemetry.istio.io",
}

// GetVerifierFromManifest creates the appropriate verifier by parsing the manifest.
func GetVerifierFromManifest(manifestPath string) (ResourceVerifier, error) {
	info, err := ParseManifestInfo(manifestPath)
	if err != nil {
		return nil, err
	}

	component := strings.ToLower(info.Kind)

	switch component {
	case "kubernetesnamespace":
		return &NamespaceVerifier{Name: info.Name}, nil

	case "kubernetesdeployment":
		return &WorkloadVerifier{
			Namespace: info.Namespace,
			Kind:      "deployment",
			Name:      info.Name,
		}, nil

	case "kubernetesstatefulset":
		return &WorkloadVerifier{
			Namespace: info.Namespace,
			Kind:      "statefulset",
			Name:      info.Name,
		}, nil

	case "kubernetessecret":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "secret",
			Name:      info.Name,
		}, nil

	case "kubernetesconfigmap":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "configmap",
			Name:      info.Name,
		}, nil

	case "kubernetesserviceaccount":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "serviceaccount",
			Name:      info.Name,
		}, nil

	// The RBAC grant deploys role/binding objects whose names derive from the
	// grant's shape, so its verifier re-reads the manifest itself.
	case "kubernetesrbac":
		return &RbacVerifier{ManifestPath: manifestPath}, nil

	// Existence is the every-lane contract; the aws-load-balancer scenario
	// (real-cluster profile) additionally asserts the cloud populated a real
	// LB address — the never-wait-at-deploy posture makes the verifier the
	// only honest place for that proof.
	case "kubernetesservice":
		if strings.Contains(manifestPath, "aws-load-balancer") {
			return &ServiceLbAddressVerifier{
				Namespace: info.Namespace,
				Name:      info.Name,
			}, nil
		}
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "service",
			Name:      info.Name,
		}, nil

	// An Ingress is a valid API object with or without a controller running in
	// the cluster, so existence (not load-balancer status) is the correct
	// verification on every lane.
	case "kubernetesingress":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "ingress",
			Name:      info.Name,
		}, nil

	// A NetworkPolicy has no runtime status of its own (enforcement lives in
	// the CNI); on the default cluster the object's existence is the
	// verifiable contract. Scenarios that run with an ENFORCING CNI (the
	// KubernetesCilium prerequisite, on the cilium-cni cluster profile) get
	// the behavioral verifier — traffic actually blocked while the policy
	// exists and flowing again after destroy.
	case "kubernetesnetworkpolicy":
		if manifestHasPrerequisite(manifestPath, "KubernetesCilium") {
			return &NetworkPolicyDenyBehavioralVerifier{
				Namespace:        info.Namespace,
				PolicyName:       info.Name,
				ClientDeployment: "e2e-netpol-client",
				BackendURL:       "http://e2e-netpol-backend." + info.Namespace + ".svc.cluster.local/",
			}, nil
		}
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "networkpolicy",
			Name:      info.Name,
		}, nil

	// Cilium: the dataplane install proof — agent rolled out, operator
	// available, and every node Ready (on the CNI-less profile the
	// NotReady→Ready transition is the proof Cilium became the CNI).
	case "kubernetescilium":
		return &CiliumInstallVerifier{
			Namespace: info.Namespace,
		}, nil

	// KEDA: install proof up to the external-metrics APIService; the
	// behavioral-scaling scenario (recognized by its scale-target fixture)
	// additionally proves a real cron-driven scale-up.
	case "kuberneteskeda":
		return &KedaInstallVerifier{
			Namespace:  info.Namespace,
			Behavioral: manifestHasPrerequisiteSuffix(manifestPath, "fixture-scale-target.yaml"),
			// The aws-sqs-irsa scenario (real-cluster profile) proves the
			// cloud pod-identity hop: real SQS depth drives a real scale.
			BehavioralSqs: strings.Contains(manifestPath, "aws-sqs-irsa"),
		}, nil

	// Cluster Autoscaler: install proof down to the reconcile loop's own
	// heartbeat (the status ConfigMap). The kind lane runs the KWOK
	// simulation arm; the aws-asg-scaling scenario (real-cluster profile)
	// additionally proves a real node-group scale-up/scale-down.
	case "kubernetesclusterautoscaler":
		return &ClusterAutoscalerInstallVerifier{
			Namespace:  info.Namespace,
			Behavioral: strings.Contains(manifestPath, "aws-asg-scaling"),
		}, nil

	// Velero: install proof down to the BackupStorageLocation handshake;
	// the behavioral scenario (recognized by name) additionally proves a
	// full backup → namespace-loss → restore cycle against the MinIO
	// fixture, with verifier-owned Backup/Restore CRs.
	case "kubernetesvelero":
		return &VeleroInstallVerifier{
			Namespace:  info.Namespace,
			Behavioral: strings.Contains(manifestPath, "behavioral-backup-restore"),
			// The aws-s3-csi scenario (real-cluster profile) puts a VOLUME
			// in the DR blast radius: CSI snapshot → real S3 → restore.
			CsiVolume: strings.Contains(manifestPath, "aws-s3-csi"),
		}, nil

	// Karpenter: the controller cannot start off AWS (region/credentials/
	// instance-type discovery are fatal at startup), so these run only on
	// the batched EKS real-cluster lanes — the profiles are deferred and
	// the kind entrypoints skip them. The controller install is verified
	// like other operators (namespace + running pods; Karpenter exposes a
	// metrics Service, so the generic Helm shape fits).
	case "kuberneteskarpenter":
		return &HelmComponentVerifier{
			Namespace:     info.Namespace,
			ComponentName: "karpenter",
		}, nil

	// Karpenter fleet CRs are cluster-scoped objects reconciled by the
	// controller; the object's presence is the verifiable contract (node
	// provisioning itself is the EKS lane's behavioral assertion).
	case "kuberneteskarpenternodepool":
		return &KarpenterNodePoolVerifier{
			Name:       info.Name,
			Behavioral: strings.Contains(manifestPath, "behavioral-node-launch"),
		}, nil
	case "kuberneteskarpenterec2nodeclass":
		return &ResourceExistenceVerifier{
			Kind: "ec2nodeclasses.karpenter.k8s.aws",
			Name: info.Name,
		}, nil

	// A claim under a WaitForFirstConsumer StorageClass is correctly Pending
	// until a pod consumes it, so existence (not Bound) is the verifiable
	// contract; the composed consumer scenario proves real binding.
	// PVC: existence on every lane; the data-source scenarios (real-cluster
	// profile, snapshot-capable CSI) prove clone/snapshot-restore
	// PROVISIONING — marker data must travel from the source volume to the
	// claim under test.
	case "kubernetespersistentvolumeclaim":
		if strings.Contains(manifestPath, "snapshot-restore") || strings.Contains(manifestPath, "clone") {
			return &PvcDataSourceVerifier{
				Name:     info.Name,
				Snapshot: strings.Contains(manifestPath, "snapshot-restore"),
			}, nil
		}
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "pvc",
			Name:      info.Name,
		}, nil

	// Cluster-scoped: kubectl ignores the empty namespace argument.
	case "kubernetesstorageclass":
		return &ResourceExistenceVerifier{
			Kind: "storageclass",
			Name: info.Name,
		}, nil

	// The quota object is the verifiable contract; the companion LimitRange
	// shares its name and is asserted by the with-limit-defaults scenario's
	// import round-trip (both resources ride the same state).
	case "kubernetesresourcequota":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "resourcequota",
			Name:      info.Name,
		}, nil

	// Cluster-scoped; preemption BEHAVIOR needs scheduling pressure and
	// rides the real-cluster lanes — the object is the verifiable contract.
	// PriorityClass: existence on every lane; the preemption scenario
	// (real-cluster profile) proves the class actually evicts lower
	// priorities under genuine scheduling pressure.
	case "kubernetespriorityclass":
		if strings.Contains(manifestPath, "preemption") {
			return &PriorityClassPreemptionVerifier{Name: info.Name}, nil
		}
		return &ResourceExistenceVerifier{
			Kind: "priorityclass",
			Name: info.Name,
		}, nil

	case "kubernetespoddisruptionbudget":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "pdb",
			Name:      info.Name,
		}, nil

	// Scaling BEHAVIOR needs a metrics source. Scenarios that install
	// metrics-server (declared in the e2e-prerequisites annotation) get the
	// behavioral verifier — ScalingActive + an actual scale-up; scenarios
	// without it keep the object as the verifiable contract.
	case "kuberneteshorizontalpodautoscaler":
		if manifestHasPrerequisite(manifestPath, "KubernetesMetricsServer") {
			return &HpaBehavioralVerifier{
				Namespace:   info.Namespace,
				Name:        info.Name,
				MinReplicas: manifestSpecInt(manifestPath, "minReplicas", 1),
			}, nil
		}
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "hpa",
			Name:      info.Name,
		}, nil

	case "kubernetescronjob":
		return &ResourceExistenceVerifier{
			Namespace: info.Namespace,
			Kind:      "cronjob",
			Name:      info.Name,
		}, nil

	case "kubernetesjob":
		return &JobVerifier{
			Namespace: info.Namespace,
			Name:      info.Name,
		}, nil

	case "kubernetesdaemonset":
		return &WorkloadVerifier{
			Namespace: info.Namespace,
			Kind:      "daemonset",
			Name:      info.Name,
		}, nil

	// CloudNativePG operator: Deployment Available + CRDs Established.
	case "kubernetescloudnativepgoperator":
		return &CnpgOperatorInstallVerifier{
			Namespace: info.Namespace,
		}, nil

	// Barman Cloud plugin for CloudNativePG: plugin Deployment Available
	// beside an operator in the same namespace, the discovery Service
	// present, the ObjectStore CRD Established; when the scenario declares
	// the operator resident, the destroy proves the resident survived.
	case "kubernetescnpgbarmancloudplugin":
		return &CnpgBarmanPluginVerifier{
			Namespace:        info.Namespace,
			ResidentOperator: strings.Contains(manifestAnnotation(manifestPath, "planton.dev/e2e-resident-prerequisites"), "KubernetesCloudNativePgOperator"),
		}, nil

	// Percona operator installs: the operator Deployment Available plus
	// the CRDs the database kinds render against, Established.
	case "kubernetesperconamongooperator":
		return &PsmdbOperatorInstallVerifier{
			Namespace:   info.Namespace,
			ReleaseName: info.Name,
		}, nil
	case "kubernetesperconamysqloperator":
		return &PxcOperatorInstallVerifier{
			Namespace:    info.Namespace,
			ReleaseName:  info.Name,
			WatchWidened: pxcWatchWidened(manifestSpecMap(manifestPath)),
		}, nil

	// A Percona-operator-managed MongoDB cluster: the resource in state
	// ready + the replica-set Service. The behavioral-failover scenario
	// (recognized by name) proves data durability through a live primary
	// loss and election; the with-backup scenario drives a real PBM
	// backup to completion.
	case "kubernetesmongodb":
		spec := manifestSpecMap(manifestPath)
		rsName, rsSize := mongodbFirstReplset(spec)
		return &PsmdbClusterVerifier{
			Namespace:       info.Namespace,
			ClusterName:     info.Name,
			ReplsetName:     rsName,
			Size:            rsSize,
			Behavioral:      strings.Contains(manifestPath, "behavioral-failover"),
			BackupProof:     strings.Contains(manifestPath, "with-backup"),
			BackupStorage:   mongodbFirstBackupStorage(spec),
			UsersSecretName: mongodbUsersSecretName(spec),
			RestoreProof:    scenarioMatches(manifestPath, "gke-", "-restore"),
		}, nil

	// A Strimzi-operator-managed KRaft Kafka cluster: the Kafka resource
	// Ready + every node pod + the bootstrap Service. The
	// behavioral-durability scenario (recognized by name) proves
	// replicated durability through a live broker loss (produce with
	// acks=all → kill a broker → consume during the outage → full
	// recovery).
	case "kuberneteskafka":
		spec := manifestSpecMap(manifestPath)
		firstPool, totalNodes := kafkaFirstPool(spec)
		return &KafkaClusterVerifier{
			Namespace:     info.Namespace,
			ClusterName:   info.Name,
			FirstPoolName: firstPool,
			TotalNodes:    totalNodes,
			BootstrapPort: kafkaFirstInternalPort(spec),
			Durability:    strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// A declared Kafka topic: the KafkaTopic resource Ready (the topic
	// operator reconciled it) AND kafka-topics --describe on the target
	// cluster reporting the declared partition count — the reconcile
	// proof runs on every lane.
	case "kuberneteskafkatopic":
		spec := manifestSpecMap(manifestPath)
		topicName, _ := spec["topicName"].(string)
		if topicName == "" {
			// Scenario manifests are written in the proto's snake_case form.
			topicName, _ = spec["topic_name"].(string)
		}
		if topicName == "" {
			topicName = info.Name
		}
		return &KafkaTopicVerifier{
			Namespace:   info.Namespace,
			CrName:      info.Name,
			TopicName:   topicName,
			ClusterName: kafkaClusterRef(spec),
			Partitions:  int(manifestSpecInt(manifestPath, "partitions", 0)),
		}, nil

	// A declared Kafka user: the KafkaUser resource Ready + the
	// operator-generated credentials Secret. The behavioral-auth
	// scenario (recognized by name) additionally produces and consumes
	// AS THE USER through the cluster's scram listener — authentication
	// and ACL authorization proven on the wire.
	case "kuberneteskafkauser":
		spec := manifestSpecMap(manifestPath)
		aclTopic, aclGroup := kafkaUserAcl(spec)
		return &KafkaUserVerifier{
			Namespace:   info.Namespace,
			Username:    info.Name,
			ClusterName: kafkaClusterRef(spec),
			AuthProof:   strings.Contains(manifestPath, "behavioral-auth"),
			ScramPort:   9094,
			AclTopic:    aclTopic,
			AclGroup:    aclGroup,
		}, nil

	// A Strimzi-operator-managed Kafka Connect cluster: the KafkaConnect
	// resource Ready + the Connect REST API Service.
	case "kuberneteskafkaconnect":
		return &KafkaConnectVerifier{
			Namespace:   info.Namespace,
			ConnectName: info.Name,
		}, nil

	// A declared connector on a Connect cluster: the KafkaConnector
	// resource Ready. The behavioral-dataflow scenario (recognized by
	// name) additionally consumes the connector's output topic on the
	// Kafka fixture cluster and asserts real source content arrived —
	// the pipe proven end-to-end.
	case "kuberneteskafkaconnector":
		return &KafkaConnectorVerifier{
			Namespace:        info.Namespace,
			ConnectorName:    info.Name,
			DataFlow:         strings.Contains(manifestPath, "behavioral-dataflow"),
			KafkaClusterName: kafkaEcosystemFixtureCluster,
			SourceTopic:      connectorDataFlowSourceTopic,
			MirroredTopic:    connectorDataFlowMirroredTopic,
		}, nil

	// A MirrorMaker 2 deployment: the KafkaMirrorMaker2 resource Ready.
	// The behavioral-migration scenario (recognized by name) produces on
	// the SOURCE fixture cluster and consumes from the TARGET fixture
	// cluster — records crossing clusters is the migration proof.
	case "kuberneteskafkamirrormaker2":
		spec := manifestSpecMap(manifestPath)
		return &KafkaMirrorMaker2Verifier{
			Namespace:       info.Namespace,
			MirrorMakerName: info.Name,
			Migration:       strings.Contains(manifestPath, "behavioral-migration"),
			SourceCluster:   kafkaEcosystemSourceCluster,
			TargetCluster:   kafkaEcosystemFixtureCluster,
			SourceAlias:     mirrorMaker2FirstSourceAlias(spec),
		}, nil

	// A Karapace schema registry: registry Deployment Available + Service
	// + a LIVE register/fetch round-trip through the Confluent-compatible
	// API on every lane (a registry that cannot persist schemas is not a
	// registry).
	case "kuberneteskarapace":
		spec := manifestSpecMap(manifestPath)
		return &KarapaceVerifier{
			Namespace:    info.Namespace,
			RegistryName: info.Name,
			Port:         int(manifestSpecInt(manifestPath, "port", 8081)),
			RestProxy:    karapaceRestProxyEnabled(spec),
		}, nil

	// A kafbat UI console: Deployment Available + Service + the console's
	// own API answering. The behavioral-observe scenario additionally
	// asserts the declared cluster is reported ONLINE; with-auth
	// scenarios assert anonymous access is refused.
	case "kuberneteskafkaui":
		spec := manifestSpecMap(manifestPath)
		clusterOnline := ""
		if strings.Contains(manifestPath, "behavioral-observe") {
			clusterOnline = kafkaUiFirstClusterName(spec)
		}
		return &KafkaUiVerifier{
			Namespace:     info.Namespace,
			ServiceName:   info.Name,
			Port:          int(manifestSpecInt(manifestPath, "servicePort", 80)),
			ClusterOnline: clusterOnline,
			Authenticated: strings.Contains(manifestPath, "with-auth"),
		}, nil

	// The Altinity ClickHouse operator: the operator Deployment
	// Available + the four chart-owned CRDs Established (kept on
	// destroy by Helm's crds/ semantics).
	case "kubernetesaltinityoperator":
		return &AltinityOperatorInstallVerifier{
			Namespace:   info.Namespace,
			ReleaseName: info.Name,
		}, nil

	// The RabbitMQ Cluster Operator: Deployment Available + the
	// RabbitmqCluster CRD Established + both admission webhook
	// configurations present (their serving cert is cert-manager-issued
	// — the operator's registry prerequisite). Every name is fixed by
	// the release manifest, including the rabbitmq-system namespace; on
	// destroy the CRD is asserted GONE (it is a document of the applied
	// manifest, not a kept CRD).
	case "kubernetesrabbitmqoperator":
		return &RabbitMqOperatorInstallVerifier{}, nil

	// An operator-managed RabbitMQ cluster: ClusterAvailable +
	// AllReplicasReady conditions, the naming contract's Services and
	// default-user Secret, plus a LIVE message round-trip through the
	// management API on every lane. The behavioral-durability scenario
	// (recognized by name) proves a quorum queue's marker survives a
	// live broker loss.
	case "kubernetesrabbitmq":
		spec := manifestSpecMap(manifestPath)
		replicas := 1
		if n, ok := specInt(spec["replicas"]); ok && n > 0 {
			replicas = n
		}
		return &RabbitMqClusterVerifier{
			Namespace:   info.Namespace,
			ClusterName: info.Name,
			Replicas:    replicas,
			Durability:  strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// An operator-managed ClickHouse cluster: status Completed with
	// every declared host reconciled, the managed Keeper when the
	// coordination calls for one, plus a LIVE SQL round-trip on every
	// lane. The behavioral-durability scenario (recognized by name)
	// proves replica-synced rows survive a live replica loss.
	case "kubernetesclickhouse":
		spec := manifestSpecMap(manifestPath)
		clusterName, totalHosts, username, password, managedKeeper := clickhouseScenarioShape(spec)
		return &ClickHouseInstallationVerifier{
			Namespace:     info.Namespace,
			ChiName:       info.Name,
			ClusterName:   clusterName,
			TotalHosts:    totalHosts,
			Username:      username,
			Password:      password,
			ManagedKeeper: managedKeeper,
			Durability:    strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// The OpenSearch Kubernetes Operator: controller-manager Available +
	// the module-owned CRDs Established (kept on destroy by design).
	case "kubernetesopensearchoperator":
		return newOpenSearchOperatorInstallVerifier(manifestPath, info.Name, info.Namespace), nil

	// An operator-managed OpenSearch cluster: phase RUNNING with every
	// declared node available, plus a LIVE index/search round-trip on
	// every lane. The behavioral-durability scenario (recognized by
	// name) proves replicated data survives a live node loss.
	case "kubernetesopensearch":
		spec := manifestSpecMap(manifestPath)
		firstPool, totalNodes := opensearchFirstPool(spec)
		return &OpenSearchClusterVerifier{
			Namespace:     info.Namespace,
			ClusterName:   info.Name,
			TotalNodes:    totalNodes,
			FirstPoolName: firstPool,
			HttpPort:      int(manifestSpecInt(manifestPath, "http_port", 9200)),
			Durability:    strings.Contains(manifestPath, "behavioral-durability"),
			Dashboards:    opensearchDashboardsEnabled(spec),
		}, nil

	// The Apache Solr Operator: operator Available + the module-owned
	// CRDs (incl. the bundled zookeeper-operator's ZookeeperCluster)
	// Established; kept on destroy by design.
	case "kubernetessolroperator":
		return newSolrOperatorInstallVerifier(manifestPath, info.Name, info.Namespace), nil

	// An operator-managed SolrCloud: every node ready + the common
	// Service. The behavioral-collection scenario (recognized by name)
	// creates a collection, indexes a document and queries it back —
	// the cluster proven working, not just running.
	case "kubernetessolr":
		spec := manifestSpecMap(manifestPath)
		return &SolrCloudVerifier{
			Namespace:   info.Namespace,
			ClusterName: info.Name,
			Replicas:    int(manifestSpecInt(manifestPath, "replicas", 3)),
			Security:    solrSecurityEnabled(spec),
			Behavioral:  strings.Contains(manifestPath, "behavioral-collection"),
		}, nil

	// A Neo4j server: the StatefulSet ready + the main Service, plus a
	// LIVE Cypher write/read round-trip whenever credentials are
	// A SeaweedFS object store: master + filer StatefulSets ready (the
	// dedicated S3 gateway Deployment too when deployed), the `-s3`
	// Service present, declared buckets created by the install hook, and
	// a live S3 put/get round-trip with the chart-materialized
	// credentials. The behavioral-durability scenario (recognized by
	// name) proves object bytes survive a volume-server pod loss through
	// the PVC.
	case "kubernetesseaweedfs":
		spec := manifestSpecMap(manifestPath)
		return &SeaweedFsVerifier{
			Namespace:             info.Namespace,
			Name:                  info.Name,
			DedicatedS3:           seaweedfsS3Dedicated(spec),
			CredentialsSecretName: seaweedfsCredentialsSecret(spec, info.Name),
			Buckets:               seaweedfsBuckets(spec),
			AdminEnabled:          seaweedfsAdminEnabled(spec),
			Durability:            strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// A kube-prometheus-stack: operator Deployment available, the
	// operator-reconciled Prometheus StatefulSet at its declared count,
	// Alertmanager/bundled-Grafana when enabled, and a LIVE metric-flow
	// proof (a healthy kube-state-metrics target + a PromQL answer). The
	// behavioral-alerting scenario (recognized by name) proves the
	// pipeline end to end via the always-firing Watchdog alert in BOTH
	// Prometheus and Alertmanager. Destroy asserts the crds-subchart
	// keep posture (the monitoring CRDs must SURVIVE uninstall).
	case "kuberneteskubeprometheusstack":
		spec := manifestSpecMap(manifestPath)
		return &KubePrometheusStackVerifier{
			Namespace:            info.Namespace,
			Name:                 info.Name,
			PrometheusReplicas:   kpsReplicas(spec, "prometheus"),
			AlertmanagerEnabled:  kpsHalfEnabled(spec, "alertmanager"),
			AlertmanagerReplicas: kpsReplicas(spec, "alertmanager"),
			GrafanaEnabled:       kpsHalfEnabled(spec, "grafana"),
			Alerting:             strings.Contains(manifestPath, "behavioral-alerting"),
		}, nil

	// A standalone Grafana: Deployment available, /api/health reporting
	// a working database, an AUTHENTICATED API round-trip as the admin
	// credentials (read from the Secret — the credential-wiring proof),
	// and every declared datasource provisioned. The
	// behavioral-persistence scenario (recognized by name) proves a
	// UI-authored dashboard survives a pod REPLACEMENT through the PVC.
	case "kubernetesgrafana":
		spec := manifestSpecMap(manifestPath)
		return &GrafanaVerifier{
			Namespace:        info.Namespace,
			Name:             info.Name,
			AdminSecretName:  grafanaAdminSecretName(spec, info.Name),
			AdminUserKey:     grafanaAdminSecretKey(spec, "user_key"),
			AdminPasswordKey: grafanaAdminSecretKey(spec, "password_key"),
			Datasources:      grafanaDatasourceNames(spec),
			Persistence:      strings.Contains(manifestPath, "behavioral-persistence"),
		}, nil

	// A Grafana Loki log store: the monolithic StatefulSet ready, the
	// gateway Service present, and a LIVE push→LogQL round-trip (a line
	// pushed through the gateway and returned by a LogQL query). With
	// tenants declared the proof authenticates AS the first tenant —
	// the gateway guards every push/query route and derives the tenant
	// from the authenticated user, so the round-trip then proves the
	// whole multi-tenant path. The behavioral-durability scenario
	// (recognized by name) proves logs survive a pod loss through the
	// PVC.
	case "kubernetesloki":
		spec := manifestSpecMap(manifestPath)
		v := &LokiVerifier{
			Namespace:      info.Namespace,
			Name:           info.Name,
			GatewayEnabled: lokiGatewayEnabled(spec),
			Durability:     strings.Contains(manifestPath, "behavioral-durability"),
		}
		if tenant := lokiFirstTenantUser(spec); tenant != "" {
			v.TenantUser = tenant
			v.TenantPassword = lokiE2eTenantPassword
		}
		return v, nil

	// A Grafana Tempo trace store: the StatefulSet ready, the Service
	// present, and a LIVE OTLP-push→trace-by-ID round-trip. The
	// behavioral-persistence scenario (recognized by name) proves traces
	// survive a pod loss through the PVC.
	case "kubernetestempo":
		return &TempoVerifier{
			Namespace:   info.Namespace,
			Name:        info.Name,
			Persistence: strings.Contains(manifestPath, "behavioral-persistence"),
		}, nil

	// A SigNoz observability platform: server StatefulSet + collector
	// rolled out, health OK (answers only with a working connection to
	// the COMPOSED ClickHouse — the registry-prerequisite fixture), and
	// the product-grade round-trip — first-admin REGISTRATION through
	// the product API, a session, an OTLP span push, and the trace
	// retrieved by ID through the authenticated query API. The
	// behavioral-state scenario (recognized by name) re-authenticates
	// and re-queries AFTER a server pod replacement — users survive on
	// the SQLite PVC, telemetry in the composed ClickHouse.
	case "kubernetessignoz":
		spec := manifestSpecMap(manifestPath)
		zipkinPort, jaegerPort := SignozReceiverPosture(spec)
		return &SignozVerifier{
			Namespace:        info.Namespace,
			Name:             info.Name,
			StateProof:       strings.Contains(manifestPath, "behavioral-state"),
			ExpectZipkinPort: zipkinPort,
			ExpectJaegerPort: jaegerPort,
		}, nil

	// An Argo CD GitOps control plane: server + repo-server Deployments
	// and the application-controller StatefulSet rolled out, the
	// APPLICATION-generated initial admin Secret read as the credential
	// proof, a session opened through the product's session API, and an
	// authenticated applications round-trip. The behavioral-gitops
	// scenario (recognized by name) runs THE GITOPS PROOF: an
	// API-declared Application auto-syncs a public repo into the
	// namespace (asserted on the synced workload), then survives a
	// server pod replacement still Synced/Healthy. Destroy asserts the
	// crds.keep posture (applications.argoproj.io must REMAIN).
	case "kubernetesargocd":
		spec := manifestSpecMap(manifestPath)
		return &ArgoCdVerifier{
			Namespace:      info.Namespace,
			Name:           info.Name,
			AdminEnabled:   argocdAdminEnabled(spec),
			ServerInsecure: argocdServerInsecure(spec),
			CrdsKeep:       argocdCrdsKeep(spec),
			GitOpsProof:    strings.Contains(manifestPath, "behavioral-gitops"),
		}, nil

	// An Argo Workflows engine: the workflow controller (and the Argo
	// server when enabled, with an authenticated version-API round-trip
	// as the runner identity) rolled out, and THE ENGINE PROOF on every
	// lane — a verifier-owned Workflow CR runs to Succeeded under the
	// runner ServiceAccount. The behavioral-resilience scenario
	// (recognized by name) replaces the controller pod and proves a
	// fresh workflow still completes. Destroy asserts the crds.keep
	// posture (workflows.argoproj.io must REMAIN).
	case "kubernetesargoworkflows":
		spec := manifestSpecMap(manifestPath)
		return &ArgoWorkflowsVerifier{
			Namespace:              info.Namespace,
			Name:                   info.Name,
			ServerEnabled:          argoWorkflowsServerEnabled(spec),
			WorkflowServiceAccount: argoWorkflowsRunnerSA(spec),
			InstanceId:             argoWorkflowsInstanceId(spec),
			ResilienceProof:        strings.Contains(manifestPath, "behavioral-resilience"),
		}, nil

	// A Temporal workflow engine: all four server Deployments rolled
	// out against the COMPOSED PostgreSQL (the registry-prerequisite
	// fixture), the Web UI answering when enabled, and THE ENGINE PROOF
	// on every lane — a real SDK worker connects through the frontend
	// gRPC endpoint and a workflow EXECUTES TO COMPLETION with its
	// result read back. The behavioral-state scenario (recognized by
	// name) replaces the history and frontend pods and proves the
	// completed workflow still describes (state lives in the database)
	// plus a fresh workflow succeeds. Destroy is clean by design (no
	// CRDs).
	case "kubernetestemporal":
		spec := manifestSpecMap(manifestPath)
		return &TemporalVerifier{
			Namespace:         info.Namespace,
			Name:              info.Name,
			WebUiEnabled:      temporalWebUiEnabled(spec),
			TemporalNamespace: temporalFirstNamespace(spec),
			StateProof:        strings.Contains(manifestPath, "behavioral-state"),
		}, nil

	// An Airflow installation: the Airflow 3 component set rolled out
	// (+ Celery workers/bundled Redis when declared), THE AUTH GATE
	// (anonymous API read rejected), the api server's own health
	// contract (metadatabase + scheduler healthy — the composed
	// database proven end to end), and THE DAG PROOF on every lane —
	// a real DAG triggered through the REST API as the admin user
	// (module-generated credential) and polled to a SUCCESSFUL run.
	// The behavioral-dag-delivery scenario (recognized by name) writes
	// a marker DAG into the shared dags volume instead, runs it, then
	// re-runs it after a UID-verified scheduler replacement. Destroy
	// is clean by design (no CRDs).
	case "kubernetesairflow":
		spec := manifestSpecMap(manifestPath)
		return &AirflowVerifier{
			Namespace:     info.Namespace,
			Name:          info.Name,
			CeleryEnabled: airflowCeleryEnabled(spec),
			BundledRedis:  airflowBundledRedis(spec),
			AdminUsername: airflowAdminUsername(spec),
			DeliveryProof: strings.Contains(manifestPath, "behavioral-dag-delivery"),
		}, nil

	// A JupyterHub installation: the hub + proxy rolled out (chart-fixed
	// bare names), THE AUTH GATE (anonymous hub-API read rejected AND a
	// wrong-password sign-in refused — the chart's open-door default
	// must be dead), a REAL sign-in with the module-generated shared
	// password, and THE SPAWN PROOF on every lane — the signed-in
	// user's server spawns a real jupyter-<username> pod through
	// KubeSpawner and the hub reports it ready; the proof server and
	// its runtime home PVC are swept. The behavioral-spawn scenario
	// (recognized by name) replaces the hub pod UID-verified and proves
	// a fresh sign-in finds the same account and spawns again — hub
	// state lives in the database. Destroy treats surviving `claim-*`
	// user PVCs as DESIGNED and sweeps them.
	case "kubernetesjupyterhub":
		return &JupyterHubVerifier{
			Namespace:     info.Namespace,
			Name:          info.Name,
			SpawnUsername: "e2everifier",
			StateProof:    strings.Contains(manifestPath, "behavioral-spawn"),
		}, nil

	// An MLflow tracking server: the module-owned Deployment rolled
	// out, /health answering, THE AUTH GATE (anonymous tracking-API
	// read rejected + upstream's admin/password1234 default dead), and
	// THE TRACKING PROOF on every lane — an experiment, a run with a
	// param + metric, an artifact round-tripped through the server's
	// own proxy, the run FINISHED. The behavioral-durability scenario
	// (recognized by name) replaces the server pod UID-verified and
	// re-reads the experiment, metric and artifact bytes — state lives
	// in the composed database and object store. Destroy is clean by
	// design (module-owned manifests, no CRDs).
	case "kubernetesmlflow":
		spec := manifestSpecMap(manifestPath)
		return &MlflowVerifier{
			Namespace:     info.Namespace,
			Name:          info.Name,
			AdminUsername: mlflowAdminUsername(spec),
			AuthEnabled:   mlflowAuthEnabled(spec),
			StateProof:    strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// A Trino cluster: coordinator + worker rollouts, THE AUTH GATE
	// (anonymous statement submission rejected — upstream ships no
	// authentication and that never deploys), and THE QUERY PROOF on
	// every lane (a real query through the statement API as the
	// generated admin, answered through real workers). The
	// full-surface scenario adds THE FEDERATION PROOF (the composed
	// PostgreSQL catalog answers, and a cross-catalog join runs in
	// one statement); the behavioral scenario adds THE RECOVERY PROOF
	// (a UID-verified coordinator replacement answers the same query
	// — the engine is stateless by design). Destroy is clean: a plain
	// Helm release plus module-owned Secrets, no CRDs.
	case "kubernetestrino":
		spec := manifestSpecMap(manifestPath)
		return &TrinoVerifier{
			Namespace:       info.Namespace,
			Name:            info.Name,
			AdminUsername:   trinoAdminUsername(spec),
			AuthEnabled:     trinoAuthEnabled(spec),
			WorkerReplicas:  trinoWorkerReplicas(spec),
			FederationProof: strings.Contains(manifestPath, "full-surface"),
			CatalogName:     "warehouse",
			RecoveryProof:   strings.Contains(manifestPath, "behavioral"),
		}, nil

	// An Apache Superset deployment: the web rollout (gated on the
	// init Job's schema migration), /health, THE AUTH GATE (anonymous
	// API read rejected + the chart's documented admin/admin default
	// dead), a real sign-in through the security API, and THE
	// DASHBOARD PROOF on every lane (a dashboard created through the
	// REST API with the JWT+CSRF contract real clients use, read
	// back, and swept). The behavioral-durability scenario replaces
	// the web pod UID-verified and finds the same dashboard from a
	// fresh session — BI state lives in the composed PostgreSQL.
	// Destroy is clean: a plain Helm release plus module-owned
	// Secrets, no CRDs.
	case "kubernetessuperset":
		spec := manifestSpecMap(manifestPath)
		return &SupersetVerifier{
			Namespace:     info.Namespace,
			Name:          info.Name,
			AdminUsername: supersetAdminUsername(spec),
			WorkerEnabled: supersetWorkerEnabled(spec),
			StateProof:    strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// The Apache Spark operator: rollout + both spark.apache.org CRDs
	// established + THE JOB PROOF on every lane (a verifier-owned
	// SparkApplication — the in-image SparkPi — runs to Succeeded
	// through REAL driver and executor pods under the chart-created
	// workload RBAC), and with a fenced workload posture THE FENCE
	// PROOF (a SparkApplication outside the fence is never reconciled).
	// Destroy asserts the crds/-directory keep posture.
	case "kubernetessparkoperator":
		spec := manifestSpecMap(manifestPath)
		return &SparkOperatorVerifier{
			Namespace:              info.Namespace,
			Name:                   info.Name,
			WorkloadNamespaces:     sparkWorkloadNamespaces(spec),
			WorkloadServiceAccount: sparkWorkloadServiceAccount(spec),
		}, nil

	// A Ray cluster declaration: the CR to ready, the head Service, THE
	// AUTH GATE (unauthenticated job submission REJECTED — token auth is
	// the catalog default) + THE RAY PROOF (a real job through the head's
	// Job Submission REST API to SUCCEEDED on the cluster's own
	// capacity), and on the behavioral-state lane THE STATE PROOF (the
	// head pod replaced; the recovered head still lists the completed
	// job — control state lived in the composed Valkey, not head-pod
	// memory).
	case "kubernetesraycluster":
		spec := manifestSpecMap(manifestPath)
		return &RayClusterVerifier{
			Namespace:           info.Namespace,
			Name:                info.Name,
			AuthDisabled:        rayClusterAuthDisabled(spec),
			ExistingTokenSecret: rayClusterExistingTokenSecret(spec),
			GcsFaultTolerance:   rayClusterGcsFaultTolerance(spec),
			StateProof:          strings.Contains(manifestPath, "behavioral-state"),
		}, nil

	// A Flink cluster declaration: the CR's JobManager to READY, THE
	// REST CONTRACT (/config answers with the declared Flink version),
	// THE STREAM PROOF on application lanes (the job reaches RUNNING
	// with TaskManagers materialized from its parallelism), and on the
	// behavioral-recovery lane THE RECOVERY PROOF (completed checkpoints
	// observed in the composed S3 store, the JobManager pod replaced,
	// the job back to RUNNING with checkpoint continuity — recovery
	// through HA metadata).
	case "kubernetesflinkdeployment":
		spec := manifestSpecMap(manifestPath)
		return &FlinkDeploymentVerifier{
			Namespace:     info.Namespace,
			Name:          info.Name,
			SessionMode:   flinkDeploymentSessionMode(spec),
			RecoveryProof: strings.Contains(manifestPath, "behavioral-recovery"),
			FlinkVersion:  flinkDeploymentVersion(spec),
		}, nil

	// A NATS messaging system: the StatefulSet rolled out and THE
	// MESSAGING PROOF on every lane — a real nats.go client completes a
	// pub/sub round-trip (authenticated as the first declared user from
	// the module-generated auth Secret when auth is on, with the auth
	// gate asserted), and with JetStream on (the kind's default) a
	// file-backed stream stores a marker a consumer reads back. The
	// behavioral-durability scenario (recognized by name) proves the
	// marker survives a server pod replacement through the JetStream
	// PVC. Destroy is clean by design (no CRDs).
	case "kubernetesnats":
		spec := manifestSpecMap(manifestPath)
		return &NatsVerifier{
			Namespace:        info.Namespace,
			Name:             info.Name,
			JetStreamEnabled: natsJetStreamEnabled(spec),
			FirstUsername:    natsFirstUsername(spec),
			NoAuthUser:       natsNoAuthUser(spec),
			NatsBoxEnabled:   natsBoxEnabled(spec),
			DurabilityProof:  strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// The Kyverno policy engine: every enabled controller rolled out,
	// the runtime-registered webhook configurations present, and THE
	// ENFORCEMENT PROOF on every lane (a verifier-owned Enforce
	// ClusterPolicy rejects a violating Pod and admits a compliant
	// one). The behavioral-enforcement scenario (recognized by name)
	// adds the mutation proof. Destroy asserts the webhook
	// configurations are GONE — the pre-delete cleanup hook is the
	// designed uninstall path.
	case "kuberneteskyverno":
		spec := manifestSpecMap(manifestPath)
		return &KyvernoVerifier{
			Namespace:         info.Namespace,
			Name:              info.Name,
			BackgroundEnabled: nestedBoolWithDefault(spec, "background_controller", "enabled", true),
			CleanupEnabled:    nestedBoolWithDefault(spec, "cleanup_controller", "enabled", true),
			ReportsEnabled:    nestedBoolWithDefault(spec, "reports_controller", "enabled", true),
			Mutation:          strings.Contains(manifestPath, "behavioral-enforcement"),
		}, nil

	// The OPA Gatekeeper engine: controller-manager replicas + the audit
	// controller rolled out, the chart-owned webhook configuration and
	// engine CRDs present, and THE ENFORCEMENT PROOF on every lane (a
	// verifier-owned CEL ConstraintTemplate + deny Constraint rejects a
	// violating Pod and admits a compliant one). The behavioral-audit
	// scenario (recognized by name) adds the audit-loop proof on a
	// pre-constraint victim. Destroy asserts webhook configurations
	// GONE and engine CRDs KEPT (the crds/-directory posture).
	case "kubernetesgatekeeper":
		return &GatekeeperVerifier{
			Namespace: info.Namespace,
			Name:      info.Name,
			Audit:     strings.Contains(manifestPath, "behavioral-audit"),
		}, nil

	// A Qdrant vector database: replicas ready, the main Service
	// present, and a live vector round-trip (collection create → upsert
	// → similarity search asserting the nearest neighbour), carrying the
	// declared API key when auth is on. The behavioral-persistence
	// scenario (recognized by name) proves vectors survive pod loss
	// through the PVC.
	case "kubernetesqdrant":
		spec := manifestSpecMap(manifestPath)
		return &QdrantVerifier{
			Namespace:        info.Namespace,
			Name:             info.Name,
			Replicas:         int(manifestSpecInt(manifestPath, "replicas", 1)),
			ApiKeySecretName: qdrantApiKeySecretName(spec, info.Name),
			Persistence:      strings.Contains(manifestPath, "behavioral-persistence"),
		}, nil

	// declared. The behavioral-persistence scenario (recognized by
	// name) proves the data survives pod loss through the PVC.
	case "kubernetesneo4j":
		spec := manifestSpecMap(manifestPath)
		return &Neo4jVerifier{
			Namespace:      info.Namespace,
			Name:           info.Name,
			AuthSecretName: neo4jAuthSecretName(spec, info.Name),
			Persistence:    strings.Contains(manifestPath, "behavioral-persistence"),
		}, nil

	// A Percona-operator-managed MySQL (XtraDB) cluster: the resource in
	// state ready + the proxy's write Service. The behavioral-durability
	// scenario proves Galera's synchronous replication through a live
	// node loss; the with-backup scenario drives a real XtraBackup to
	// completion.
	case "kubernetesmysql":
		spec := manifestSpecMap(manifestPath)
		return &PxcClusterVerifier{
			Namespace:     info.Namespace,
			ClusterName:   info.Name,
			Size:          manifestSpecInt(manifestPath, "instances", 3),
			ProxyService:  mysqlProxyService(info.Name, spec),
			Behavioral:    strings.Contains(manifestPath, "behavioral-durability"),
			BackupProof:   strings.Contains(manifestPath, "with-backup"),
			BackupStorage: mysqlFirstBackupStorage(spec),
		}, nil

	// A Valkey instance from the official chart: workload ready + the
	// write Service. The behavioral-persistence scenario proves
	// durability through a pod loss; the replication scenario proves
	// propagation through the read Service.
	case "kubernetesvalkey":
		spec := manifestSpecMap(manifestPath)
		replication, replicas := valkeyReplication(spec)
		_, hasAuth := spec["auth"]
		return &ValkeyVerifier{
			Namespace:        info.Namespace,
			Name:             info.Name,
			Replication:      replication,
			Replicas:         replicas,
			AuthDeclared:     hasAuth,
			PersistenceProof: strings.Contains(manifestPath, "behavioral-persistence"),
			ReplicationProof: strings.Contains(manifestPath, "with-replication"),
			ServicePort:      valkeyServicePort(spec),
		}, nil

	// A CloudNativePG-managed PostgreSQL cluster: Ready condition + every
	// declared instance ready + the -rw Service. The behavioral-failover
	// scenario (recognized by name) additionally proves data durability
	// through a live primary loss and promotion.
	case "kubernetespostgres":
		return &CnpgClusterVerifier{
			Namespace:     info.Namespace,
			ClusterName:   info.Name,
			Instances:     manifestSpecInt(manifestPath, "instances", 1),
			Behavioral:    strings.Contains(manifestPath, "behavioral-failover"),
			BackupProof:   strings.Contains(manifestPath, "with-backup"),
			RecoveryProof: scenarioMatches(manifestPath, "gke-", "-recovery"),
			// The recovery reads the source's credentials from the Secret the
			// manifest's recovery block names (`<source>-app`).
			RecoverySourceCluster: strings.TrimSuffix(manifestNestedRecoveryOwnerSecret(manifestPath), "-app"),
			PluginRequired:        manifestDeclaresBarmanPlugin(manifestPath),
		}, nil

	// cert-manager installation: the three component Deployments must be
	// Available and the core CRDs Established — the preconditions every
	// Issuer/Certificate apply depends on.
	case "kubernetescertmanager":
		return &CertManagerInstallVerifier{
			Namespace:     info.Namespace,
			ComponentName: info.Name,
		}, nil

	// Issuers with in-cluster backends (self-signed, CA) reach Ready with
	// no external dependency, so Ready is the verifiable contract on the
	// kind cluster. ClusterIssuer is cluster-scoped (no namespace).
	case "kubernetesclusterissuer":
		return &IssuerVerifier{
			Kind: "clusterissuer",
			Name: info.Name,
		}, nil

	case "kubernetesissuer":
		return &IssuerVerifier{
			Kind:      "issuer",
			Name:      info.Name,
			Namespace: info.Namespace,
		}, nil

	// Real issuance proof: Ready condition + signed material present in the
	// target TLS Secret.
	case "kubernetescertificate":
		secretName, err := manifestSpecString(manifestPath, "secretName")
		if err != nil {
			return nil, err
		}
		return &CertificateVerifier{
			Name:       info.Name,
			Namespace:  info.Namespace,
			SecretName: secretName,
		}, nil

	// Istio control-plane installation: istiod Available + the Istio CRDs
	// Established; ambient scenarios additionally require the istio-cni and
	// ztunnel DaemonSets fully ready. The istiod name carries the revision
	// suffix when the scenario names one.
	case "kubernetesistio":
		istiodName := "istiod"
		if revision, _ := manifestSpecString(manifestPath, "revision"); revision != "" {
			istiodName = "istiod-" + revision
		}
		dataplaneMode, _ := manifestSpecString(manifestPath, "dataplaneMode")
		return &IstioInstallVerifier{
			Namespace:  info.Namespace,
			IstiodName: istiodName,
			Ambient:    dataplaneMode == "ambient",
		}, nil

	// ExternalDNS installation: the controller Deployment must be Available
	// (fullname pinned to metadata.name — multi-instance per cluster is
	// first-class, so the release's own Deployment is the contract, not the
	// namespace).
	case "kubernetesexternaldns":
		// The aws-route53-irsa scenario (real-cluster profile) proves REAL
		// record writes: verifier-owned source, records asserted in the
		// hosted zone via the AWS API, then removed with the source.
		if strings.Contains(manifestPath, "aws-route53-irsa") {
			return &ExternalDnsRoute53Verifier{
				Namespace:     info.Namespace,
				ComponentName: info.Name,
			}, nil
		}
		return &ExternalDnsInstallVerifier{
			Namespace:     info.Namespace,
			ComponentName: info.Name,
		}, nil

	// External Secrets Operator installation: the three component
	// Deployments must be Available and the core CRDs Established — the
	// preconditions every SecretStore/ExternalSecret apply depends on.
	case "kubernetesexternalsecretsoperator":
		return &ExternalSecretsOperatorInstallVerifier{
			Namespace:     info.Namespace,
			ComponentName: info.Name,
		}, nil

	// Stores with in-cluster backends (the fake provider) reach Ready with
	// no external dependency, so Ready is the verifiable contract on the
	// kind cluster. ClusterSecretStore is cluster-scoped (no namespace).
	case "kubernetesclustersecretstore":
		return &SecretStoreVerifier{
			Kind: "clustersecretstore",
			Name: info.Name,
			// The aws-sm-irsa scenario (real-cluster profile) proves a real
			// Secrets Manager read through the keyless IRSA identity.
			BehavioralAwsSm: strings.Contains(manifestPath, "aws-sm-irsa"),
		}, nil

	case "kubernetessecretstore":
		return &SecretStoreVerifier{
			Kind:      "secretstore",
			Name:      info.Name,
			Namespace: info.Namespace,
		}, nil

	// Real sync proof: Ready condition + synced data present in the
	// materialized Secret.
	case "kubernetesexternalsecret":
		secretName, err := externalSecretTargetName(manifestPath)
		if err != nil {
			return nil, err
		}
		return &ExternalSecretSyncVerifier{
			Name:       info.Name,
			Namespace:  info.Namespace,
			SecretName: secretName,
		}, nil

	// ingress-nginx installation: controller Deployment Available + the
	// instance's IngressClass present. Fullname pinned to metadata.name —
	// multiple controllers per cluster are first-class, so the release's
	// own objects are the contract, not the namespace.
	case "kubernetesingressnginx":
		className := manifestNestedSpecString(manifestPath, "ingressClass", "name")
		if className == "" {
			className = "nginx"
		}
		return &IngressNginxInstallVerifier{
			Namespace:        info.Namespace,
			ComponentName:    info.Name,
			IngressClassName: className,
			// The aws-nlb scenario (real-cluster profile) validates the
			// documented AWS annotation recipe down to a provisioned LB.
			LbAddress: strings.Contains(manifestPath, "aws-nlb"),
		}, nil

	// metrics-server installation, verified to METRIC FLOW (Deployment
	// Available + APIService Available + kubectl top returning values).
	case "kubernetesmetricsserver":
		return &MetricsServerInstallVerifier{
			Namespace: info.Namespace,
		}, nil

	case "kubernetesmanifest":
		return &ConfigGroupVerifier{
			Namespace:    info.Namespace,
			ManifestPath: manifestPath,
		}, nil

	// A real Helm release of an arbitrary chart: the Helm verifier's
	// namespace + running-pods + service assertions hold for any chart that
	// deploys a workload (the e2e chart, podinfo, deploys one pod and one
	// service), plus the CRD lifecycle the module promises for the CRDs the
	// scenario declares it expects.
	case "kuberneteshelmrelease":
		return newHelmReleaseVerifier(manifestPath, info.Name, info.Namespace), nil

	// The Tekton Operator: operator + webhook rolled out in the fixed
	// tekton-operator namespace, the operator.tekton.dev CRDs
	// established — and THE DESIGN INVARIANT proven: NO TektonConfig
	// exists after install (auto-install is module-disabled; the
	// KubernetesTekton declaration owns the configuration). Destroy
	// asserts the manifest-bundle posture: everything gone, CRDs
	// included.
	case "kubernetestektonoperator":
		return &TektonOperatorVerifier{
			Name: info.Name,
		}, nil

	// A Tekton installation (the TektonConfig declaration the operator
	// reconciles): TektonConfig Ready, the per-profile component
	// rollouts asserted BOTH ways (dashboard on `all` only), the pruner
	// CronJob when declared — and THE ENGINE PROOF on every lane: a
	// verifier-owned TaskRun runs to Succeeded. Destroy asserts the
	// operator-alive teardown the two-kind grain exists for: no
	// TektonInstallerSet left behind.
	case "kubernetestekton":
		spec := manifestSpecMap(manifestPath)
		return &TektonVerifier{
			Name:            info.Name,
			Profile:         tektonProfile(spec),
			TargetNamespace: tektonTargetNamespace(spec),
			PrunerDeclared:  tektonPrunerDeclared(spec),
		}, nil

	// The Keycloak Operator install (release-manifest bundle): the
	// operator rolled out in the declared namespace, all four
	// k8s.keycloak.org CRDs established — and THE DESIGN INVARIANT
	// proven: no Keycloak server exists after installing the operator
	// alone (the KubernetesKeycloak declaration kind owns servers).
	// Destroy asserts workloads AND CRDs gone (the bundle posture).
	case "kuberneteskeycloakoperator":
		return &KeycloakOperatorVerifier{
			Namespace: info.Namespace,
		}, nil

	// An operator-managed Keycloak server: the Keycloak CR Ready (its
	// boolean-status condition; HasErrors warnings tolerated — a
	// documented legitimate state), the StatefulSet at the declared
	// instance count, the operator's naming contract
	// (`-service`/`-discovery`/`-initial-admin`) — and THE PRODUCT
	// PROOF on every lane: a real admin login (master-realm password
	// grant with the bootstrap credentials) plus an authenticated
	// admin-API read. The behavioral-durability scenario (recognized
	// by name) creates a realm, replaces pod 0, and re-reads the realm
	// through a fresh login — configuration surviving pod replacement
	// is the database-durability proof.
	case "kuberneteskeycloak":
		spec := manifestSpecMap(manifestPath)
		instances, https, port, adminSecret, generated := keycloakScenarioShape(spec, info.Name)
		return &KeycloakVerifier{
			Namespace:            info.Namespace,
			Name:                 info.Name,
			Instances:            instances,
			Https:                https,
			Port:                 port,
			AdminSecretName:      adminSecret,
			GeneratedAdminSecret: generated,
			Behavioral:           strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// The OpenBao secrets manager: THE SEAL LIFECYCLE IS THE PROOF —
	// sealed pods asserted NotReady-by-design, the verifier performs
	// the real init/unseal bootstrap (readiness must FLIP), then a KV
	// round-trip proves the server serves secrets. Dev mode skips
	// init/unseal. The behavioral-raft scenario (recognized by name)
	// replaces pod 0, re-unseals it (restart = sealed, the Shamir
	// truth) and re-reads the marker. NEVER the generic Helm fallback:
	// waiting on readiness hangs every fresh install by design.
	case "kubernetesopenbao":
		spec := manifestSpecMap(manifestPath)
		mode, replicas := openBaoScenarioShape(spec)
		return &OpenBaoVerifier{
			Namespace:  info.Namespace,
			Name:       info.Name,
			Mode:       mode,
			Replicas:   replicas,
			Behavioral: strings.Contains(manifestPath, "behavioral-raft"),
		}, nil

	// The OpenFGA authorization engine: deployment rolled out (the
	// migration init container gates on the datastore), the client
	// Service present — and THE ZANZIBAR PROOF on every lane: a live
	// store → model → tuple → Check round-trip asserting BOTH
	// decisions (granted ALLOWED, ungranted DENIED). With pre-shared
	// keys declared the proof runs authenticated and asserts the
	// unauthenticated 401 first (the auth gate).
	case "kubernetesopenfga":
		spec := manifestSpecMap(manifestPath)
		return &OpenFgaVerifier{
			Namespace: info.Namespace,
			Name:      info.Name,
			ApiKey:    openFgaPresharedKey(spec),
		}, nil

	// The OpenTelemetry Operator: manager rolled out (its pod mounts
	// the cert-manager-issued webhook Secret — the rollout IS the
	// cert-issuance proof), all four module-owned opentelemetry.io
	// CRDs Established, THE ADMISSION GATE (an invalid collector CR
	// REJECTED by the fail-closed webhook) and THE CONVERSION PROOF
	// (a v1beta1-written probe CR read back through v1alpha1 — the
	// call only converts when the CA injector has patched the KEPT
	// CRD's conversion caBundle, the trust seam this kind's
	// module-owned-CRD design hangs on). Destroy asserts the designed
	// keep: workloads gone, CRDs retained.
	case "kubernetesoteloperator":
		return newOtelOperatorVerifier(manifestPath, info.Name, info.Namespace), nil

	// The KubeRay operator: rollout + the three ray.io CRDs established
	// + THE INVARIANT (installing the operator alone deploys NO Ray
	// cluster — the KubernetesRayCluster kind owns every cluster), and
	// with a fenced watch posture THE FENCE PROOF (a RayCluster outside
	// the watched namespaces is never reconciled). Destroy asserts the
	// crds/-directory keep posture.
	case "kuberneteskuberayoperator":
		spec := manifestSpecMap(manifestPath)
		return &KubeRayOperatorVerifier{
			Namespace:       info.Namespace,
			Name:            info.Name,
			WatchNamespaces: specStringList(spec, "watch_namespaces", "watchNamespaces"),
		}, nil

	// The Flink operator: rollout + the four flink.apache.org CRDs
	// established + THE INVARIANT (no FlinkDeployment after installing
	// the operator alone). Webhook lanes prove the module-generated
	// keystore Secret (the chart's hardcoded default never ships) and
	// THE ADMISSION GATE (an invalid FlinkDeployment — standbys without
	// HA — REJECTED at admission by the fail-closed webhook, with the
	// namespaceSelector's scoping proven on fenced lanes); the
	// webhook-less lane proves THE POSTURE CONTRAST (the same invalid
	// CR accepted — validation deferred to the reconcile loop, the
	// honest trade of that arm). Destroy asserts the crds/-directory
	// keep posture.
	case "kubernetesflinkoperator":
		spec := manifestSpecMap(manifestPath)
		return &FlinkOperatorVerifier{
			Namespace:       info.Namespace,
			Name:            info.Name,
			WebhookEnabled:  flinkOperatorWebhookEnabled(spec),
			WatchNamespaces: specStringList(spec, "watch_namespaces", "watchNamespaces"),
		}, nil

	// An operator-managed OpenTelemetry Collector: the mode's workload
	// rolled out — and THE PIPELINE PROOF on every lane (a real
	// OTLP/HTTP push whose marker lands in the debug exporter output;
	// telemetry THROUGH the declared pipeline, never just a running
	// pod). The daemonset lane (spec mode) adds THE FILELOG PROOF
	// (node log files actually ingested — the run-as-root pattern);
	// the behavioral-pipeline scenario (recognized by name) adds THE
	// RECONCILE PROOF (a verifier-patched config rolled onto live
	// pipeline behavior).
	case "kubernetesotelcollector":
		spec := manifestSpecMap(manifestPath)
		mode, _ := spec["mode"].(string)
		return &OtelCollectorVerifier{
			Namespace:  info.Namespace,
			Name:       info.Name,
			Daemonset:  mode == "daemonset",
			Behavioral: strings.Contains(manifestPath, "behavioral-pipeline"),
		}, nil

	// Harbor container registry: every stateless component rolled out,
	// the front-door Service present — and THE REGISTRY PROOF on every
	// lane: login as the module-generated admin, create a project, OCI
	// push/pull round-trip, asserting the unauthenticated 401 first
	// (the auth gate). The behavioral-durability scenario (recognized
	// by name) proves the artifact survives a UID-verified registry
	// pod replacement through a fresh port-forward (dead-tunnel class).
	case "kubernetesharbor":
		return &HarborVerifier{
			Namespace:  info.Namespace,
			Name:       info.Name,
			Durability: strings.Contains(manifestPath, "behavioral-durability"),
		}, nil

	// A Locust load-testing cluster: master and worker rollouts, THE
	// AUTH GATE (anonymous stats read bounced to the login —
	// upstream's open web UI never ships), THE LOGIN PROOF (wrong
	// password refused, the module-generated credential signs in
	// through the platform-managed backend), and THE SWARM PROOF on
	// every lane — a real distributed test through the master's own
	// REST API drives real requests through registered workers at the
	// composed fixture target with zero failures. The behavioral
	// scenario (recognized by name) adds THE RECONNECT PROOF: a
	// UID-verified master replacement, the SAME session still
	// authenticating (the stable session-signing key design), workers
	// re-registered, and a second swarm. Destroy is clean: a plain
	// Helm release plus module-owned ConfigMaps and Secret.
	case "kuberneteslocust":
		spec := manifestSpecMap(manifestPath)
		return &LocustVerifier{
			Namespace:       info.Namespace,
			Name:            info.Name,
			WebLoginEnabled: locustWebLoginEnabled(spec),
			Username:        locustUsername(spec),
			WorkerReplicas:  locustWorkerReplicas(spec),
			ReconnectProof:  strings.Contains(manifestPath, "behavioral"),
		}, nil

	// The GitHub Actions runner scale set controller: the controller
	// rolled out at the declared replica count and every
	// actions.github.com CRD established. Destroy asserts the
	// chart-owned CRD posture: the CRDs delete WITH the release.
	case "kubernetesgharunnerscalesetcontroller":
		spec := manifestSpecMap(manifestPath)
		return &GhaRunnerScaleSetControllerVerifier{
			Namespace: info.Namespace,
			Name:      info.Name,
			Replicas:  ghaControllerReplicas(spec),
		}, nil

	// A GitHub Actions runner scale set: the credential Secret present,
	// the AutoscalingRunnerSet rendered and OBSERVED by the controller
	// (the reconcile-attempt proof — no GitHub account needed). The
	// behavioral-github scenario (recognized by name) adds THE
	// REGISTRATION PROOF with a real credential from the fenced env
	// tokens: listener Running + the declared idle runners online.
	case "kubernetesgharunnerscaleset":
		spec := manifestSpecMap(manifestPath)
		return &GhaRunnerScaleSetVerifier{
			Namespace:         info.Namespace,
			Name:              info.Name,
			ScaleSetName:      ghaScaleSetName(spec, info.Name),
			AuthSecretName:    ghaAuthSecretName(spec, info.Name),
			MinRunners:        ghaMinRunners(spec),
			RegistrationProof: strings.Contains(manifestPath, "behavioral-github"),
		}, nil

	// A Planton runner from the official chart: the module-created token
	// Secret present, the Deployment pinned to one replica with the
	// Recreate strategy, and the enrollment env wiring on the pod
	// template. Deliberately no pod-readiness assertion — the kind lanes
	// run with a fake token whose refused join is designed behavior.
	case "kubernetesplantonrunner":
		return &PlantonRunnerVerifier{
			Namespace: info.Namespace,
			Name:      info.Name,
		}, nil

	// The Planton operator: the manager Available in its namespace, the
	// chart's two definitions Established — and THE DESIGN INVARIANT
	// proven: NO PlantonPlatform exists after installing the operator
	// alone (platforms are always deliberate declarations). Destroy
	// asserts the keep dial the manifest chose: the definitions survive
	// (the default) or leave with the release.
	case "kubernetesplantonoperator":
		return newPlantonOperatorInstallVerifier(info.Namespace, manifestPath), nil

	// A declared Planton platform: the PlantonPlatform reaches phase
	// Ready (the operator's per-component gates all pass inside it) and
	// the first-visit handles exist (gateway Service, setup-code
	// Secret). Destroy polls the garbage-collected drain — the operator
	// has no finalizers; teardown is owner-reference GC.
	case "kubernetesplantonplatform":
		return newPlantonPlatformVerifier(info.Namespace, info.Name, manifestPath), nil

	default:
		if crdNames, ok := crdInstallKinds[component]; ok {
			return &CRDInstallVerifier{
				ComponentName: info.Name,
				CRDNames:      crdNames,
			}, nil
		}
		// HTTPRoute: scenarios that install a real Gateway API implementation
		// (KubernetesIstio prerequisite) get the behavioral verifier — the
		// route Accepted, its Gateway Programmed, and a live request routed
		// through the auto-provisioned gateway. Scenarios without it keep the
		// object as the verifiable contract.
		if component == "kuberneteshttproute" && manifestHasPrerequisite(manifestPath, "KubernetesIstio") {
			hostname, _ := manifestSpecFirstString(manifestPath, "hostnames")
			return &GatewayRoutingBehavioralVerifier{
				Namespace:   info.Namespace,
				RouteName:   info.Name,
				GatewayName: "e2e-istio-gw",
				Hostname:    hostname,
			}, nil
		}
		// Gateway: the aws-lb-address scenario (real-cluster profile, LB-
		// default mesh fixture) proves Programmed + a real cloud address in
		// .status.addresses — the half the kind lanes pin away.
		if component == "kubernetesgateway" && strings.Contains(manifestPath, "aws-lb-address") {
			return &GatewayLbAddressVerifier{
				Namespace: info.Namespace,
				Name:      info.Name,
			}, nil
		}
		if gw, ok := gatewayApiKinds[component]; ok {
			namespace := info.Namespace
			if gw.clusterScoped {
				namespace = ""
			}
			return &ResourceExistenceVerifier{
				Namespace: namespace,
				Kind:      gw.resource,
				Name:      info.Name,
			}, nil
		}
		// AuthorizationPolicy: scenarios that install a real mesh
		// (KubernetesIstio prerequisite) get the behavioral verifier — a
		// meshed client's request must actually be DENIED (403) and succeed
		// again once the policy is destroyed. Scenarios without it keep the
		// object as the verifiable contract.
		if component == "kubernetesauthorizationpolicy" && manifestHasPrerequisite(manifestPath, "KubernetesIstio") {
			return &AuthzDenyBehavioralVerifier{
				Namespace:        info.Namespace,
				PolicyName:       info.Name,
				ClientDeployment: "e2e-authz-client",
				BackendURL:       "http://e2e-authz-backend." + info.Namespace + ".svc.cluster.local/",
			}, nil
		}
		if resource, ok := istioApiKinds[component]; ok {
			return &ResourceExistenceVerifier{
				Namespace: info.Namespace,
				Kind:      resource,
				Name:      info.Name,
			}, nil
		}
		if operatorKinds[component] {
			return &OperatorComponentVerifier{
				Namespace:     info.Namespace,
				ComponentName: info.Name,
			}, nil
		}
		if helmTier2Kinds[component] {
			return &HelmComponentVerifier{
				Namespace:     info.Namespace,
				ComponentName: info.Name,
			}, nil
		}
		return &GenericVerifier{Component: component}, nil
	}
}
