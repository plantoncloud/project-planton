package module

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/plantonhq/planton/pkg/cloudflare/r2"
	"github.com/pulumi/pulumi-cloudflare/sdk/v6/go/cloudflare"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// bucket provisions the R2 bucket and its bucket-scoped configuration (managed
// public domain, custom domains, CORS, lifecycle, lock) and exports outputs.
func bucket(
	ctx *pulumi.Context,
	locals *Locals,
	cloudflareProvider *cloudflare.Provider,
) (*cloudflare.R2Bucket, error) {
	spec := locals.CloudflareR2Bucket.Spec

	// Jurisdiction is part of the bucket identity and is applied to the bucket
	// and every bucket-scoped sub-resource. Empty -> nil -> provider default.
	var jurisdiction pulumi.StringPtrInput
	if spec.GetJurisdiction() != "" {
		jurisdiction = pulumi.String(spec.GetJurisdiction())
	}

	// 1. Assemble the bucket arguments. The location hint and storage class are
	// omitted when unset so the provider applies its defaults. The enum value
	// names match the strings the provider expects, so .String() is used directly.
	bucketArgs := &cloudflare.R2BucketArgs{
		AccountId:    pulumi.String(spec.GetAccountId()),
		Name:         pulumi.String(spec.GetBucketName()),
		Jurisdiction: jurisdiction,
	}
	if spec.GetLocation() != 0 {
		bucketArgs.Location = pulumi.String(spec.GetLocation().String())
	}
	if spec.GetStorageClass() != 0 {
		bucketArgs.StorageClass = pulumi.String(spec.GetStorageClass().String())
	}

	// 2. Create the bucket.
	createdBucket, err := cloudflare.NewR2Bucket(
		ctx,
		"bucket",
		bucketArgs,
		pulumi.Provider(cloudflareProvider),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Cloudflare R2 bucket")
	}
	dependsOnBucket := pulumi.DependsOn([]pulumi.Resource{createdBucket})

	// 3. Managed public access over the r2.dev domain (development-grade; custom
	// domains are the production path).
	var managedDomain *cloudflare.R2ManagedDomain
	if spec.GetPublicAccess() {
		managedDomain, err = cloudflare.NewR2ManagedDomain(ctx, "managed-domain", &cloudflare.R2ManagedDomainArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			Enabled:      pulumi.Bool(true),
			Jurisdiction: jurisdiction,
		}, pulumi.Provider(cloudflareProvider), dependsOnBucket)
		if err != nil {
			return nil, errors.Wrap(err, "failed to enable Cloudflare R2 managed (r2.dev) domain")
		}
	}

	// 4. Custom domains (one resource per enabled custom domain).
	customDomainUrls := pulumi.StringArray{}
	for _, cd := range spec.GetCustomDomains() {
		if !cd.GetEnabled() {
			continue
		}
		args := &cloudflare.R2CustomDomainArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			ZoneId:       pulumi.String(cd.GetZoneId().GetValue()),
			Domain:       pulumi.String(cd.GetDomain()),
			Enabled:      pulumi.Bool(true),
			Jurisdiction: jurisdiction,
		}
		if cd.GetMinTls() != "" {
			args.MinTls = pulumi.String(cd.GetMinTls())
		}
		if len(cd.GetCiphers()) > 0 {
			args.Ciphers = toStringArray(cd.GetCiphers())
		}
		name := fmt.Sprintf("custom-domain-%s", strings.ReplaceAll(cd.GetDomain(), ".", "-"))
		if _, err := cloudflare.NewR2CustomDomain(ctx, name, args, pulumi.Provider(cloudflareProvider), dependsOnBucket); err != nil {
			return nil, errors.Wrapf(err, "failed to create Cloudflare R2 custom domain %q", cd.GetDomain())
		}
		customDomainUrls = append(customDomainUrls, pulumi.String("https://"+cd.GetDomain()))
	}

	// 5. CORS configuration.
	if spec.GetCors() != nil && len(spec.GetCors().GetRules()) > 0 {
		rules := cloudflare.R2BucketCorsRuleArray{}
		for _, r := range spec.GetCors().GetRules() {
			methods := pulumi.StringArray{}
			for _, m := range r.GetAllowed().GetMethods() {
				methods = append(methods, pulumi.String(m.String()))
			}
			allowed := cloudflare.R2BucketCorsRuleAllowedArgs{
				Methods: methods,
				Origins: toStringArray(r.GetAllowed().GetOrigins()),
			}
			if len(r.GetAllowed().GetHeaders()) > 0 {
				allowed.Headers = toStringArray(r.GetAllowed().GetHeaders())
			}
			ruleArgs := cloudflare.R2BucketCorsRuleArgs{Allowed: allowed}
			if r.GetId() != "" {
				ruleArgs.Id = pulumi.String(r.GetId())
			}
			if len(r.GetExposeHeaders()) > 0 {
				ruleArgs.ExposeHeaders = toStringArray(r.GetExposeHeaders())
			}
			if r.GetMaxAgeSeconds() != 0 {
				ruleArgs.MaxAgeSeconds = pulumi.Float64(float64(r.GetMaxAgeSeconds()))
			}
			rules = append(rules, ruleArgs)
		}
		if _, err := cloudflare.NewR2BucketCors(ctx, "cors", &cloudflare.R2BucketCorsArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			Jurisdiction: jurisdiction,
			Rules:        rules,
		}, pulumi.Provider(cloudflareProvider), dependsOnBucket); err != nil {
			return nil, errors.Wrap(err, "failed to configure Cloudflare R2 bucket CORS")
		}
	}

	// 6. Lifecycle configuration.
	if spec.GetLifecycle() != nil && len(spec.GetLifecycle().GetRules()) > 0 {
		rules := cloudflare.R2BucketLifecycleRuleArray{}
		for _, r := range spec.GetLifecycle().GetRules() {
			ruleArgs := cloudflare.R2BucketLifecycleRuleArgs{
				Id:         pulumi.String(r.GetId()),
				Enabled:    pulumi.Bool(r.GetEnabled()),
				Conditions: cloudflare.R2BucketLifecycleRuleConditionsArgs{Prefix: pulumi.String(r.GetConditions().GetPrefix())},
			}
			if r.GetAbortMultipartUploadsTransition() != nil {
				ruleArgs.AbortMultipartUploadsTransition = cloudflare.R2BucketLifecycleRuleAbortMultipartUploadsTransitionArgs{
					Condition: cloudflare.R2BucketLifecycleRuleAbortMultipartUploadsTransitionConditionArgs{
						MaxAge: pulumi.Int(int(r.GetAbortMultipartUploadsTransition().GetMaxAgeSeconds())),
						Type:   pulumi.String("Age"),
					},
				}
			}
			if r.GetDeleteObjectsTransition() != nil {
				c := r.GetDeleteObjectsTransition().GetCondition()
				condArgs := cloudflare.R2BucketLifecycleRuleDeleteObjectsTransitionConditionArgs{
					Type: pulumi.String(c.GetType().String()),
				}
				if c.GetMaxAgeSeconds() != 0 {
					condArgs.MaxAge = pulumi.Int(int(c.GetMaxAgeSeconds()))
				}
				if c.GetDate() != "" {
					condArgs.Date = pulumi.String(c.GetDate())
				}
				ruleArgs.DeleteObjectsTransition = cloudflare.R2BucketLifecycleRuleDeleteObjectsTransitionArgs{Condition: condArgs}
			}
			if len(r.GetStorageClassTransitions()) > 0 {
				transitions := cloudflare.R2BucketLifecycleRuleStorageClassTransitionArray{}
				for _, t := range r.GetStorageClassTransitions() {
					c := t.GetCondition()
					condArgs := cloudflare.R2BucketLifecycleRuleStorageClassTransitionConditionArgs{
						Type: pulumi.String(c.GetType().String()),
					}
					if c.GetMaxAgeSeconds() != 0 {
						condArgs.MaxAge = pulumi.Int(int(c.GetMaxAgeSeconds()))
					}
					if c.GetDate() != "" {
						condArgs.Date = pulumi.String(c.GetDate())
					}
					transitions = append(transitions, cloudflare.R2BucketLifecycleRuleStorageClassTransitionArgs{
						Condition:    condArgs,
						StorageClass: pulumi.String("InfrequentAccess"),
					})
				}
				ruleArgs.StorageClassTransitions = transitions
			}
			rules = append(rules, ruleArgs)
		}
		if _, err := cloudflare.NewR2BucketLifecycle(ctx, "lifecycle", &cloudflare.R2BucketLifecycleArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			Jurisdiction: jurisdiction,
			Rules:        rules,
		}, pulumi.Provider(cloudflareProvider), dependsOnBucket); err != nil {
			return nil, errors.Wrap(err, "failed to configure Cloudflare R2 bucket lifecycle")
		}
	}

	// 7. Lock (retention) configuration.
	if spec.GetLock() != nil && len(spec.GetLock().GetRules()) > 0 {
		rules := cloudflare.R2BucketLockRuleArray{}
		for _, r := range spec.GetLock().GetRules() {
			c := r.GetCondition()
			condArgs := cloudflare.R2BucketLockRuleConditionArgs{
				Type: pulumi.String(c.GetType().String()),
			}
			if c.GetMaxAgeSeconds() != 0 {
				condArgs.MaxAgeSeconds = pulumi.Int(int(c.GetMaxAgeSeconds()))
			}
			if c.GetDate() != "" {
				condArgs.Date = pulumi.String(c.GetDate())
			}
			ruleArgs := cloudflare.R2BucketLockRuleArgs{
				Id:        pulumi.String(r.GetId()),
				Enabled:   pulumi.Bool(r.GetEnabled()),
				Condition: condArgs,
			}
			if r.GetPrefix() != "" {
				ruleArgs.Prefix = pulumi.String(r.GetPrefix())
			}
			rules = append(rules, ruleArgs)
		}
		if _, err := cloudflare.NewR2BucketLock(ctx, "lock", &cloudflare.R2BucketLockArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			Jurisdiction: jurisdiction,
			Rules:        rules,
		}, pulumi.Provider(cloudflareProvider), dependsOnBucket); err != nil {
			return nil, errors.Wrap(err, "failed to configure Cloudflare R2 bucket lock")
		}
	}

	// 8. Event notifications: forward object events to Cloudflare Queues.
	for i, en := range spec.GetEventNotifications() {
		rules := cloudflare.R2BucketEventNotificationRuleArray{}
		for _, r := range en.GetRules() {
			// Cloudflare serializes the FULL rule object on read -- an unset
			// filter comes back as "" (measured live 2026-08-26 on the
			// terraform module: a rule created without suffix refreshed to
			// suffix = "" and the omit-when-empty mapping drifted forever;
			// this engine carried the same latent class). Always send the
			// strings: "" and absent mean the same thing to the API and the
			// state then matches the echo.
			ruleArgs := cloudflare.R2BucketEventNotificationRuleArgs{
				Actions:     toStringArray(r.GetActions()),
				Description: pulumi.String(r.GetDescription()),
				Prefix:      pulumi.String(r.GetPrefix()),
				Suffix:      pulumi.String(r.GetSuffix()),
			}
			rules = append(rules, ruleArgs)
		}
		if _, err := cloudflare.NewR2BucketEventNotification(ctx, fmt.Sprintf("event-notification-%d", i), &cloudflare.R2BucketEventNotificationArgs{
			AccountId:    pulumi.String(spec.GetAccountId()),
			BucketName:   createdBucket.Name,
			QueueId:      pulumi.String(en.GetQueue().GetValue()),
			Jurisdiction: jurisdiction,
			Rules:        rules,
		}, pulumi.Provider(cloudflareProvider), dependsOnBucket); err != nil {
			return nil, errors.Wrap(err, "failed to configure Cloudflare R2 bucket event notification")
		}
	}

	// 9. Export stack outputs. The S3 endpoint is the jurisdiction's host: a
	// bucket created in "eu" (or "fedramp", "us") is served ONLY through
	// <account>.<jurisdiction>.r2.cloudflarestorage.com -- the default host
	// fails rather than redirects -- so every URL this stack hands out is
	// composed from the shared host table, never from the default host.
	s3Endpoint := r2.S3Endpoint(spec.GetAccountId(), spec.GetJurisdiction())
	ctx.Export(OpBucketName, createdBucket.Name)
	ctx.Export(OpAccountId, pulumi.String(spec.GetAccountId()))
	ctx.Export(OpJurisdiction, pulumi.String(r2.NormalizeJurisdiction(spec.GetJurisdiction())))
	ctx.Export(OpS3Endpoint, pulumi.String(s3Endpoint))
	ctx.Export(OpBucketUrl, pulumi.Sprintf("%s/%s", s3Endpoint, createdBucket.Name))
	ctx.Export(OpCustomDomainUrls, customDomainUrls)
	if managedDomain != nil {
		ctx.Export(OpPublicUrl, managedDomain.Domain.ApplyT(func(d string) string {
			return "https://" + d
		}).(pulumi.StringOutput))
	} else {
		ctx.Export(OpPublicUrl, pulumi.String(""))
	}

	return createdBucket, nil
}

// toStringArray converts a Go string slice into a pulumi.StringArray input.
func toStringArray(in []string) pulumi.StringArray {
	out := pulumi.StringArray{}
	for _, s := range in {
		out = append(out, pulumi.String(s))
	}
	return out
}
