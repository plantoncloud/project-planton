output "route_name" {
  description = "Name of the created HTTPRoute (equals metadata.name)."
  value       = var.metadata.name
}

output "namespace" {
  description = "Namespace of the created HTTPRoute."
  value       = var.spec.namespace
}

output "first_host" {
  description = "The first hostname the route matches (spec.hostnames[0]); empty when the route declares none. Named as the Ingress kind names its first rule's host."
  value       = try(var.spec.hostnames[0], "")
}
