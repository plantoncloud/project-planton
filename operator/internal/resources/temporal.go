package resources

import "fmt"

const (
	TemporalHelmChartVersion = "0.62.0"
	TemporalFrontendGRPCPort = 7233
	// temporalFrontendGRPCPortName is the name the Temporal chart gives its
	// frontend's gRPC Service port. The "grpc-" prefix is load-bearing for
	// meshes that pick the upstream protocol from the port name: the front
	// door's route to this port must speak HTTP/2 cleartext to the frontend.
	temporalFrontendGRPCPortName = "grpc-rpc"
	TemporalFrontendHTTPPort     = 7243
	TemporalWebUIPort            = 8080
	TemporalDefaultDB            = "temporal"
	TemporalVisibilityDB         = "temporal_visibility"
	TemporalPostgresDriver       = "postgres12"
)

// TemporalHelmValues builds the Helm values map for rendering the Temporal
// chart. The PostgreSQL connection details come from the shared connection
// seam; Temporal's own schema job creates its two databases (the
// createDatabase toggle below), which is why the connection user must be able
// to CREATE DATABASE.
func TemporalHelmValues(crName, namespace string) map[string]any {
	conn := PostgreSQLConnection(crName, namespace)

	sqlConfig := func(database string) map[string]any {
		return map[string]any{
			"driver": "sql",
			"sql": map[string]any{
				"driver":         TemporalPostgresDriver,
				"host":           conn.Host,
				"port":           conn.Port,
				"database":       database,
				"user":           conn.User,
				"existingSecret": conn.SecretName,
				"secretKey":      conn.PassKey,
			},
		}
	}

	return map[string]any{
		"fullnameOverride": fmt.Sprintf("%s-temporal", crName),

		"cassandra":  map[string]any{"enabled": false},
		"mysql":      map[string]any{"enabled": false},
		"postgresql": map[string]any{"enabled": false},

		"server": map[string]any{
			"config": map[string]any{
				"persistence": map[string]any{
					"driver":     "sql",
					"default":    sqlConfig(TemporalDefaultDB),
					"visibility": sqlConfig(TemporalVisibilityDB),
				},
			},
		},

		"schema": map[string]any{
			"createDatabase": map[string]any{"enabled": true},
			"setup":          map[string]any{"enabled": true},
			"update":         map[string]any{"enabled": true},
		},

		"prometheus":          map[string]any{"enabled": false},
		"grafana":             map[string]any{"enabled": false},
		"kubePrometheusStack": map[string]any{"enabled": false},
		"elasticsearch":       map[string]any{"enabled": false},
	}
}

// TemporalFrontendServiceName returns the Kubernetes Service name for the
// Temporal frontend, which is the primary gRPC endpoint applications connect to.
func TemporalFrontendServiceName(crName string) string {
	return fmt.Sprintf("%s-temporal-frontend", crName)
}

// TemporalWebUIServiceName returns the Kubernetes Service name for the
// Temporal web UI.
func TemporalWebUIServiceName(crName string) string {
	return fmt.Sprintf("%s-temporal-web", crName)
}

// TemporalFrontendEndpoint returns the in-cluster FQDN for the Temporal
// frontend gRPC service.
func TemporalFrontendEndpoint(crName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		TemporalFrontendServiceName(crName), namespace, TemporalFrontendGRPCPort)
}
