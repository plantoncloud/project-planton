package resources

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// The Barman Cloud plugin is CloudNativePG's backup engine: it archives WAL
// continuously, takes base backups on a schedule, and restores a cluster from
// an object store. CloudNativePG deprecated its in-tree backup path in favour
// of this plugin, so a cluster with no plugin cannot back itself up -- and a
// cluster that DECLARES a backup with no plugin on the cluster is parked by
// the operator in its "unknown plugin" phase with no instances at all.
//
// The plugin is a shared cluster capability, exactly like CloudNativePG
// itself: one release, in CloudNativePG's own namespace (the operator
// discovers plugins only through Services in its own namespace), serving
// every PostgreSQL cluster on the cluster -- the platform's own database and
// any database an adopter deploys through Planton. It is therefore installed
// and removed by the same detect-or-install gate and janitor the other shared
// sub-operators use, never as a platform-owned object.
//
// The chart bakes the plugin's gRPC Service name into its TLS certificate and
// fixes its two TLS Secrets, its Certificates and its config ConfigMap by name,
// so a second release in the same namespace would fight the first over all of
// them: the release name is a constant, and the identity pins below are
// re-asserted over the chart's defaults so nothing can rename them.
const (
	// BarmanCloudPluginChartVersion is the vendored chart's version
	// (Makefile: BARMAN_PLUGIN_HELM_CHART_VERSION). Chart 0.7.0 ships plugin
	// v0.13.0; the pin is the same one the OSS catalog's
	// kubernetescnpgbarmancloudplugin kind renders, so a platform-operator
	// cluster and a catalog-installed cluster run the same plugin.
	BarmanCloudPluginChartVersion = "0.7.0"

	// BarmanCloudPluginReleaseName is the fixed Helm release name (equal to
	// the chart name, so the chart's fullname collapses to it). The ObjectStore
	// definition the chart keeps on uninstall is re-adopted by a later install
	// ONLY when the release name and namespace match, so it never varies.
	BarmanCloudPluginReleaseName = "plugin-barman-cloud"

	// BarmanCloudPluginDeploymentName is the plugin's controller Deployment,
	// the readiness target the sub-operator gate waits on.
	BarmanCloudPluginDeploymentName = "plugin-barman-cloud"

	// BarmanCloudPluginServiceName is the gRPC Service CloudNativePG dials.
	// The chart's own values file says: DO NOT CHANGE -- it is the name the
	// server certificate is issued for.
	BarmanCloudPluginServiceName = "barman-cloud"

	// BarmanCloudPluginName is the name a Cluster's spec.plugins entry and a
	// ScheduledBackup's pluginConfiguration refer to the plugin by; it is the
	// value of the cnpg.io/pluginName label on the plugin's Service.
	BarmanCloudPluginName = "barman-cloud.cloudnative-pg.io"

	// BarmanCloudObjectStoreCRDName is the definition whose presence means
	// "the plugin is installed" -- the detect probe of the sub-operator gate,
	// and the one CRD the plugin release carries.
	BarmanCloudObjectStoreCRDName = "objectstores.barmancloud.cnpg.io"

	// CloudNativePGNamespace is where the vendored CloudNativePG release
	// installs its controller, and therefore the only namespace the plugin
	// can live in. Shared by the two releases: the janitor removes the plugin
	// before CloudNativePG so the namespace is never swept from under a
	// plugin that is still marked as ours.
	CloudNativePGNamespace = "cnpg-system"

	barmanCloudAPIGroup   = "barmancloud.cnpg.io"
	barmanCloudAPIVersion = "v1"
)

// BarmanCloudPluginHelmValues builds the values the plugin chart is rendered
// with. The chart's defaults are right for a shared install (one replica,
// cert-manager-issued TLS between operator and plugin, the ObjectStore CRD as
// a release resource); the values here only pin the identity the plugin's
// TLS and discovery depend on, the same pins the OSS catalog's plugin kind
// re-asserts last so no override can move them.
func BarmanCloudPluginHelmValues() map[string]any {
	return map[string]any{
		// The CRD renders as a template (not Helm's install-once crds/
		// directory), so it rides the same apply and the same removal as the
		// rest of the release -- "uninstall removes what install added".
		"crds": map[string]any{
			"create": true,
		},
		"nameOverride":      "",
		"fullnameOverride":  "",
		"namespaceOverride": "",
		"service": map[string]any{
			"name": BarmanCloudPluginServiceName,
		},
	}
}

// LoadBarmanCloudPluginManifests renders the embedded plugin chart into the
// objects the sub-operator gate applies: the ObjectStore definition, the
// plugin Deployment and its Service, the ConfigMap naming the sidecar image,
// the RBAC the plugin needs cluster-wide, and the cert-manager Issuer and
// Certificates that mint the operator-to-plugin TLS pair. It is the
// chart-shaped sibling of LoadCloudNativePGManifests: same loader contract,
// rendered from a chart archive instead of parsed from a release YAML.
//
// cert-manager is a prerequisite the caller checks before installing (the
// Certificates never become ready without it and the plugin never serves);
// this loader renders unconditionally so the janitor can enumerate exactly
// what an install put on the cluster.
func LoadBarmanCloudPluginManifests() ([]*unstructured.Unstructured, error) {
	return RenderHelmChart(barmanPluginChartData, BarmanCloudPluginReleaseName, CloudNativePGNamespace, BarmanCloudPluginHelmValues())
}
