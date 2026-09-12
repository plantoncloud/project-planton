package module

const (
	OpRouteName = "route_name"
	OpNamespace = "namespace"
	// The route's first hostname, named as the Ingress kind names its first
	// rule's host so one reader serves both carriers.
	OpFirstHost = "first_host"
)
