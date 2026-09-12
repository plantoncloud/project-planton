package module

import (
	kubernetesplantonplatformv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesplantonplatform/v1alpha1"
)

// platformSpecBody renders the typed spec into the PlantonPlatform CR's
// spec — a plain nested map, because the CR is applied UNTYPED (see
// main.go). Keys render ONLY when the manifest declared them, so the
// operator's own defaulting stays authoritative for everything unset —
// the same posture as the `planton` Helm chart's verbatim
// pass-through. This function must stay in byte lockstep with the
// Terraform module's locals.platform_spec.
//
// Three-state optionals (the default-true toggles, the defaulted
// scalars) render exactly when the proto field is PRESENT — an explicit
// `enabled: true` is faithfully forwarded even though it matches the
// CRD default, because presence is the user's deliberate statement.
func platformSpecBody(locals *Locals) map[string]interface{} {
	spec := locals.Spec

	out := map[string]interface{}{
		// Required by the CRD and by this spec — the one field every
		// platform declares.
		"version": spec.GetVersion(),
	}

	// ---- license ---------------------------------------------------------------
	if l := spec.GetLicense(); l != nil {
		license := map[string]interface{}{}
		if l.GetKey() != "" {
			license["key"] = l.GetKey()
		}
		if ref := l.GetSecretKeyRef(); ref != nil {
			license["secretKeyRef"] = map[string]interface{}{
				"name": ref.GetName(),
				"key":  ref.GetKey(),
			}
		}
		if len(license) > 0 {
			out["license"] = license
		}
	}

	// ---- storage ---------------------------------------------------------------
	if s := spec.GetStorage(); s != nil {
		storage := map[string]interface{}{}
		if s.GetStorageClassName() != "" {
			storage["storageClassName"] = s.GetStorageClassName()
		}
		if s.GetSize() != "" {
			storage["size"] = s.GetSize()
		}
		if len(storage) > 0 {
			out["storage"] = storage
		}
	}

	// ---- database --------------------------------------------------------------
	if d := spec.GetDatabase(); d != nil {
		database := map[string]interface{}{}
		if pg := d.GetPostgresql(); pg != nil {
			postgresql := map[string]interface{}{}
			if pg.Replicas != nil {
				postgresql["replicas"] = int(pg.GetReplicas())
			}
			if pg.GetStorageSize() != "" {
				postgresql["storageSize"] = pg.GetStorageSize()
			}
			if pg.GetStorageClassName() != "" {
				postgresql["storageClassName"] = pg.GetStorageClassName()
			}
			if len(postgresql) > 0 {
				database["postgresql"] = postgresql
			}
		}
		if r := d.GetRedis(); r != nil {
			redis := map[string]interface{}{}
			if r.GetStorageSize() != "" {
				redis["storageSize"] = r.GetStorageSize()
			}
			if r.GetStorageClassName() != "" {
				redis["storageClassName"] = r.GetStorageClassName()
			}
			if len(redis) > 0 {
				database["redis"] = redis
			}
		}
		if len(database) > 0 {
			out["database"] = database
		}
	}

	// ---- ingress ---------------------------------------------------------------
	if i := spec.GetIngress(); i != nil {
		ingress := map[string]interface{}{}
		if i.GetEnabled() {
			ingress["enabled"] = true
		}
		if i.GetHostname() != "" {
			ingress["hostname"] = i.GetHostname()
		}
		if i.GetIngressClassName() != "" {
			ingress["ingressClassName"] = i.GetIngressClassName()
		}
		// The Gateway API front door: the fork's other arm (the CRD refuses
		// it beside ingressClassName). Rendered only when the manifest named
		// a Gateway; the operator reads the Gateway's listeners for
		// everything else. name and namespace are KubernetesGateway foreign
		// keys -- resolved to their literal values before the module runs --
		// and the PlantonPlatform resource takes the plain strings.
		if g := i.GetGatewayRef(); g != nil {
			gatewayRef := map[string]interface{}{
				"name": g.GetName().GetValue(),
			}
			if ns := g.GetNamespace().GetValue(); ns != "" {
				gatewayRef["namespace"] = ns
			}
			if g.GetSectionName() != "" {
				gatewayRef["sectionName"] = g.GetSectionName()
			}
			ingress["gatewayRef"] = gatewayRef
		}
		if len(i.GetAnnotations()) > 0 {
			ingress["annotations"] = stringMapToInterface(i.GetAnnotations())
		}
		if t := i.GetTls(); t != nil {
			tls := map[string]interface{}{}
			if t.GetSecretName() != "" {
				tls["secretName"] = t.GetSecretName()
			}
			if iss := t.GetIssuer(); iss != nil {
				issuer := map[string]interface{}{
					"name": iss.GetName(),
				}
				if iss.Kind != nil && iss.GetKind() != "" {
					issuer["kind"] = iss.GetKind()
				}
				tls["issuer"] = issuer
			}
			ingress["tls"] = tls
		}
		// The reachability declaration renders on presence, like every
		// defaulted three-state string: an omitted value is left to the
		// CRD's own default (auto) rather than spelled out here, so a
		// manifest that never mentions reachability produces the same CR
		// before and after the field existed.
		if i.Reachability != nil && i.GetReachability() != "" {
			ingress["reachability"] = i.GetReachability()
		}
		if len(ingress) > 0 {
			out["ingress"] = ingress
		}
	}

	// ---- gateway ---------------------------------------------------------------
	if g := spec.GetGateway(); g != nil && g.LocalPort != nil {
		out["gateway"] = map[string]interface{}{
			"localPort": int(g.GetLocalPort()),
		}
	}

	// ---- identity --------------------------------------------------------------
	if id := spec.GetIdentity(); id != nil {
		identity := map[string]interface{}{}
		if id.Realm != nil && id.GetRealm() != "" {
			identity["realm"] = id.GetRealm()
		}
		if id.GetAdminEmail() != "" {
			identity["adminEmail"] = id.GetAdminEmail()
		}
		if len(identity) > 0 {
			out["identity"] = identity
		}
	}

	// ---- bootstrap -------------------------------------------------------------
	if b := spec.GetBootstrap(); b != nil {
		bootstrap := map[string]interface{}{}
		if org := b.GetOrganization(); org != nil {
			organization := map[string]interface{}{}
			if org.Slug != nil && org.GetSlug() != "" {
				organization["slug"] = org.GetSlug()
			}
			if org.GetName() != "" {
				organization["name"] = org.GetName()
			}
			if len(organization) > 0 {
				bootstrap["organization"] = organization
			}
		}
		if env := b.GetEnvironment(); env != nil {
			environment := map[string]interface{}{}
			if env.Slug != nil && env.GetSlug() != "" {
				environment["slug"] = env.GetSlug()
			}
			if env.GetName() != "" {
				environment["name"] = env.GetName()
			}
			if len(environment) > 0 {
				bootstrap["environment"] = environment
			}
		}
		if len(b.GetAdmins()) > 0 {
			admins := make([]interface{}, 0, len(b.GetAdmins()))
			for _, a := range b.GetAdmins() {
				admins = append(admins, a)
			}
			bootstrap["admins"] = admins
		}
		if b.IacProvisioner != nil && b.GetIacProvisioner() != "" {
			bootstrap["iacProvisioner"] = b.GetIacProvisioner()
		}
		if sb := b.GetSecretBackend(); sb != nil {
			secretBackend := map[string]interface{}{
				"type": sb.GetType(),
			}
			if aws := sb.GetAwsSecretsManager(); aws != nil {
				secretBackend["awsSecretsManager"] = map[string]interface{}{
					"region":    aws.GetRegion(),
					"kmsKeyArn": aws.GetKmsKeyArn(),
				}
			}
			bootstrap["secretBackend"] = secretBackend
		}
		if len(bootstrap) > 0 {
			out["bootstrap"] = bootstrap
		}
	}

	// ---- runner ----------------------------------------------------------------
	if r := spec.GetRunner(); r != nil {
		runner := map[string]interface{}{}
		if r.Enabled != nil {
			runner["enabled"] = r.GetEnabled()
		}
		if r.GetStorageSize() != "" {
			runner["storageSize"] = r.GetStorageSize()
		}
		if r.GetStorageClassName() != "" {
			runner["storageClassName"] = r.GetStorageClassName()
		}
		if len(r.GetServiceAccountAnnotations()) > 0 {
			runner["serviceAccountAnnotations"] = stringMapToInterface(r.GetServiceAccountAnnotations())
		}
		if r.GetCloudCredentialsSecretName() != "" {
			runner["cloudCredentialsSecretName"] = r.GetCloudCredentialsSecretName()
		}
		if len(runner) > 0 {
			out["runner"] = runner
		}
	}

	// ---- build -----------------------------------------------------------------
	if b := spec.GetBuild(); b != nil && b.Enabled != nil {
		out["build"] = map[string]interface{}{
			"enabled": b.GetEnabled(),
		}
	}

	// ---- remote runners --------------------------------------------------------
	// Renders on presence, like build: a manifest that never mentions remote
	// runners produces the same CR as before the field existed (the operator's
	// default is off).
	if rr := spec.GetRemoteRunners(); rr != nil && rr.Enabled != nil {
		out["remoteRunners"] = map[string]interface{}{
			"enabled": rr.GetEnabled(),
		}
	}

	// ---- email -----------------------------------------------------------------
	// One declaration for both senders. The two provider arms render only
	// when declared (the spec's CEL already holds exactly one); defaulted
	// scalars (port, security, from.name) render on presence only, so an
	// omitted value is left to the CRD's own default. Credentials are Secret
	// names and Secret key references — never values.
	if e := spec.GetEmail(); e != nil {
		email := map[string]interface{}{}
		if from := e.GetFrom(); from != nil {
			fromBody := map[string]interface{}{}
			if from.GetAddress() != "" {
				fromBody["address"] = from.GetAddress()
			}
			if from.Name != nil && from.GetName() != "" {
				fromBody["name"] = from.GetName()
			}
			if len(fromBody) > 0 {
				email["from"] = fromBody
			}
		}
		if e.GetReplyTo() != "" {
			email["replyTo"] = e.GetReplyTo()
		}
		if s := e.GetSmtp(); s != nil {
			smtp := map[string]interface{}{
				"host": s.GetHost(),
			}
			if s.Port != nil {
				smtp["port"] = int(s.GetPort())
			}
			if s.Security != nil && s.GetSecurity() != "" {
				smtp["security"] = s.GetSecurity()
			}
			if s.GetCredentialsSecretName() != "" {
				smtp["credentialsSecretName"] = s.GetCredentialsSecretName()
			}
			if o := s.GetOauth2(); o != nil {
				smtp["oauth2"] = map[string]interface{}{
					"user":            o.GetUser(),
					"tokenUrl":        o.GetTokenUrl(),
					"scope":           o.GetScope(),
					"clientId":        o.GetClientId(),
					"clientSecretRef": secretKeyRefMap(o.GetClientSecretRef()),
				}
			}
			if ref := s.GetCaBundleSecretRef(); ref != nil {
				smtp["caBundleSecretRef"] = secretKeyRefMap(ref)
			}
			email["smtp"] = smtp
		}
		if r := e.GetResend(); r != nil {
			email["resend"] = map[string]interface{}{
				"apiKeySecretRef": secretKeyRefMap(r.GetApiKeySecretRef()),
			}
		}
		if len(email) > 0 {
			out["email"] = email
		}
	}

	// ---- vault -----------------------------------------------------------------
	if v := spec.GetVault(); v != nil {
		vault := map[string]interface{}{}
		if v.Enabled != nil {
			vault["enabled"] = v.GetEnabled()
		}
		if v.InitMode != nil && v.GetInitMode() != "" {
			vault["initMode"] = v.GetInitMode()
		}
		if v.GetStorageSize() != "" {
			vault["storageSize"] = v.GetStorageSize()
		}
		if v.GetStorageClassName() != "" {
			vault["storageClassName"] = v.GetStorageClassName()
		}
		if len(vault) > 0 {
			out["vault"] = vault
		}
	}

	// ---- components ------------------------------------------------------------
	if c := spec.GetComponents(); c != nil {
		components := map[string]interface{}{}
		if g := c.GetGraph(); g != nil {
			graph := map[string]interface{}{}
			if g.GetEnabled() {
				graph["enabled"] = true
			}
			if g.GetStorageSize() != "" {
				graph["storageSize"] = g.GetStorageSize()
			}
			if g.GetStorageClassName() != "" {
				graph["storageClassName"] = g.GetStorageClassName()
			}
			if len(graph) > 0 {
				components["graph"] = graph
			}
		}
		if len(components) > 0 {
			out["components"] = components
		}
	}

	// ---- prerequisites ---------------------------------------------------------
	if p := spec.GetPrerequisites(); p != nil {
		prerequisites := map[string]interface{}{}
		if p.PostgresOperator != nil && p.GetPostgresOperator() != "" {
			prerequisites["postgresOperator"] = p.GetPostgresOperator()
		}
		if p.TektonPipelines != nil && p.GetTektonPipelines() != "" {
			prerequisites["tektonPipelines"] = p.GetTektonPipelines()
		}
		if len(prerequisites) > 0 {
			out["prerequisites"] = prerequisites
		}
	}

	// ---- controlPlane / console --------------------------------------------------
	if cp := spec.GetControlPlane(); cp != nil {
		controlPlane := map[string]interface{}{}
		if img := imageMap(cp.GetImage()); img != nil {
			controlPlane["image"] = img
		}
		if cp.Replicas != nil {
			controlPlane["replicas"] = int(cp.GetReplicas())
		}
		if cp.GetExternalConfigSecretName() != "" {
			controlPlane["externalConfigSecretName"] = cp.GetExternalConfigSecretName()
		}
		if len(cp.GetServiceAccountAnnotations()) > 0 {
			controlPlane["serviceAccountAnnotations"] = stringMapToInterface(cp.GetServiceAccountAnnotations())
		}
		if len(controlPlane) > 0 {
			out["controlPlane"] = controlPlane
		}
	}
	if co := spec.GetConsole(); co != nil {
		console := map[string]interface{}{}
		if img := imageMap(co.GetImage()); img != nil {
			console["image"] = img
		}
		if co.Replicas != nil {
			console["replicas"] = int(co.GetReplicas())
		}
		if co.GetExternalConfigSecretName() != "" {
			console["externalConfigSecretName"] = co.GetExternalConfigSecretName()
		}
		if len(console) > 0 {
			out["console"] = console
		}
	}

	return out
}

// imageMap renders an image override, only the halves that are set.
func imageMap(img *kubernetesplantonplatformv1alpha1.KubernetesPlantonPlatformImage) map[string]interface{} {
	if img == nil {
		return nil
	}
	out := map[string]interface{}{}
	if img.GetRepository() != "" {
		out["repository"] = img.GetRepository()
	}
	if img.GetTag() != "" {
		out["tag"] = img.GetTag()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// secretKeyRefMap renders a by-reference credential as the CR's
// {name, key} pair. Nil in, nil out, so a caller can assign it under an
// optional key without a presence check of its own; the required references
// (the OAuth2 client secret, the Resend API key) are held non-nil by the
// spec's validation before this runs.
func secretKeyRefMap(ref *kubernetesplantonplatformv1alpha1.KubernetesPlantonPlatformSecretKeyRef) map[string]interface{} {
	if ref == nil {
		return nil
	}
	return map[string]interface{}{
		"name": ref.GetName(),
		"key":  ref.GetKey(),
	}
}

// stringMapToInterface converts a map[string]string into the
// map[string]interface{} the untyped CR body expects.
func stringMapToInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
