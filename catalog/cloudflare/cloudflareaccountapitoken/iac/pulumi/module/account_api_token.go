package module

import (
	"encoding/json"

	"github.com/pkg/errors"
	cloudflareaccountapitokenv1alpha1 "github.com/plantonhq/planton/catalog/cloudflare/cloudflareaccountapitoken/v1alpha1"
	"github.com/plantonhq/planton/pkg/cloudflare/r2"
	"github.com/pulumi/pulumi-cloudflare/sdk/v6/go/cloudflare"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// accountApiToken creates the account-owned API token. Each policy's
// `resources` travels to Cloudflare as ONE raw JSON object; the spec types
// it (whole-resource grant or nested sub-resource scoping per entry) and
// this module serializes each entry back to the API's shape.
//
// The token's secret value is returned by Cloudflare exactly once, on
// create. Cloudflare canonically re-orders policies and permission groups
// server-side; treat their order as insignificant.
func accountApiToken(
	ctx *pulumi.Context,
	locals *Locals,
	cloudflareProvider *cloudflare.Provider,
) error {
	spec := locals.CloudflareAccountApiToken.Spec

	policies := cloudflare.AccountTokenPolicyArray{}
	for _, policy := range spec.Policies {
		permissionGroups := cloudflare.AccountTokenPolicyPermissionGroupArray{}
		for _, id := range policy.PermissionGroupIds {
			permissionGroups = append(permissionGroups, cloudflare.AccountTokenPolicyPermissionGroupArgs{
				Id: pulumi.String(id),
			})
		}

		resourcesJson, err := serializeResources(policy.Resources)
		if err != nil {
			return errors.Wrap(err, "failed to serialize policy resources")
		}

		policies = append(policies, cloudflare.AccountTokenPolicyArgs{
			Effect:           pulumi.String(policy.Effect),
			PermissionGroups: permissionGroups,
			Resources:        pulumi.String(resourcesJson),
		})
	}

	args := &cloudflare.AccountTokenArgs{
		AccountId: pulumi.String(spec.AccountId),
		Name:      pulumi.String(spec.Name),
		Policies:  policies,
	}

	if spec.ExpiresOn != "" {
		args.ExpiresOn = pulumi.String(spec.ExpiresOn)
	}
	if spec.NotBefore != "" {
		args.NotBefore = pulumi.String(spec.NotBefore)
	}
	if spec.Status != "" {
		args.Status = pulumi.String(spec.Status)
	}

	if spec.Condition != nil && spec.Condition.RequestIp != nil {
		requestIp := cloudflare.AccountTokenConditionRequestIpArgs{}
		if len(spec.Condition.RequestIp.InCidrs) > 0 {
			requestIp.Ins = pulumi.ToStringArray(spec.Condition.RequestIp.InCidrs)
		}
		if len(spec.Condition.RequestIp.NotInCidrs) > 0 {
			requestIp.NotIns = pulumi.ToStringArray(spec.Condition.RequestIp.NotInCidrs)
		}
		args.Condition = cloudflare.AccountTokenConditionArgs{
			RequestIp: requestIp,
		}
	}

	createdToken, err := cloudflare.NewAccountToken(
		ctx,
		"account_api_token",
		args,
		pulumi.Provider(cloudflareProvider),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create account api token")
	}

	ctx.Export(OpTokenId, createdToken.ID())
	ctx.Export(OpValue, createdToken.Value)

	// The same token, in the shape R2's S3 API authenticates: Cloudflare
	// defines the access key id as the token's id and the secret access key as
	// the SHA-256 of the token's value (derived by the shared pkg/cloudflare/r2
	// helper so every module agrees). The derivation runs inside the secret
	// Output's apply, so the hash never leaves the secret boundary; ToSecret
	// re-marks the result because a transform of a secret is not automatically
	// one in every SDK path.
	ctx.Export(OpR2AccessKeyId, createdToken.ID().ToStringOutput())
	ctx.Export(OpR2SecretAccessKey, pulumi.ToSecret(createdToken.Value.ApplyT(func(v string) string {
		return r2.S3SecretAccessKey(v)
	}).(pulumi.StringOutput)))

	return nil
}

// serializeResources rebuilds the API's raw JSON object from the typed
// spec map: a whole-resource grant serializes to its permission string, a
// nested scoping to a sub-resource object (exactly one per entry, enforced
// by spec validation).
func serializeResources(resources map[string]*cloudflareaccountapitokenv1alpha1.CloudflareAccountApiTokenResourceScope) (string, error) {
	body := map[string]interface{}{}
	for key, scope := range resources {
		if scope.Permission != "" {
			body[key] = scope.Permission
			continue
		}
		body[key] = scope.Subresources
	}

	serialized, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(serialized), nil
}
