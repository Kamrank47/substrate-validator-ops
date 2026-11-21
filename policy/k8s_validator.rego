package main

import rego.v1

# Enforce Midnight validator Kubernetes manifest guardrails.
# Requires StatefulSet (never Deployment), non-root + read-only rootfs,
# liveness/readiness probes, and blocks LoadBalancer/NodePort exposure of 9944 or 9615.

as_list(x) := x if {
	is_array(x)
} else := [x]

restricted_ports := {22, 9615, 9944}

deny contains msg if {
	input.kind == "Deployment"
	contains(lower(input.metadata.name), "midnight-node")
	msg := sprintf("Deployment/%s: Midnight validators hold persistent chain state and identity and must use StatefulSet, not Deployment", [input.metadata.name])
}

# Inspect PodSpec across StatefulSet and Deployment
pod_spec := input.spec.template.spec if {
	input.kind in {"StatefulSet", "Deployment", "DaemonSet"}
}

pod_spec := input.spec if {
	input.kind == "Pod"
}

deny contains msg if {
	pod_spec
	sc := object.get(pod_spec, "securityContext", {})
	sc.runAsNonRoot != true
	msg := sprintf("%s/%s: pod securityContext.runAsNonRoot must be true", [input.kind, input.metadata.name])
}

deny contains msg if {
	pod_spec
	some c in as_list(pod_spec.containers)
	sc := object.get(c, "securityContext", {})
	sc.readOnlyRootFilesystem != true
	msg := sprintf("%s/%s container %s: securityContext.readOnlyRootFilesystem must be true", [input.kind, input.metadata.name, c.name])
}

deny contains msg if {
	pod_spec
	some c in as_list(pod_spec.containers)
	sc := object.get(c, "securityContext", {})
	sc.allowPrivilegeEscalation != false
	msg := sprintf("%s/%s container %s: securityContext.allowPrivilegeEscalation must be false", [input.kind, input.metadata.name, c.name])
}

deny contains msg if {
	pod_spec
	some c in as_list(pod_spec.containers)
	not c.readinessProbe
	msg := sprintf("%s/%s container %s: readinessProbe is required for safe RPC/P2P traffic gating", [input.kind, input.metadata.name, c.name])
}

deny contains msg if {
	pod_spec
	some c in as_list(pod_spec.containers)
	not c.livenessProbe
	msg := sprintf("%s/%s container %s: livenessProbe is required", [input.kind, input.metadata.name, c.name])
}

# Prevent public Service exposure of validator RPC (9944) or Prometheus metrics (9615)
deny contains msg if {
	input.kind == "Service"
	input.spec.type in {"LoadBalancer", "NodePort"}
	some p in as_list(input.spec.ports)
	some bad_port in restricted_ports
	p.port == bad_port
	msg := sprintf("Service/%s (%s): never expose restricted port %d externally (keep RPC 9944 and metrics 9615 internal-only)", [input.metadata.name, input.spec.type, bad_port])
}
