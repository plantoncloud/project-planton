# KubernetesCronJob

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesCronJobSpec** runs work on a recurring schedule on a Kubernetes
cluster as a batch/v1 CronJob: at each scheduled time the controller creates a
Job from `job_template`, and that Job runs pods to completion. This is the kind
for scheduled work — nightly backups, report generation, periodic cleanup. For
one-shot work use KubernetesJob; for always-on services use KubernetesDeployment.

The spec splits cleanly in two: scheduling controls (schedule, time zone,
concurrency, history retention) live at the top level, and everything about the
work itself lives in `job_template`, which mirrors KubernetesJobSpec's batch
controls.

DEPLOY-TARGET CONTRACT: this kind is a Service Hub deployment target for
scheduled workloads built from user code. Deployment pipelines inject the
freshly built artifact at `spec.job_template.container.app.image` (repo + tag).
This path is part of the kind's public contract and must survive any future
spec evolution.

## Example

```yaml
# Local development / offline-proof manifest exercising the full spec surface,
# including arms the live E2E scenarios exclude — the offline tofu plan and
# pulumi preview proofs run against this file.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCronJob
metadata:
  name: hack-cronjob
  id: hack-cronjob-id
  org: hack-org
  env: hack-env
spec:
  namespace:
    value: hack-cronjob-ns
  create_namespace: true
  schedule: "*/15 * * * *"
  time_zone: America/New_York
  starting_deadline_seconds: 300
  concurrency_policy: Replace
  suspend: false
  successful_jobs_history_limit: 5
  failed_jobs_history_limit: 2
  job_template:
    container:
      app:
        image:
          repo: busybox
          tag: "1.36"
        image_pull_policy: IfNotPresent
        working_dir: /work
        command: ["/bin/sh", "-c"]
        args: ["echo run index $JOB_COMPLETION_INDEX && cat /work/marker.txt && echo done"]
        resources:
          requests:
            cpu: "10m"
            memory: "32Mi"
          limits:
            cpu: "100m"
            memory: "64Mi"
        env:
          variables:
            - name: SYNC_TARGET
              value: hack-target
            - name: POD_NAME
              field_ref:
                field_path: metadata.name
            - name: MEM_LIMIT
              resource_field_ref:
                resource: limits.memory
          secrets:
            - name: API_TOKEN
              value: hack-token
            - name: DB_PASSWORD
              secret_ref:
                name: hack-existing-secret
                key: password
          env_from:
            - config_map_ref:
                name: hack-env-config
                optional: true
        volume_mounts:
          - name: work
            mount_path: /work
            empty_dir:
              size_limit: 32Mi
          - name: host-data
            mount_path: /host-data
            read_only: true
            host_path:
              path: /var/tmp
              type: Directory
        lifecycle:
          post_start:
            exec:
              command: ["/bin/sh", "-c", "true"]
        security_context:
          run_as_non_root: true
          read_only_root_filesystem: false
          allow_privilege_escalation: false
          capabilities:
            drop: ["ALL"]
          seccomp_profile:
            type: RuntimeDefault
      sidecars:
        # Sidecars in a run's pod must exit for the Job to complete; this one does.
        - name: log-flusher
          image:
            repo: busybox
            tag: "1.36"
          command: ["/bin/sh", "-c", "echo logs flushed"]
          resources:
            requests:
              cpu: "5m"
              memory: "16Mi"
            limits:
              cpu: "50m"
              memory: "32Mi"
          volume_mounts:
            - name: work
              mount_path: /work
    pod:
      service_account:
        value: hack-service-account
      automount_service_account_token: false
      image_pull_secrets:
        - value: hack-registry-secret
      # A login declared on the workload itself; the module materializes it into the
      # <workload>-image-pull Secret. A real manifest carries a $secret/ reference here
      # -- this offline-proof file uses a literal so the plan renders without a backend.
      image_registries:
        - server: ghcr.io
          username: hack-pull-bot
          password: hack-pull-token
      init_containers:
        - name: init-marker
          image:
            repo: busybox
            tag: "1.36"
          command: ["/bin/sh", "-c", "echo prepared > /work/marker.txt"]
          volume_mounts:
            - name: work
              mount_path: /work
      labels:
        team: hack
      annotations:
        example.org/owner: hack
      scheduling:
        node_selector:
          kubernetes.io/os: linux
        tolerations:
          - key: dedicated
            operator: Equal
            value: batch
            effect: NoSchedule
        node_affinity:
          preferred:
            - weight: 50
              term:
                match_expressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values: ["arm64", "amd64"]
        pod_anti_affinity:
          preferred:
            - weight: 100
              term:
                match_labels:
                  app: hack-cronjob
                topology_key: kubernetes.io/hostname
        topology_spread_constraints:
          - max_skew: 1
            topology_key: topology.kubernetes.io/zone
            when_unsatisfiable: ScheduleAnyway
      security_context:
        run_as_user: 1000
        run_as_group: 1000
        run_as_non_root: true
        fs_group: 1000
        fs_group_change_policy: OnRootMismatch
      termination_grace_period_seconds: 30
      dns_policy: ClusterFirst
      dns_config:
        options:
          - name: ndots
            value: "2"
      host_aliases:
        - ip: 203.0.113.10
          hostnames: ["legacy.internal.example"]
      priority_class_name: ""
      runtime_class_name: ""
    parallelism: 2
    completions: 4
    completion_mode: Indexed
    backoff_limit: 6
    backoff_limit_per_index: 2
    max_failed_indexes: 1
    active_deadline_seconds: 600
    ttl_seconds_after_finished: 3600
    restart_policy: Never
    pod_failure_policy:
      rules:
        - action: FailJob
          on_exit_codes:
            container_name: ""
            operator: In
            values: [42]
        - action: FailIndex
          on_exit_codes:
            container_name: log-flusher
            operator: NotIn
            values: [1, 2]
        - action: Ignore
          on_pod_conditions:
            - type: DisruptionTarget
              status: "True"
        - action: Count
          on_pod_conditions:
            - type: ConfigIssue
              status: Unknown
    # success_policy is deliberately absent here: the offline plan proof runs
    # through BOTH engines from this one manifest, and the Terraform provider
    # cannot express success_policy (see the PARITY-EXCEPTION in the modules).
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.schedule` | `string` | yes |  |  |
| `spec.timeZone` | `string` |  |  |  |
| `spec.startingDeadlineSeconds` | `int64` |  |  |  |
| `spec.concurrencyPolicy` | `string` |  | `Forbid` |  |
| `spec.suspend` | `bool` |  |  |  |
| `spec.successfulJobsHistoryLimit` | `int32` |  | `3` |  |
| `spec.failedJobsHistoryLimit` | `int32` |  | `1` |  |
| `spec.jobTemplate` | `KubernetesCronJobJobTemplate` | yes |  |  |
| `spec.jobTemplate.container` | `KubernetesCronJobContainer` | yes |  |  |
| `spec.jobTemplate.container.app` | `WorkloadContainer` | yes |  |  |
| `spec.jobTemplate.container.app.name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.image` | `WorkloadContainerImage` | yes |  |  |
| `spec.jobTemplate.container.app.image.repo` | `string` |  |  |  |
| `spec.jobTemplate.container.app.image.tag` | `string` |  |  |  |
| `spec.jobTemplate.container.app.imagePullPolicy` | `string` |  |  |  |
| `spec.jobTemplate.container.app.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.args` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.workingDir` | `string` |  |  |  |
| `spec.jobTemplate.container.app.ports` | `[]WorkloadContainerPort` |  |  |  |
| `spec.jobTemplate.container.app.ports[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.ports[].containerPort` | `int32` | yes |  |  |
| `spec.jobTemplate.container.app.ports[].networkProtocol` | `string` |  |  |  |
| `spec.jobTemplate.container.app.ports[].appProtocol` | `string` |  |  |  |
| `spec.jobTemplate.container.app.ports[].servicePort` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.ports[].hostPort` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.env` | `ContainerEnv` |  |  |  |
| `spec.jobTemplate.container.app.env.variables` | `[]EnvVar` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].configMapKeyRef` | `ConfigMapKeyRef` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].configMapKeyRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].configMapKeyRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].configMapKeyRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].fieldRef` | `ObjectFieldRef` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].fieldRef.apiVersion` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].fieldRef.fieldPath` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].resourceFieldRef` | `ResourceFieldRef` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].resourceFieldRef.containerName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.variables[].resourceFieldRef.resource` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.variables[].resourceFieldRef.divisor` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets` | `[]SecretEnvVar` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.secrets[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].secretRef` | `KubernetesSecretKeyRef` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].secretRef.namespace` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.secrets[].secretRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.secrets[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.secrets[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.secrets[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom` | `[]EnvFromSource` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].prefix` | `string` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].configMapRef` | `ConfigMapRef` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].configMapRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].configMapRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].secretRef` | `SecretRef` |  |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.env.envFrom[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.resources` | `ContainerResources` |  |  |  |
| `spec.jobTemplate.container.app.resources.limits` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.container.app.resources.limits.cpu` | `string` |  |  |  |
| `spec.jobTemplate.container.app.resources.limits.memory` | `string` |  |  |  |
| `spec.jobTemplate.container.app.resources.requests` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.container.app.resources.requests.cpu` | `string` |  |  |  |
| `spec.jobTemplate.container.app.resources.requests.memory` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.app.livenessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.app.readinessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.app.startupProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts` | `[]VolumeMount` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].mountPath` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].subPath` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].configMap` | `ConfigMapVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].configMap.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].configMap.key` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].configMap.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].configMap.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].secret` | `SecretVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].secret.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].secret.key` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].secret.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].secret.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].hostPath` | `HostPathVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].hostPath.path` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].hostPath.type` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].emptyDir` | `EmptyDirVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].emptyDir.medium` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].emptyDir.sizeLimit` | `string` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].pvc` | `PvcVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].pvc.claimName` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].pvc.readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken` | `ServiceAccountTokenVolumeSource` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.audience` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.expirationSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle` | `WorkloadContainerLifecycle` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.postStart.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.container.app.lifecycle.preStop.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.app.securityContext` | `WorkloadContainerSecurityContext` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.privileged` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.runAsUser` | `int64` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.runAsGroup` | `int64` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.runAsNonRoot` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.readOnlyRootFilesystem` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.allowPrivilegeEscalation` | `bool` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.capabilities` | `WorkloadCapabilities` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.capabilities.add` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.capabilities.drop` | `[]string` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.seccompProfile` | `WorkloadSeccompProfile` |  |  |  |
| `spec.jobTemplate.container.app.securityContext.seccompProfile.type` | `string` | yes |  |  |
| `spec.jobTemplate.container.app.securityContext.seccompProfile.localhostProfile` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars` | `[]WorkloadContainer` |  |  |  |
| `spec.jobTemplate.container.sidecars[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].image` | `WorkloadContainerImage` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].image.repo` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].image.tag` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].imagePullPolicy` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].args` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].workingDir` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].ports` | `[]WorkloadContainerPort` |  |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].containerPort` | `int32` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].networkProtocol` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].appProtocol` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].servicePort` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].ports[].hostPort` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env` | `ContainerEnv` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables` | `[]EnvVar` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef` | `ConfigMapKeyRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].fieldRef` | `ObjectFieldRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].fieldRef.apiVersion` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].fieldRef.fieldPath` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef` | `ResourceFieldRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.containerName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.resource` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.divisor` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets` | `[]SecretEnvVar` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].secretRef` | `KubernetesSecretKeyRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.namespace` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom` | `[]EnvFromSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].prefix` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef` | `ConfigMapRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef` | `SecretRef` |  |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources` | `ContainerResources` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.limits` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.limits.cpu` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.limits.memory` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.requests` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.requests.cpu` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].resources.requests.memory` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].livenessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].readinessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].startupProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts` | `[]VolumeMount` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].mountPath` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].subPath` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].configMap` | `ConfigMapVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.key` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].secret` | `SecretVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].secret.name` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].secret.key` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].secret.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].secret.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath` | `HostPathVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath.path` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath.type` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir` | `EmptyDirVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir.medium` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir.sizeLimit` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].pvc` | `PvcVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].pvc.claimName` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].pvc.readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken` | `ServiceAccountTokenVolumeSource` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.audience` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.expirationSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle` | `WorkloadContainerLifecycle` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.postStart.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.container.sidecars[].lifecycle.preStop.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext` | `WorkloadContainerSecurityContext` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.privileged` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.runAsUser` | `int64` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.runAsGroup` | `int64` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.runAsNonRoot` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.readOnlyRootFilesystem` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.allowPrivilegeEscalation` | `bool` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.capabilities` | `WorkloadCapabilities` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.capabilities.add` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.capabilities.drop` | `[]string` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.seccompProfile` | `WorkloadSeccompProfile` |  |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.seccompProfile.type` | `string` | yes |  |  |
| `spec.jobTemplate.container.sidecars[].securityContext.seccompProfile.localhostProfile` | `string` |  |  |  |
| `spec.jobTemplate.pod` | `WorkloadPod` |  |  |  |
| `spec.jobTemplate.pod.serviceAccount` | `string \| valueFrom` |  |  | KubernetesServiceAccount (`status.outputs.service_account_name`) |
| `spec.jobTemplate.pod.automountServiceAccountToken` | `bool` |  |  |  |
| `spec.jobTemplate.pod.imagePullSecrets` | `[]string \| valueFrom` |  |  | KubernetesSecret (`spec.name`) |
| `spec.jobTemplate.pod.imageRegistries` | `[]WorkloadImageRegistry` |  |  |  |
| `spec.jobTemplate.pod.imageRegistries[].server` | `string` | yes |  |  |
| `spec.jobTemplate.pod.imageRegistries[].username` | `string` | yes |  |  |
| `spec.jobTemplate.pod.imageRegistries[].password` | `string` (sensitive) | yes |  |  |
| `spec.jobTemplate.pod.imageRegistries[].email` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers` | `[]WorkloadContainer` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].image` | `WorkloadContainerImage` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].image.repo` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].image.tag` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].imagePullPolicy` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].args` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].workingDir` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].ports` | `[]WorkloadContainerPort` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].containerPort` | `int32` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].networkProtocol` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].appProtocol` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].servicePort` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].ports[].hostPort` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env` | `ContainerEnv` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables` | `[]EnvVar` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef` | `ConfigMapKeyRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef` | `ObjectFieldRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef.apiVersion` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef.fieldPath` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef` | `ResourceFieldRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.containerName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.resource` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.divisor` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets` | `[]SecretEnvVar` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef` | `KubernetesSecretKeyRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.namespace` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.key` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom` | `ValueFromRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.kind` | `enum` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.env` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.fieldPath` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom` | `[]EnvFromSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].prefix` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef` | `ConfigMapRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef` | `SecretRef` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef.optional` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources` | `ContainerResources` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.limits` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.limits.cpu` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.limits.memory` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.requests` | `CpuMemory` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.requests.cpu` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].resources.requests.memory` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].livenessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].readinessProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe` | `Probe` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.initialDelaySeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.periodSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.timeoutSeconds` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.successThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.failureThreshold` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.grpc` | `GRPCAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.grpc.port` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.grpc.service` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].startupProbe.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts` | `[]VolumeMount` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].mountPath` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].subPath` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap` | `ConfigMapVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.key` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].secret` | `SecretVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.key` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.defaultMode` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath` | `HostPathVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath.path` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath.type` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir` | `EmptyDirVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir.medium` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir.sizeLimit` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc` | `PvcVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc.claimName` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc.readOnly` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken` | `ServiceAccountTokenVolumeSource` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.audience` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.expirationSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle` | `WorkloadContainerLifecycle` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.postStart.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop` | `WorkloadLifecycleHandler` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.exec` | `ExecAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.exec.command` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet` | `HTTPGetAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.path` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.scheme` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders` | `[]HTTPHeader` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders[].name` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket` | `TCPSocketAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.portNumber` | `int32` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.portName` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.host` | `string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.sleep` | `SleepAction` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].lifecycle.preStop.sleep.seconds` | `int64` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext` | `WorkloadContainerSecurityContext` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.privileged` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.runAsUser` | `int64` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.runAsGroup` | `int64` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.runAsNonRoot` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.readOnlyRootFilesystem` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.allowPrivilegeEscalation` | `bool` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.capabilities` | `WorkloadCapabilities` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.capabilities.add` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.capabilities.drop` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile` | `WorkloadSeccompProfile` |  |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile.type` | `string` | yes |  |  |
| `spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile.localhostProfile` | `string` |  |  |  |
| `spec.jobTemplate.pod.labels` | `map<string, string>` |  |  |  |
| `spec.jobTemplate.pod.annotations` | `map<string, string>` |  |  |  |
| `spec.jobTemplate.pod.scheduling` | `WorkloadScheduling` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations[].key` | `string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations[].operator` | `string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations[].effect` | `string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity` | `WorkloadNodeAffinity` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.required` | `[]WorkloadNodeSelectorTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions` | `[]WorkloadNodeSelectorRequirement` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].key` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].operator` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].values` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred` | `[]WorkloadPreferredNodeSelectorTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].weight` | `int32` |  |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term` | `WorkloadNodeSelectorTerm` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions` | `[]WorkloadNodeSelectorRequirement` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].key` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].operator` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].values` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity` | `WorkloadPodAffinity` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.required` | `[]WorkloadPodAffinityTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.required[].matchLabels` | `map<string, string>` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.required[].topologyKey` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.required[].namespaces` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred` | `[]WorkloadWeightedPodAffinityTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred[].weight` | `int32` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term` | `WorkloadPodAffinityTerm` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.matchLabels` | `map<string, string>` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.topologyKey` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.namespaces` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity` | `WorkloadPodAffinity` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.required` | `[]WorkloadPodAffinityTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].matchLabels` | `map<string, string>` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].topologyKey` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].namespaces` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred` | `[]WorkloadWeightedPodAffinityTerm` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].weight` | `int32` |  |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term` | `WorkloadPodAffinityTerm` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.matchLabels` | `map<string, string>` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.topologyKey` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.namespaces` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.scheduling.topologySpreadConstraints` | `[]WorkloadTopologySpreadConstraint` |  |  |  |
| `spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].maxSkew` | `int32` |  |  |  |
| `spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].topologyKey` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].whenUnsatisfiable` | `string` | yes |  |  |
| `spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].matchLabels` | `map<string, string>` |  |  |  |
| `spec.jobTemplate.pod.scheduling.schedulerName` | `string` |  |  |  |
| `spec.jobTemplate.pod.securityContext` | `WorkloadPodSecurityContext` |  |  |  |
| `spec.jobTemplate.pod.securityContext.runAsUser` | `int64` |  |  |  |
| `spec.jobTemplate.pod.securityContext.runAsGroup` | `int64` |  |  |  |
| `spec.jobTemplate.pod.securityContext.runAsNonRoot` | `bool` |  |  |  |
| `spec.jobTemplate.pod.securityContext.fsGroup` | `int64` |  |  |  |
| `spec.jobTemplate.pod.securityContext.fsGroupChangePolicy` | `string` |  |  |  |
| `spec.jobTemplate.pod.securityContext.supplementalGroups` | `[]int64` |  |  |  |
| `spec.jobTemplate.pod.securityContext.sysctls` | `[]WorkloadSysctl` |  |  |  |
| `spec.jobTemplate.pod.securityContext.sysctls[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.securityContext.sysctls[].value` | `string` | yes |  |  |
| `spec.jobTemplate.pod.securityContext.seccompProfile` | `WorkloadSeccompProfile` |  |  |  |
| `spec.jobTemplate.pod.securityContext.seccompProfile.type` | `string` | yes |  |  |
| `spec.jobTemplate.pod.securityContext.seccompProfile.localhostProfile` | `string` |  |  |  |
| `spec.jobTemplate.pod.terminationGracePeriodSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.pod.dnsPolicy` | `string` |  |  |  |
| `spec.jobTemplate.pod.dnsConfig` | `WorkloadPodDnsConfig` |  |  |  |
| `spec.jobTemplate.pod.dnsConfig.nameservers` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.dnsConfig.searches` | `[]string` |  |  |  |
| `spec.jobTemplate.pod.dnsConfig.options` | `[]WorkloadPodDnsConfigOption` |  |  |  |
| `spec.jobTemplate.pod.dnsConfig.options[].name` | `string` | yes |  |  |
| `spec.jobTemplate.pod.dnsConfig.options[].value` | `string` |  |  |  |
| `spec.jobTemplate.pod.hostAliases` | `[]WorkloadHostAlias` |  |  |  |
| `spec.jobTemplate.pod.hostAliases[].ip` | `string` | yes |  |  |
| `spec.jobTemplate.pod.hostAliases[].hostnames` | `[]string` | yes |  |  |
| `spec.jobTemplate.pod.hostNetwork` | `bool` |  |  |  |
| `spec.jobTemplate.pod.hostPid` | `bool` |  |  |  |
| `spec.jobTemplate.pod.priorityClassName` | `string` |  |  |  |
| `spec.jobTemplate.pod.runtimeClassName` | `string` |  |  |  |
| `spec.jobTemplate.parallelism` | `int32` |  | `1` |  |
| `spec.jobTemplate.completions` | `int32` |  | `1` |  |
| `spec.jobTemplate.completionMode` | `string` |  | `NonIndexed` |  |
| `spec.jobTemplate.backoffLimit` | `int32` |  | `6` |  |
| `spec.jobTemplate.backoffLimitPerIndex` | `uint32` |  |  |  |
| `spec.jobTemplate.maxFailedIndexes` | `uint32` |  |  |  |
| `spec.jobTemplate.activeDeadlineSeconds` | `int64` |  |  |  |
| `spec.jobTemplate.ttlSecondsAfterFinished` | `int32` |  |  |  |
| `spec.jobTemplate.restartPolicy` | `string` |  | `Never` |  |
| `spec.jobTemplate.podFailurePolicy` | `KubernetesCronJobPodFailurePolicy` |  |  |  |
| `spec.jobTemplate.podFailurePolicy.rules` | `[]KubernetesCronJobPodFailurePolicyRule` | yes |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].action` | `string` | yes |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onExitCodes` | `KubernetesCronJobPodFailurePolicyOnExitCodes` |  |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.containerName` | `string` |  |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.operator` | `string` | yes |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.values` | `[]int32` | yes |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onPodConditions` | `[]KubernetesCronJobPodFailurePolicyOnPodCondition` |  |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onPodConditions[].type` | `string` | yes |  |  |
| `spec.jobTemplate.podFailurePolicy.rules[].onPodConditions[].status` | `string` |  | `True` |  |
| `spec.jobTemplate.successPolicy` | `KubernetesCronJobSuccessPolicy` |  |  |  |
| `spec.jobTemplate.successPolicy.rules` | `[]KubernetesCronJobSuccessPolicyRule` | yes |  |  |
| `spec.jobTemplate.successPolicy.rules[].succeededIndexes` | `string` |  |  |  |
| `spec.jobTemplate.successPolicy.rules[].succeededCount` | `int32` |  |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

The namespace to run the CronJob in. Accepts a literal namespace name or a
reference to a KubernetesNamespace resource, so an infra chart creates the
namespace and the CronJob in one run.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the module creates the namespace if it does not exist. Leave false
when the namespace is owned by a KubernetesNamespace resource or pre-exists.

### spec.schedule

`string` · required

When to run, as a standard 5-field cron expression: minute (0-59),
hour (0-23), day of month (1-31), month (1-12), day of week (0-6, Sunday=0).
E.g. "0 3 * * *" is daily at 03:00, "0,15,30,45 * * * *" is every 15
minutes, "0 9 * * 1-5" is weekdays at 09:00. Use standard cron only —
@-style macros like "@daily" are not portable across controller versions.

- rule: Schedule must be a standard 5-field cron expression: minute hour day-of-month month day-of-week (e.g. "0 3 * * *" for daily at 03:00, "*/15 * * * *" for every 15 minutes)
- rule: {"required":true}

### spec.timeZone

`string` · optional (explicit presence)

IANA time zone name the schedule is evaluated in, e.g. "America/New_York"
or "Asia/Kolkata". Unset means the schedule runs in the local time zone of
the cluster's kube-controller-manager — usually UTC, but not guaranteed, so
set this whenever the wall-clock time matters.

### spec.startingDeadlineSeconds

`int64` · optional (explicit presence)

How many seconds late a run may start when its scheduled time was missed
(controller downtime, unschedulable cluster) before it counts as failed and
is skipped. Unset means no deadline — late runs start whenever possible.
Regardless of this setting, the controller gives up and stops scheduling
after 100 consecutive missed runs, so very frequent schedules benefit from
an explicit deadline to keep that counter bounded.

- rule: {"int64":{"gte":"1"}}

### spec.concurrencyPolicy

`string` · optional (explicit presence)

What happens when the next run comes due while the previous run is still
going. "Forbid" (default here): skip the new run — the previous one keeps
going. "Allow": run them concurrently. "Replace": cancel the running job
and start the new one. This default is deliberately safer than upstream
(Kubernetes defaults to Allow): overlapping cron runs are the classic
scheduled-workload incident — two backups writing the same target, two
migrations racing — so overlap is opt-in here.

- default: `Forbid`
- rule: Concurrency policy must be one of "Allow", "Forbid", or "Replace"

### spec.suspend

`bool`

When true, the controller stops scheduling future runs. Jobs already
running are unaffected — this pauses the schedule, it does not stop work
in flight.

### spec.successfulJobsHistoryLimit

`int32` · optional (explicit presence)

How many completed successful Jobs are retained (for logs and status
inspection). Kubernetes defaults to 3; 0 deletes each successful Job as
soon as it finishes.

- default: `3`
- rule: {"int32":{"gte":0}}

### spec.failedJobsHistoryLimit

`int32` · optional (explicit presence)

How many failed Jobs are retained. Kubernetes defaults to 1; keep at least
one so failure logs survive for post-mortems.

- default: `1`
- rule: {"int32":{"gte":0}}

### spec.jobTemplate

`KubernetesCronJobJobTemplate` · required

The Job created at each scheduled run: its containers, pod configuration,
and batch controls (parallelism, retries, deadlines, failure and success
policies).

- rule: {"required":true}

### spec.jobTemplate.container

`KubernetesCronJobContainer` · required

The containers of every run's pods: one main application container that
performs the work and any sidecars. All containers share the pod's network
namespace and volumes.

- rule: {"required":true}

### spec.jobTemplate.container.app

`WorkloadContainer` · required

The main application container — the process that performs the scheduled
work and whose exit code decides the pod's success or failure. Its image is
the pipeline injection point.

- rule: {"required":true}

### spec.jobTemplate.container.app.name

`string`

The container's name, unique within the pod. Required for sidecars and init
containers (Kubernetes rejects unnamed containers); for the main app container the
module defaults it when omitted, so minimal manifests stay minimal. Must be a valid
DNS label: lowercase alphanumeric and hyphens, starting and ending alphanumeric.

- rule: Container name must be a lowercase DNS label (alphanumeric and hyphens, starting and ending with an alphanumeric character)

### spec.jobTemplate.container.app.image

`WorkloadContainerImage` · required

The container image, split into repository and tag so deployment pipelines can
inject a freshly built tag without rewriting the whole reference. A container
carries no pull credential of its own: the kubelet pulls every image in a pod
with the pod's pull secrets, so a private registry's login is declared once,
pod-wide — on `pod.image_registries` (the login itself) or `pod.image_pull_secrets`
(a Secret declared beside the workload), or on the ServiceAccount the pod runs as.

- rule: Image repo is required — the repository half of the image reference (e.g. "nginx" or "ghcr.io/acme/api")
- rule: Image tag is required — pin a version (e.g. "1.27.1"); avoid "latest" for anything you intend to roll back
- rule: {"required":true}

### spec.jobTemplate.container.app.image.repo

`string`

The repository of the image (e.g. "nginx" or "ghcr.io/acme/checkout").

### spec.jobTemplate.container.app.image.tag

`string`

The tag of the image (e.g. "1.27.1"). Pin a version; "latest" cannot be rolled back.

### spec.jobTemplate.container.app.imagePullPolicy

`string`

When the kubelet pulls the image. "IfNotPresent" (the Kubernetes default for tagged
images) reuses a cached copy; "Always" re-resolves the tag on every pod start —
required when a mutable tag like a branch name is reused across builds; "Never"
only uses pre-loaded images (air-gapped nodes, kind-loaded test images).

- rule: Image pull policy must be one of "Always", "IfNotPresent", or "Never"

### spec.jobTemplate.container.app.command

`[]string`

Entrypoint override (Kubernetes `command`, Docker ENTRYPOINT). The image's
entrypoint runs when omitted. Not executed in a shell — provide argv elements,
e.g. ["/bin/sh", "-c", "exec my-server"].

### spec.jobTemplate.container.app.args

`[]string`

Arguments to the entrypoint (Kubernetes `args`, Docker CMD). The image's CMD is
used when omitted. Variable references like $(VAR_NAME) are expanded from the
container's environment by the kubelet.

### spec.jobTemplate.container.app.workingDir

`string`

Working directory for the entrypoint. Defaults to the image's configured WORKDIR.

### spec.jobTemplate.container.app.ports

`[]WorkloadContainerPort`

Network ports this container exposes. Purely informational to Kubernetes for plain
pod-to-pod traffic, but load-bearing here: named ports are referenced by probes,
and `service_port` drives the Service wiring on kinds that create one
(Deployment, StatefulSet).

### spec.jobTemplate.container.app.ports[].name

`string` · required

Port name, e.g. "http", "grpc", "metrics". Must be a lowercase DNS label that
starts and ends alphanumeric. Named ports are referenced by probes and become the
Service port names on service-fronted kinds.

- rule: Port name must contain only lowercase alphanumeric characters and hyphens, and start and end with an alphanumeric character (e.g. "http", "grpc-web")
- rule: {"required":true}

### spec.jobTemplate.container.app.ports[].containerPort

`int32` · required

The port number the container listens on (1–65535).

- rule: {"required":true,"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.app.ports[].networkProtocol

`string`

L4 protocol of the port. Defaults to "TCP" when omitted — the overwhelmingly
common case, so minimal manifests need not repeat it.

- rule: The network protocol must be one of "TCP", "UDP", or "SCTP"

### spec.jobTemplate.container.app.ports[].appProtocol

`string`

Application protocol hint (e.g. "http", "grpc", "https"). Propagated to the
Service port's appProtocol on service-fronted kinds, where meshes and L7 load
balancers use it to pick the right protocol handling.

### spec.jobTemplate.container.app.ports[].servicePort

`int32`

The port the workload's Kubernetes Service exposes for this container port.
Only meaningful on kinds that create a Service (Deployment, StatefulSet); other
kinds ignore it. E.g. containerPort 8080 with servicePort 80 serves the app on
the conventional port while the process binds an unprivileged one. External
exposure is composed separately with first-class ingress kinds referencing the
workload's exported Service handle — workloads never create ingress themselves.

- rule: Service port must be between 1 and 65535

### spec.jobTemplate.container.app.ports[].hostPort

`int32`

Exposes the container port directly on the node's IP (hostPort). Chiefly a
DaemonSet pattern (node-level agents that must be reachable on every node);
on other kinds it constrains scheduling to one pod per node per port — prefer
a Service unless node-local reachability is the point.

- rule: Host port must be between 1 and 65535

### spec.jobTemplate.container.app.env

`ContainerEnv`

Environment configuration: plain variables (with Kubernetes-native value sources
and Planton cross-resource references), secret variables (materialized into a
managed Kubernetes Secret), and bulk envFrom imports.

### spec.jobTemplate.container.app.env.variables

`[]EnvVar`

Individual environment variables (non-sensitive).

### spec.jobTemplate.container.app.env.variables[].name

`string` · required

The environment variable name.
Must be a valid C_IDENTIFIER: starts with a letter or underscore,
followed by letters, digits, or underscores.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].value

`string`

Direct literal value.

### spec.jobTemplate.container.app.env.variables[].valueFrom

`ValueFromRef`

Reference to another Planton resource's field.
The orchestrator resolves this and populates the value before invoking IaC modules.

### spec.jobTemplate.container.app.env.variables[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.container.app.env.variables[].valueFrom.env

`string`

### spec.jobTemplate.container.app.env.variables[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].valueFrom.fieldPath

`string`

### spec.jobTemplate.container.app.env.variables[].configMapKeyRef

`ConfigMapKeyRef`

Reference to a key in a Kubernetes ConfigMap.

### spec.jobTemplate.container.app.env.variables[].configMapKeyRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].configMapKeyRef.key

`string` · required

Key within the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].configMapKeyRef.optional

`bool`

If true, the env var is silently skipped when the ConfigMap or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.container.app.env.variables[].fieldRef

`ObjectFieldRef`

Reference to a pod-level field (metadata.name, status.podIP, etc.).

### spec.jobTemplate.container.app.env.variables[].fieldRef.apiVersion

`string`

Version of the schema. Defaults to "v1".

### spec.jobTemplate.container.app.env.variables[].fieldRef.fieldPath

`string` · required

Path of the field to select (e.g., "metadata.name", "status.podIP").

- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].resourceFieldRef

`ResourceFieldRef`

Reference to container resource limits or requests (limits.cpu, requests.memory, etc.).

### spec.jobTemplate.container.app.env.variables[].resourceFieldRef.containerName

`string`

Container name. Required for init containers; defaults to the current
container for regular containers.

### spec.jobTemplate.container.app.env.variables[].resourceFieldRef.resource

`string` · required

Resource to select (e.g., "limits.cpu", "requests.memory").

- rule: {"required":true}

### spec.jobTemplate.container.app.env.variables[].resourceFieldRef.divisor

`string`

Specifies the output format of the exposed resource.
For CPU: "1" means cores. For memory: "1", "1Ki", "1Mi", "1Gi".

### spec.jobTemplate.container.app.env.secrets

`[]SecretEnvVar`

Individual secret environment variables (sensitive).

### spec.jobTemplate.container.app.env.secrets[].name

`string` · required

The environment variable name.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.container.app.env.secrets[].value

`string`

Literal string value.
A Kubernetes Secret is automatically created and the environment variable
references that secret.

### spec.jobTemplate.container.app.env.secrets[].secretRef

`KubernetesSecretKeyRef`

Reference to a key within an existing Kubernetes Secret.

### spec.jobTemplate.container.app.env.secrets[].secretRef.namespace

`string`

The namespace of the Kubernetes Secret.
If not specified, defaults to the namespace where the component is deployed.
Note: Cross-namespace secret references may not be supported by all Helm charts.

### spec.jobTemplate.container.app.env.secrets[].secretRef.name

`string` · required

The name of the Kubernetes Secret.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.secrets[].secretRef.key

`string` · required

The key within the Kubernetes Secret that contains the value.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.secrets[].secretRef.optional

`bool`

If true, the env var is silently skipped when the Secret or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.container.app.env.secrets[].valueFrom

`ValueFromRef`

Reference to another Planton resource's secret output field.
The orchestrator resolves this before invoking IaC modules.

### spec.jobTemplate.container.app.env.secrets[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.container.app.env.secrets[].valueFrom.env

`string`

### spec.jobTemplate.container.app.env.secrets[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.container.app.env.secrets[].valueFrom.fieldPath

`string`

### spec.jobTemplate.container.app.env.envFrom

`[]EnvFromSource`

Bulk import of environment variables from ConfigMaps or Secrets.

### spec.jobTemplate.container.app.env.envFrom[].prefix

`string`

Optional prefix prepended to each imported key name.
For example, prefix "APP_" with key "PORT" produces env var "APP_PORT".

### spec.jobTemplate.container.app.env.envFrom[].configMapRef

`ConfigMapRef`

Import all keys from a ConfigMap.

### spec.jobTemplate.container.app.env.envFrom[].configMapRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.envFrom[].configMapRef.optional

`bool`

If true, the ConfigMap is allowed to not exist without blocking pod startup.

### spec.jobTemplate.container.app.env.envFrom[].secretRef

`SecretRef`

Import all keys from a Secret.

### spec.jobTemplate.container.app.env.envFrom[].secretRef.name

`string` · required

Name of the Secret.

- rule: {"required":true}

### spec.jobTemplate.container.app.env.envFrom[].secretRef.optional

`bool`

If true, the Secret is allowed to not exist without blocking pod startup.

### spec.jobTemplate.container.app.resources

`ContainerResources`

CPU and memory requests and limits. Requests drive scheduling and are what the
pod is guaranteed; limits are the ceiling enforced at runtime (CPU is throttled,
memory overage is OOM-killed). Omitting limits entirely leaves the container
unbounded — acceptable for batch work on dedicated nodes, risky on shared ones.

### spec.jobTemplate.container.app.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.jobTemplate.container.app.resources.limits.cpu

`string`

### spec.jobTemplate.container.app.resources.limits.memory

`string`

### spec.jobTemplate.container.app.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.jobTemplate.container.app.resources.requests.cpu

`string`

### spec.jobTemplate.container.app.resources.requests.memory

`string`

### spec.jobTemplate.container.app.livenessProbe

`Probe`

Liveness probe: restarts the container when it fails. Detects deadlocked or
wedged processes. Keep it strictly about "is the process alive" — checking
downstream dependencies here turns a dependency blip into a restart storm.

### spec.jobTemplate.container.app.livenessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.app.livenessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.app.livenessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.app.livenessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.app.livenessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.app.livenessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.app.livenessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.app.livenessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.app.livenessProbe.httpGet.portName

`string`

### spec.jobTemplate.container.app.livenessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.app.livenessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.app.livenessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.app.livenessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.app.livenessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.app.livenessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.app.livenessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.app.livenessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.app.livenessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.app.livenessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.app.livenessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.app.livenessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.app.readinessProbe

`Probe`

Readiness probe: removes the pod from Service endpoints while it fails. This is
the probe that makes rolling updates zero-downtime — traffic only reaches pods
that report ready.

### spec.jobTemplate.container.app.readinessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.app.readinessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.app.readinessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.app.readinessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.app.readinessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.app.readinessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.app.readinessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.app.readinessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.app.readinessProbe.httpGet.portName

`string`

### spec.jobTemplate.container.app.readinessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.app.readinessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.app.readinessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.app.readinessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.app.readinessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.app.readinessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.app.readinessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.app.readinessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.app.readinessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.app.readinessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.app.readinessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.app.readinessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.app.startupProbe

`Probe`

Startup probe: holds off liveness and readiness checking until the app has
started, so slow-booting applications are not killed mid-initialization. Size
`failure_threshold × period_seconds` to the worst-case startup time.

### spec.jobTemplate.container.app.startupProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.app.startupProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.app.startupProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.app.startupProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.app.startupProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.app.startupProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.app.startupProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.app.startupProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.app.startupProbe.httpGet.portName

`string`

### spec.jobTemplate.container.app.startupProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.app.startupProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.app.startupProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.app.startupProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.app.startupProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.app.startupProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.app.startupProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.app.startupProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.app.startupProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.app.startupProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.app.startupProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.app.startupProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.app.volumeMounts

`[]VolumeMount`

Volume mounts for this container. Each entry both declares the mount path and
carries its volume source (ConfigMap, Secret, HostPath, EmptyDir, or PVC); the
module derives the pod-level volume list from the union of all containers'
mounts, de-duplicating by name — so two containers sharing an EmptyDir simply
declare the same mount name and source.

### spec.jobTemplate.container.app.volumeMounts[].name

`string` · required

Name of the volume mount. Must be unique within the container.
Used to correlate with the volume definition.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].mountPath

`string` · required

Path within the container at which the volume should be mounted.
Must be an absolute path.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].readOnly

`bool`

Whether the volume should be mounted read-only.
Default is false.

### spec.jobTemplate.container.app.volumeMounts[].subPath

`string`

Path within the volume from which the container's volume should be mounted.
Defaults to "" (volume's root).
Useful for mounting a subdirectory of a volume.

### spec.jobTemplate.container.app.volumeMounts[].configMap

`ConfigMapVolumeSource`

ConfigMap volume source.
Use this to mount a ConfigMap as a file or directory.

### spec.jobTemplate.container.app.volumeMounts[].configMap.name

`string` · required

Name of the ConfigMap to mount.
Can reference a ConfigMap defined in spec.config_maps or an existing one in the namespace.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].configMap.key

`string`

Specific key from the ConfigMap to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.container.app.volumeMounts[].configMap.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.
Example: key="config" path="app.yaml" mounts the "config" key as "app.yaml"

### spec.jobTemplate.container.app.volumeMounts[].configMap.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.
Use 0755 (493 in decimal) for executable scripts.

### spec.jobTemplate.container.app.volumeMounts[].secret

`SecretVolumeSource`

Secret volume source.
Use this to mount a Secret as a file or directory.

### spec.jobTemplate.container.app.volumeMounts[].secret.name

`string` · required

Name of the Secret to mount.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].secret.key

`string`

Specific key from the Secret to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.container.app.volumeMounts[].secret.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.

### spec.jobTemplate.container.app.volumeMounts[].secret.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.

### spec.jobTemplate.container.app.volumeMounts[].hostPath

`HostPathVolumeSource`

HostPath volume source.
Use this to mount a file or directory from the host node's filesystem.
Common for DaemonSets that need access to node-level resources.

### spec.jobTemplate.container.app.volumeMounts[].hostPath.path

`string` · required

Path on the host to mount.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].hostPath.type

`string`

Type of the host path.
Valid values:
  "" - Empty string (default) means no check is performed before mounting
  "DirectoryOrCreate" - Create directory if it doesn't exist
  "Directory" - Directory must exist
  "FileOrCreate" - Create file if it doesn't exist
  "File" - File must exist
  "Socket" - UNIX socket must exist
  "CharDevice" - Character device must exist
  "BlockDevice" - Block device must exist

- rule: Type must be one of: "", "DirectoryOrCreate", "Directory", "FileOrCreate", "File", "Socket", "CharDevice", "BlockDevice"

### spec.jobTemplate.container.app.volumeMounts[].emptyDir

`EmptyDirVolumeSource`

EmptyDir volume source.
Use this for temporary storage that is erased when the pod is removed.
Useful for scratch space, caching, or sharing data between containers.

### spec.jobTemplate.container.app.volumeMounts[].emptyDir.medium

`string`

Medium for the empty directory.
"" (default) uses the node's default medium (typically disk).
"Memory" uses a tmpfs (RAM-backed filesystem).

Memory-backed volumes are faster but:
- Count against container memory limits
- Are lost on node restart
- Should have sizeLimit set to prevent OOM

- rule: Medium must be either "" or "Memory"

### spec.jobTemplate.container.app.volumeMounts[].emptyDir.sizeLimit

`string`

Size limit for the empty directory.
Format: Kubernetes quantity (e.g., "1Gi", "500Mi").
Only strictly enforced when medium is "Memory".
For disk-backed volumes, this is a best-effort limit.

### spec.jobTemplate.container.app.volumeMounts[].pvc

`PvcVolumeSource`

PersistentVolumeClaim volume source.
Use this to mount an existing PVC.
For StatefulSets, this can reference a volumeClaimTemplate.

### spec.jobTemplate.container.app.volumeMounts[].pvc.claimName

`string` · required

Name of the PersistentVolumeClaim to mount.
For StatefulSets, this can be the name of a volumeClaimTemplate.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].pvc.readOnly

`bool`

Whether the PVC should be mounted read-only.
Default is false.

### spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken

`ServiceAccountTokenVolumeSource`

Projected ServiceAccount token volume source.
Use this to mount a short-lived, audience-bound identity token that the
kubelet issues for the pod's ServiceAccount and rotates automatically.

### spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.audience

`string` · required

Intended audience of the token. The receiving service must identify
itself with this audience when verifying the token; a token minted for a
different audience is rejected. Required: an audience-less token would be
replayable against any service in the cluster.

- rule: {"required":true}

### spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.expirationSeconds

`int64`

Requested lifetime of the token in seconds. The kubelet starts rotating
the token when it passes 80% of its lifetime or 24 hours, whichever is
shorter. Defaults to 3600 (1 hour). The Kubernetes API enforces a
minimum of 600 (10 minutes).

- rule: Expiration must be at least 600 seconds (the Kubernetes API minimum)

### spec.jobTemplate.container.app.volumeMounts[].serviceAccountToken.path

`string`

Filename for the token relative to the mount path.
Defaults to "token".

### spec.jobTemplate.container.app.lifecycle

`WorkloadContainerLifecycle`

Lifecycle hooks. `post_start` runs immediately after the container starts (the
container is not Running until it completes); `pre_stop` runs before termination
and is the standard lever for connection draining — e.g. a short sleep that keeps
the endpoint serving while load balancers converge on the terminating state.

### spec.jobTemplate.container.app.lifecycle.postStart

`WorkloadLifecycleHandler`

Runs immediately after the container is created. The container does not reach
Running until the hook completes; a failing post_start kills the container per
its restart policy.

### spec.jobTemplate.container.app.lifecycle.postStart.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.container.app.lifecycle.postStart.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.portNumber

`int32`

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.portName

`string`

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.app.lifecycle.postStart.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.portName

`string`

### spec.jobTemplate.container.app.lifecycle.postStart.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.app.lifecycle.postStart.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.container.app.lifecycle.postStart.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.container.app.lifecycle.preStop

`WorkloadLifecycleHandler`

Runs before the container is terminated by the kubelet (pod deletion, rolling
update, eviction). The termination grace period starts BEFORE the hook runs, so
keep `pod.termination_grace_period_seconds` larger than the hook's worst-case
duration. The classic zero-downtime pattern is a short sleep here so the pod
keeps serving while endpoint removal propagates.

### spec.jobTemplate.container.app.lifecycle.preStop.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.container.app.lifecycle.preStop.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.portNumber

`int32`

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.portName

`string`

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.app.lifecycle.preStop.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.portName

`string`

### spec.jobTemplate.container.app.lifecycle.preStop.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.app.lifecycle.preStop.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.container.app.lifecycle.preStop.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.container.app.securityContext

`WorkloadContainerSecurityContext`

Container-level security hardening. Settings here override the pod-level
security context for this container only.

### spec.jobTemplate.container.app.securityContext.privileged

`bool`

Runs the container with full host access — equivalent to root on the node.
Required by some node-level agents (device managers, network plugins). Never
combine with untrusted images.

### spec.jobTemplate.container.app.securityContext.runAsUser

`int64` · optional (explicit presence)

UID the container process runs as. Overrides the image's USER directive.

### spec.jobTemplate.container.app.securityContext.runAsGroup

`int64` · optional (explicit presence)

Primary GID the container process runs as.

### spec.jobTemplate.container.app.securityContext.runAsNonRoot

`bool` · optional (explicit presence)

Refuses to start the container if its effective user is root. The standard
baseline hardening — it catches images that silently default to UID 0.

### spec.jobTemplate.container.app.securityContext.readOnlyRootFilesystem

`bool` · optional (explicit presence)

Mounts the container's root filesystem read-only. Pair with EmptyDir mounts for
paths the app must write (e.g. /tmp).

### spec.jobTemplate.container.app.securityContext.allowPrivilegeEscalation

`bool` · optional (explicit presence)

Whether the process can gain more privileges than its parent (setuid binaries,
file capabilities). The restricted Pod Security Standard requires this to be
false. Always true when `privileged` is set, so leave it unset in that case.

### spec.jobTemplate.container.app.securityContext.capabilities

`WorkloadCapabilities`

Linux capabilities to add or drop. The restricted profile drops ALL and adds
back only NET_BIND_SERVICE when needed. Capability names are uppercase without
the CAP_ prefix (e.g. "NET_ADMIN", "SYS_TIME").

### spec.jobTemplate.container.app.securityContext.capabilities.add

`[]string`

Capabilities to add (e.g. "NET_BIND_SERVICE").

### spec.jobTemplate.container.app.securityContext.capabilities.drop

`[]string`

Capabilities to drop. Use ["ALL"] as the hardened baseline.

### spec.jobTemplate.container.app.securityContext.seccompProfile

`WorkloadSeccompProfile`

Seccomp syscall filter for the container. "RuntimeDefault" is the hardened
baseline; "Localhost" selects a node-local profile file via `localhost_profile`.

- rule: localhost_profile is required when type is "Localhost" and must be empty otherwise

### spec.jobTemplate.container.app.securityContext.seccompProfile.type

`string` · required

Profile type: "RuntimeDefault" (the container runtime's default filter — the
recommended baseline), "Unconfined" (no filtering), or "Localhost" (a profile
file installed on the node, named via localhost_profile).

- rule: Seccomp profile type must be one of "RuntimeDefault", "Unconfined", or "Localhost"
- rule: {"required":true}

### spec.jobTemplate.container.app.securityContext.seccompProfile.localhostProfile

`string`

Path of the profile file relative to the node's seccomp profile root. Required
when (and only meaningful when) type is "Localhost".

### spec.jobTemplate.container.sidecars

`[]WorkloadContainer`

Sidecar containers running alongside the app in every run's pods — log
shippers, proxies. Sidecars are full containers: probes, mounts, security
context, and lifecycle hooks all apply. Each sidecar must be named.
WARNING: a Job pod only completes when ALL its containers exit — a sidecar
that never exits keeps every run from completing (and with
concurrency_policy "Forbid", blocks all future runs). Use
termination-aware sidecars, or have the app signal them to shut down when
its work is done.

- rule: Every sidecar container must have a name

### spec.jobTemplate.container.sidecars[].name

`string`

The container's name, unique within the pod. Required for sidecars and init
containers (Kubernetes rejects unnamed containers); for the main app container the
module defaults it when omitted, so minimal manifests stay minimal. Must be a valid
DNS label: lowercase alphanumeric and hyphens, starting and ending alphanumeric.

- rule: Container name must be a lowercase DNS label (alphanumeric and hyphens, starting and ending with an alphanumeric character)

### spec.jobTemplate.container.sidecars[].image

`WorkloadContainerImage` · required

The container image, split into repository and tag so deployment pipelines can
inject a freshly built tag without rewriting the whole reference. A container
carries no pull credential of its own: the kubelet pulls every image in a pod
with the pod's pull secrets, so a private registry's login is declared once,
pod-wide — on `pod.image_registries` (the login itself) or `pod.image_pull_secrets`
(a Secret declared beside the workload), or on the ServiceAccount the pod runs as.

- rule: Image repo is required — the repository half of the image reference (e.g. "nginx" or "ghcr.io/acme/api")
- rule: Image tag is required — pin a version (e.g. "1.27.1"); avoid "latest" for anything you intend to roll back
- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].image.repo

`string`

The repository of the image (e.g. "nginx" or "ghcr.io/acme/checkout").

### spec.jobTemplate.container.sidecars[].image.tag

`string`

The tag of the image (e.g. "1.27.1"). Pin a version; "latest" cannot be rolled back.

### spec.jobTemplate.container.sidecars[].imagePullPolicy

`string`

When the kubelet pulls the image. "IfNotPresent" (the Kubernetes default for tagged
images) reuses a cached copy; "Always" re-resolves the tag on every pod start —
required when a mutable tag like a branch name is reused across builds; "Never"
only uses pre-loaded images (air-gapped nodes, kind-loaded test images).

- rule: Image pull policy must be one of "Always", "IfNotPresent", or "Never"

### spec.jobTemplate.container.sidecars[].command

`[]string`

Entrypoint override (Kubernetes `command`, Docker ENTRYPOINT). The image's
entrypoint runs when omitted. Not executed in a shell — provide argv elements,
e.g. ["/bin/sh", "-c", "exec my-server"].

### spec.jobTemplate.container.sidecars[].args

`[]string`

Arguments to the entrypoint (Kubernetes `args`, Docker CMD). The image's CMD is
used when omitted. Variable references like $(VAR_NAME) are expanded from the
container's environment by the kubelet.

### spec.jobTemplate.container.sidecars[].workingDir

`string`

Working directory for the entrypoint. Defaults to the image's configured WORKDIR.

### spec.jobTemplate.container.sidecars[].ports

`[]WorkloadContainerPort`

Network ports this container exposes. Purely informational to Kubernetes for plain
pod-to-pod traffic, but load-bearing here: named ports are referenced by probes,
and `service_port` drives the Service wiring on kinds that create one
(Deployment, StatefulSet).

### spec.jobTemplate.container.sidecars[].ports[].name

`string` · required

Port name, e.g. "http", "grpc", "metrics". Must be a lowercase DNS label that
starts and ends alphanumeric. Named ports are referenced by probes and become the
Service port names on service-fronted kinds.

- rule: Port name must contain only lowercase alphanumeric characters and hyphens, and start and end with an alphanumeric character (e.g. "http", "grpc-web")
- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].ports[].containerPort

`int32` · required

The port number the container listens on (1–65535).

- rule: {"required":true,"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.sidecars[].ports[].networkProtocol

`string`

L4 protocol of the port. Defaults to "TCP" when omitted — the overwhelmingly
common case, so minimal manifests need not repeat it.

- rule: The network protocol must be one of "TCP", "UDP", or "SCTP"

### spec.jobTemplate.container.sidecars[].ports[].appProtocol

`string`

Application protocol hint (e.g. "http", "grpc", "https"). Propagated to the
Service port's appProtocol on service-fronted kinds, where meshes and L7 load
balancers use it to pick the right protocol handling.

### spec.jobTemplate.container.sidecars[].ports[].servicePort

`int32`

The port the workload's Kubernetes Service exposes for this container port.
Only meaningful on kinds that create a Service (Deployment, StatefulSet); other
kinds ignore it. E.g. containerPort 8080 with servicePort 80 serves the app on
the conventional port while the process binds an unprivileged one. External
exposure is composed separately with first-class ingress kinds referencing the
workload's exported Service handle — workloads never create ingress themselves.

- rule: Service port must be between 1 and 65535

### spec.jobTemplate.container.sidecars[].ports[].hostPort

`int32`

Exposes the container port directly on the node's IP (hostPort). Chiefly a
DaemonSet pattern (node-level agents that must be reachable on every node);
on other kinds it constrains scheduling to one pod per node per port — prefer
a Service unless node-local reachability is the point.

- rule: Host port must be between 1 and 65535

### spec.jobTemplate.container.sidecars[].env

`ContainerEnv`

Environment configuration: plain variables (with Kubernetes-native value sources
and Planton cross-resource references), secret variables (materialized into a
managed Kubernetes Secret), and bulk envFrom imports.

### spec.jobTemplate.container.sidecars[].env.variables

`[]EnvVar`

Individual environment variables (non-sensitive).

### spec.jobTemplate.container.sidecars[].env.variables[].name

`string` · required

The environment variable name.
Must be a valid C_IDENTIFIER: starts with a letter or underscore,
followed by letters, digits, or underscores.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].value

`string`

Direct literal value.

### spec.jobTemplate.container.sidecars[].env.variables[].valueFrom

`ValueFromRef`

Reference to another Planton resource's field.
The orchestrator resolves this and populates the value before invoking IaC modules.

### spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.env

`string`

### spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].valueFrom.fieldPath

`string`

### spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef

`ConfigMapKeyRef`

Reference to a key in a Kubernetes ConfigMap.

### spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.key

`string` · required

Key within the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].configMapKeyRef.optional

`bool`

If true, the env var is silently skipped when the ConfigMap or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.container.sidecars[].env.variables[].fieldRef

`ObjectFieldRef`

Reference to a pod-level field (metadata.name, status.podIP, etc.).

### spec.jobTemplate.container.sidecars[].env.variables[].fieldRef.apiVersion

`string`

Version of the schema. Defaults to "v1".

### spec.jobTemplate.container.sidecars[].env.variables[].fieldRef.fieldPath

`string` · required

Path of the field to select (e.g., "metadata.name", "status.podIP").

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef

`ResourceFieldRef`

Reference to container resource limits or requests (limits.cpu, requests.memory, etc.).

### spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.containerName

`string`

Container name. Required for init containers; defaults to the current
container for regular containers.

### spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.resource

`string` · required

Resource to select (e.g., "limits.cpu", "requests.memory").

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.variables[].resourceFieldRef.divisor

`string`

Specifies the output format of the exposed resource.
For CPU: "1" means cores. For memory: "1", "1Ki", "1Mi", "1Gi".

### spec.jobTemplate.container.sidecars[].env.secrets

`[]SecretEnvVar`

Individual secret environment variables (sensitive).

### spec.jobTemplate.container.sidecars[].env.secrets[].name

`string` · required

The environment variable name.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.secrets[].value

`string`

Literal string value.
A Kubernetes Secret is automatically created and the environment variable
references that secret.

### spec.jobTemplate.container.sidecars[].env.secrets[].secretRef

`KubernetesSecretKeyRef`

Reference to a key within an existing Kubernetes Secret.

### spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.namespace

`string`

The namespace of the Kubernetes Secret.
If not specified, defaults to the namespace where the component is deployed.
Note: Cross-namespace secret references may not be supported by all Helm charts.

### spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.name

`string` · required

The name of the Kubernetes Secret.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.key

`string` · required

The key within the Kubernetes Secret that contains the value.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.secrets[].secretRef.optional

`bool`

If true, the env var is silently skipped when the Secret or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom

`ValueFromRef`

Reference to another Planton resource's secret output field.
The orchestrator resolves this before invoking IaC modules.

### spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.env

`string`

### spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.secrets[].valueFrom.fieldPath

`string`

### spec.jobTemplate.container.sidecars[].env.envFrom

`[]EnvFromSource`

Bulk import of environment variables from ConfigMaps or Secrets.

### spec.jobTemplate.container.sidecars[].env.envFrom[].prefix

`string`

Optional prefix prepended to each imported key name.
For example, prefix "APP_" with key "PORT" produces env var "APP_PORT".

### spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef

`ConfigMapRef`

Import all keys from a ConfigMap.

### spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.envFrom[].configMapRef.optional

`bool`

If true, the ConfigMap is allowed to not exist without blocking pod startup.

### spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef

`SecretRef`

Import all keys from a Secret.

### spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef.name

`string` · required

Name of the Secret.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].env.envFrom[].secretRef.optional

`bool`

If true, the Secret is allowed to not exist without blocking pod startup.

### spec.jobTemplate.container.sidecars[].resources

`ContainerResources`

CPU and memory requests and limits. Requests drive scheduling and are what the
pod is guaranteed; limits are the ceiling enforced at runtime (CPU is throttled,
memory overage is OOM-killed). Omitting limits entirely leaves the container
unbounded — acceptable for batch work on dedicated nodes, risky on shared ones.

### spec.jobTemplate.container.sidecars[].resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.jobTemplate.container.sidecars[].resources.limits.cpu

`string`

### spec.jobTemplate.container.sidecars[].resources.limits.memory

`string`

### spec.jobTemplate.container.sidecars[].resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.jobTemplate.container.sidecars[].resources.requests.cpu

`string`

### spec.jobTemplate.container.sidecars[].resources.requests.memory

`string`

### spec.jobTemplate.container.sidecars[].livenessProbe

`Probe`

Liveness probe: restarts the container when it fails. Detects deadlocked or
wedged processes. Keep it strictly about "is the process alive" — checking
downstream dependencies here turns a dependency blip into a restart storm.

### spec.jobTemplate.container.sidecars[].livenessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.sidecars[].livenessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].livenessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].livenessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].livenessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.portName

`string`

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.sidecars[].livenessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.sidecars[].livenessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.sidecars[].livenessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.sidecars[].livenessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.sidecars[].livenessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.sidecars[].livenessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.sidecars[].livenessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.sidecars[].readinessProbe

`Probe`

Readiness probe: removes the pod from Service endpoints while it fails. This is
the probe that makes rolling updates zero-downtime — traffic only reaches pods
that report ready.

### spec.jobTemplate.container.sidecars[].readinessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.sidecars[].readinessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].readinessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].readinessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].readinessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.portName

`string`

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.sidecars[].readinessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.sidecars[].readinessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.sidecars[].readinessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.sidecars[].readinessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.sidecars[].readinessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.sidecars[].readinessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.sidecars[].readinessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.sidecars[].startupProbe

`Probe`

Startup probe: holds off liveness and readiness checking until the app has
started, so slow-booting applications are not killed mid-initialization. Size
`failure_threshold × period_seconds` to the worst-case startup time.

### spec.jobTemplate.container.sidecars[].startupProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.container.sidecars[].startupProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].startupProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].startupProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].startupProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.portName

`string`

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.sidecars[].startupProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.sidecars[].startupProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.container.sidecars[].startupProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.container.sidecars[].startupProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.portName

`string`

### spec.jobTemplate.container.sidecars[].startupProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.sidecars[].startupProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.container.sidecars[].startupProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.sidecars[].volumeMounts

`[]VolumeMount`

Volume mounts for this container. Each entry both declares the mount path and
carries its volume source (ConfigMap, Secret, HostPath, EmptyDir, or PVC); the
module derives the pod-level volume list from the union of all containers'
mounts, de-duplicating by name — so two containers sharing an EmptyDir simply
declare the same mount name and source.

### spec.jobTemplate.container.sidecars[].volumeMounts[].name

`string` · required

Name of the volume mount. Must be unique within the container.
Used to correlate with the volume definition.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].mountPath

`string` · required

Path within the container at which the volume should be mounted.
Must be an absolute path.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].readOnly

`bool`

Whether the volume should be mounted read-only.
Default is false.

### spec.jobTemplate.container.sidecars[].volumeMounts[].subPath

`string`

Path within the volume from which the container's volume should be mounted.
Defaults to "" (volume's root).
Useful for mounting a subdirectory of a volume.

### spec.jobTemplate.container.sidecars[].volumeMounts[].configMap

`ConfigMapVolumeSource`

ConfigMap volume source.
Use this to mount a ConfigMap as a file or directory.

### spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.name

`string` · required

Name of the ConfigMap to mount.
Can reference a ConfigMap defined in spec.config_maps or an existing one in the namespace.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.key

`string`

Specific key from the ConfigMap to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.
Example: key="config" path="app.yaml" mounts the "config" key as "app.yaml"

### spec.jobTemplate.container.sidecars[].volumeMounts[].configMap.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.
Use 0755 (493 in decimal) for executable scripts.

### spec.jobTemplate.container.sidecars[].volumeMounts[].secret

`SecretVolumeSource`

Secret volume source.
Use this to mount a Secret as a file or directory.

### spec.jobTemplate.container.sidecars[].volumeMounts[].secret.name

`string` · required

Name of the Secret to mount.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].secret.key

`string`

Specific key from the Secret to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.container.sidecars[].volumeMounts[].secret.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.

### spec.jobTemplate.container.sidecars[].volumeMounts[].secret.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.

### spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath

`HostPathVolumeSource`

HostPath volume source.
Use this to mount a file or directory from the host node's filesystem.
Common for DaemonSets that need access to node-level resources.

### spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath.path

`string` · required

Path on the host to mount.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].hostPath.type

`string`

Type of the host path.
Valid values:
  "" - Empty string (default) means no check is performed before mounting
  "DirectoryOrCreate" - Create directory if it doesn't exist
  "Directory" - Directory must exist
  "FileOrCreate" - Create file if it doesn't exist
  "File" - File must exist
  "Socket" - UNIX socket must exist
  "CharDevice" - Character device must exist
  "BlockDevice" - Block device must exist

- rule: Type must be one of: "", "DirectoryOrCreate", "Directory", "FileOrCreate", "File", "Socket", "CharDevice", "BlockDevice"

### spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir

`EmptyDirVolumeSource`

EmptyDir volume source.
Use this for temporary storage that is erased when the pod is removed.
Useful for scratch space, caching, or sharing data between containers.

### spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir.medium

`string`

Medium for the empty directory.
"" (default) uses the node's default medium (typically disk).
"Memory" uses a tmpfs (RAM-backed filesystem).

Memory-backed volumes are faster but:
- Count against container memory limits
- Are lost on node restart
- Should have sizeLimit set to prevent OOM

- rule: Medium must be either "" or "Memory"

### spec.jobTemplate.container.sidecars[].volumeMounts[].emptyDir.sizeLimit

`string`

Size limit for the empty directory.
Format: Kubernetes quantity (e.g., "1Gi", "500Mi").
Only strictly enforced when medium is "Memory".
For disk-backed volumes, this is a best-effort limit.

### spec.jobTemplate.container.sidecars[].volumeMounts[].pvc

`PvcVolumeSource`

PersistentVolumeClaim volume source.
Use this to mount an existing PVC.
For StatefulSets, this can reference a volumeClaimTemplate.

### spec.jobTemplate.container.sidecars[].volumeMounts[].pvc.claimName

`string` · required

Name of the PersistentVolumeClaim to mount.
For StatefulSets, this can be the name of a volumeClaimTemplate.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].pvc.readOnly

`bool`

Whether the PVC should be mounted read-only.
Default is false.

### spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken

`ServiceAccountTokenVolumeSource`

Projected ServiceAccount token volume source.
Use this to mount a short-lived, audience-bound identity token that the
kubelet issues for the pod's ServiceAccount and rotates automatically.

### spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.audience

`string` · required

Intended audience of the token. The receiving service must identify
itself with this audience when verifying the token; a token minted for a
different audience is rejected. Required: an audience-less token would be
replayable against any service in the cluster.

- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.expirationSeconds

`int64`

Requested lifetime of the token in seconds. The kubelet starts rotating
the token when it passes 80% of its lifetime or 24 hours, whichever is
shorter. Defaults to 3600 (1 hour). The Kubernetes API enforces a
minimum of 600 (10 minutes).

- rule: Expiration must be at least 600 seconds (the Kubernetes API minimum)

### spec.jobTemplate.container.sidecars[].volumeMounts[].serviceAccountToken.path

`string`

Filename for the token relative to the mount path.
Defaults to "token".

### spec.jobTemplate.container.sidecars[].lifecycle

`WorkloadContainerLifecycle`

Lifecycle hooks. `post_start` runs immediately after the container starts (the
container is not Running until it completes); `pre_stop` runs before termination
and is the standard lever for connection draining — e.g. a short sleep that keeps
the endpoint serving while load balancers converge on the terminating state.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart

`WorkloadLifecycleHandler`

Runs immediately after the container is created. The container does not reach
Running until the hook completes; a failing post_start kills the container per
its restart policy.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.portName

`string`

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.portName

`string`

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.container.sidecars[].lifecycle.postStart.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.container.sidecars[].lifecycle.preStop

`WorkloadLifecycleHandler`

Runs before the container is terminated by the kubelet (pod deletion, rolling
update, eviction). The termination grace period starts BEFORE the hook runs, so
keep `pod.termination_grace_period_seconds` larger than the hook's worst-case
duration. The classic zero-downtime pattern is a short sleep here so the pod
keeps serving while endpoint removal propagates.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.portName

`string`

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.portNumber

`int32`

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.portName

`string`

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.container.sidecars[].lifecycle.preStop.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.container.sidecars[].securityContext

`WorkloadContainerSecurityContext`

Container-level security hardening. Settings here override the pod-level
security context for this container only.

### spec.jobTemplate.container.sidecars[].securityContext.privileged

`bool`

Runs the container with full host access — equivalent to root on the node.
Required by some node-level agents (device managers, network plugins). Never
combine with untrusted images.

### spec.jobTemplate.container.sidecars[].securityContext.runAsUser

`int64` · optional (explicit presence)

UID the container process runs as. Overrides the image's USER directive.

### spec.jobTemplate.container.sidecars[].securityContext.runAsGroup

`int64` · optional (explicit presence)

Primary GID the container process runs as.

### spec.jobTemplate.container.sidecars[].securityContext.runAsNonRoot

`bool` · optional (explicit presence)

Refuses to start the container if its effective user is root. The standard
baseline hardening — it catches images that silently default to UID 0.

### spec.jobTemplate.container.sidecars[].securityContext.readOnlyRootFilesystem

`bool` · optional (explicit presence)

Mounts the container's root filesystem read-only. Pair with EmptyDir mounts for
paths the app must write (e.g. /tmp).

### spec.jobTemplate.container.sidecars[].securityContext.allowPrivilegeEscalation

`bool` · optional (explicit presence)

Whether the process can gain more privileges than its parent (setuid binaries,
file capabilities). The restricted Pod Security Standard requires this to be
false. Always true when `privileged` is set, so leave it unset in that case.

### spec.jobTemplate.container.sidecars[].securityContext.capabilities

`WorkloadCapabilities`

Linux capabilities to add or drop. The restricted profile drops ALL and adds
back only NET_BIND_SERVICE when needed. Capability names are uppercase without
the CAP_ prefix (e.g. "NET_ADMIN", "SYS_TIME").

### spec.jobTemplate.container.sidecars[].securityContext.capabilities.add

`[]string`

Capabilities to add (e.g. "NET_BIND_SERVICE").

### spec.jobTemplate.container.sidecars[].securityContext.capabilities.drop

`[]string`

Capabilities to drop. Use ["ALL"] as the hardened baseline.

### spec.jobTemplate.container.sidecars[].securityContext.seccompProfile

`WorkloadSeccompProfile`

Seccomp syscall filter for the container. "RuntimeDefault" is the hardened
baseline; "Localhost" selects a node-local profile file via `localhost_profile`.

- rule: localhost_profile is required when type is "Localhost" and must be empty otherwise

### spec.jobTemplate.container.sidecars[].securityContext.seccompProfile.type

`string` · required

Profile type: "RuntimeDefault" (the container runtime's default filter — the
recommended baseline), "Unconfined" (no filtering), or "Localhost" (a profile
file installed on the node, named via localhost_profile).

- rule: Seccomp profile type must be one of "RuntimeDefault", "Unconfined", or "Localhost"
- rule: {"required":true}

### spec.jobTemplate.container.sidecars[].securityContext.seccompProfile.localhostProfile

`string`

Path of the profile file relative to the node's seccomp profile root. Required
when (and only meaningful when) type is "Localhost".

### spec.jobTemplate.pod

`WorkloadPod`

Pod-level configuration shared by every run's pods: identity
(ServiceAccount reference), init containers, scheduling, security
hardening, DNS, and termination behavior.

### spec.jobTemplate.pod.serviceAccount

`string | valueFrom`

The ServiceAccount pods run as. Accepts a literal ServiceAccount name or a
reference to a KubernetesServiceAccount resource, so an infra chart deploys the
identity (with its workload-identity binding and pull secrets) and the workload
in one run. When omitted, pods run as the namespace's `default` ServiceAccount.
Permissions attach to this identity through KubernetesRbac grants; cloud access
federates through the identity's own workload_identity configuration.

- references: KubernetesServiceAccount (`status.outputs.service_account_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesServiceAccount, name: <that resource's name>, fieldPath: status.outputs.service_account_name}} -- a bare string does not parse

### spec.jobTemplate.pod.automountServiceAccountToken

`bool` · optional (explicit presence)

Whether pods receive a projected kube-apiserver token mount. Tri-state like the
Kubernetes API: unset defers to the ServiceAccount/cluster default, false hardens
pods that never call the Kubernetes API (a security-baseline recommendation for
ordinary app workloads), true forces the mount.

### spec.jobTemplate.pod.imagePullSecrets

`[]string | valueFrom`

Docker-registry Secrets, declared BESIDE this workload, that the kubelet pulls
its images with. Each entry is a literal Secret name or a reference: by default a
KubernetesSecret (its `docker_config_json` arm) resolved at `spec.name`; a
KubernetesExternalSecret fed from a cloud secrets manager is named with an
explicit `kind` and `fieldPath: status.outputs.secret_name` (the name the
operator materializes).

Three ways a private image gets pulled, and when to use each:
- Nothing declared — the cluster's own identity reaches the registry (EKS nodes
  to ECR, GKE nodes to Artifact Registry, AKS kubelet identity to ACR, DOKS
  registry integration). No Secret, no field.
- `image_registries` — the login belongs to this workload alone; the module owns
  the Secret.
- `image_pull_secrets` — the login is a resource beside the workload: share one
  Secret across workloads, or attach it once on the KubernetesServiceAccount the
  pods run as (then no entry is needed here).

- references: KubernetesSecret (`spec.name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesSecret, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.jobTemplate.pod.imageRegistries

`[]WorkloadImageRegistry`

Image registries this workload pulls from that need a login, with the login,
declared on the workload itself. List a registry here ONLY when it needs one: a
registry the cluster's own identity reaches (EKS nodes to ECR, GKE nodes to
Artifact Registry, AKS kubelet identity to ACR, DOKS registry integration) is not
listed at all.

The module materializes the logins into ONE docker-registry Secret named
`<workload>-image-pull` in the workload's namespace — the twin of the
`<workload>-env-secrets` Secret it builds from the containers' `env.secrets` —
and attaches it to the pod beside `image_pull_secrets`. The Secret is created,
rolled, and destroyed with the workload and follows it into preview
environments. Use this when the login belongs to this workload alone; when
several workloads share a registry, declare the credential once (a
KubernetesSecret or a KubernetesExternalSecret) and name it in
`image_pull_secrets` or on the shared KubernetesServiceAccount.

Example (YAML):
```yaml
imageRegistries:
  - server: ghcr.io
    username: acme-pull-bot
    password: $secret/ghcr-pull-token
```

- rule: Two imageRegistries entries name the same registry server — a workload holds one login per registry; merge them into one entry

### spec.jobTemplate.pod.imageRegistries[].server

`string` · required

Registry host as it appears in the image reference — "ghcr.io", "quay.io",
"123456789012.dkr.ecr.us-east-1.amazonaws.com". Docker Hub's login is keyed by
"https://index.docker.io/v1/", the one exception to the bare-host rule.

- rule: {"required":true,"string":{"minLen":"1"}}

### spec.jobTemplate.pod.imageRegistries[].username

`string` · required

Registry username — for GitHub Container Registry, the GitHub login of the token's owner.

- rule: {"required":true,"string":{"minLen":"1"}}

### spec.jobTemplate.pod.imageRegistries[].password

`string` · required · sensitive

Password or access token for `username`. Never plaintext in a manifest: only a
`$secret/<slug>` reference to an organization secret is accepted, and the
runner resolves it inside your infrastructure at deploy time before the module
writes the Secret. Prefer a scoped read-only token (GHCR `read:packages`) over an
account password.

- rule: {"required":true,"string":{"minLen":"1"}}

### spec.jobTemplate.pod.imageRegistries[].email

`string`

Optional email some registries record on the login; most ignore it.

### spec.jobTemplate.pod.initContainers

`[]WorkloadContainer`

Init containers, run to completion in order before app containers start.
Standard uses: schema migrations, config templating, waiting on dependencies.
A failing init container blocks the pod per its restart policy.

### spec.jobTemplate.pod.initContainers[].name

`string`

The container's name, unique within the pod. Required for sidecars and init
containers (Kubernetes rejects unnamed containers); for the main app container the
module defaults it when omitted, so minimal manifests stay minimal. Must be a valid
DNS label: lowercase alphanumeric and hyphens, starting and ending alphanumeric.

- rule: Container name must be a lowercase DNS label (alphanumeric and hyphens, starting and ending with an alphanumeric character)

### spec.jobTemplate.pod.initContainers[].image

`WorkloadContainerImage` · required

The container image, split into repository and tag so deployment pipelines can
inject a freshly built tag without rewriting the whole reference. A container
carries no pull credential of its own: the kubelet pulls every image in a pod
with the pod's pull secrets, so a private registry's login is declared once,
pod-wide — on `pod.image_registries` (the login itself) or `pod.image_pull_secrets`
(a Secret declared beside the workload), or on the ServiceAccount the pod runs as.

- rule: Image repo is required — the repository half of the image reference (e.g. "nginx" or "ghcr.io/acme/api")
- rule: Image tag is required — pin a version (e.g. "1.27.1"); avoid "latest" for anything you intend to roll back
- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].image.repo

`string`

The repository of the image (e.g. "nginx" or "ghcr.io/acme/checkout").

### spec.jobTemplate.pod.initContainers[].image.tag

`string`

The tag of the image (e.g. "1.27.1"). Pin a version; "latest" cannot be rolled back.

### spec.jobTemplate.pod.initContainers[].imagePullPolicy

`string`

When the kubelet pulls the image. "IfNotPresent" (the Kubernetes default for tagged
images) reuses a cached copy; "Always" re-resolves the tag on every pod start —
required when a mutable tag like a branch name is reused across builds; "Never"
only uses pre-loaded images (air-gapped nodes, kind-loaded test images).

- rule: Image pull policy must be one of "Always", "IfNotPresent", or "Never"

### spec.jobTemplate.pod.initContainers[].command

`[]string`

Entrypoint override (Kubernetes `command`, Docker ENTRYPOINT). The image's
entrypoint runs when omitted. Not executed in a shell — provide argv elements,
e.g. ["/bin/sh", "-c", "exec my-server"].

### spec.jobTemplate.pod.initContainers[].args

`[]string`

Arguments to the entrypoint (Kubernetes `args`, Docker CMD). The image's CMD is
used when omitted. Variable references like $(VAR_NAME) are expanded from the
container's environment by the kubelet.

### spec.jobTemplate.pod.initContainers[].workingDir

`string`

Working directory for the entrypoint. Defaults to the image's configured WORKDIR.

### spec.jobTemplate.pod.initContainers[].ports

`[]WorkloadContainerPort`

Network ports this container exposes. Purely informational to Kubernetes for plain
pod-to-pod traffic, but load-bearing here: named ports are referenced by probes,
and `service_port` drives the Service wiring on kinds that create one
(Deployment, StatefulSet).

### spec.jobTemplate.pod.initContainers[].ports[].name

`string` · required

Port name, e.g. "http", "grpc", "metrics". Must be a lowercase DNS label that
starts and ends alphanumeric. Named ports are referenced by probes and become the
Service port names on service-fronted kinds.

- rule: Port name must contain only lowercase alphanumeric characters and hyphens, and start and end with an alphanumeric character (e.g. "http", "grpc-web")
- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].ports[].containerPort

`int32` · required

The port number the container listens on (1–65535).

- rule: {"required":true,"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.pod.initContainers[].ports[].networkProtocol

`string`

L4 protocol of the port. Defaults to "TCP" when omitted — the overwhelmingly
common case, so minimal manifests need not repeat it.

- rule: The network protocol must be one of "TCP", "UDP", or "SCTP"

### spec.jobTemplate.pod.initContainers[].ports[].appProtocol

`string`

Application protocol hint (e.g. "http", "grpc", "https"). Propagated to the
Service port's appProtocol on service-fronted kinds, where meshes and L7 load
balancers use it to pick the right protocol handling.

### spec.jobTemplate.pod.initContainers[].ports[].servicePort

`int32`

The port the workload's Kubernetes Service exposes for this container port.
Only meaningful on kinds that create a Service (Deployment, StatefulSet); other
kinds ignore it. E.g. containerPort 8080 with servicePort 80 serves the app on
the conventional port while the process binds an unprivileged one. External
exposure is composed separately with first-class ingress kinds referencing the
workload's exported Service handle — workloads never create ingress themselves.

- rule: Service port must be between 1 and 65535

### spec.jobTemplate.pod.initContainers[].ports[].hostPort

`int32`

Exposes the container port directly on the node's IP (hostPort). Chiefly a
DaemonSet pattern (node-level agents that must be reachable on every node);
on other kinds it constrains scheduling to one pod per node per port — prefer
a Service unless node-local reachability is the point.

- rule: Host port must be between 1 and 65535

### spec.jobTemplate.pod.initContainers[].env

`ContainerEnv`

Environment configuration: plain variables (with Kubernetes-native value sources
and Planton cross-resource references), secret variables (materialized into a
managed Kubernetes Secret), and bulk envFrom imports.

### spec.jobTemplate.pod.initContainers[].env.variables

`[]EnvVar`

Individual environment variables (non-sensitive).

### spec.jobTemplate.pod.initContainers[].env.variables[].name

`string` · required

The environment variable name.
Must be a valid C_IDENTIFIER: starts with a letter or underscore,
followed by letters, digits, or underscores.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].value

`string`

Direct literal value.

### spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom

`ValueFromRef`

Reference to another Planton resource's field.
The orchestrator resolves this and populates the value before invoking IaC modules.

### spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.env

`string`

### spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].valueFrom.fieldPath

`string`

### spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef

`ConfigMapKeyRef`

Reference to a key in a Kubernetes ConfigMap.

### spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.key

`string` · required

Key within the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].configMapKeyRef.optional

`bool`

If true, the env var is silently skipped when the ConfigMap or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef

`ObjectFieldRef`

Reference to a pod-level field (metadata.name, status.podIP, etc.).

### spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef.apiVersion

`string`

Version of the schema. Defaults to "v1".

### spec.jobTemplate.pod.initContainers[].env.variables[].fieldRef.fieldPath

`string` · required

Path of the field to select (e.g., "metadata.name", "status.podIP").

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef

`ResourceFieldRef`

Reference to container resource limits or requests (limits.cpu, requests.memory, etc.).

### spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.containerName

`string`

Container name. Required for init containers; defaults to the current
container for regular containers.

### spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.resource

`string` · required

Resource to select (e.g., "limits.cpu", "requests.memory").

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.variables[].resourceFieldRef.divisor

`string`

Specifies the output format of the exposed resource.
For CPU: "1" means cores. For memory: "1", "1Ki", "1Mi", "1Gi".

### spec.jobTemplate.pod.initContainers[].env.secrets

`[]SecretEnvVar`

Individual secret environment variables (sensitive).

### spec.jobTemplate.pod.initContainers[].env.secrets[].name

`string` · required

The environment variable name.

- rule: Must be a valid C_IDENTIFIER (start with letter/underscore, contain only letters, digits, underscores)
- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.secrets[].value

`string`

Literal string value.
A Kubernetes Secret is automatically created and the environment variable
references that secret.

### spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef

`KubernetesSecretKeyRef`

Reference to a key within an existing Kubernetes Secret.

### spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.namespace

`string`

The namespace of the Kubernetes Secret.
If not specified, defaults to the namespace where the component is deployed.
Note: Cross-namespace secret references may not be supported by all Helm charts.

### spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.name

`string` · required

The name of the Kubernetes Secret.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.key

`string` · required

The key within the Kubernetes Secret that contains the value.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.secrets[].secretRef.optional

`bool`

If true, the env var is silently skipped when the Secret or key does not exist
(instead of blocking pod startup).

### spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom

`ValueFromRef`

Reference to another Planton resource's secret output field.
The orchestrator resolves this before invoking IaC modules.

### spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.kind

`enum`

Allowed values (use exactly as shown):

- `unspecified` -- 0: Default/unspecified
- `TestCloudResourceGeneric` -- 1–49: Test/dev/custom
- `TestCloudResourceKubernetes`
- `AwsAlb` -- 1000–1999: AWS resources AwsSubnet is a prerequisite because an ALB requires at least two subnets in different availability zones -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsCertManagerCert`
- `AwsCloudFront`
- `AwsDynamodb`
- `AwsEcrRepo`
- `AwsEcsCluster`
- `AwsEcsService` -- AwsEcsCluster, AwsEcsTaskDefinition, and AwsSubnet are prerequisites because a service schedules a referenced task-definition revision into a referenced live cluster and places task network interfaces into referenced subnets -- all three references must resolve first.
- `AwsEksCluster` -- AwsSubnet and AwsIamRole are prerequisites because the control plane attaches its network interfaces into referenced subnets and assumes a referenced cluster role that must already carry AmazonEKSClusterPolicy.
- `AwsIamRole`
- `AwsLambda`
- `AwsRdsCluster`
- `AwsRdsInstance`
- `AwsRoute53Zone`
- `AwsS3Bucket`
- `AwsLbTargetGroup` -- AwsVpc is a prerequisite because a target group's health checks and target registrations live inside one VPC -- the spec's vpc_id reference must resolve before the group can be created.
- `AwsSecurityGroup` -- AwsVpc is a prerequisite because every security group is created in a VPC; the E2E install profile resolves vpc_id against the VPC prerequisite.
- `AwsVpc`
- `AwsEksNodeGroup` -- AwsEksCluster is a prerequisite because nodes register with a live control plane; AwsIamRole and AwsSubnet back the node role and worker subnet references.
- `AwsIamUser`
- `AwsKmsKey`
- `AwsEc2Instance`
- `AwsClientVpn` -- Every Client VPN endpoint requires an ACM server certificate at create time; the imported self-signed fixture satisfies it. Subnets/VPC are optional composition (a zero-association endpoint is valid) -- composed scenarios declare them via the e2e-prerequisites annotation.
- `AwsDocumentDb`
- `AwsRoute53DnsRecord` -- AwsRoute53Zone is a prerequisite because every record lives inside a hosted zone -- the spec's zone_id reference must resolve before the record can be created.
- `AwsS3ObjectSet` -- AwsS3Bucket is a prerequisite because the object set's bucket reference is required -- objects cannot exist without the bucket that holds them.
- `AwsSqsQueue`
- `AwsSnsTopic`
- `AwsEventBridgeBus`
- `AwsEventBridgeRule`
- `AwsIamOidcProvider`
- `AwsIamPolicy`
- `AwsIamInstanceProfile` -- AwsIamRole is a prerequisite because an instance profile is a wrapper that must contain a role to be useful -- the profile's spec requires a role reference, so the role must be deployed first.
- `AwsLbListener` -- AwsAlb and AwsLbTargetGroup are prerequisites because a listener is an attachment point on a load balancer and its default action almost always forwards to a target group -- both references must resolve before the listener can be created.
- `AwsLbListenerRule` -- AwsLbListener is a prerequisite because a rule only exists as an attachment on a listener -- the listener_arn reference must resolve before the rule can be created.
- `AwsLaunchTemplate`
- `AwsAutoScalingGroup` -- AwsSubnet and AwsLaunchTemplate are prerequisites because a group cannot exist without subnets to place capacity in and a launch template to launch from -- the spec's subnets and launch_template references must resolve before the group can be created.
- `AwsEksAddon` -- AwsEksCluster is a prerequisite because an add-on installs onto a live control plane -- the spec's cluster_name reference must resolve before the add-on can be created.
- `AwsEksFargateProfile` -- AwsEksCluster, AwsIamRole, and AwsSubnet are prerequisites because a Fargate profile attaches to a live control plane, runs pods as a referenced pod-execution role, and launches them into referenced private subnets -- all three references must resolve first.
- `AwsEksAccessEntry` -- AwsEksCluster and AwsIamRole are prerequisites because an access entry grants a referenced IAM principal access to a live control plane -- both references must resolve before the entry can be created.
- `AwsEcsTaskDefinition` -- AwsIamRole is a prerequisite because the kind's default posture -- Fargate with the awslogs logging default -- is rejected by AWS at registration time without an execution role the agent can assume.
- `AwsHttpApiGateway`
- `AwsStepFunction` -- AwsIamRole is a prerequisite because a state machine cannot be created without an execution role it can assume -- the spec's role_arn reference must resolve before the CreateStateMachine call.
- `AwsHttpApiVpcLink` -- AwsSubnet is a prerequisite because a VPC link is a set of managed ENIs provisioned into referenced subnets -- the subnet references must resolve before the link can be created. Security groups are optional on the link, so they compose per-scenario rather than as a registry prerequisite.
- `AwsHttpApiDomain` -- AwsCertManagerCert is a prerequisite because a custom domain cannot be created without a TLS certificate in the same region covering the domain -- the spec's certificate_arn reference must resolve first.
- `AwsVpcEndpoint` -- AwsVpcEndpoint's composed E2E scenarios reference the AwsVpc prerequisite's outputs (vpc_id + default_route_table_id for gateway endpoints) and the AwsSubnet pair's subnet_id outputs (interface endpoints), so both are genuine deploy-order prerequisites.
- `AwsElasticacheUser`
- `AwsElasticacheUserGroup` -- AwsElasticacheUser is a genuine prerequisite: AWS refuses to create a user group that does not contain a user named "default", so a group's composed E2E scenario must resolve a deployed user's outputs.
- `AwsRedshiftServerlessNamespace`
- `AwsRedshiftServerlessWorkgroup` -- The namespace is a genuine prerequisite: a workgroup attaches to exactly one namespace by name at create time, so its composed E2E scenario must resolve a deployed namespace's outputs. AwsSubnet is a prerequisite because Redshift Serverless requires the workgroup's subnets to span three availability zones.
- `AwsRedisElasticache` -- AwsSubnet is a prerequisite because the module builds an ElastiCache subnet group from referenced subnets -- the spec's subnet references must resolve before the replication group can deploy.
- `AwsOpenSearchDomain`
- `AwsMemcachedElasticache`
- `AwsServerlessElasticache`
- `AwsNlb` -- AwsSubnet is a prerequisite because an NLB requires at least one subnet mapping -- the spec's subnet references must resolve before the load balancer can be created.
- `AwsElasticIp`
- `AwsTransitGateway`
- `AwsGlobalAccelerator`
- `AwsSubnet`
- `AwsInternetGateway`
- `AwsNatGateway` -- AwsInternetGateway is a prerequisite because a public NAT gateway can only become available once the VPC it sits in has an internet gateway attached (AWS rejects the create otherwise) -- so the gateway must be deployed first. AwsVpc is a prerequisite because a REGIONAL NAT gateway (availability_mode = regional) references the VPC directly instead of a subnet.
- `AwsEgressOnlyInternetGateway`
- `AwsElasticFileSystem` -- AwsSubnet and AwsSecurityGroup are prerequisites because mount targets (required, min 1) place the file system's NFS endpoints into subnets and attach security groups -- both references must resolve before the CreateMountTarget calls.
- `AwsEfsAccessPoint` -- AwsElasticFileSystem is a prerequisite because an access point is created INTO a file system -- the spec's required file_system_id reference must resolve before the CreateAccessPoint call.
- `AwsFsxLustreFileSystem`
- `AwsFsxOpenzfsFileSystem`
- `AwsFsxWindowsFileSystem` -- Every Windows file system must join an Active Directory domain; the directory itself is external infrastructure (AWS Managed Microsoft AD or a self-managed domain), so only the network dependency is a declarable prerequisite.
- `AwsFsxOntapFileSystem`
- `AwsFsxOntapStorageVirtualMachine`
- `AwsFsxOntapVolume`
- `AwsFsxDataRepositoryAssociation`
- `AwsCognitoUserPool`
- `AwsCognitoIdentityProvider` -- AwsCognitoUserPool is a prerequisite because an identity provider is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateIdentityProvider call.
- `AwsCognitoUserPoolClient` -- AwsCognitoUserPool is a prerequisite because an app client is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateUserPoolClient call.
- `AwsCognitoResourceServer` -- AwsCognitoUserPool is a prerequisite because a resource server is created INTO a pool -- the spec's required user_pool_id reference must resolve before the CreateResourceServer call.
- `AwsWafWebAcl`
- `AwsWafIpSet`
- `AwsWafRegexPatternSet`
- `AwsCloudwatchLogGroup`
- `AwsCloudwatchAlarm`
- `AwsCloudwatchCompositeAlarm`
- `AwsKinesisStream`
- `AwsKinesisFirehose` -- Every Firehose destination requires an S3 configuration (the primary target for extended_s3; the failed/all-document backup for the rest) and an IAM role Firehose assumes to write to it, so both are hard deploy prerequisites.
- `AwsKinesisStreamConsumer` -- A consumer registers against exactly one stream and cannot exist without it.
- `AwsAthenaWorkgroup`
- `AwsGlueCatalogDatabase`
- `AwsRedshiftCluster`
- `AwsSagemakerDomain` -- AI/ML A domain cannot exist without VPC subnets and a SageMaker execution role (default_user_settings.execution_role_arn is required), so both are hard deploy prerequisites.
- `AwsAppRunnerService` -- A service can run entirely on companion defaults, so the App Runner family's kinds are dependency-free leaves except the VPC connector (which cannot exist without subnets and security groups). A service's companion references (auto scaling / VPC connector / observability / WAF) are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsAppRunnerAutoScalingConfiguration`
- `AwsAppRunnerVpcConnector`
- `AwsAppRunnerObservabilityConfiguration`
- `AwsTransitGatewayVpcAttachment` -- AwsTransitGateway is a prerequisite because an attachment cannot exist without the gateway it attaches to; AwsSubnet because the attachment provisions an ENI into at least one subnet (the VPC arrives transitively through the subnet's own prerequisites).
- `AwsTransitGatewayRouteTable` -- Only the gateway is a hard prerequisite: a route table can exist empty. Associations, propagations, and routes referencing attachments are optional composition -- scenarios declare them via the e2e-prerequisites annotation.
- `AwsBatchComputeEnvironment` -- A MANAGED compute environment always launches into VPC subnets, so the subnet is a hard deploy prerequisite (security groups are required only for the Fargate types -- scenario-declared, not a registry edge).
- `AwsBatchJobQueue` -- A job queue cannot exist without at least one VALID compute environment to map onto.
- `AwsBatchSchedulingPolicy`
- `AwsBatchJobDefinition`
- `AwsCodeBuildProject` -- CI/CD
- `AwsCodePipeline`
- `AwsMwaaEnvironment` -- Workflow / Orchestration AwsSubnet and AwsSecurityGroup are prerequisites because the environment's network interfaces are placed in referenced private subnets and AWS requires at least one attached security group at creation.
- `AwsNeptuneCluster` -- Graph Database
- `AwsMemorydbCluster` -- A cluster always launches into a subnet group; the subnets are the hard deploy prerequisite. The ACL it attaches is optional composition (the built-in "open-access" ACL needs no resource) -- scenarios declare the ACL/user chain via the e2e-prerequisites annotation.
- `AwsMemorydbUser`
- `AwsMemorydbAcl` -- An empty ACL is valid (MemoryDB has no mandatory "default" member), so the user is optional composition -- the composed scenario declares it via the e2e-prerequisites annotation, never a registry edge.
- `AwsMskCluster` -- Streaming AwsSubnet and AwsSecurityGroup are prerequisites because brokers are placed in referenced subnets and AWS requires at least one attached security group at creation.
- `AwsMskServerlessCluster` -- AwsSubnet is a prerequisite because the serverless cluster's network interfaces are placed in referenced subnets (security groups are optional -- AWS attaches the VPC default group when none are referenced).
- `AwsLambdaEventSourceMapping` -- AwsLambda is a prerequisite because a mapping cannot exist without the function it invokes (a required reference). Event sources (SQS, Kinesis, DynamoDB, MSK) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsSnsSubscription` -- AwsSnsTopic is a prerequisite because a subscription cannot exist without the topic it subscribes to (a required reference). Endpoints (SQS queues, Lambda functions, Firehose streams) are optional composition -- scenarios declare them via the e2e-prerequisites annotation rather than taxing every consumer's chain.
- `AwsPlantonRunner` -- AwsSubnet is a prerequisite because the runner appliance places its network interfaces into referenced subnets -- the placement reference must resolve before the appliance can deploy.
- `AwsRoute53HealthCheck`
- `AwsSesConfigurationSet` -- Both SES kinds are dependency-free leaves: an identity's configuration set is optional composition (scenarios declare it via the e2e-prerequisites annotation), and a configuration set's event destinations reference other kinds only optionally.
- `AwsSesEmailIdentity`
- `AwsSecretsManagerSecret` -- A dependency-free leaf: the KMS key, rotation Lambda, and external rotation role references are all optional composition -- scenarios declare them via the e2e-prerequisites annotation, never registry edges.
- `AwsOpenSearchServerlessCollection` -- A dependency-free leaf: the collection-scoped encryption/network/ data-access/retention policies are module-rendered, and the KMS key and data-access principal references are optional composition (e2e-prerequisites annotation).
- `AwsBedrockGuardrail` -- A dependency-free leaf: the KMS key reference is optional composition (e2e-prerequisites annotation); published versions are folded satellites of the guardrail itself.
- `AwsBedrockCustomModel` -- AwsIamRole is a prerequisite because Bedrock assumes the job role to read training data and write outputs; the S3 locations and KMS key are optional composition (e2e-prerequisites annotation).
- `AwsBedrockInferenceProfile` -- A dependency-free leaf: the model source is a foundation model or an AWS system-defined cross-region profile, never a customer resource.
- `AwsBedrockProvisionedThroughput` -- A dependency-free leaf in the registry: capacity is typically bought for an AwsBedrockCustomModel (the default reference), but foundation model ARNs are equally legal, so the edge is optional composition.
- `AwsBedrockModelAccess` -- A dependency-free leaf: the agreement covers an AWS-listed foundation model, never a customer resource.
- `AwsBedrockInvocationLogging` -- Region settings singleton (one invocation-logging configuration per account+region; identity = the region). Delivery destinations are optional references (at least one of CloudWatch/S3, enforced by CEL), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsBedrockAgent` -- AwsIamRole is a prerequisite because the Bedrock service assumes the agent resource role to invoke models, action-group Lambdas, and knowledge bases; the guardrail, KMS key, provisioned throughput, and collaborator/knowledge-base edges are optional composition (e2e-prerequisites annotation). Action groups, aliases, collaborators, and knowledge-base associations are folded satellites of the agent.
- `AwsBedrockKnowledgeBase` -- AwsIamRole is a prerequisite because the Bedrock service assumes the knowledge-base role to read data sources, call the embedding model, and read/write the vector store; the vector-store and data-source reference edges (OpenSearch, S3, Secrets Manager, ...) are optional composition (e2e-prerequisites annotation). Data sources are folded satellites of the knowledge base.
- `AwsBedrockFlow` -- AwsIamRole is a prerequisite because the Bedrock service assumes the flow execution role to invoke the models, agents, knowledge bases, and Lambdas its nodes reference; the node-level reference edges are optional composition (e2e-prerequisites annotation).
- `AwsBedrockPrompt` -- A dependency-free leaf: variants target AWS-listed foundation models by ID; targeting another agent's alias is optional composition (e2e-prerequisites annotation).
- `AwsBedrockAgentCoreRuntime` -- AwsIamRole is a prerequisite because the AgentCore service assumes the runtime role to pull the container image or read the S3 code bundle and to run the hosted agent; the code-bundle S3 bucket and VPC placement edges are optional composition (e2e-prerequisites annotation). Endpoints and the runtime's resource policy are folded satellites of the runtime.
- `AwsBedrockAgentCoreGateway` -- AwsIamRole is a prerequisite because the gateway assumes its role to reach targets (invoke Lambdas, sign SigV4 requests); the target and credential-provider reference edges (runtime, Lambda, Identity providers, policy engine) are optional composition (e2e-prerequisites annotation). Targets are folded satellites of the gateway - AWS deletes them before the gateway at destroy.
- `AwsBedrockAgentCoreMemory` -- A dependency-free leaf for built-in strategies: the execution role (custom strategies, Kinesis delivery), KMS key, and Kinesis stream edges are optional composition (e2e-prerequisites annotation). Strategies are folded satellites of the memory - AWS serializes their changes through the parent.
- `AwsBedrockAgentCoreIdentity` -- A dependency-free leaf: workload identities, credential providers, and the Cedar policy engine with its policies are all name-keyed arms of one identity-and-access bundle; the KMS key edge is optional composition (e2e-prerequisites annotation). The account/region token-vault CMK is deliberately NOT modeled here (settings singleton).
- `AwsBedrockAgentCoreTools` -- A dependency-free leaf in the SANDBOX/PUBLIC postures: the execution role (recordings, certificates), S3, Secrets Manager, and VPC edges are optional composition (e2e-prerequisites annotation). Browsers, profiles, and code interpreters are name-keyed arms of one tools bundle; AWS exposes no update - every field change recreates the tool.
- `AwsBedrockAgentCoreEvaluation` -- The AgentCore Evaluations bundle - evaluators (LLM-judge or Lambda scorers), harnesses (repeatable agent test benches), and online evaluation configs (continuous scoring of sampled production sessions). Deploys standalone - no arm requires an agent runtime to exist. No registry prerequisite: every arm is optional, so no dependency is required for the kind to function (scenarios compose IAM roles via annotations).
- `AwsBedrockAgentCoreTokenVault` -- Account/region settings singleton: sets the KMS key on the ONE default AgentCore token vault. The KMS reference is conditional on key_type (CEL-enforced), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsSagemakerModel` -- The immutable serving definition (container image + artifacts + execution role) that endpoints deploy - one container or an inference pipeline.
- `AwsSagemakerEndpoint` -- A real-time inference endpoint WITH its folded endpoint configuration - the configuration is immutable upstream, so the modules roll name-suffixed configurations create-before-destroy and repoint the endpoint.
- `AwsSagemakerNotebookInstance` -- A managed Jupyter notebook EC2 instance with its folded lifecycle configuration (bootstrap scripts).
- `AwsSagemakerFeatureGroup` -- A Feature Store feature group - online and/or offline stores over a declared feature schema.
- `AwsSagemakerModelRegistry` -- A model registry package group with its folded resource policy - model package VERSIONS register into it imperatively (training pipelines), never declaratively.
- `AwsSagemakerPipeline` -- An ML workflow DAG (the SageMaker pipeline-definition JSON) that executions run against - free to create, billed per execution.
- `AwsSagemakerImage` -- A named registry entry exposing YOUR container images to Studio, with folded AWS-numbered versions (append-only by position).
- `AwsSagemakerMlflowServer` -- The classic hourly-billed managed MLflow tracking server (~25 min to provision; billed hourly from creation whether or not anyone is tracking). The serverless successor is AwsSagemakerMlflowApp.
- `AwsSagemakerMlflowApp` -- The serverless MLflow 3.x deployment (billed per use) - standalone, associating with SageMaker domains; NOT a tracking-server satellite.
- `AwsRestApiGateway` -- A full REST API (API Gateway v1): the resource/method tree with inline integrations (or an imported OpenAPI document), one stage with an explicit hash-triggered deployment, and the API-scoped satellites (authorizers, models, validators, gateway responses, policy, documentation, client certificate). Self-contained: a MOCK-integration API needs no other resource.
- `AwsRestApiDomain` -- A custom domain for REST APIs with base-path mappings and - for PRIVATE domains - VPC-endpoint access associations. AwsCertManagerCert is a prerequisite because the domain cannot be created without a TLS certificate covering it.
- `AwsRestApiUsagePlan` -- A usage plan metering REST API consumers - stage coverage, quota, throttles, and the API keys it admits. No registry prerequisite: a plan is valid with no stage coverage (scenarios compose the REST API via annotations).
- `AwsRestApiVpcLink` -- A REST API VPC link fronting an internal Network Load Balancer so REST integrations reach private services. AwsNlb is a prerequisite because AWS rejects link creation without the target balancer.
- `AwsApiGatewayAccountSettings` -- Region settings singleton (one API Gateway account object per account+region; identity = the region). The CloudWatch role is an optional reference (unset = the explicit no-logging posture), so prerequisites stay empty and E2E fixtures ride scenario annotations.
- `AwsCloudTrail` -- The account's API audit trail. AwsS3Bucket is a prerequisite because AWS rejects trail creation without a delivery bucket carrying the CloudTrail service-principal policy. 1240 opens the governance sub-band (1240-1249).
- `AwsConfigRecorder` -- Region singleton (one AWS Config recorder per region, named "default" by AWS; identity = the region). AwsIamRole is a prerequisite because the recorder cannot exist without its service role.
- `AwsConfigRule` -- One AWS Config compliance rule (managed, custom-lambda, or custom-policy; account- or organization-scoped) with optional auto-remediation. Managed rules need no prerequisites; the custom-lambda arm's function reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsGuardDuty` -- Region singleton (AWS allows one GuardDuty detector per account+region; the detector has no name - identity = the region). Satellite references (S3 export bucket, KMS key) are conditional, so E2E fixtures ride scenario annotations.
- `AwsCloudTrailEventDataStore` -- CloudTrail Lake: a queryable, immutable event data store with its own retention and billing lifecycle - no trail required. The KMS key reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigAggregator` -- AWS Config cross-account/cross-region aggregation: the aggregator (collector side) and/or the reciprocal authorization grants (source-account side). Works with zero recorders; the org-source role reference is conditional, so E2E fixtures ride scenario annotations.
- `AwsConfigConformancePack` -- An AWS Config conformance pack (account- or organization-scoped): a template bundle that creates its own Config rules. Deployment requires an active Config recorder in the region (a service-side requirement, not a spec reference), so E2E fixtures ride scenario annotations.
- `AwsGuardDutyMalwareProtectionPlan` -- GuardDuty Malware Protection for S3: scans new objects in one bucket - a standalone plan protecting a bucket, not a detector satellite (its schema carries no detector reference). The execution role and the protected bucket are required references.
- `AwsBackupVault` -- An AWS Backup vault - the encrypted container recovery points live in, as either a standard vault (with its lock, access policy, and notification satellites) or a logically air-gapped vault (AWS's own VaultType discriminator). The KMS and SNS references are conditional, so E2E fixtures ride scenario annotations. 1250 opens the backup sub-band (1250-1259).
- `AwsBackupPlan` -- An AWS Backup plan: scheduled backup rules plus the resource selections that assign resources to them. AwsBackupVault is a prerequisite because every rule requires a target vault; the selections' IAM role is conditional and rides scenario annotations.
- `AwsBackupFramework` -- A Backup Audit Manager framework: compliance controls evaluating backup posture. No schema-required references (the Config recorder its evaluations need is a lane fixture, not a spec reference).
- `AwsBackupReportPlan` -- A Backup Audit Manager report plan: scheduled compliance/job reports delivered to S3. AwsS3Bucket is a prerequisite because the delivery channel's bucket is required.
- `AwsBackupRestoreTestingPlan` -- An AWS Backup restore testing plan with its folded selections: scheduled restore tests proving recovery points actually restore. Vault targeting accepts the "*" wildcard, so fixtures are conditional and ride scenario annotations.
- `AwsBackupSettings` -- Account/region settings singleton for AWS Backup: the account's global settings (cross-account backup) and the region's resource-type opt-in/management preferences. Both provider deletes are no-ops - settings persist after destroy.
- `AwsSsmParameter` -- An SSM Parameter Store entry (String/StringList/SecureString). The parameter's name is an explicit spec field - names are hierarchical paths ("/prod/db/url") metadata.name cannot carry. The KMS reference is conditional (SecureString only), so E2E fixtures ride scenario annotations. 1260 opens the SSM sub-band (1260-1269).
- `AwsSsmDocument` -- A customer-owned SSM document (Command/Automation/Session/...): reusable action definitions managed nodes and automations execute. State Manager associations are their own AwsSsmAssociation kind - an association binds ANY document (AWS-managed included), so it is not this document's satellite.
- `AwsSsmMaintenanceWindow` -- An SSM maintenance window with its folded target registrations and tasks (Run Command / Automation / Lambda / Step Functions) - the targets and tasks are true window satellites (ForceNew window_id edges). Identity is the AWS-generated "mw-..." id.
- `AwsSsmPatchBaseline` -- An SSM patch baseline with its folded patch-group registrations and the account/region default-baseline designation (delete RESTORES AWS's own predefined default for the OS). Identity is the AWS-generated "pb-..." id.
- `AwsSsmAssociation` -- A State Manager association: the binding of an SSM document to targets on a schedule. Split from the document kind because the document reference is a free string with no structural edge - associations routinely bind AWS-managed documents (AWS-RunShellScript, ...) with no user document anywhere, so no registry prerequisite either. Identity is the AWS-generated association UUID.
- `AwsOrganization` -- THE AWS Organization of the deploying account - creating it makes the caller the management account. Trusted service access, delegated administrators, the org's singleton resource policy, and centralized root-access management (IAM's organizations features - a management-account act requiring iam.amazonaws.com trusted access) fold in (none has a life of its own; the standalone service-access resource fights the org's own argument with a perpetual diff). Deleting this deletes the entire organization. 1270 opens the Organizations sub-band (1270-1279).
- `AwsOrganizationalUnit` -- An organizational unit in the org's OU tree. The display name is an explicit spec field (OU names allow spaces metadata.name cannot carry); the parent reference (root or parent OU) is required and immutable, so the organization is a registry prerequisite.
- `AwsOrganizationAccount` -- A MEMBER account of the organization: creation, OU placement, and the account-level settings satellites (alternate/primary contacts, opt-in region enablement) fold onto the created account's ID. Destroy is never a clean delete (remove-from-org or ~90-day close) - taught on the spec. No registry prerequisite by the schema-required-only rule (the OU parent reference is optional).
- `AwsOrganizationPolicy` -- An Organizations policy (SCP and its twelve sibling types) with its folded attachments to roots, OUs, and member accounts. The policy type must be enabled on the organization first; AWS-managed policies are never adopted. No registry prerequisite by the schema-required-only rule (attachments are optional).
- `AwsBudget` -- A Budgets budget (COST/USAGE/RI/Savings Plans coverage and utilization) with its folded budget actions as name-keyed satellites - an action exists only on its budget and fires an IAM-policy application, an SCP attachment, or SSM instance stops when a threshold breaches. Budgets is account-global (served from us-east-1; the spec region is the provider endpoint). 1280 opens the cost-management sub-band (1280-1289).
- `AwsCostAnomalyMonitor` -- A Cost Explorer anomaly monitor (DIMENSIONAL over one dimension, or CUSTOM over a CE expression) with its folded alert subscriptions - a subscription's monitor list is the structural edge that makes it this monitor's satellite. Account-global; AWS identifies both by ARN.
- `AwsCostCategory` -- A Cost Explorer cost category: ordered rules (regular expression rules or inherited-value rules) over the recursive CE expression tree, plus split-charge rules. The account's cost-allocation-tag activation toggle is deliberately NOT folded here - it is a per-tag-key account feature with no edge to any category, so many category instances would fight over one account object.
- `AwsIamGroup` -- An IAM group with its folded declarative membership (the authoritative users list) and group policies - name-keyed inline documents plus managed-policy attachments. IAM is global; identity is the group name (renames update in place, the ARN recomputes). 1290 opens the IAM P1 sub-band (1290-1299).
- `AwsIamSamlProvider` -- An IAM SAML identity provider: the account's federation trust anchor, created from the IdP's metadata XML (a public document carrying certificates, not a secret). Identity is the provider ARN; the name is write-once.
- `AwsIamAccountSettings` -- Account settings singleton for IAM (a GLOBAL service - one object per ACCOUNT, not per region): the sign-in alias, the password policy, and the STS global-endpoint token version. Destroy contracts DIFFER per arm (each taught on its arm): the alias truly deletes, the password policy resets to AWS defaults, the STS preference is a no-op delete that persists.
- `AwsCloudwatchDashboard` -- A CloudWatch dashboard: one named dashboard whose widget layout is the dashboard-body JSON document (modeled as a typed Struct, the catalog's uniform policy-document idiom). Dashboards are untaggable at AWS. Identity is the dashboard name; every change is an in-place PutDashboard upsert. 1300 opens the CloudWatch observability P1 sub-band (1300-1309).
- `AwsCloudwatchSynthetics` -- CloudWatch Synthetics: a canary (a scheduled scripted probe running from an S3-staged code bundle under an execution role, writing run artifacts to S3) plus the grouping surface - owned groups and the canary's group associations (joins by group NAME, so shared groups are referenced, never fought over). A groups-only instance manages shared groups with no canary.
- `AwsCloudwatchLogDelivery` -- CloudWatch Logs delivery: the two ways logs leave CloudWatch. The vended-log arm pivots on a delivery SOURCE (one AWS resource whose service vends logs) with name-keyed deliveries fanning out to delivery destinations (S3 / CloudWatch Logs / Firehose / X-Ray), each created inline or referenced by ARN. The cross-account arm is the legacy Kinesis subscription destination with its access policy (whose delete is a no-op at AWS - the policy persists).
- `AwsCloudwatchLogAccountPolicy` -- A CloudWatch Logs account-level policy: one policy object per (name, type) pair per region - data protection, subscription filter, field index, transformer, or metric extraction - applied account-wide, optionally narrowed by selection criteria. Standalone account configuration, never a per-log-group satellite.
- `AwsCloudwatchLogAnomalyDetector` -- A CloudWatch Logs anomaly detector: one detector trains over a LIST of log groups (multi-parent scope - never a single group's satellite), surfacing anomalies on a chosen evaluation frequency with a bounded visibility window.
- `AwsCloudwatchLogResourcePolicy` -- A CloudWatch Logs resource policy: the account-scoped named policy (or resource-scoped policy on one log group ARN) that grants AWS services permission to write logs - Route53 query logging, EventBridge, and friends. Exactly one scope per instance.
- `AwsManagedPrometheus` -- An Amazon Managed Prometheus workspace with its folded satellites: workspace configuration (retention, label-set limits - a created-via-update singleton whose delete is a no-op at AWS), the alert manager definition (strictly one per workspace), name-keyed rule group namespaces, query logging, the workspace resource policy, and alias-keyed anomaly detectors. Scrapers are deliberately NOT folded here - a scraper can target CloudWatch with zero AMP workspaces, so it is its own kind.
- `AwsManagedPrometheusScraper` -- An Amazon Managed Prometheus scraper: the agentless collector. Source is an EKS cluster or a bare VPC placement (both replace-on-change); destination is an AMP workspace or a CloudWatch dataset. Carries its own scraper logging configuration satellite. Scrape configuration is optional on the EKS arm (AWS publishes a default, resolved at deploy) and required on the VPC arm.
- `AwsEventBridgePipe` -- An EventBridge Pipe: one point-to-point integration reading from one source (SQS, Kinesis, DynamoDB streams, MSK or self-managed Kafka, ActiveMQ/RabbitMQ), optionally filtering and enriching in-flight, and delivering to one target (ECS, Batch, Lambda, Step Functions, Kinesis, SQS, Redshift, SageMaker, CloudWatch Logs, EventBridge buses, HTTP via API destinations). The source is fixed for life (replace-on-change); the target swaps in place. 1310 opens the EventBridge extras P1 sub-band (1310-1319).
- `AwsEventBridgeScheduler` -- An EventBridge Scheduler schedule: cron/rate/one-time invocation of one target under an execution role, with flexible time windows, retry policy, and a dead-letter queue. The schedule GROUP is folded own-XOR-existing (a name-and-tags container - the provider's own update path is tags-only); unset means AWS's default group.
- `AwsEventBridgeApiDestination` -- An EventBridge API destination with its connection: the authenticated HTTP(S) endpoint rules, pipes, and schedules invoke. Two independently deployable arms - the CONNECTION (the shareable auth trust anchor: api-key, basic, or OAuth credentials that AWS stores in Secrets Manager) and the DESTINATION (endpoint + method + rate limit) whose connection is owned inline or referenced by ARN.
- `AwsVpcPeering` -- A VPC peering connection, as a request-XOR-accept mode union: the REQUEST arm creates the peering from its VPC toward a peer VPC (same-account auto-accept supported; cross-account/cross-region stays pending until accepted), the ACCEPT arm adopts and accepts a pending connection by ID from the accepter side. DNS-resolution options fold into both arms. 1320 opens the VPC networking P1 sub-band (1320-1329).
- `AwsNetworkAcl` -- A network ACL: the stateless subnet-level firewall - ordered ingress/egress rules (allow or deny, evaluated by rule number) and the subnet associations, all folded in-line as the single declarative owner (the standalone rule/association resources are the same payload and fight the in-line form).
- `AwsManagedPrefixList` -- A customer-managed prefix list: a named, versioned set of CIDR blocks that security-group rules, NACL rules, and route tables reference as one object. Entries fold in-line; max_entries is the capacity contract (referencing consumes that many rule slots regardless of how many entries exist).
- `AwsEbsVolume` -- A standalone EBS volume as a create-XOR-copy union (fresh in a zone, or cloned from another volume) with attachments managed in-line. 1330 opens the block & object storage sub-band (1330-1339).
- `AwsEbsSnapshot` -- An EBS snapshot as a three-way source union (snapshot a volume, copy a snapshot, or import a disk image) with archive tiering, fast snapshot restore, and cross-account share grants in-line.
- `AwsS3DirectoryBucket` -- An S3 directory bucket (S3 Express One Zone): single-AZ, single-digit-millisecond object storage. The modules derive the mandated "{name}--{zone_id}--x-s3" bucket name.
- `AwsS3TableBucket` -- An S3 table bucket (S3 Tables - managed Apache Iceberg storage) with its namespaces, tables, policies, and replication folded in-line as the single declarative owner.
- `AwsS3VectorBucket` -- An S3 vector bucket (AI embedding storage with similarity query) with its vector indexes folded in-line - the natural backend for Bedrock knowledge bases.
- `AwsDlmLifecyclePolicy` -- A Data Lifecycle Manager policy: account-level, tag-targeted snapshot/AMI automation (create, retain, archive, copy cross-region, share, deprecate) as a default-XOR-custom mode union. AwsIamRole is a prerequisite because DLM acts through a required execution role.
- `AwsRoute53ResolverEndpoint` -- A Route 53 Resolver endpoint (the hybrid-DNS bridge between a VPC and outside networks) with its forwarding rules and their VPC associations managed in-line. Subnets place the ENIs and security groups guard them - both schema-required. 1340 opens the DNS & service discovery sub-band (1340-1349).
- `AwsRoute53ResolverFirewall` -- A Route 53 Resolver DNS Firewall rule group with its domain lists, filtering rules, and VPC associations managed in-line - the DNS-layer block/allow policy for VPC egress queries. AwsVpc is a prerequisite because the association arm filters a referenced VPC.
- `AwsRoute53ResolverQueryLog` -- A Resolver query logging configuration (every DNS query VPCs make through the resolver, to CloudWatch Logs / S3 / Firehose) with its VPC associations managed in-line. AwsVpc is a prerequisite because the association arm logs a referenced VPC.
- `AwsCloudMapNamespace` -- An AWS Cloud Map namespace (HTTP-XOR-private-DNS-XOR-public-DNS) with its discoverable services and statically registered instances managed in-line - the service-discovery registry ECS and custom applications look each other up in. AwsVpc is a prerequisite because the private-DNS arm binds its hosted zone to a referenced VPC.
- `AwsAppSyncApi` -- An AppSync API - AWS's managed API service, as a GraphQL API (SDL schema, resolvers over data sources, caching, the MERGED federation variant) XOR an Events API (real-time pub/sub over channel namespaces) - with its data sources, resolvers, functions, types, API keys, and custom domain managed in-line. Every backend reference (data source targets, roles, the certificate) is optional, so no registry prerequisite - lanes exercise the fixture-free arms.
- `AwsLambdaLayer` -- A Lambda layer version - a shared code archive (libraries, custom runtimes) functions attach by ARN - with its cross-account and organization share grants managed in-line. The archive lives in S3 (an optional reference, so no registry prerequisite - lanes compose their own bucket fixture). 1351 sits in the app & data services sub-band (1350-1359; 1350 opens it with AwsAppSyncApi).
- `AwsRdsProxy` -- An RDS Proxy - the managed connection pool between connection-hungry applications and a database - with its connection-pool tuning, additional endpoints, and database target managed in-line. AwsIamRole is a prerequisite because the proxy assumes a required role to read database credentials from Secrets Manager; AwsSubnet because the proxy's network interfaces require at least two subnets.
- `AwsAuroraDsql` -- An Aurora DSQL cluster - serverless, PostgreSQL-compatible distributed SQL with active-active multi-region pairing managed in-line. No prerequisites: a single-region cluster deploys from defaults alone (the KMS and peer references are optional arms).
- `AwsEcrRegistrySettings` -- Region settings singleton (one private ECR registry per account+region): the registry policy, scanning configuration, replication rules, pull-through cache rules, repository creation templates, account settings, and pull-time update exclusions. Repository-scoped surface stays on AwsEcrRepo.
- `AwsPrivateCa` -- An AWS Private Certificate Authority with composed activation (a ROOT self-signs at apply; a subordinate activates from a parent AwsPrivateCa), issued certificates, the ACM renewal permission, and the resource policy managed in-line. No prerequisites: the S3 (CRL) and parent-CA references are optional arms.
- `AwsSesAccountSettings` -- Account/region settings singleton (one SES account object per account+region): the suppression list and VDM posture. 1360 opens the SES P1 sub-band (1360-1369).
- `AzureResourceGroup` -- 2000–2999: Azure resources
- `AzureAksCluster` -- AzureResourceGroup is the only required parent: the cluster is created inside a referenced resource group. Subnet is optional on the default node pool (AKS provisions managed networking when unset).
- `AzureAksNodePool` -- AzureAksCluster is a prerequisite because a node pool attaches to an existing cluster by ARM ID; the resource group chains transitively.
- `AzureContainerRegistry` -- AzureResourceGroup is a prerequisite because a container registry is created inside a resource group.
- `AzureDnsZone` -- AzureResourceGroup is a prerequisite because the DNS zone is created inside a referenced resource group that must already exist.
- `AzureKeyVault` -- AzureResourceGroup is a prerequisite because a key vault is created inside a referenced resource group in composed environments.
- `AzureVirtualNetwork` -- AzureResourceGroup is a prerequisite because a virtual network is created inside a referenced resource group in composed environments.
- `AzureNatGateway` -- AzureResourceGroup is a prerequisite because a NAT gateway is created inside a referenced resource group in composed environments.
- `AzureVirtualMachine` -- AzureNetworkInterface is a prerequisite because a virtual machine attaches at least one NIC (the subnet, network, and resource group chain transitively through the NIC's own prerequisites).
- `AzureStorageAccount` -- AzureResourceGroup is a prerequisite because a storage account is created inside a referenced resource group in composed environments.
- `AzureDnsRecord` -- AzureDnsZone is a prerequisite because a record set is created inside a referenced zone (the resource group chains transitively through the zone). Public DNS zone names are not globally unique, so a shared zone fixture is safe to recreate across scenarios.
- `AzureSubnet` -- AzureVirtualNetwork is a prerequisite because a subnet is an ARM child of a referenced network -- the network must exist before the subnet can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzureNetworkSecurityGroup` -- AzureResourceGroup is a prerequisite because a network security group is created inside a referenced resource group in composed environments.
- `AzurePublicIp` -- AzureResourceGroup is a prerequisite because a public IP is created inside a referenced resource group in composed environments.
- `AzurePrivateEndpoint` -- AzureSubnet is a prerequisite because a private endpoint draws its private IP from a referenced subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite). The connection target is polymorphic and the DNS zones / ASGs are optional, so none of those are prerequisites.
- `AzurePrivateDnsZone` -- AzureResourceGroup is a prerequisite because a private DNS zone is created inside a referenced resource group in composed environments.
- `AzureApplicationGateway` -- AzureSubnet is a prerequisite because a gateway cannot exist without its dedicated gateway_ip_configuration subnet (the network and resource group chain transitively through the subnet's own prerequisites); public frontends additionally reference a public IP, but private-only gateways are legal, so it is not a registry prerequisite.
- `AzureLoadBalancer` -- AzureResourceGroup is a prerequisite because a load balancer is created inside a referenced resource group (frontends additionally reference subnets or public IPs, but neither is universally required, so they are not registry prerequisites).
- `AzureRouteTable` -- AzureResourceGroup is a prerequisite because a route table is created inside a referenced resource group in composed environments.
- `AzurePrivateDnsZoneVirtualNetworkLink` -- AzurePrivateDnsZone and AzureVirtualNetwork are prerequisites because a virtual network link is a child resource of a referenced zone and binds it to a referenced network -- both must exist before the link can be written. (The resource group arrives transitively through the zone's and network's own prerequisite declarations.)
- `AzureVirtualNetworkPeering` -- AzureVirtualNetwork is a prerequisite because a peering is an ARM child of its local network and binds it to a remote network -- the local network must exist before the peering can be written. (The resource group arrives transitively through the network's own prerequisite declaration.)
- `AzurePublicIpPrefix` -- AzureResourceGroup is a prerequisite because a public IP prefix is created inside a referenced resource group in composed environments.
- `AzureNetworkInterface` -- AzureSubnet is a prerequisite because a network interface's IP configurations deploy into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureManagedDisk` -- AzureResourceGroup is a prerequisite because a managed disk is created inside a resource group.
- `AzureVirtualMachineScaleSet` -- AzureSubnet is a prerequisite because every scale-set instance's network interface deploys into a subnet (the virtual network and resource group chain transitively through the subnet's own prerequisite).
- `AzureKeyVaultKey` -- AzureKeyVault is a prerequisite because a key is a data-plane object inside a referenced vault -- the vault must exist before the key can be written (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultCertificate` -- AzureKeyVault is a prerequisite because a certificate is a data-plane object inside a referenced vault -- the vault must exist before the certificate can be enrolled or imported (the resource group chains transitively through the vault's own prerequisite).
- `AzureKeyVaultSecret` -- AzureKeyVault is a prerequisite because a secret is a data-plane object inside a referenced vault -- the vault must exist before the secret can be written (the resource group chains transitively through the vault's own prerequisite). Part of the Key Vault family (2005, 2025-2026) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureWebApplicationFirewallPolicy` -- AzureResourceGroup is a prerequisite because a WAF policy is created inside a referenced resource group; the Application Gateways that attach the policy reference it, never the reverse.
- `AzureApplicationSecurityGroup` -- AzureResourceGroup is a prerequisite because an application security group is created inside a referenced resource group; network interfaces, scale-set IP configurations, and NSG security rules reference the group, never the reverse.
- `AzureDiskEncryptionSet` -- AzureKeyVaultKey is a prerequisite because a disk encryption set wraps customer data with a referenced key -- the key (and its vault, which chains transitively) must exist before the set can resolve the key URL at create time.
- `AzurePostgresqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureRedisCache` -- AzureResourceGroup is a prerequisite because the cache is created inside a referenced resource group (VNet injection additionally references a dedicated subnet, but only the Premium tier supports it, so it is not a registry prerequisite).
- `AzureCosmosdbAccount` -- AzureResourceGroup is a prerequisite because the account is created inside a referenced resource group.
- `AzureMssqlServer` -- AzureResourceGroup is a prerequisite because the logical server is created inside a referenced resource group.
- `AzureMysqlFlexibleServer` -- AzureResourceGroup is a prerequisite because a server is created inside a referenced resource group (VNet injection additionally references a delegated subnet and a private DNS zone, but neither is universally required, so they are not registry prerequisites).
- `AzureMssqlDatabase` -- The parent logical server is referenced via server_id, not auto-deployed: E2E scenarios declare their own server fixture (minimal-server.yaml or the pool-attach chain through AzureMssqlElasticPool) so sequential subtests never destroy and recreate the same globally unique server_name.
- `AzureMssqlElasticPool` -- AzureMssqlServer is a prerequisite because every elastic pool lives on a referenced logical server (the server's resource group is transitive).
- `AzureRedisLinkedServer` -- The target and linked caches are referenced via ARM ids, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicy` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureRedisCacheAccessPolicyAssignment` -- The parent cache is referenced via redis_cache_id, not auto-deployed: caches are the slowest-provisioning resources in the Azure catalog and their names are globally unique, so E2E scenarios declare their own cache fixtures instead of a registry prerequisite recreating a shared one per run.
- `AzureContainerAppEnvironment` -- AzureResourceGroup is a prerequisite because the environment is created inside a referenced resource group that must already exist.
- `AzureContainerApp` -- AzureContainerAppEnvironment is a prerequisite because every app runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureServicePlan` -- AzureResourceGroup is a prerequisite because the plan is created inside a referenced resource group that must already exist.
- `AzureFunctionApp` -- AzureServicePlan is a prerequisite because a function app runs on a referenced plan (the resource group arrives transitively through the plan). The required storage account is deliberately NOT a registry prerequisite: storage-account names are globally unique, so scenarios bring their own scenario-local account fixtures.
- `AzureLinuxWebApp` -- AzureServicePlan is a prerequisite because a web app runs on a referenced plan (the resource group arrives transitively through the plan).
- `AzureContainerAppJob` -- AzureContainerAppEnvironment is a prerequisite because a job runs inside a referenced environment (the resource group arrives transitively through it).
- `AzureContainerAppEnvironmentStorage` -- AzureContainerAppEnvironment is a prerequisite because the storage registration lives on a referenced environment. The Azure Files share and storage account are deliberately NOT registry prerequisites: storage-account names are globally unique, so scenarios bring their own scenario-local account + share fixtures.
- `AzureContainerAppEnvironmentDaprComponent` -- AzureContainerAppEnvironment is a prerequisite because the Dapr component is registered on a referenced environment.
- `AzureContainerAppEnvironmentCertificate` -- AzureContainerAppEnvironment is a prerequisite because the certificate is stored on a referenced environment.
- `AzureContainerAppEnvironmentManagedCertificate` -- AzureContainerAppEnvironment is a prerequisite because the managed certificate is provisioned on a referenced environment.
- `AzureLogAnalyticsWorkspace` -- AzureResourceGroup is a prerequisite because the workspace is created inside a referenced resource group that must already exist.
- `AzureApplicationInsights` -- AzureLogAnalyticsWorkspace is a prerequisite because workspace-based Application Insights stores its telemetry in a referenced workspace (the resource group chains transitively through the workspace).
- `AzureMonitorDiagnosticSetting` -- AzureLogAnalyticsWorkspace is a prerequisite because the setting's scenarios route a fixture workspace's telemetry (the workspace doubles as target and destination); the target itself is polymorphic.
- `AzureMonitorActionGroup` -- AzureResourceGroup is a prerequisite because the action group is created inside a referenced resource group that must already exist.
- `AzureMonitorMetricAlert` -- AzureMonitorActionGroup is a prerequisite because a metric alert's actions fire into a referenced action group (the resource group chains transitively); alert scopes are polymorphic.
- `AzureMonitorScheduledQueryAlert` -- AzureLogAnalyticsWorkspace is a prerequisite because the rule queries a referenced workspace scope; AzureMonitorActionGroup because its action fires into a referenced action group.
- `AzureMonitorActivityLogAlert` -- AzureMonitorActionGroup is a prerequisite because an activity log alert's actions fire into a referenced action group (the resource group chains transitively). The alert itself is subscription-global and its scopes are polymorphic.
- `AzureApplicationInsightsStandardWebTest` -- AzureApplicationInsights is a prerequisite because a standard web test binds to a referenced Application Insights component (the resource group chains transitively through the component).
- `AzureUserAssignedIdentity` -- AzureResourceGroup is a prerequisite because the identity is created inside a referenced resource group that must already exist.
- `AzureRoleAssignment` -- AzureResourceGroup and AzureUserAssignedIdentity are prerequisites because an assignment grants a role at a referenced scope (most commonly a resource group) to a referenced principal (most commonly a managed identity) -- both must exist before the grant can be written.
- `AzureRoleDefinition` -- AzureResourceGroup is a prerequisite because a custom role definition is created at a referenced scope, most commonly a resource group in composed environments -- the scope must exist before the definition can be written.
- `AzureFederatedIdentityCredential` -- AzureUserAssignedIdentity is the prerequisite because a federated identity credential is a child resource of a referenced managed identity -- the identity must exist before the credential can be written on it. (The resource group arrives transitively through the identity's own prerequisite declaration.)
- `AzureServiceBusNamespace` -- AzureResourceGroup is a prerequisite because a Service Bus namespace is created inside a referenced resource group in composed environments. The namespace is the container every Service Bus messaging entity (queue, topic, subscription, authorization rule, geo-DR pairing) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureEventHubNamespace` -- AzureResourceGroup is a prerequisite because an Event Hub namespace is created inside a referenced resource group in composed environments. The namespace is the container every Event Hubs entity (event hub, consumer group, authorization rule, schema group, geo-DR pairing, customer-managed key) nests under. The child kinds deliberately declare NO registry prerequisite: namespace names are globally unique with a post-delete name hold, so E2E composes them with scenario-local namespace fixtures instead of a shared recreate-per-scenario prerequisite.
- `AzureServiceBusQueue`
- `AzureServiceBusTopic`
- `AzureServiceBusSubscription`
- `AzureServiceBusAuthorizationRule`
- `AzureServiceBusDisasterRecoveryConfig`
- `AzureEventHub`
- `AzureEventHubConsumerGroup`
- `AzureEventHubAuthorizationRule`
- `AzureFrontDoorProfile` -- AzureResourceGroup is a prerequisite because a Front Door profile is created inside a referenced resource group in composed environments. The profile is the container every Front Door delivery resource (endpoint, origin group, origin, route) nests under.
- `AzureFrontDoorEndpoint` -- AzureFrontDoorProfile is a prerequisite because an endpoint is an ARM child of a referenced profile -- the profile must exist before the endpoint can be written. (The resource group arrives transitively through the profile's own prerequisite declaration.)
- `AzureFrontDoorOriginGroup` -- AzureFrontDoorProfile is a prerequisite because an origin group is an ARM child of a referenced profile.
- `AzureFrontDoorOrigin` -- AzureFrontDoorOriginGroup is a prerequisite because an origin is an ARM child of a referenced origin group (the profile and resource group chain transitively).
- `AzureFrontDoorRoute` -- A route attaches to an endpoint (its ARM parent) and forwards to an origin group whose origins must exist before ARM accepts the route -- so both the endpoint and the origin chain are genuine deploy-order prerequisites.
- `AzureFrontDoorRuleSet` -- AzureFrontDoorProfile is a prerequisite because a rule set is an ARM child of a referenced profile. The rules live inside the set (they form one ordered policy document); routes attach the set by ARM ID.
- `AzureFrontDoorCustomDomain` -- AzureFrontDoorProfile is a prerequisite because a custom domain is an ARM child of a referenced profile. The DNS zone and (for bring-your-own certificates) the Front Door secret are optional references, not deploy-order prerequisites.
- `AzureFrontDoorSecret` -- AzureFrontDoorSecret is a prerequisite-light kind: only the profile (its ARM parent) must exist. The Key Vault certificate it wraps is a reference resolved before the module runs; its vault chain is exercised through scenario-local fixtures in E2E.
- `AzureFrontDoorFirewallPolicy` -- AzureResourceGroup is a prerequisite because the Front Door WAF policy is created inside a referenced resource group -- it is a GLOBAL resource, not a profile child (a different ARM type than the regional Application Gateway WAF policy). Security policies attach it to profiles; the policy itself depends on nothing else.
- `AzureFrontDoorSecurityPolicy` -- A security policy is an ARM child of a profile that associates a referenced WAF policy with referenced domains -- so the endpoint (the default-domain association target; the profile arrives transitively through it) and the WAF policy are genuine deploy-order prerequisites.
- `AzureStorageContainer` -- None of the storage data-service kinds declares a registry prerequisite on AzureStorageAccount: account names are GLOBALLY unique and Azure holds a just-deleted name, so a recreate-per-scenario fixture would hang -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the storage_account_id reference itself.
- `AzureStorageShare`
- `AzureStorageQueue`
- `AzureStorageTable`
- `AzureStorageEncryptionScope`
- `AzureStorageDataLakeGen2Filesystem`
- `AzureStorageLocalUser`
- `AzureStorageObjectReplication`
- `AzureCosmosdbSqlDatabase` -- None of the Cosmos DB data-service kinds declares a registry prerequisite on AzureCosmosdbAccount: account names are GLOBALLY unique DNS labels, so a recreate-per-scenario fixture would risk name-reuse hangs -- their E2E scenarios declare scenario-local account fixtures instead. Deploy ordering in composed environments still flows from the cosmosdb_account_id / parent-database references themselves.
- `AzureCosmosdbSqlContainer`
- `AzureCosmosdbMongoDatabase`
- `AzureCosmosdbMongoCollection`
- `AzureCosmosdbSqlRoleDefinition`
- `AzureCosmosdbSqlRoleAssignment`
- `AzureManagedRedis` -- AzureResourceGroup is the cluster's only registry prerequisite: the cluster is created inside a referenced resource group. The geo-replication and access-policy-assignment children declare NO prerequisite on AzureManagedRedis: clusters are expensive, slow-provisioning parents, so their E2E scenarios declare scenario-local cluster fixtures instead of recreating a shared one per scenario. Deploy ordering in composed environments still flows from the managed_redis_id references themselves.
- `AzureManagedRedisGeoReplication`
- `AzureManagedRedisAccessPolicyAssignment`
- `AzureEventHubDisasterRecoveryConfig`
- `AzureEventHubSchemaGroup`
- `AzureEventHubCluster` -- AzureResourceGroup is a prerequisite because a dedicated Event Hubs cluster is created inside a referenced resource group in composed environments. Note: clusters cannot be deleted for 4 hours after creation (Azure's moratorium), so E2E treats this kind as offline-gated.
- `AzureEventHubNamespaceCustomerManagedKey`
- `AzureMssqlFailoverGroup` -- AzureMssqlServer is a prerequisite because a failover group is created on a referenced primary logical server and points at a partner server; the primary (and its resource group, which chains transitively) must exist before the group can be written.
- `AzureContainerAppCustomDomain` -- AzureContainerApp is a prerequisite because the domain binding lives in a referenced app's ingress configuration (the environment and resource group chain transitively through the app).
- `AzureFirewallPolicy`
- `AzureFirewallPolicyRuleCollectionGroup` -- AzureFirewallPolicy is a prerequisite because a rule collection group is a child document of a referenced policy (the resource group chains transitively through the policy).
- `AzureFirewall` -- AzureSubnet is a prerequisite because a VNet-deployed firewall's data path lives in a dedicated subnet that must be named exactly "AzureFirewallSubnet" (the virtual network and resource group chain transitively through the subnet). The E2E install profile publishes a fixture subnet with that exact name and a /26 prefix.
- `AzureIpGroup`
- `AzureVirtualNetworkGateway` -- AzureSubnet is a prerequisite because every virtual network gateway lives in a dedicated subnet named exactly "GatewaySubnet" (the virtual network and resource group chain transitively through the subnet); the subnet install profile publishes a fixture instance with that exact ARM name. AzurePublicIp is a prerequisite because a VPN-type gateway (the default shape) requires a public IP per ip configuration; the address install profile publishes a dedicated zone-redundant instance (a gateway binds its address exclusively, and the AZ gateway SKUs require zones on it).
- `AzureVirtualNetworkGatewayConnection` -- Both gateways are prerequisites: a connection joins a virtual network gateway to a far side, and the site-to-site far side is a local network gateway (the GatewaySubnet, VNet, and resource group chain transitively through the virtual network gateway).
- `AzureLocalNetworkGateway`
- `AzurePrivateLinkService` -- AzureSubnet is the sole prerequisite: every NAT ip configuration draws its address from a subnet with private-link-service network policies disabled (the subnet install profile publishes a fixture instance with that flag off). The Standard load balancer whose frontend the service typically fronts is NOT a registry prerequisite -- the spec's destination is an exactly-one-of (load balancer frontend OR fixed destination IP), so scenarios that use the load-balancer shape declare it via the planton.dev/e2e-prerequisites annotation instead.
- `AzureExpressRouteCircuit`
- `AzureExpressRouteCircuitPeering` -- The circuit is the prerequisite: a peering is an ARM child of the circuit, addressed by the circuit's name (the resource group chains transitively through the circuit).
- `AzureExpressRouteGateway` -- The hub is the prerequisite: ARM requires an ExpressRoute Gateway to be deployed INTO a Virtual WAN hub (the WAN and resource group chain transitively through the hub).
- `AzureExpressRoutePort` -- ExpressRoute Port: your own physical port pair on a Microsoft edge router (ExpressRoute Direct), from whose bandwidth circuits are carved. Self-contained -- only the resource group is required.
- `AzureVirtualWan` -- Virtual WAN: the umbrella of Azure's managed hub-and-spoke networking, under which virtual hubs and their gateways are created. Self-contained -- only the resource group is required.
- `AzureVirtualHub` -- The WAN is the prerequisite: this kind models the Virtual WAN hub (virtual_wan_id is required; standalone hubs are the legacy Route Server construction, which has its own ARM surface). The resource group chains transitively through the WAN.
- `AzureVirtualHubConnection` -- Both sides of the attachment are prerequisites: the hub being joined and the spoke virtual network being attached.
- `AzureVpnGateway` -- The hub is the prerequisite: ARM deploys a Virtual WAN VPN gateway INTO a virtual hub (virtual_hub_id is required and immutable; the WAN and resource group chain transitively through the hub). ARM allows one VPN gateway per hub.
- `AzureVpnGatewayConnection` -- Both ends of the tunnel are prerequisites: a connection is an ARM child of the VPN gateway and pins each of its links to a specific link of the remote VPN site (the hub, WAN, and resource group chain transitively through the gateway).
- `AzureVpnSite` -- The WAN is the prerequisite: a VPN site is the Virtual WAN world's address-book entry for one branch location (virtual_wan_id is required; the classic-world sibling without a WAN is AzureLocalNetworkGateway). The resource group chains transitively through the WAN.
- `AzurePointToSiteVpnGateway` -- The hub and the server configuration are both prerequisites: a point-to-site VPN gateway deploys INTO a virtual hub (one P2S gateway per hub, a slot separate from the hub's site-to-site VPN gateway) and is born pointing at the VPN server configuration that defines how its users authenticate -- both ARM-required and fixed at creation. The WAN and resource group chain transitively through the hub.
- `AzureVpnServerConfiguration` -- Self-contained -- only the resource group is required: a VPN server configuration is the reusable "who may connect and how" authentication policy (Entra ID / certificate / RADIUS) that point-to-site VPN gateways attach to; it references no other Azure resource.
- `AzureCognitiveAccount` -- Self-contained -- only the resource group is required: an Azure AI services account (Azure OpenAI, the multi-service AIServices account, the single-service accounts) needs no other Azure resource; subnets (network rules), Key Vault keys (CMK), storage accounts and user-assigned identities are optional references.
- `AzureCognitiveDeployment` -- An ARM child of its account: a model deployment (which model runs, at which throughput class) exists only on an Azure AI services account of kind "OpenAI" or "AIServices".
- `AzureCognitiveAccountProject` -- An ARM child of its account: an AI Foundry project exists only on an "AIServices"-kind account with project management enabled.
- `AzureMachineLearningWorkspace` -- The workspace REQUIRES all three companion services at creation (default storage, secrets vault, telemetry) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureMachineLearningDatastore` -- An ARM child of its workspace. The storage target (container, filesystem or share) is scenario-declared via the e2e-prerequisites annotation -- only the blob scenario needs a container, so it is not a kind-wide prerequisite.
- `AzureMachineLearningComputeCluster` -- An ARM child of its workspace (.../computes/{name}) -- the auto-scaling pool of VMs training jobs run on.
- `AzureMachineLearningComputeInstance` -- An ARM child of its workspace (.../computes/{name}) -- a single always-on VM serving as one data scientist's cloud workstation.
- `AzureAiFoundry` -- The hub REQUIRES both companion services at creation (secrets vault, default storage) -- genuine deploy-order prerequisites, each with its own fixture profile.
- `AzureAiFoundryProject` -- Deploys into its hub's resource group (the provider derives the group from the hub reference -- the project spec carries none).
- `AzureSearchService`
- `AzureMachineLearningOnlineEndpoint` -- An ARM child of its workspace (.../onlineEndpoints/{name}) -- the stable scoring address applications call. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningOnlineDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the running copy of a model the endpoint's traffic map routes to.
- `AzureMachineLearningBatchEndpoint` -- An ARM child of its workspace (.../batchEndpoints/{name}) -- the stable address batch scoring jobs are submitted to. azurerm carries no ML endpoint resources; the modules write the raw ARM shape at a pinned api-version (azapi / azure-native).
- `AzureMachineLearningBatchDeployment` -- An ARM child of its endpoint (.../deployments/{name}) -- the job recipe (model, compute, batching behavior) the endpoint's default-deployment pointer routes submissions to.
- `AzureRecoveryServicesVault` -- The Recovery Services vault (Microsoft.RecoveryServices/vaults) -- the safe that classic Azure Backup data and Site Recovery configuration live in. Backup policies and protected items are ARM children of a vault.
- `AzureBackupPolicyVm` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern IaaS VM backups.
- `AzureBackupProtectedVm` -- An ARM child of its vault (.../protectedItems/...) -- the binding that puts one virtual machine under a backup policy's protection.
- `AzureBackupPolicyFileShare` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules that govern Azure Files share backups (snapshot or vaulted).
- `AzureBackupProtectedFileShare` -- An ARM child of its vault (.../protectedItems/AzureFileShare;...) -- the binding that puts one Azure Files share under a backup policy's protection. The share's storage account must already be registered with the vault (AzureBackupContainerStorageAccount). Prerequisite ORDER is load-bearing for teardown (destroy runs in reverse): the registration must list AFTER the share so it unregisters FIRST -- Azure Backup holds a DoNotDelete lock on a registered storage account, and a share delete under that lock fails ScopeLocked.
- `AzureDataProtectionBackupVault` -- The Data Protection backup vault (Microsoft.DataProtection/ backupVaults) -- the safe that MODERN Azure Backup data lives in (managed disks, blob storage, AKS clusters, MySQL/PostgreSQL flexible servers, Data Lake storage). Backup policies and backup instances are ARM children of a vault.
- `AzureDataProtectionBackupPolicy` -- An ARM child of its vault (.../backupPolicies/{name}) -- the schedule and retention rules for ONE Data Protection datasource type (blob storage, disk, Kubernetes cluster, MySQL/PostgreSQL flexible server, or Data Lake storage), modeled as one kind with variant blocks.
- `AzureDataProtectionBackupInstance` -- An ARM child of its vault (.../backupInstances/{name}) -- the binding that puts ONE datasource (a managed disk, a storage account's blob services, an AKS cluster, a MySQL/PostgreSQL flexible server, or a Data Lake storage account) under a Data Protection backup policy, modeled as one kind with variant blocks. The vault's managed identity must hold the datasource roles Azure Backup requires BEFORE the instance is created.
- `AzureBastionHost` -- AzureSubnet and AzurePublicIp are prerequisites because a dedicated-infrastructure Bastion host (Basic/Standard/Premium -- the default shapes) deploys into a subnet named exactly "AzureBastionSubnet" and binds a Standard static public IP EXCLUSIVELY (the virtual network and resource group chain transitively through the subnet). The Developer SKU instead attaches to a virtual network directly and uses neither.
- `AzureNetworkWatcherFlowLog` -- AzureVirtualNetwork and AzureStorageAccount are prerequisites because a flow log records a network-scoped target (a virtual network in the common case; subnets and network interfaces chain through the network) into a referenced storage account. The regional Network Watcher parent is NOT a prerequisite: Azure auto-creates it ("NetworkWatcher_{region}" in "NetworkWatcherRG") the moment the region hosts a virtual network, and the flow log references it by name. Traffic Analytics' Log Analytics workspace is an optional arm, declared by scenarios that use it.
- `AzurePrivateDnsResolver` -- AzureVirtualNetwork and AzureSubnet are prerequisites because a DNS Private Resolver anchors to a referenced virtual network (at most ONE resolver per network -- Azure enforces it) and each of its inbound/outbound endpoints occupies its own dedicated subnet delegated to "Microsoft.Network/dnsResolvers" (the resource group chains transitively through the network and subnets).
- `AzurePrivateDnsResolverForwardingRuleset` -- AzurePrivateDnsResolver is a prerequisite because a DNS forwarding ruleset steers a resolver's OUTBOUND endpoints -- it binds their ARM ids (at most 2, same resolver) at creation. (The resource group and network chain transitively through the resolver's own prerequisite declarations.)
- `AzurePrivateDnsRecord` -- AzurePrivateDnsZone is a prerequisite because every record set is created inside a referenced private DNS zone (the resource group chains transitively through the zone's own prerequisite).
- `AzureTrafficManagerProfile` -- AzureResourceGroup is a prerequisite because a Traffic Manager profile is created inside a referenced resource group (the profile itself is a global service -- the group only holds its metadata record).
- `AzureTrafficManagerEndpoint` -- AzureTrafficManagerProfile is a prerequisite because every endpoint is created inside a referenced profile -- it is the destination a profile steers traffic to (the resource group chains transitively through the profile's own prerequisite).
- `AzureMonitorAutoscaleSetting` -- AzureResourceGroup is a prerequisite because an autoscale setting is created inside a referenced resource group. The scalable TARGET it controls is a no-default reference (many kinds can be scaled), so no target kind is declared here -- scenarios declare their own target fixture.
- `AzureMonitorDataCollectionRule` -- The Azure Monitor data collection rule (DCR) -- the routing table declaring what telemetry the Azure Monitor Agent collects and where it lands. AzureResourceGroup is a prerequisite because a rule is created inside a referenced resource group; AzureLogAnalyticsWorkspace because a workspace is the canonical destination a rule routes to (the smoke scenario's shape). Machines attach to a rule with AzureMonitorDataCollectionRuleAssociation resources.
- `AzureEventgridTopic` -- The Azure Event Grid custom topic -- the HTTPS endpoint an application publishes its own events to, fanned out to handlers by event subscriptions. One topic is one event stream with its own endpoint and access keys; for many streams behind one endpoint see AzureEventgridDomain.
- `AzureEventgridDomain` -- The Azure Event Grid domain -- ONE publishing endpoint and one pair of access keys serving many event streams (domain topics), the multi-tenant pattern. Topics inside the domain are auto-managed by Azure or declared explicitly as AzureEventgridDomainTopic resources.
- `AzureEventgridSystemTopic` -- The Azure Event Grid system topic -- the subscription surface for events AZURE ITSELF publishes about one of your resources (a storage account's blob events, a resource group's lifecycle events). One system topic per source resource per topic type; event subscriptions attach to it to route events to handlers.
- `AzureEventgridEventSubscription` -- The Azure Event Grid event subscription -- the delivery instruction routing events from a source (a custom topic, domain, domain topic, system topic, resource group, or subscription) to a handler (a Function, Event Hub, Service Bus queue/topic, storage queue, hybrid connection, or webhook), with filtering, retry, and dead-letter behavior.
- `AzureEventgridNamespace` -- The Azure Event Grid namespace -- the capacity-scaled hub of the newer Event Grid: hosts CloudEvents namespace topics and an optional MQTT broker behind one set of regional endpoints, sized in throughput units.
- `AzureDataFactory` -- The Azure Data Factory -- the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID.
- `AzureDataFactoryPipeline` -- One unit of work inside an Azure Data Factory ({factory_id}/pipelines/{name}) -- an ordered set of activities that executes as a whole when triggered.
- `AzureDataFactoryDataFlow` -- A Data Factory data flow ({factory_id}/dataflows/{name}) -- a visually-designed data transformation executed on managed Spark, or, as a flowlet, a reusable snippet other data flows embed. One kind covers both provider forms (they share one schema and one name namespace inside the factory).
- `AzureDataFactoryLinkedService` -- A Data Factory linked service ({factory_id}/linkedservices/{name}) -- a saved connection in the factory's address book: where an external system lives and how to authenticate to it. One kind covers every connection type Azure models as a first-class resource (storage, SQL family, Cosmos DB, Databricks, Key Vault, SFTP, web APIs, and more) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryDataset` -- A Data Factory dataset ({factory_id}/datasets/{name}) -- a named view of data inside a system a linked service already connects to: which container and path, which table, which file format. One kind covers every data shape Azure models as a first-class dataset resource (delimited text/CSV, JSON, Parquet, binary, blob, HTTP, the SQL family, Snowflake, Cosmos DB) as variants in one factory-scoped name namespace, plus the raw-JSON custom form.
- `AzureDataFactoryTrigger` -- A Data Factory trigger ({factory_id}/triggers/{name}) -- the instruction that starts pipelines automatically: on a clock schedule, per contiguous tumbling window, on storage blob events, or on Event Grid custom events. One kind covers all four provider trigger resources as variants (one ARM namespace, one started/stopped lifecycle).
- `AzureDataFactoryIntegrationRuntime` -- A Data Factory integration runtime ({factory_id}/integrationRuntimes/{name}) -- the compute engine a factory's pipelines, data flows, and copy activities run on. One kind covers all three engine flavors as variants in one factory-scoped name namespace: the managed data-flow compute, the managed SSIS package runtime, and the self-hosted agent registration (which issues the authorization keys agents join with).
- `AzureComputeGallery` -- The Azure Compute Gallery -- the shared library an organization keeps its approved VM images in. Image definitions (AzureComputeGalleryImage) live inside it; VMs and scale sets deploy from their published, region-replicated versions.
- `AzureComputeGalleryImage` -- A gallery image ({gallery_id}/images/{name}) -- one image definition inside a Compute Gallery (marketplace-style identity, OS type, security posture) plus its published versions, each replicated to its own target regions. VMs deploy from a version's ARM ID or from the definition's ID to get the latest version.
- `AzureAvailabilitySet` -- The availability set -- the classic pre-zones placement grouping that spreads VMs across separate fault and update domains so one hardware failure or maintenance window cannot take them all down. VMs join the set at creation.
- `AzureDiskSnapshot` -- The managed disk snapshot -- a point-in-time copy of a disk used for backup, cloning, and as the source of gallery image versions. Incremental snapshots store only the delta since the previous snapshot of the same disk.
- `AzureContainerInstance` -- The Azure Container Instance container group -- serverless containers billed per second: one or more containers sharing a lifecycle, network, and volumes (plus one-shot init containers), with no cluster or VM to manage. Public, subnet-private, or IP-less postures.
- `AzureFunctionAppFlexConsumption` -- The Azure Function App on the Flex Consumption plan -- Azure's newest serverless Functions hosting model: per-instance memory selection, a configurable scale-out ceiling, always-ready instance pools, and explicit blob-container deployment storage. Requires an FC1-SKU service plan, which is deliberately NOT a registry prerequisite: the shared plan fixture serves the classic app tiers, and an FC1 plan is cheap to create per scenario (no idle compute cost), so scenarios bring their own plan fixture -- the same reasoning that keeps the globally-unique storage account scenario-local for AzureFunctionApp.
- `AzureMongoCluster` -- The Azure Cosmos DB for MongoDB vCore cluster -- Azure's modern managed MongoDB: a real MongoDB engine on dedicated vCore tiers with sharding, zone-redundant HA, and point-in-time restore.
- `AzureFabricCapacity` -- The Microsoft Fabric capacity -- the billing and compute anchor of Microsoft Fabric: workspaces assign themselves to a capacity, and its F-SKU sets how much compute every workload on it shares. azurerm's entire Fabric surface is this one resource (workspaces and items live in Microsoft's dedicated fabric provider).
- `AzureBackupContainerStorageAccount` -- Registers a storage account with a Recovery Services vault as a backup container (.../protectionContainers/StorageContainer;...) -- one registration per storage-account-and-vault pair, required BEFORE any of the account's file shares can be protected. Part of the backup family (2175-2179) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureDataProtectionResourceGuard` -- The Data Protection Resource Guard (Microsoft.DataProtection/ resourceGuards) -- the approval gate behind Multi-User Authorization: privileged vault operations (disabling soft delete, reducing retention) require an approval through a guard, which typically lives in a DIFFERENT administrator's scope. Vaults reference a guard by its ARM ID. Part of the backup family (2175-2182) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePrivateDnsResolverVirtualNetworkLink` -- The attachment that makes a DNS forwarding ruleset take effect in one virtual network ({ruleset_id}/virtualNetworkLinks/{name}) -- one link per ruleset-network pair, up to 500 per ruleset, spokes joining and leaving independently (which is why the link is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). Part of the DNS Private Resolver family (2186-2187) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMonitorDataCollectionRuleAssociation` -- The attachment that puts ONE machine under an Azure Monitor data collection rule ({target_id}/providers/Microsoft.Insights/ dataCollectionRuleAssociations/{name}) -- an extension resource on the TARGET machine, many per rule, machines joining and leaving monitoring independently (which is why the association is a standalone kind, exactly like AzurePrivateDnsZoneVirtualNetworkLink). AzureVirtualMachine is a prerequisite because the smoke scenario attaches the fixture VM; the rule prerequisite chains the rule's own install manifest. Part of the Monitor family (2191-2192) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridDomainTopic` -- One named event stream inside an Azure Event Grid domain ({domain_id}/topics/{name}) -- the per-tenant mailbox of the multi-tenant pattern: many per domain, each with its own subscriptions and lifecycle, tenants joining and leaving without touching the domain (which is why the domain topic is a standalone kind, exactly like AzureEventHubConsumerGroup on a shared hub). Part of the Event Grid family (2193-2194) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureEventgridNamespaceTopic` -- One named CloudEvents stream inside an Azure Event Grid namespace ({namespace_id}/topics/{name}) -- many per namespace, publishers and teams creating and deleting their own against the shared namespace (which is why the topic is a standalone kind, exactly like AzureEventgridDomainTopic and AzureEventHubConsumerGroup). Part of the Event Grid family (2193-2197) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzureMongoClusterUser` -- Grants one Microsoft Entra principal access to an Azure Cosmos DB for MongoDB vCore cluster ({cluster_id}/users/{object_id}) -- an access binding, not a password user: many per cluster, principals joining and leaving independently (which is why the grant is a standalone kind, the access-grant class of AzureRoleAssignment). Part of the Mongo vCore family (2211) despite the out-of-run number -- enum numbers are pinned by the registry snapshot; never renumber.
- `AzurePlantonRunner` -- AzureContainerAppEnvironment is a prerequisite because the runner appliance is a Container App, and every Container App runs inside an environment -- the environment reference must resolve before the appliance can deploy.
- `GcpArtifactRegistryRepo` -- 3000–3999: GCP resources
- `GcpTargetHttpsProxy` -- The URL map is the parent a proxy cannot exist without; the classic compute certificate kinds and the SSL policy are the fixture parents the committed scenarios attach. The Certificate Manager certificate list (certificate_manager_certificates, honored only by the cross-region internal ALB) is optional composition -- a scenario that arms it declares GcpCertManagerCert via the e2e-prerequisites annotation, never a registry edge that would tax every proxy and forwarding-rule chain.
- `GcpCloudFunction`
- `GcpCloudRun`
- `GcpCloudSql`
- `GcpDnsZone`
- `GcpGcsBucket`
- `GcpGkeCluster`
- `GcpIamCustomRole`
- `GcpProject`
- `GcpVpcNetwork`
- `GcpSubnetwork`
- `GcpRouterNat`
- `GcpGkeNodePool`
- `GcpServiceAccount`
- `GcpGkeWorkloadIdentityBinding`
- `GcpCertManagerCert`
- `GcpComputeInstance`
- `GcpDnsRecord`
- `GcpProjectIamMember`
- `GcpFirewallRule`
- `GcpGlobalAddress`
- `GcpCloudArmorPolicy`
- `GcpHealthCheck`
- `GcpBackendBucket`
- `GcpBackendService`
- `GcpRegionNetworkEndpointGroup`
- `GcpUrlMap`
- `GcpManagedSslCertificate`
- `GcpTargetHttpProxy`
- `GcpAlloydbCluster`
- `GcpRedisInstance`
- `GcpFirestoreDatabase`
- `GcpSpannerInstance`
- `GcpSpannerDatabase`
- `GcpBigtableInstance`
- `GcpMemorystoreInstance`
- `GcpCloudSqlDatabase`
- `GcpCloudSqlUser`
- `GcpAlloydbInstance`
- `GcpAlloydbUser`
- `GcpSpannerBackupSchedule`
- `GcpBigtableTable`
- `GcpFirestoreBackupSchedule`
- `GcpFirestoreIndex`
- `GcpBigQueryDataset`
- `GcpDataprocCluster`
- `GcpDataprocAutoscalingPolicy`
- `GcpBigQueryTable`
- `GcpPubSubTopic`
- `GcpPubSubSubscription`
- `GcpCloudTasksQueue`
- `GcpCloudSchedulerJob`
- `GcpPubSubSchema`
- `GcpVertexAiNotebook`
- `GcpVertexAiEndpoint`
- `GcpVertexAiIndex`
- `GcpVertexAiIndexEndpoint` -- Vector Search IndexEndpoint — distinct from the online-prediction GcpVertexAiEndpoint (671); different GCP resources, different kinds.
- `GcpVertexAiDeployedIndex`
- `GcpCloudComposerEnvironment`
- `GcpCloudComposerUserWorkloadsSecret`
- `GcpCloudComposerUserWorkloadsConfigMap`
- `GcpKmsKeyRing`
- `GcpKmsKey`
- `GcpKmsKeyIamMember`
- `GcpFilestoreInstance`
- `GcpWorkloadIdentityPool` -- 3101–3109: IAM/identity family (overflow block; the 3000–3022 foundation/security sub-band is fully allocated)
- `GcpWorkloadIdentityPoolProvider`
- `GcpServiceAccountIamMember`
- `GcpGlobalForwardingRule` -- 3110–3119: networking/load-balancer family (overflow block; the 3023–3029 LB sub-band is fully allocated)
- `GcpSslPolicy`
- `GcpSslCertificate`
- `GcpServiceNetworkingConnection`
- `GcpAddress`
- `GcpServiceConnectionPolicy`
- `GcpCertManagerDnsAuthorization`
- `GcpCertificateMap` -- GcpCertManagerCert is a prerequisite because a map entry binds hostnames to EXISTING certificates — the canonical map references a certificate fixture's resource name.
- `GcpCloudRunJob` -- 3120–3129: GCP serverless overflow
- `GcpServerlessVpcConnector`
- `GcpComputeDisk` -- 3130–3139: GCP compute overflow (the 3000–3022 foundation sub-band that holds GcpComputeInstance is fully allocated)
- `GcpComputeMig` -- GcpVpcNetwork is a prerequisite because the canonical group runs its fleet on a dedicated custom-mode VPC — a managed instance group's template must attach every VM to a network, and the default VPC is never assumed.
- `GcpMonitoringNotificationChannel` -- 3140–3149: GCP observability & log routing
- `GcpMonitoringAlertPolicy` -- GcpMonitoringNotificationChannel is a prerequisite because the policy's canonical shape references a channel to notify — a policy without a delivery endpoint measures but never pages.
- `GcpMonitoringUptimeCheck`
- `GcpLoggingSink` -- GcpGcsBucket is a prerequisite because the canonical sink exports to a Cloud Storage bucket — the cheapest destination that proves the whole writer-identity grant flow.
- `GcpMonitoringDashboard`
- `GcpMonitoringSlo`
- `GcpLogBucket`
- `GcpLogMetric`
- `GcpSecretManagerSecret` -- 3150–3159: GCP security & identity GcpServiceAccount is a prerequisite because the canonical secret grants secretAccessor to a workload service account — the access story the kind exists to model.
- `GcpIdentityPlatformConfig`
- `GcpIdentityPlatformTenant` -- GcpIdentityPlatformConfig is a prerequisite because tenants exist only in projects whose Identity Platform config enables multi_tenant.allow_tenants — a tenant without the initialized, tenant-enabled project config cannot be created at all.
- `GcpIamOauthClient`
- `GcpIamDenyPolicy`
- `GcpCloudRunDomainMapping` -- 3160–3169: GCP serverless edge GcpCloudRun is a prerequisite because a domain mapping exists only to point a verified domain at a running Cloud Run service — the route it maps must already exist for the mapping to be created at all.
- `GcpWorkflow`
- `GcpEventarcTrigger` -- GcpCloudRun is a prerequisite because the canonical trigger routes a Pub/Sub messagePublished event to a Cloud Run service — the destination story the kind exists to model.
- `GcpEventarcMessageBus`
- `GcpPlantonRunner`
- `KubernetesNamespace` -- 4000–4999: Kubernetes resources, organized in family sub-bands (4030–4069 also hosts CNI/autoscaling/DR addons; 4130–4149 hosts analytics & ML; 4190–4199 reserved for growth) 4000–4029: Kubernetes building blocks (core API primitives)
- `KubernetesDeployment`
- `KubernetesStatefulSet`
- `KubernetesDaemonSet`
- `KubernetesJob`
- `KubernetesCronJob`
- `KubernetesService`
- `KubernetesSecret`
- `KubernetesManifest`
- `KubernetesHelmRelease`
- `KubernetesConfigMap`
- `KubernetesServiceAccount`
- `KubernetesRbac` -- Bundles the RBAC grant grain (Role/ClusterRole + its binding) into one component: "grant these permissions to these subjects in this scope".
- `KubernetesIngress`
- `KubernetesNetworkPolicy`
- `KubernetesPersistentVolumeClaim`
- `KubernetesStorageClass`
- `KubernetesResourceQuota` -- Manages the namespace-governance pair: the ResourceQuota plus an optional companion LimitRange (per-object defaults/bounds) — two API objects, one governance story.
- `KubernetesPriorityClass`
- `KubernetesPodDisruptionBudget`
- `KubernetesHorizontalPodAutoscaler`
- `KubernetesCertManager` -- 4030–4069: Kubernetes foundation addons (certs, DNS, secrets, ingress, Gateway API, mesh, CNI/autoscaling/DR)
- `KubernetesClusterIssuer` -- KubernetesCertManager is a prerequisite for the three cert-manager CR kinds below: ClusterIssuer/Issuer/Certificate are cert-manager custom resources — without the controller and its CRDs they cannot be applied.
- `KubernetesIssuer`
- `KubernetesCertificate`
- `KubernetesExternalDns`
- `KubernetesExternalSecretsOperator`
- `KubernetesClusterSecretStore` -- KubernetesExternalSecretsOperator is a prerequisite for the three external-secrets CR kinds below: ClusterSecretStore/SecretStore/ ExternalSecret are external-secrets custom resources — without the operator and its CRDs they cannot be applied.
- `KubernetesSecretStore`
- `KubernetesExternalSecret`
- `KubernetesIngressNginx`
- `KubernetesGatewayApiCrds`
- `KubernetesGatewayClass`
- `KubernetesGateway`
- `KubernetesListenerSet`
- `KubernetesHttpRoute`
- `KubernetesGrpcRoute`
- `KubernetesTcpRoute`
- `KubernetesUdpRoute`
- `KubernetesTlsRoute`
- `KubernetesReferenceGrant`
- `KubernetesBackendTlsPolicy`
- `KubernetesIstioBaseCrds`
- `KubernetesIstio`
- `KubernetesDestinationRule` -- Istio API components (mesh traffic policy, security, telemetry). The seven typed resources below (4053–4059) require the Istio CRDs on the cluster, provided by the lightweight CRDs-only KubernetesIstioBaseCrds (851) — NOT the full mesh KubernetesIstio (852).
- `KubernetesServiceEntry`
- `KubernetesPeerAuthentication`
- `KubernetesRequestAuthentication`
- `KubernetesAuthorizationPolicy`
- `KubernetesTelemetry`
- `KubernetesEnvoyFilter`
- `KubernetesMetricsServer`
- `KubernetesCilium`
- `KubernetesKeda`
- `KubernetesKarpenter`
- `KubernetesKarpenterNodePool`
- `KubernetesKarpenterEc2NodeClass`
- `KubernetesClusterAutoscaler`
- `KubernetesVelero`
- `KubernetesKubePrometheusStack` -- 4070–4089: Kubernetes observability
- `KubernetesGrafana`
- `KubernetesSignoz` -- KubernetesClickHouse is a prerequisite because SigNoz stores every trace, metric and log in ClickHouse and deploys none of its own — the telemetry store is composed, never bundled.
- `KubernetesLoki`
- `KubernetesTempo`
- `KubernetesOtelOperator` -- The operator's admission webhooks (failurePolicy Fail) are served with a cert-manager Certificate in the default posture — cert-manager must be running before the operator installs.
- `KubernetesOtelCollector`
- `KubernetesKyverno` -- 4080–4099: Kubernetes security, policy, and identity
- `KubernetesGatekeeper`
- `KubernetesKeycloak` -- Keycloak declarations compose the official Keycloak Operator (which reconciles the Keycloak CR this kind renders) and, on the recommended postgres vendor, a KubernetesPostgres database — both must resolve before the CR can converge.
- `KubernetesOpenBao`
- `KubernetesOpenFga` -- OpenFGA requires a datastore; the recommended arm composes a KubernetesPostgres database (the sandbox memory arm needs nothing, but the registry declares the shape real deployments require).
- `KubernetesKeycloakOperator`
- `KubernetesCloudNativePgOperator` -- 4100–4129: Kubernetes data platforms
- `KubernetesPostgres`
- `KubernetesValkey`
- `KubernetesPerconaMysqlOperator`
- `KubernetesMysql`
- `KubernetesPerconaMongoOperator`
- `KubernetesMongodb`
- `KubernetesStrimziKafkaOperator`
- `KubernetesKafka` -- container_kind: a Strimzi Kafka cluster is a place in the provider's own model — KafkaTopic and KafkaUser declarations BELONG to one cluster (the strimzi.io/cluster label) and are drawn inside its box. Clients that merely talk to the cluster (Connect, MirrorMaker2, UI, Karapace) carry containment_exempt on their bootstrap/trust references.
- `KubernetesKafkaTopic`
- `KubernetesKafkaUser`
- `KubernetesKafkaConnect` -- container_kind: a Connect cluster hosts the connectors deployed INTO it (KafkaConnector's strimzi.io/cluster label names its Connect cluster) — the same room shape as KubernetesKafka above.
- `KubernetesKafkaConnector`
- `KubernetesKafkaMirrorMaker2`
- `KubernetesKarapace`
- `KubernetesKafkaUi`
- `KubernetesOpenSearchOperator`
- `KubernetesOpenSearch`
- `KubernetesAltinityOperator`
- `KubernetesClickHouse`
- `KubernetesSolrOperator`
- `KubernetesSolr`
- `KubernetesNeo4j`
- `KubernetesSeaweedFs`
- `KubernetesQdrant`
- `KubernetesRabbitMqOperator` -- The RabbitMQ Cluster Operator's release manifest ships admission webhooks whose serving certificate is a cert-manager Certificate — cert-manager must be running before the operator installs.
- `KubernetesRabbitMq`
- `KubernetesCnpgBarmanCloudPlugin` -- The Barman Cloud plugin is CloudNativePG's object-store backup path, installed as its own Helm release beside the operator. It registers with the operator over CNPG-I and issues its TLS through cert-manager, so both are prerequisites: nothing to register with, or no certificates, and the release never becomes ready.
- `KubernetesAirflow` -- 4130–4149: Kubernetes analytics and ML KubernetesPostgres is a prerequisite because Airflow's metadata database composes a KubernetesPostgres by default (the spec's FK defaults resolve onto its outputs) and the migration Job needs the database reachable before the server components start.
- `KubernetesSparkOperator`
- `KubernetesKubeRayOperator`
- `KubernetesRayCluster` -- KubernetesKubeRayOperator is a prerequisite because this kind declares the RayCluster custom resource that only the operator's CRDs admit and only the operator reconciles into head and worker pods.
- `KubernetesFlinkOperator` -- KubernetesCertManager is a prerequisite because the Flink operator's chart, with its default-on admission webhook, renders cert-manager Issuer/Certificate resources and trusts the API server through cert-manager's CA injection — there is no self-signed fallback at the pinned chart, and the webhooks are fail-closed.
- `KubernetesFlinkDeployment` -- KubernetesFlinkOperator is a prerequisite because this kind declares the FlinkDeployment custom resource that only the operator's CRDs admit and only the operator reconciles into a running Flink cluster.
- `KubernetesJupyterHub` -- KubernetesPostgres is a prerequisite because JupyterHub's hub database composes a KubernetesPostgres in its external-database arm (the spec's FK defaults resolve onto its outputs) and the hub pod mounts that database's credential Secret before it can start.
- `KubernetesMlflow` -- KubernetesPostgres is a prerequisite because MLflow's backend store composes a KubernetesPostgres in its production arm (FK defaults onto its outputs; the module composes the connection URI from its credential Secret), and KubernetesSeaweedFs because the artifact store's S3-compatible arm FK-defaults onto the SeaweedFS endpoint and credential Secret.
- `KubernetesTrino` -- KubernetesPostgres is a prerequisite because Trino's postgres catalogs compose a KubernetesPostgres (the catalog host and credential FK-default onto its outputs), and the pods read that database's credential Secret to resolve catalog passwords from environment.
- `KubernetesSuperset` -- KubernetesPostgres is a prerequisite because Superset's REQUIRED metadata database composes a KubernetesPostgres (FK defaults onto its outputs; the module composes the environment Secret from its credential Secret), and KubernetesValkey because the cache/broker arm FK-defaults onto a KubernetesValkey's service and password Secret.
- `KubernetesArgocd` -- 4150–4169: Kubernetes GitOps and CI/CD
- `KubernetesArgoWorkflows`
- `KubernetesTektonOperator`
- `KubernetesTekton` -- KubernetesTektonOperator is a prerequisite because this kind declares the TektonConfig custom resource that only the operator's CRDs admit and only the operator reconciles into running components.
- `KubernetesGhaRunnerScaleSetController`
- `KubernetesGhaRunnerScaleSet` -- KubernetesGhaRunnerScaleSetController is a prerequisite because this kind renders an AutoscalingRunnerSet custom resource that only the controller's CRDs admit and only the controller reconciles into listener and runner pods.
- `KubernetesHarbor`
- `KubernetesJenkins`
- `KubernetesTemporal` -- 4170–4189: Kubernetes app platforms KubernetesPostgres is a prerequisite because the recommended (and E2E-proven) database composition backs Temporal's default and visibility stores with a CloudNativePG cluster.
- `KubernetesNats`
- `KubernetesLocust`
- `KubernetesPlantonRunner`
- `KubernetesPlantonOperator`
- `KubernetesPlantonPlatform` -- KubernetesPlantonOperator is a prerequisite because this kind declares the PlantonPlatform custom resource that only the operator's CRD admits and only the operator reconciles into a running platform.
- `DigitalOceanApp` -- 5000–5999: DigitalOcean resources
- `DigitalOceanBucket`
- `DigitalOceanContainerRegistry`
- `DigitalOceanDatabaseCluster`
- `DigitalOceanDnsZone`
- `DigitalOceanDroplet` -- No VPC prerequisite: the droplet spec's vpc reference is optional — an omitted vpc places the droplet in the region's default VPC.
- `DigitalOceanFirewall`
- `DigitalOceanFunction`
- `DigitalOceanKubernetesCluster` -- DigitalOceanVpc is a prerequisite because the cluster spec's vpc reference is required: the control plane and every node pool live inside one VPC, resolved to the DigitalOceanVpc's exported vpc_id output.
- `DigitalOceanKubernetesNodePool` -- DigitalOceanKubernetesCluster is a prerequisite because a node pool is API-addressed under its owning cluster: the spec's cluster reference is required and the pool cannot exist first.
- `DigitalOceanLoadBalancer` -- No registry prerequisite: the load balancer's vpc reference is optional (DigitalOcean places it in the region's default VPC when unset, and GLOBAL balancers take no VPC at all). Scenarios that exercise VPC placement declare it per-scenario via the e2e-prerequisites annotation.
- `DigitalOceanVolume`
- `DigitalOceanVpc`
- `DigitalOceanCertificate`
- `DigitalOceanDnsRecord` -- DigitalOceanDnsZone is a prerequisite because a record is API-addressed under its domain: the spec's domain reference is required, resolved to the DigitalOceanDnsZone's exported zone_name output.
- `DigitalOceanDatabaseUser` -- An additional user on a managed database cluster: the cluster reference is required, resolved to the DigitalOceanDatabaseCluster's exported cluster_id output.
- `DigitalOceanDatabaseDb` -- An additional logical database on a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseConnectionPool` -- A PgBouncer connection pool on a managed PostgreSQL cluster: the cluster reference is required.
- `DigitalOceanDatabaseFirewall` -- The inbound trusted-sources rule set of a managed database cluster: the cluster reference is required.
- `DigitalOceanDatabaseReplica` -- A read-only replica of a managed database cluster: the primary cluster reference is required.
- `DigitalOceanDatabaseKafkaTopic` -- A topic on a managed Kafka cluster with the full per-topic configuration block; the owning cluster reference is required.
- `DigitalOceanDatabaseKafkaSchema` -- One schema subject registered in a managed Kafka cluster's schema registry; the owning cluster reference is required.
- `DigitalOceanProject` -- The account-level organizational container; membership is carried on the project itself as resource URNs.
- `DigitalOceanSshKey` -- An SSH public key registered on the account, referenced by droplets and droplet autoscale pools at create time.
- `DigitalOceanMonitorAlert` -- An alert policy on DigitalOcean's built-in metrics for droplets, load balancers, and managed database clusters. All entity targeting is optional, so there is no registry prerequisite.
- `DigitalOceanUptimeCheck` -- An availability/latency probe on an external endpoint with composed alert rules; the target is outside the account, so there is no registry prerequisite.
- `DigitalOceanReservedIp` -- A static public IP address (IPv4 or IPv6) reserved in a region and optionally assigned to a droplet. The droplet attachment is an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanVpcPeering` -- A private-network peering connection between exactly two VPCs; both VPC references are required.
- `DigitalOceanSpacesKey` -- An access-key pair for Spaces object storage. Bucket grants are an optional composition seam, so there is no registry prerequisite.
- `DigitalOceanCdn` -- A CDN endpoint serving a Spaces bucket's content from the global edge: the origin reference is required, resolved to the DigitalOceanBucket's exported bucket_domain_name output.
- `DigitalOceanDropletAutoscalePool` -- A pool of identical droplets DigitalOcean keeps at a fixed size or scales on utilization. The template's ssh_keys reference is required (the API mandates SSH keys), resolved to the DigitalOceanSshKey's exported ssh_key_id output.
- `CloudflareDnsZone` -- 7000–7999: Cloudflare resources
- `CloudflareKvNamespace`
- `CloudflareR2Bucket`
- `CloudflareWorker`
- `CloudflareLoadBalancer` -- CloudflareDnsZone and CloudflareLoadBalancerPool are prerequisites because a load balancer is a DNS-level construct inside a zone (the spec's zone_id reference must resolve) and traffic must land somewhere (the required fallback_pool reference must resolve to a live pool).
- `CloudflareD1Database`
- `CloudflareZeroTrustAccessApplication`
- `CloudflareDnsRecord` -- CloudflareDnsZone is a prerequisite because every record lives inside a zone -- the spec's zone_id reference must resolve before the record can be created.
- `CloudflareRuleset` -- CloudflareDnsZone is a prerequisite because zone-scoped rulesets (the common case; the spec's zone_id reference defaults to the zone kind) must resolve their zone first. Account-scoped rulesets simply leave the reference unused.
- `CloudflareWorkersKvPair` -- CloudflareKvNamespace is a prerequisite because a KV pair is written into a namespace -- the spec's namespace_id reference must resolve first.
- `CloudflareHyperdriveConfig`
- `CloudflareLoadBalancerPool`
- `CloudflareLoadBalancerMonitor`
- `CloudflareZeroTrustAccessPolicy`
- `CloudflareZeroTrustAccessGroup`
- `CloudflareQueue`
- `CloudflarePagesProject`
- `CloudflareZeroTrustTunnel`
- `CloudflareZeroTrustTunnelVirtualNetwork`
- `CloudflareZeroTrustTunnelRoute` -- CloudflareZeroTrustTunnel is a prerequisite because a route steers a CIDR through an existing tunnel -- the spec's tunnel_id reference must resolve first. (The optional virtual_network_id is scenario-declared, not a registry prerequisite.)
- `CloudflareList`
- `CloudflareListItem` -- CloudflareList is a prerequisite because an item exists only inside a list -- the spec's list_id reference must resolve first.
- `CloudflareTurnstileWidget`
- `CloudflareEmailRoutingZone` -- CloudflareDnsZone is a prerequisite because email routing is enabled ON a zone -- the spec's zone_id reference must resolve first.
- `CloudflareEmailRoutingRule` -- CloudflareDnsZone is a prerequisite because a routing rule lives in a zone's email routing configuration -- the spec's zone_id reference must resolve first. CloudflareEmailRoutingZone is deliberately NOT a prerequisite: the API accepts rule creation on a zone whose Email Routing was never enabled (measured live 2026-08-26 -- POST zones/{id}/email/routing/rules answered 200 on a fresh zone with routing status "unconfigured"). Enablement gates mail FLOW, not rule lifecycle, so it is an operational concern the kind docs teach, not a create-time dependency. (Forward destinations reference CloudflareEmailRoutingAddress only for forward-type rules, so that edge is scenario-declared.)
- `CloudflareEmailRoutingAddress`
- `CloudflareOriginCaCertificate`
- `CloudflareCertificatePack` -- CloudflareDnsZone is a prerequisite because a certificate pack is ordered for a zone's hostnames -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostname` -- CloudflareDnsZone is a prerequisite because a custom hostname (SSL for SaaS) is provisioned inside a zone -- the spec's zone_id reference must resolve first.
- `CloudflareCustomHostnameFallbackOrigin` -- CloudflareDnsZone is a prerequisite because the fallback origin is a zone-level SSL-for-SaaS setting -- the spec's zone_id reference must resolve first.
- `CloudflareZeroTrustAccessIdentityProvider` -- No prerequisites: identity providers are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessServiceToken` -- No prerequisites: service tokens are account-scoped in the canonical case (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustOrganization` -- No prerequisites: the organization is an account-scoped configuration singleton (the optional zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareZeroTrustAccessInfrastructureTarget` -- No prerequisites: targets are account-scoped, and the virtual-network reference is an optional per-manifest edge (omitted = the account's default virtual network).
- `CloudflareZeroTrustMcpPortal` -- No prerequisites: portals are account-scoped, and the servers[] rows' MCP-server references are optional per-manifest edges.
- `CloudflareZeroTrustMcpServer` -- No prerequisites: MCP server registrations are account-scoped and self-contained -- portals reference them, not the reverse.
- `CloudflareZeroTrustGatewayPolicy` -- No prerequisites: Gateway policies are account-scoped, and their list / virtual-network references are optional per-manifest edges.
- `CloudflareZeroTrustList` -- No prerequisites: Zero Trust lists are account-scoped and self-contained.
- `CloudflareZeroTrustGatewaySettings` -- No prerequisites: the Gateway configuration is an account-scoped singleton, and its certificate reference is an optional per-manifest edge.
- `CloudflareZeroTrustDnsLocation` -- No prerequisites: DNS locations are account-scoped and self-contained.
- `CloudflareZeroTrustDeviceDefaultProfile` -- No prerequisites: the default device profile is an account-scoped configuration singleton; its virtual-network and zone-certificate references are optional per-manifest edges.
- `CloudflareZeroTrustDeviceCustomProfile` -- No prerequisites: custom device profiles are account-scoped, and the virtual-network reference is an optional per-manifest edge.
- `CloudflareZeroTrustDevicePostureRule` -- No prerequisites: posture rules are account-scoped and self-contained (list and integration references are literal UUIDs today).
- `CloudflareZoneTlsSettings` -- CloudflareDnsZone is a prerequisite because TLS settings are zone-scoped configuration -- the spec's zone_id reference must resolve first.
- `CloudflareCustomSslCertificate` -- CloudflareDnsZone is a prerequisite because a custom certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareMtlsCertificate` -- No prerequisites: mTLS certificates are account-scoped uploads and self-contained -- consumers (zone TLS CA associations, Authenticated Origin Pulls rows, Workers mTLS bindings) reference them, not the reverse.
- `CloudflareAuthenticatedOriginPulls` -- CloudflareDnsZone is a prerequisite because Authenticated Origin Pulls enablement configures an existing zone -- the spec's zone_id reference must resolve first. The per-hostname certificate edge is optional and scenario-declared, never a registry prerequisite.
- `CloudflareAuthenticatedOriginPullsCertificate` -- CloudflareDnsZone is a prerequisite because the client certificate is uploaded to an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareZoneSettings` -- CloudflareDnsZone is a prerequisite because zone settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareCacheSettings` -- CloudflareDnsZone is a prerequisite because cache settings configure an existing zone -- the spec's zone_id reference must resolve first.
- `CloudflareIpAccessRule` -- No prerequisites: IP Access rules are account-scoped in the canonical case (the zone scope is a per-manifest choice, not a structural dependency).
- `CloudflareBotManagement` -- CloudflareDnsZone is a prerequisite because Bot Management is zone-singleton configuration -- the spec's zone_id reference must resolve first.
- `CloudflareSnippet` -- CloudflareDnsZone is a prerequisite because snippets deploy to a zone -- the spec's zone_id reference must resolve first.
- `CloudflareSnippetRules` -- CloudflareDnsZone and CloudflareSnippet are prerequisites: the rules table is zone-scoped and every rule invokes a snippet by name.
- `CloudflareWaitingRoom` -- CloudflareDnsZone is a prerequisite because waiting rooms sit on a zone's host+path -- the spec's zone_id reference must resolve first.
- `CloudflareWaitingRoomEvent` -- CloudflareWaitingRoom is a prerequisite because events run on a room (and the room's own chain brings the zone).
- `CloudflareLogpushJob` -- No prerequisites: logpush jobs are dual-scope (account or zone) and the zone reference is optional -- zone-scoped lanes declare the edge at the scenario level.
- `CloudflareNotificationPolicy` -- No prerequisites: every delivery mechanism (email, PagerDuty, webhook) is optional -- policies referencing a webhook declare the edge at the scenario level.
- `CloudflareNotificationWebhook` -- No prerequisites: a webhook destination is account-scoped and self-contained -- notification policies reference it, not the reverse.
- `CloudflareWebAnalyticsSite` -- No prerequisites: a site is identified by host OR zone, and the zone reference is optional -- zone-measured lanes declare the edge at the scenario level.
- `CloudflareWorkflow` -- CloudflareWorker is a prerequisite because a workflow registers a class exported by a DEPLOYED Worker script -- the spec's script_name reference must resolve first.
- `CloudflareSecretsStore` -- No prerequisites: the Secrets Store is an account-scoped container and self-contained -- consumers (store secrets, Worker bindings, AI Gateway authentication) reference it, not the reverse.
- `CloudflareSecretsStoreSecret` -- CloudflareSecretsStore is a prerequisite because every secret lives inside a store -- the spec's store_id reference must resolve first.
- `CloudflareAiGateway` -- No prerequisites: the gateway is account-scoped and self-contained; its optional Secrets Store link (BYO provider keys) is a scenario-level composition, not a structural requirement.
- `CloudflareAccountApiToken` -- No prerequisites: an account-owned API token is self-contained; the resources its policies cover are identifiers, not references.
- `CloudflareHealthcheck` -- CloudflareDnsZone is a prerequisite because standalone health checks are zone-scoped -- the spec's zone_id reference must resolve first.
- `Auth0Connection` -- 8000–8999: Auth0 resources
- `Auth0Client`
- `Auth0EventStream`
- `Auth0ResourceServer`
- `Auth0Action`
- `Auth0Role`
- `OpenFgaStore` -- 9000–9999: OpenFGA resources Note: OpenFGA is Terraform-only - there is no Pulumi provider available. Pulumi modules for OpenFGA resources are pass-through placeholders.
- `OpenFgaAuthorizationModel`
- `OpenFgaRelationshipTuple`

### spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.env

`string`

### spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.secrets[].valueFrom.fieldPath

`string`

### spec.jobTemplate.pod.initContainers[].env.envFrom

`[]EnvFromSource`

Bulk import of environment variables from ConfigMaps or Secrets.

### spec.jobTemplate.pod.initContainers[].env.envFrom[].prefix

`string`

Optional prefix prepended to each imported key name.
For example, prefix "APP_" with key "PORT" produces env var "APP_PORT".

### spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef

`ConfigMapRef`

Import all keys from a ConfigMap.

### spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef.name

`string` · required

Name of the ConfigMap.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.envFrom[].configMapRef.optional

`bool`

If true, the ConfigMap is allowed to not exist without blocking pod startup.

### spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef

`SecretRef`

Import all keys from a Secret.

### spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef.name

`string` · required

Name of the Secret.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].env.envFrom[].secretRef.optional

`bool`

If true, the Secret is allowed to not exist without blocking pod startup.

### spec.jobTemplate.pod.initContainers[].resources

`ContainerResources`

CPU and memory requests and limits. Requests drive scheduling and are what the
pod is guaranteed; limits are the ceiling enforced at runtime (CPU is throttled,
memory overage is OOM-killed). Omitting limits entirely leaves the container
unbounded — acceptable for batch work on dedicated nodes, risky on shared ones.

### spec.jobTemplate.pod.initContainers[].resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.jobTemplate.pod.initContainers[].resources.limits.cpu

`string`

### spec.jobTemplate.pod.initContainers[].resources.limits.memory

`string`

### spec.jobTemplate.pod.initContainers[].resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.jobTemplate.pod.initContainers[].resources.requests.cpu

`string`

### spec.jobTemplate.pod.initContainers[].resources.requests.memory

`string`

### spec.jobTemplate.pod.initContainers[].livenessProbe

`Probe`

Liveness probe: restarts the container when it fails. Detects deadlocked or
wedged processes. Keep it strictly about "is the process alive" — checking
downstream dependencies here turns a dependency blip into a restart storm.

### spec.jobTemplate.pod.initContainers[].livenessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.pod.initContainers[].livenessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].livenessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].livenessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].livenessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.portName

`string`

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.pod.initContainers[].livenessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.pod.initContainers[].livenessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.pod.initContainers[].livenessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.pod.initContainers[].livenessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.pod.initContainers[].livenessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.pod.initContainers[].livenessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.pod.initContainers[].livenessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.pod.initContainers[].readinessProbe

`Probe`

Readiness probe: removes the pod from Service endpoints while it fails. This is
the probe that makes rolling updates zero-downtime — traffic only reaches pods
that report ready.

### spec.jobTemplate.pod.initContainers[].readinessProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.pod.initContainers[].readinessProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].readinessProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].readinessProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].readinessProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.portName

`string`

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.pod.initContainers[].readinessProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.pod.initContainers[].readinessProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.pod.initContainers[].readinessProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.pod.initContainers[].readinessProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.portName

`string`

### spec.jobTemplate.pod.initContainers[].readinessProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.pod.initContainers[].readinessProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.pod.initContainers[].readinessProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.pod.initContainers[].startupProbe

`Probe`

Startup probe: holds off liveness and readiness checking until the app has
started, so slow-booting applications are not killed mid-initialization. Size
`failure_threshold × period_seconds` to the worst-case startup time.

### spec.jobTemplate.pod.initContainers[].startupProbe.initialDelaySeconds

`int32`

Number of seconds after the container has started before liveness or readiness probes are initiated.
Defaults to 0 seconds. Minimum value is 0.

### spec.jobTemplate.pod.initContainers[].startupProbe.periodSeconds

`int32`

How often (in seconds) to perform the probe.
Default to 10 seconds. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].startupProbe.timeoutSeconds

`int32`

Number of seconds after which the probe times out.
Defaults to 1 second. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].startupProbe.successThreshold

`int32`

Minimum consecutive successes for the probe to be considered successful after having failed.
Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].startupProbe.failureThreshold

`int32`

Minimum consecutive failures for the probe to be considered failed after having succeeded.
Defaults to 3. Minimum value is 1.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet

`HTTPGetAction`

HTTPGet specifies the http request to perform.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.portName

`string`

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.pod.initContainers[].startupProbe.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.pod.initContainers[].startupProbe.grpc

`GRPCAction`

GRPC specifies an action involving a GRPC port.

### spec.jobTemplate.pod.initContainers[].startupProbe.grpc.port

`int32`

Port number of the gRPC service.
Number must be in the range 1 to 65535.

- rule: {"int32":{"lte":65535,"gte":1}}

### spec.jobTemplate.pod.initContainers[].startupProbe.grpc.service

`string`

Service is the name of the service to check.
If not specified, the default behavior defined by gRPC is used.
For standard gRPC health checks, leave empty to check overall server health.

### spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket

`TCPSocketAction`

TCPSocket specifies an action involving a TCP port.

### spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.portName

`string`

### spec.jobTemplate.pod.initContainers[].startupProbe.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.pod.initContainers[].startupProbe.exec

`ExecAction`

Exec specifies a command to execute inside the container.

### spec.jobTemplate.pod.initContainers[].startupProbe.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.pod.initContainers[].volumeMounts

`[]VolumeMount`

Volume mounts for this container. Each entry both declares the mount path and
carries its volume source (ConfigMap, Secret, HostPath, EmptyDir, or PVC); the
module derives the pod-level volume list from the union of all containers'
mounts, de-duplicating by name — so two containers sharing an EmptyDir simply
declare the same mount name and source.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].name

`string` · required

Name of the volume mount. Must be unique within the container.
Used to correlate with the volume definition.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].mountPath

`string` · required

Path within the container at which the volume should be mounted.
Must be an absolute path.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].readOnly

`bool`

Whether the volume should be mounted read-only.
Default is false.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].subPath

`string`

Path within the volume from which the container's volume should be mounted.
Defaults to "" (volume's root).
Useful for mounting a subdirectory of a volume.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap

`ConfigMapVolumeSource`

ConfigMap volume source.
Use this to mount a ConfigMap as a file or directory.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.name

`string` · required

Name of the ConfigMap to mount.
Can reference a ConfigMap defined in spec.config_maps or an existing one in the namespace.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.key

`string`

Specific key from the ConfigMap to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.
Example: key="config" path="app.yaml" mounts the "config" key as "app.yaml"

### spec.jobTemplate.pod.initContainers[].volumeMounts[].configMap.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.
Use 0755 (493 in decimal) for executable scripts.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].secret

`SecretVolumeSource`

Secret volume source.
Use this to mount a Secret as a file or directory.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.name

`string` · required

Name of the Secret to mount.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.key

`string`

Specific key from the Secret to mount as a single file.
If not specified, all keys are mounted as files in the directory.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.path

`string`

If key is specified, this is the filename to use for the mounted file.
Defaults to the key name if not specified.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].secret.defaultMode

`int32`

Mode bits to use on created files. Must be a value between 0 and 0777.
Defaults to 0644.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath

`HostPathVolumeSource`

HostPath volume source.
Use this to mount a file or directory from the host node's filesystem.
Common for DaemonSets that need access to node-level resources.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath.path

`string` · required

Path on the host to mount.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].hostPath.type

`string`

Type of the host path.
Valid values:
  "" - Empty string (default) means no check is performed before mounting
  "DirectoryOrCreate" - Create directory if it doesn't exist
  "Directory" - Directory must exist
  "FileOrCreate" - Create file if it doesn't exist
  "File" - File must exist
  "Socket" - UNIX socket must exist
  "CharDevice" - Character device must exist
  "BlockDevice" - Block device must exist

- rule: Type must be one of: "", "DirectoryOrCreate", "Directory", "FileOrCreate", "File", "Socket", "CharDevice", "BlockDevice"

### spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir

`EmptyDirVolumeSource`

EmptyDir volume source.
Use this for temporary storage that is erased when the pod is removed.
Useful for scratch space, caching, or sharing data between containers.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir.medium

`string`

Medium for the empty directory.
"" (default) uses the node's default medium (typically disk).
"Memory" uses a tmpfs (RAM-backed filesystem).

Memory-backed volumes are faster but:
- Count against container memory limits
- Are lost on node restart
- Should have sizeLimit set to prevent OOM

- rule: Medium must be either "" or "Memory"

### spec.jobTemplate.pod.initContainers[].volumeMounts[].emptyDir.sizeLimit

`string`

Size limit for the empty directory.
Format: Kubernetes quantity (e.g., "1Gi", "500Mi").
Only strictly enforced when medium is "Memory".
For disk-backed volumes, this is a best-effort limit.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc

`PvcVolumeSource`

PersistentVolumeClaim volume source.
Use this to mount an existing PVC.
For StatefulSets, this can reference a volumeClaimTemplate.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc.claimName

`string` · required

Name of the PersistentVolumeClaim to mount.
For StatefulSets, this can be the name of a volumeClaimTemplate.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].pvc.readOnly

`bool`

Whether the PVC should be mounted read-only.
Default is false.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken

`ServiceAccountTokenVolumeSource`

Projected ServiceAccount token volume source.
Use this to mount a short-lived, audience-bound identity token that the
kubelet issues for the pod's ServiceAccount and rotates automatically.

### spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.audience

`string` · required

Intended audience of the token. The receiving service must identify
itself with this audience when verifying the token; a token minted for a
different audience is rejected. Required: an audience-less token would be
replayable against any service in the cluster.

- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.expirationSeconds

`int64`

Requested lifetime of the token in seconds. The kubelet starts rotating
the token when it passes 80% of its lifetime or 24 hours, whichever is
shorter. Defaults to 3600 (1 hour). The Kubernetes API enforces a
minimum of 600 (10 minutes).

- rule: Expiration must be at least 600 seconds (the Kubernetes API minimum)

### spec.jobTemplate.pod.initContainers[].volumeMounts[].serviceAccountToken.path

`string`

Filename for the token relative to the mount path.
Defaults to "token".

### spec.jobTemplate.pod.initContainers[].lifecycle

`WorkloadContainerLifecycle`

Lifecycle hooks. `post_start` runs immediately after the container starts (the
container is not Running until it completes); `pre_stop` runs before termination
and is the standard lever for connection draining — e.g. a short sleep that keeps
the endpoint serving while load balancers converge on the terminating state.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart

`WorkloadLifecycleHandler`

Runs immediately after the container is created. The container does not reach
Running until the hook completes; a failing post_start kills the container per
its restart policy.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.portName

`string`

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.portName

`string`

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.pod.initContainers[].lifecycle.postStart.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop

`WorkloadLifecycleHandler`

Runs before the container is terminated by the kubelet (pod deletion, rolling
update, eviction). The termination grace period starts BEFORE the hook runs, so
keep `pod.termination_grace_period_seconds` larger than the hook's worst-case
duration. The classic zero-downtime pattern is a short sleep here so the pod
keeps serving while endpoint removal propagates.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.exec

`ExecAction`

Execute a command inside the container; non-zero exit fails the hook.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.exec.command

`[]string`

Command is the command line to execute inside the container.
The command is run in the container's root filesystem.
The command's exit status is used to determine the health:
- 0: Success
- Non-zero: Failure

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet

`HTTPGetAction`

Perform an HTTP GET against the container.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.path

`string`

Path to access on the HTTP server.
Defaults to '/'.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.portName

`string`

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.host

`string`

Host name to connect to, defaults to the pod IP.
You probably want to set "Host" in http_headers instead.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.scheme

`string`

Scheme to use for connecting to the host (HTTP or HTTPS).
Defaults to HTTP.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders

`[]HTTPHeader`

Custom headers to set in the request.
HTTP allows repeated headers.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders[].name

`string`

The header field name.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.httpGet.httpHeaders[].value

`string`

The header field value.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket

`TCPSocketAction`

Open a TCP connection against the container.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.portNumber

`int32`

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.portName

`string`

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.tcpSocket.host

`string`

Host name to connect to, defaults to the pod IP.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.sleep

`SleepAction`

Pause for a fixed number of seconds — the idiomatic pre_stop drain hook,
with no shell or sleep binary required in the image.

### spec.jobTemplate.pod.initContainers[].lifecycle.preStop.sleep.seconds

`int64`

Seconds to sleep.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.pod.initContainers[].securityContext

`WorkloadContainerSecurityContext`

Container-level security hardening. Settings here override the pod-level
security context for this container only.

### spec.jobTemplate.pod.initContainers[].securityContext.privileged

`bool`

Runs the container with full host access — equivalent to root on the node.
Required by some node-level agents (device managers, network plugins). Never
combine with untrusted images.

### spec.jobTemplate.pod.initContainers[].securityContext.runAsUser

`int64` · optional (explicit presence)

UID the container process runs as. Overrides the image's USER directive.

### spec.jobTemplate.pod.initContainers[].securityContext.runAsGroup

`int64` · optional (explicit presence)

Primary GID the container process runs as.

### spec.jobTemplate.pod.initContainers[].securityContext.runAsNonRoot

`bool` · optional (explicit presence)

Refuses to start the container if its effective user is root. The standard
baseline hardening — it catches images that silently default to UID 0.

### spec.jobTemplate.pod.initContainers[].securityContext.readOnlyRootFilesystem

`bool` · optional (explicit presence)

Mounts the container's root filesystem read-only. Pair with EmptyDir mounts for
paths the app must write (e.g. /tmp).

### spec.jobTemplate.pod.initContainers[].securityContext.allowPrivilegeEscalation

`bool` · optional (explicit presence)

Whether the process can gain more privileges than its parent (setuid binaries,
file capabilities). The restricted Pod Security Standard requires this to be
false. Always true when `privileged` is set, so leave it unset in that case.

### spec.jobTemplate.pod.initContainers[].securityContext.capabilities

`WorkloadCapabilities`

Linux capabilities to add or drop. The restricted profile drops ALL and adds
back only NET_BIND_SERVICE when needed. Capability names are uppercase without
the CAP_ prefix (e.g. "NET_ADMIN", "SYS_TIME").

### spec.jobTemplate.pod.initContainers[].securityContext.capabilities.add

`[]string`

Capabilities to add (e.g. "NET_BIND_SERVICE").

### spec.jobTemplate.pod.initContainers[].securityContext.capabilities.drop

`[]string`

Capabilities to drop. Use ["ALL"] as the hardened baseline.

### spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile

`WorkloadSeccompProfile`

Seccomp syscall filter for the container. "RuntimeDefault" is the hardened
baseline; "Localhost" selects a node-local profile file via `localhost_profile`.

- rule: localhost_profile is required when type is "Localhost" and must be empty otherwise

### spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile.type

`string` · required

Profile type: "RuntimeDefault" (the container runtime's default filter — the
recommended baseline), "Unconfined" (no filtering), or "Localhost" (a profile
file installed on the node, named via localhost_profile).

- rule: Seccomp profile type must be one of "RuntimeDefault", "Unconfined", or "Localhost"
- rule: {"required":true}

### spec.jobTemplate.pod.initContainers[].securityContext.seccompProfile.localhostProfile

`string`

Path of the profile file relative to the node's seccomp profile root. Required
when (and only meaningful when) type is "Localhost".

### spec.jobTemplate.pod.labels

`map<string, string>`

Extra labels stamped on the POD TEMPLATE (and therefore every pod), merged with
the workload's own selector and governance labels. This is where cross-cutting
markers pods must carry go — e.g. `azure.workload.identity/use: "true"` for AKS
workload identity, or mesh sidecar-injection toggles. Keys here must not collide
with the selector labels the module derives.

### spec.jobTemplate.pod.annotations

`map<string, string>`

Extra annotations stamped on the pod template — e.g. prometheus.io scrape hints
or mesh proxy tuning. Changing pod-template annotations rolls pods, which is
also the idiomatic config-reload lever.

### spec.jobTemplate.pod.scheduling

`WorkloadScheduling`

Where pods may schedule: node selection, taint tolerations, affinity rules, and
zone/host spreading. Omit to let the scheduler place pods anywhere.

### spec.jobTemplate.pod.scheduling.nodeSelector

`map<string, string>`

Simple hard node filter: every listed label must match the node. The right tool
for "run on the GPU pool" — reach for node_affinity only when you need operators
(In/NotIn/Exists) or soft preferences.

### spec.jobTemplate.pod.scheduling.tolerations

`[]WorkloadToleration`

Taint tolerations. A toleration does not attract pods to tainted nodes — it only
permits scheduling there; pair with node_selector or affinity to target them.

### spec.jobTemplate.pod.scheduling.tolerations[].key

`string`

Taint key to tolerate. Empty key with operator "Exists" tolerates every taint.

### spec.jobTemplate.pod.scheduling.tolerations[].operator

`string`

How key/value match: "Equal" (default — value must match too) or "Exists"
(key presence alone matches).

- rule: Toleration operator must be either "Equal" or "Exists"

### spec.jobTemplate.pod.scheduling.tolerations[].value

`string`

Taint value to match when operator is "Equal".

### spec.jobTemplate.pod.scheduling.tolerations[].effect

`string`

Which taint effect is tolerated: "NoSchedule", "PreferNoSchedule", or
"NoExecute". Empty tolerates all effects for the key.

- rule: Toleration effect must be one of "NoSchedule", "PreferNoSchedule", or "NoExecute"

### spec.jobTemplate.pod.scheduling.tolerations[].tolerationSeconds

`int64` · optional (explicit presence)

For "NoExecute" taints only: how many seconds already-running pods stay bound
after the taint appears. Unset means tolerate forever.

### spec.jobTemplate.pod.scheduling.nodeAffinity

`WorkloadNodeAffinity`

Expressive node selection: hard requirements and weighted soft preferences over
node labels.

### spec.jobTemplate.pod.scheduling.nodeAffinity.required

`[]WorkloadNodeSelectorTerm`

Hard requirement. The outer list ORs its terms; expressions within one term AND.

### spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions

`[]WorkloadNodeSelectorRequirement` · required

- rule: {"repeated":{"minItems":"1"}}
- rule: In/NotIn require at least one value, Gt/Lt exactly one, Exists/DoesNotExist none

### spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].key

`string` · required

Node label key, e.g. "topology.kubernetes.io/zone".

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].operator

`string` · required

Operator: "In"/"NotIn" (value set), "Exists"/"DoesNotExist" (key presence), or
"Gt"/"Lt" (single integer value, as strings — the Kubernetes API convention).

- rule: Operator must be one of "In", "NotIn", "Exists", "DoesNotExist", "Gt", or "Lt"
- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.nodeAffinity.required[].matchExpressions[].values

`[]string`

Values for the operator: required non-empty for In/NotIn, exactly one integer
string for Gt/Lt, and must be empty for Exists/DoesNotExist.

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred

`[]WorkloadPreferredNodeSelectorTerm`

Weighted soft preferences.

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].weight

`int32`

Preference weight, 1–100. Higher weights dominate placement scoring.

- rule: {"int32":{"lte":100,"gte":1}}

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term

`WorkloadNodeSelectorTerm` · required

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions

`[]WorkloadNodeSelectorRequirement` · required

- rule: {"repeated":{"minItems":"1"}}
- rule: In/NotIn require at least one value, Gt/Lt exactly one, Exists/DoesNotExist none

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].key

`string` · required

Node label key, e.g. "topology.kubernetes.io/zone".

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].operator

`string` · required

Operator: "In"/"NotIn" (value set), "Exists"/"DoesNotExist" (key presence), or
"Gt"/"Lt" (single integer value, as strings — the Kubernetes API convention).

- rule: Operator must be one of "In", "NotIn", "Exists", "DoesNotExist", "Gt", or "Lt"
- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.nodeAffinity.preferred[].term.matchExpressions[].values

`[]string`

Values for the operator: required non-empty for In/NotIn, exactly one integer
string for Gt/Lt, and must be empty for Exists/DoesNotExist.

### spec.jobTemplate.pod.scheduling.podAffinity

`WorkloadPodAffinity`

Attract pods toward nodes/zones already running matching pods (co-location with
a cache, for example).

### spec.jobTemplate.pod.scheduling.podAffinity.required

`[]WorkloadPodAffinityTerm`

Hard rules — unschedulable until satisfied. Use sparingly; they can deadlock rollouts.

### spec.jobTemplate.pod.scheduling.podAffinity.required[].matchLabels

`map<string, string>` · required

Labels of the pods to match against — for self-anti-affinity, the workload's own
selector labels (exported as the `selector_labels` stack output).

- rule: {"map":{"minPairs":"1"}}

### spec.jobTemplate.pod.scheduling.podAffinity.required[].topologyKey

`string` · required

Node label defining the domain: "kubernetes.io/hostname" separates by node,
"topology.kubernetes.io/zone" by zone.

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAffinity.required[].namespaces

`[]string`

Namespaces whose pods are considered. Empty means the workload's own namespace.

### spec.jobTemplate.pod.scheduling.podAffinity.preferred

`[]WorkloadWeightedPodAffinityTerm`

Weighted soft rules — the scheduler's tiebreakers.

### spec.jobTemplate.pod.scheduling.podAffinity.preferred[].weight

`int32`

Preference weight, 1–100.

- rule: {"int32":{"lte":100,"gte":1}}

### spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term

`WorkloadPodAffinityTerm` · required

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.matchLabels

`map<string, string>` · required

Labels of the pods to match against — for self-anti-affinity, the workload's own
selector labels (exported as the `selector_labels` stack output).

- rule: {"map":{"minPairs":"1"}}

### spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.topologyKey

`string` · required

Node label defining the domain: "kubernetes.io/hostname" separates by node,
"topology.kubernetes.io/zone" by zone.

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAffinity.preferred[].term.namespaces

`[]string`

Namespaces whose pods are considered. Empty means the workload's own namespace.

### spec.jobTemplate.pod.scheduling.podAntiAffinity

`WorkloadPodAffinity`

Repel pods from nodes/zones already running matching pods — the classic
high-availability pattern is anti-affinity on the workload's own labels across
`kubernetes.io/hostname`.

### spec.jobTemplate.pod.scheduling.podAntiAffinity.required

`[]WorkloadPodAffinityTerm`

Hard rules — unschedulable until satisfied. Use sparingly; they can deadlock rollouts.

### spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].matchLabels

`map<string, string>` · required

Labels of the pods to match against — for self-anti-affinity, the workload's own
selector labels (exported as the `selector_labels` stack output).

- rule: {"map":{"minPairs":"1"}}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].topologyKey

`string` · required

Node label defining the domain: "kubernetes.io/hostname" separates by node,
"topology.kubernetes.io/zone" by zone.

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.required[].namespaces

`[]string`

Namespaces whose pods are considered. Empty means the workload's own namespace.

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred

`[]WorkloadWeightedPodAffinityTerm`

Weighted soft rules — the scheduler's tiebreakers.

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].weight

`int32`

Preference weight, 1–100.

- rule: {"int32":{"lte":100,"gte":1}}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term

`WorkloadPodAffinityTerm` · required

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.matchLabels

`map<string, string>` · required

Labels of the pods to match against — for self-anti-affinity, the workload's own
selector labels (exported as the `selector_labels` stack output).

- rule: {"map":{"minPairs":"1"}}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.topologyKey

`string` · required

Node label defining the domain: "kubernetes.io/hostname" separates by node,
"topology.kubernetes.io/zone" by zone.

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.podAntiAffinity.preferred[].term.namespaces

`[]string`

Namespaces whose pods are considered. Empty means the workload's own namespace.

### spec.jobTemplate.pod.scheduling.topologySpreadConstraints

`[]WorkloadTopologySpreadConstraint`

Even distribution of replicas across topology domains (zones, hosts). Preferred
over hostname anti-affinity for large replica counts because skew is bounded
rather than binary.

### spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].maxSkew

`int32`

Maximum allowed difference in matching-pod counts between any two domains.
1 is the strictest even spread.

- rule: {"int32":{"gte":1}}

### spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].topologyKey

`string` · required

Node label defining the domains to spread across (e.g.
"topology.kubernetes.io/zone").

- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].whenUnsatisfiable

`string` · required

What happens when the constraint cannot be met: "DoNotSchedule" (hard — pod
stays Pending) or "ScheduleAnyway" (soft — scheduler minimizes skew).

- rule: whenUnsatisfiable must be either "DoNotSchedule" or "ScheduleAnyway"
- rule: {"required":true}

### spec.jobTemplate.pod.scheduling.topologySpreadConstraints[].matchLabels

`map<string, string>`

Labels selecting the pods counted per domain. Omit to have the module default to
the workload's own selector labels — self-spreading, the overwhelmingly common
intent.

### spec.jobTemplate.pod.scheduling.schedulerName

`string`

Hand pods to a non-default scheduler installed in the cluster. Leave empty for
the standard scheduler.

### spec.jobTemplate.pod.securityContext

`WorkloadPodSecurityContext`

Pod-level security context: the user/group identity and filesystem ownership
every container inherits unless it overrides them in its own security context.

### spec.jobTemplate.pod.securityContext.runAsUser

`int64` · optional (explicit presence)

UID all container processes run as unless overridden per container.

### spec.jobTemplate.pod.securityContext.runAsGroup

`int64` · optional (explicit presence)

Primary GID all container processes run as unless overridden per container.

### spec.jobTemplate.pod.securityContext.runAsNonRoot

`bool` · optional (explicit presence)

Refuse to start any container whose effective user is root.

### spec.jobTemplate.pod.securityContext.fsGroup

`int64` · optional (explicit presence)

GID that owns mounted volumes and is added to every container's supplemental
groups — the standard fix for "permission denied" on persistent volumes written
by non-root apps.

### spec.jobTemplate.pod.securityContext.fsGroupChangePolicy

`string`

When volume ownership is re-chowned to fs_group: "Always" (default) or
"OnRootMismatch" (skip the recursive chown when the root already matches —
dramatically faster pod starts on large volumes).

- rule: fsGroupChangePolicy must be either "Always" or "OnRootMismatch"

### spec.jobTemplate.pod.securityContext.supplementalGroups

`[]int64`

Additional group IDs applied to all container processes.

### spec.jobTemplate.pod.securityContext.sysctls

`[]WorkloadSysctl`

Kernel parameters set for the pod. Only safe sysctls (or those the cluster
administrator has allow-listed on the kubelet) are admitted.

### spec.jobTemplate.pod.securityContext.sysctls[].name

`string` · required

Sysctl name, e.g. "net.core.somaxconn".

- rule: {"required":true}

### spec.jobTemplate.pod.securityContext.sysctls[].value

`string` · required

Sysctl value, e.g. "1024".

- rule: {"required":true}

### spec.jobTemplate.pod.securityContext.seccompProfile

`WorkloadSeccompProfile`

Pod-wide seccomp profile; containers may override with their own.

- rule: localhost_profile is required when type is "Localhost" and must be empty otherwise

### spec.jobTemplate.pod.securityContext.seccompProfile.type

`string` · required

Profile type: "RuntimeDefault" (the container runtime's default filter — the
recommended baseline), "Unconfined" (no filtering), or "Localhost" (a profile
file installed on the node, named via localhost_profile).

- rule: Seccomp profile type must be one of "RuntimeDefault", "Unconfined", or "Localhost"
- rule: {"required":true}

### spec.jobTemplate.pod.securityContext.seccompProfile.localhostProfile

`string`

Path of the profile file relative to the node's seccomp profile root. Required
when (and only meaningful when) type is "Localhost".

### spec.jobTemplate.pod.terminationGracePeriodSeconds

`int64` · optional (explicit presence)

Seconds the kubelet waits between SIGTERM and SIGKILL at pod termination.
Kubernetes defaults to 30 when unset. Size it to cover pre_stop hooks plus the
app's own drain time — the grace clock starts before the hook runs.

- rule: {"int64":{"gte":"0"}}

### spec.jobTemplate.pod.dnsPolicy

`string`

Pod DNS resolution policy. "ClusterFirst" (the Kubernetes default) resolves
cluster services first; "Default" inherits the node's resolver;
"ClusterFirstWithHostNet" is what host-network pods need to keep resolving
cluster services; "None" hands control entirely to `dns_config`.

- rule: DNS policy must be one of "ClusterFirst", "ClusterFirstWithHostNet", "Default", or "None"

### spec.jobTemplate.pod.dnsConfig

`WorkloadPodDnsConfig`

Custom DNS parameters merged into (or, with dns_policy "None", replacing) the
generated resolv.conf.

### spec.jobTemplate.pod.dnsConfig.nameservers

`[]string`

Nameserver IPs (max 3 total after merging with the policy's servers).

- rule: {"repeated":{"maxItems":"3"}}

### spec.jobTemplate.pod.dnsConfig.searches

`[]string`

Search domains for hostname lookup.

### spec.jobTemplate.pod.dnsConfig.options

`[]WorkloadPodDnsConfigOption`

resolv.conf options, e.g. name "ndots" value "2". Value may be empty for flags.

### spec.jobTemplate.pod.dnsConfig.options[].name

`string` · required

- rule: {"required":true}

### spec.jobTemplate.pod.dnsConfig.options[].value

`string`

### spec.jobTemplate.pod.hostAliases

`[]WorkloadHostAlias`

Static host-file entries injected into every container's /etc/hosts — the
escape hatch for names that no reachable DNS serves.

### spec.jobTemplate.pod.hostAliases[].ip

`string` · required

The IP the hostnames resolve to.

- rule: {"required":true,"string":{"ip":true}}

### spec.jobTemplate.pod.hostAliases[].hostnames

`[]string` · required

Hostnames mapped to the IP.

- rule: {"repeated":{"minItems":"1"}}

### spec.jobTemplate.pod.hostNetwork

`bool`

Run pods in the node's network namespace. A DaemonSet agent pattern (node
monitoring, CNI components). Host-network pods share the node's port space —
combine with `dns_policy: ClusterFirstWithHostNet` if they resolve cluster
services.

### spec.jobTemplate.pod.hostPid

`bool`

Share the node's PID namespace (process-visibility agents). Requires elevated
trust in the workload.

### spec.jobTemplate.pod.priorityClassName

`string`

PriorityClass name controlling scheduling and eviction precedence. References a
cluster-scoped PriorityClass (e.g. "system-cluster-critical" or one created by a
KubernetesPriorityClass resource).

### spec.jobTemplate.pod.runtimeClassName

`string`

RuntimeClass selecting an alternate container runtime configuration (e.g. gVisor
or Kata sandboxing) installed by the cluster administrator.

### spec.jobTemplate.parallelism

`int32` · optional (explicit presence)

Maximum number of pods running at any one time within a run. The controller
runs fewer when the remaining work is smaller (completions minus
successes). Defaults to 1 — sequential execution; raise it to fan out
parallel batch work.

- default: `1`
- rule: {"int32":{"gte":0}}

### spec.jobTemplate.completions

`int32` · optional (explicit presence)

Number of successful pod completions required for each run's Job to
succeed. Defaults to 1. In Indexed mode this is also the number of
indexes — each pod is assigned an index from 0 to completions-1 and the Job
completes when every index has one successful pod.

- default: `1`
- rule: {"int32":{"gte":0}}

### spec.jobTemplate.completionMode

`string` · optional (explicit presence)

How pod completions are tracked. "NonIndexed" (default): pods are
interchangeable and the Job completes after `completions` successes.
"Indexed": each pod receives a completion index (0 to completions-1,
exposed via the batch.kubernetes.io/job-completion-index annotation and the
pod hostname), so each pod can claim its own partition of the work — the
mode for parallel processing of partitioned data.

- default: `NonIndexed`
- rule: Completion mode must be either "NonIndexed" or "Indexed"

### spec.jobTemplate.backoffLimit

`int32` · optional (explicit presence)

Number of pod-failure retries before a run's Job is marked failed.
Kubernetes defaults to 6, with exponential back-off between retries. Note:
when backoff_limit_per_index is set, upstream stops counting failures
globally and this limit is effectively unlimited unless set explicitly.

- default: `6`
- rule: {"int32":{"gte":0}}

### spec.jobTemplate.backoffLimitPerIndex

`uint32` · optional (explicit presence)

Retry budget per completion index instead of per Job — one flaky partition
exhausts only its own retries rather than the whole run's. Indexed mode
only, and requires restart_policy "Never". Unset keeps the global
backoff_limit behavior.

### spec.jobTemplate.maxFailedIndexes

`uint32` · optional (explicit presence)

Maximum number of indexes that may fail (exhaust their per-index retries)
before the run's Job is marked failed and terminated. Requires
backoff_limit_per_index. Unset lets every index run to its own outcome and
the Job completes with whatever mix of succeeded and failed indexes
results.

### spec.jobTemplate.activeDeadlineSeconds

`int64` · optional (explicit presence)

Hard wall-clock limit in seconds for each run's Job, counted from its
start time. When exceeded, all running pods are killed and the Job is
marked failed — regardless of remaining retries. Unset means no deadline.
Especially valuable on CronJobs with concurrency_policy "Forbid", where a
hung run silently blocks every subsequent run.

- rule: {"int64":{"gte":"1"}}

### spec.jobTemplate.ttlSecondsAfterFinished

`int32` · optional (explicit presence)

Seconds after a run's Job finishes (success or failure) before the Job and
its pods are automatically deleted. 0 deletes immediately on finish; unset
leaves cleanup to the CronJob's history limits — the usual choice here,
since the history limits already bound retention.

- rule: {"int32":{"gte":0}}

### spec.jobTemplate.restartPolicy

`string` · optional (explicit presence)

What the kubelet does when a container in a run's pod exits non-zero:
"Never" (default) leaves the pod failed and lets the Job controller create
a replacement pod, keeping one pod per attempt for debugging; "OnFailure"
restarts the container in place, reusing the pod. "Always" is invalid for
Jobs — it would restart even successfully completed containers, so the Job
could never finish.

- default: `Never`
- rule: Restart policy must be either "Never" or "OnFailure" — "Always" would restart completed containers and the Job could never finish

### spec.jobTemplate.podFailurePolicy

`KubernetesCronJobPodFailurePolicy`

Fine-grained handling of pod failures based on container exit codes or pod
conditions — e.g. fail a run immediately on a "misconfiguration" exit code,
or ignore failures caused by node disruption so they don't burn the retry
budget. Omit for the default behavior (every failure counts toward
backoff_limit).

### spec.jobTemplate.podFailurePolicy.rules

`[]KubernetesCronJobPodFailurePolicyRule` · required

Ordered rules; the first rule matching a pod failure decides its handling.

- rule: {"repeated":{"minItems":"1"}}
- rule: Set exactly one of on_exit_codes or on_pod_conditions per rule

### spec.jobTemplate.podFailurePolicy.rules[].action

`string` · required

What happens when the rule matches. "FailJob": the run's Job is marked
failed immediately and all running pods are terminated, ignoring
backoff_limit — for unrecoverable errors like misconfiguration. "Ignore":
the failure does not count toward backoff_limit and a replacement pod is
created — for infrastructure-caused failures like node disruption.
"Count": the default handling, the failure counts toward backoff_limit.
"FailIndex": only the pod's completion index is marked failed and not
retried — Indexed mode only, and requires backoff_limit_per_index.

- rule: Action must be one of "FailJob", "Ignore", "Count", or "FailIndex"
- rule: {"required":true}

### spec.jobTemplate.podFailurePolicy.rules[].onExitCodes

`KubernetesCronJobPodFailurePolicyOnExitCodes`

Trigger on container exit codes — e.g. exit code 42 meaning "bad input,
do not retry".

### spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.containerName

`string`

Restricts the exit-code check to the named container (app, sidecar, or
init container). Empty matches any container in the pod.

### spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.operator

`string` · required

How exit codes relate to `values`: "In" matches when a failed container's
exit code is in the set; "NotIn" matches when it is not.

- rule: Operator must be either "In" or "NotIn"
- rule: {"required":true}

### spec.jobTemplate.podFailurePolicy.rules[].onExitCodes.values

`[]int32` · required

The exit codes checked against the operator. Must not contain duplicates,
and cannot contain 0 for the "In" operator — exit code 0 is success and is
excluded from the check.

- rule: {"repeated":{"minItems":"1"}}

### spec.jobTemplate.podFailurePolicy.rules[].onPodConditions

`[]KubernetesCronJobPodFailurePolicyOnPodCondition`

Trigger on pod conditions — e.g. type "DisruptionTarget" to recognize
pods killed by node drains or preemption. The rule matches when at least
one pattern matches an actual pod condition.

### spec.jobTemplate.podFailurePolicy.rules[].onPodConditions[].type

`string` · required

The pod condition type to match, e.g. "DisruptionTarget" (pod terminated
by node drain, preemption, or taint eviction).

- rule: {"required":true}

### spec.jobTemplate.podFailurePolicy.rules[].onPodConditions[].status

`string` · optional (explicit presence)

The condition status that must be present for the match. Defaults to
"True" — the overwhelmingly common intent.

- default: `True`
- rule: Condition status must be one of "True", "False", or "Unknown"

### spec.jobTemplate.successPolicy

`KubernetesCronJobSuccessPolicy`

Early-success criteria for Indexed runs — declare the Job succeeded once
the leading indexes (or enough indexes) finish, without waiting for every
index. Omit for the default behavior (success requires `completions`
successful pods).

### spec.jobTemplate.successPolicy.rules

`[]KubernetesCronJobSuccessPolicyRule` · required

Alternative success rules; the Job succeeds when any one of them is met.

- rule: {"repeated":{"minItems":"1"}}
- rule: Each success rule must set at least one of succeeded_indexes or succeeded_count

### spec.jobTemplate.successPolicy.rules[].succeededIndexes

`string`

Indexes that must succeed, as comma-separated integers or ranges within
0 to completions-1 — e.g. "0" (leader only) or "0-2,4".

- rule: Succeeded indexes must be comma-separated integers or ranges (e.g. "0", "0-2,4")

### spec.jobTemplate.successPolicy.rules[].succeededCount

`int32` · optional (explicit presence)

Minimum number of succeeded indexes. When combined with succeeded_indexes,
only successes within that set are counted.

- rule: {"int32":{"gte":1}}

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesCronJob, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | The namespace the CronJob was created in. |
| `status.outputs.cron_job_name` | `string` | The name of the CronJob object as created in the cluster. |
| `status.outputs.schedule` | `string` | The effective cron expression the CronJob runs on. Exported so dependents and audits read the schedule from outputs — the deployed truth — rather than from the spec. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |
| `spec.jobTemplate.pod.serviceAccount` | KubernetesServiceAccount | `status.outputs.service_account_name` |
| `spec.jobTemplate.pod.imagePullSecrets` | KubernetesSecret | `spec.name` |

## See Also

- [Overview](../README.md)
