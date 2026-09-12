package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	wafv1 "github.com/plantonhq/planton/catalog/aws/awswafwebacl/v1alpha1"
	eventhubnsv1 "github.com/plantonhq/planton/catalog/azure/azureeventhubnamespace/v1alpha1"
	composerv1 "github.com/plantonhq/planton/catalog/gcp/gcpcloudcomposerenvironment/v1alpha1"
	costestimatev1 "github.com/plantonhq/planton/finops/componentcostestimate/v1"
	"github.com/plantonhq/planton/pkg/finops/costderivation"
	"github.com/plantonhq/planton/pkg/finops/costestimator"
	"github.com/plantonhq/planton/pkg/finops/pricebook"
	"github.com/plantonhq/planton/pkg/protobufyaml"
	foreignkeyv1 "github.com/plantonhq/planton/shared/foreignkey/v1"
)

// TestPresetReplayHonesty pins the honesty semantics of derivation-driven
// estimates -- the behaviors that make a replayed estimate trustworthy and
// that a careless derivation edit could silently lose. Every pin is
// price-refresh-proof: quantities, preset sets, and line presence are
// functions of the presets and the rules, never of the volatile dollar
// values.
//
//   - A preset whose configuration the derivation cannot price (the
//     launch-template node group) is ABSENT -- never a guessed number.
//   - A scale-to-zero Spot pool prices to the literal "0.00" with no
//     line items and its market-rate story in the exclusions.
//   - Same-meter same-price lines merge (the warm pool's floor and
//     pooled root volumes are ONE line whose quantity is the sum).
//   - A read replica prices under STATED assumptions (the basis says
//     "assumed"), and conditional lines exist exactly for the
//     configurations that incur them (no public-IPv4 line on an
//     internal load balancer).
func TestPresetReplayHonesty(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "catalog")); err != nil {
		t.Skip("catalog source tree not present (bazel sandbox); runs under go test and the lint.catalog-data lane")
	}
	if derived, err := costderivation.Discover(root); err != nil || len(derived) == 0 {
		t.Skip("no cost derivations authored yet")
	}

	summary, err := Generate(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(summary.Problems) > 0 {
		t.Fatalf("coherence problems: %v", summary.Problems)
	}

	t.Run("awseksnodegroup", func(t *testing.T) {
		presets := generatedPresets(t, summary, "awseksnodegroup")

		if _, exists := presets["03-launch-template"]; exists {
			t.Error("the launch-template preset is priced -- its rate is delegated to the referenced AwsLaunchTemplate and must be refused")
		}

		spot, ok := presets["02-spot-cost-optimized"]
		if !ok {
			t.Fatal("the spot preset is missing -- scale-to-zero must price to an honest 0.00, not disappear")
		}
		if len(spot.GetLineItems()) != 0 || spot.GetTotalListCost() != "0.00" {
			t.Errorf("spot preset: want zero lines and total 0.00, got %d lines and %q",
				len(spot.GetLineItems()), spot.GetTotalListCost())
		}

		warm, ok := presets["04-warm-pool"]
		if !ok {
			t.Fatal("the warm-pool preset is missing")
		}
		volumes := linesForMeter(warm, "node root volumes")
		if len(volumes) != 1 {
			t.Fatalf("warm pool root volumes: want ONE merged line, got %d", len(volumes))
		}
		if volumes[0].GetPricingQuantity() != "400" {
			t.Errorf("warm pool root volumes quantity: got %q, want 400 (2 floor + 2 pooled nodes x 100 GiB)",
				volumes[0].GetPricingQuantity())
		}
	})

	t.Run("awsrdsinstance", func(t *testing.T) {
		presets := generatedPresets(t, summary, "awsrdsinstance")
		replica, ok := presets["03-read-replica"]
		if !ok {
			t.Fatal("the read-replica preset is missing -- it prices under stated assumptions")
		}
		for _, line := range replica.GetLineItems() {
			if line.GetSkuMeter() == "instance hours" && !strings.Contains(line.GetQuantityBasis(), "assumed") {
				t.Errorf("replica instance line basis does not state its engine assumption: %q", line.GetQuantityBasis())
			}
		}
		storage := linesForMeter(replica, "provisioned storage")
		if len(storage) != 1 || storage[0].GetPricingQuantity() != "50" {
			t.Errorf("replica storage: want one line of 50 GB-months (the stated assumption), got %+v", storage)
		}
	})

	t.Run("awsalb", func(t *testing.T) {
		presets := generatedPresets(t, summary, "awsalb")
		if internal, ok := presets["02-internal-hardened"]; !ok {
			t.Fatal("the internal preset is missing")
		} else if lines := linesForMeter(internal, "public IPv4 address usage"); len(lines) != 0 {
			t.Error("an internal load balancer wears a public IPv4 line -- the conditional rule leaked")
		}
		if facing, ok := presets["01-internet-facing"]; !ok {
			t.Fatal("the internet-facing preset is missing")
		} else if lines := linesForMeter(facing, "public IPv4 address usage"); len(lines) != 1 || lines[0].GetPricingQuantity() != "1460" {
			t.Errorf("internet-facing IPv4: want one line of 1460 IP-hours (2 subnets x 730), got %+v", lines)
		}
	})

	t.Run("awsapprunnerservice", func(t *testing.T) {
		// The reference-presence fixture: a preset whose committed floor
		// rides a REFERENCED auto-scaling configuration refuses by the
		// reference's presence -- never priced at the account-default
		// floor of 1, which would be knowably wrong.
		presets := generatedPresets(t, summary, "awsapprunnerservice")
		if _, exists := presets["02-production-vpc-encrypted"]; exists {
			t.Error("the ASC-referencing preset is priced -- its floor lives on the referenced AwsAppRunnerAutoScalingConfiguration and must be refused")
		}
		public, ok := presets["01-basic-public-image"]
		if !ok {
			t.Fatal("the basic preset is missing -- no reference, so the account-default floor prices under a stated assumption")
		}
		if lines := linesForMeter(public, "provisioned instance memory"); len(lines) != 1 || lines[0].GetPricingQuantity() != "1460" {
			t.Errorf("basic preset memory: want one line of 1460 GB-hours (2048 MB / 1024 x 730), got %+v", lines)
		}
	})

	t.Run("azureeventhubnamespace", func(t *testing.T) {
		// The reference-presence fixture on the refusal side: a namespace
		// wired onto a dedicated cluster BY REFERENCE bills through the
		// cluster's capacity units, so the committed derivation must
		// refuse it rather than pricing tier meters the placement
		// supersedes. No committed preset places on a cluster, so the
		// pin evaluates the committed rules against a synthetic
		// cluster-placed manifest.
		derivation, err := costderivation.Load(root, "azureeventhubnamespace")
		if err != nil {
			t.Fatalf("loading the committed derivation: %v", err)
		}
		entries, err := pricebook.Entries(root, "azure")
		if err != nil {
			t.Fatalf("loading the azure price book: %v", err)
		}
		manifest := &eventhubnsv1.AzureEventHubNamespace{Spec: &eventhubnsv1.AzureEventHubNamespaceSpec{
			Region: "eastus",
			Sku:    eventhubnsv1.AzureEventHubNamespaceSku_STANDARD,
			DedicatedClusterId: &foreignkeyv1.StringValueOrRef{
				LiteralOrRef: &foreignkeyv1.StringValueOrRef_ValueFrom{
					ValueFrom: &foreignkeyv1.ValueFromRef{Name: "shared-dedicated-cluster"},
				},
			},
		}}
		preset, refusal, err := costestimator.Evaluate(manifest, derivation.GetSpec(), entries)
		if err != nil || preset != nil {
			t.Fatalf("Evaluate(cluster-placed): err=%v preset=%+v, want a refusal", err, preset)
		}
		if refusal == nil || !strings.Contains(refusal.Reason, "dedicated cluster") {
			t.Errorf("a cluster-placed namespace must refuse naming the cluster placement, got %+v", refusal)
		}
	})

	t.Run("awselasticip", func(t *testing.T) {
		// The reference-presence semantics: an address attached to an
		// instance BY REFERENCE is attached -- it wears the in-use meter
		// basis, not the idle one.
		presets := generatedPresets(t, summary, "awselasticip")
		attached, ok := presets["03-instance-attached"]
		if !ok {
			t.Fatal("the instance-attached preset is missing")
		}
		lines := linesForMeter(attached, "public IPv4 address usage")
		if len(lines) != 1 {
			t.Fatalf("instance-attached IPv4: want one line, got %d", len(lines))
		}
		if !strings.Contains(lines[0].GetQuantityBasis(), "associated with an instance") {
			t.Errorf("instance-attached basis reads %q -- a reference-attached address must price as in-use, not idle", lines[0].GetQuantityBasis())
		}
	})

	t.Run("kubernetespostgres", func(t *testing.T) {
		// The capacity standard's acceptance oracle: replaying the presets
		// through the capacity derivation reproduces the hand-verified
		// footprints -- summed replicas, merged data+WAL storage, no
		// priced lines, and the honesty exclusion on every preset.
		presets := generatedPresets(t, summary, "kubernetespostgres")
		// Every shipped preset replays -- counted from the preset directory,
		// not pinned, so a new preset cannot leave this assertion stale (it
		// sat at 3 while the kind shipped 4).
		shipped, err := filepath.Glob(filepath.Join(root, "catalog/kubernetes/kubernetespostgres/presets/*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(presets) != len(shipped) {
			t.Fatalf("kubernetespostgres: want all %d shipped presets, got %d", len(shipped), len(presets))
		}
		ha, ok := presets["02-production-ha"]
		if !ok {
			t.Fatal("the production-HA preset is missing")
		}
		footprint := ha.GetCapacityFootprint()
		for _, check := range []struct{ name, got, want string }{
			{"cpu_requests", footprint.GetCpuRequests(), "3"},
			{"memory_requests", footprint.GetMemoryRequests(), "6Gi"},
			{"cpu_limits", footprint.GetCpuLimits(), "6"},
			{"memory_limits", footprint.GetMemoryLimits(), "12Gi"},
			{"persistent_storage", footprint.GetPersistentStorage(), "360Gi"},
		} {
			if check.got != check.want {
				t.Errorf("production-HA %s: got %q, want %q", check.name, check.got, check.want)
			}
		}
		for key, preset := range presets {
			if len(preset.GetLineItems()) != 0 || preset.GetTotalListCost() != "" {
				t.Errorf("%s: cluster-capacity presets carry no priced lines or totals", key)
			}
			if len(preset.GetExclusions()) == 0 {
				t.Errorf("%s: a footprint without exclusions hides that its dollar value is the cluster's economics", key)
			}
		}
	})

	t.Run("awsvpc", func(t *testing.T) {
		presets := generatedPresets(t, summary, "awsvpc")
		if len(presets) != 3 {
			t.Fatalf("awsvpc: want all 3 presets priced, got %d", len(presets))
		}
		for key, preset := range presets {
			if len(preset.GetLineItems()) != 0 || preset.GetTotalListCost() != "0.00" {
				t.Errorf("%s: a VPC is free -- want zero lines and total 0.00, got %d lines and %q",
					key, len(preset.GetLineItems()), preset.GetTotalListCost())
			}
			if len(preset.GetExclusions()) == 0 {
				t.Errorf("%s: a zero estimate without exclusions hides where the network's real spend lives", key)
			}
		}
	})

	t.Run("awsrdscluster", func(t *testing.T) {
		// The repeated-message expansion's acceptance oracle: each element
		// of the instances list prices its own class, identical elements
		// merge to one line whose quantity is the sum, and a Serverless v2
		// cluster at min_capacity 0 is the honest zero (its serverless
		// instances are filtered, not priced and not refused).
		presets := generatedPresets(t, summary, "awsrdscluster")
		if len(presets) != 4 {
			t.Fatalf("awsrdscluster: want all 4 presets, got %d", len(presets))
		}
		if ha, ok := presets["01-aurora-postgresql"]; !ok {
			t.Fatal("the aurora-postgresql preset is missing")
		} else if lines := linesForMeter(ha, "instance hours"); len(lines) != 1 || lines[0].GetPricingQuantity() != "1460" {
			t.Errorf("aurora-postgresql instance hours: want ONE merged line of 1460 (2 identical instances x 730), got %+v", lines)
		}
		if single, ok := presets["04-mysql-s3-migration"]; !ok {
			t.Fatal("the s3-migration preset is missing")
		} else if lines := linesForMeter(single, "instance hours"); len(lines) != 1 || lines[0].GetPricingQuantity() != "730" {
			t.Errorf("s3-migration instance hours: want one line of 730 (1 writer), got %+v", lines)
		}
		if serverless, ok := presets["03-aurora-serverless-v2"]; !ok {
			t.Fatal("the serverless-v2 preset is missing -- min_capacity 0 must price to an honest 0.00, not disappear")
		} else if len(serverless.GetLineItems()) != 0 || serverless.GetTotalListCost() != "0.00" {
			t.Errorf("serverless-v2: want zero lines and total 0.00, got %d lines and %q",
				len(serverless.GetLineItems()), serverless.GetTotalListCost())
		}
	})

	t.Run("awsdocumentdb", func(t *testing.T) {
		// The serverless floor prices through a root rule while the
		// expanded rule filters the db.serverless elements: min_capacity
		// 0.5 x 730 = 365 DCU-hours -- the committed spend of a service
		// that never pauses to zero.
		presets := generatedPresets(t, summary, "awsdocumentdb")
		if serverless, ok := presets["02-serverless"]; !ok {
			t.Fatal("the serverless preset is missing")
		} else {
			if lines := linesForMeter(serverless, "serverless capacity"); len(lines) != 1 || lines[0].GetPricingQuantity() != "365" {
				t.Errorf("serverless capacity: want one line of 365 (0.5 DCU floor x 730), got %+v", lines)
			}
			if lines := linesForMeter(serverless, "instance hours"); len(lines) != 0 {
				t.Error("the db.serverless instance leaked into the provisioned instance-hours rule")
			}
		}
	})

	t.Run("awswafwebacl", func(t *testing.T) {
		// The existential-condition fixture: a rule referencing a
		// subscription-bearing managed rule group -- an identity buried
		// inside the repeated rules tree -- must refuse the whole
		// estimate. No committed preset references one, so the pin
		// evaluates the committed rules against a synthetic manifest.
		presets := generatedPresets(t, summary, "awswafwebacl")
		if len(presets) != 4 {
			t.Fatalf("awswafwebacl: want all 4 presets priced, got %d", len(presets))
		}
		if prod, ok := presets["03-production-web-app"]; !ok {
			t.Fatal("the production preset is missing")
		} else if lines := linesForMeter(prod, "rule monthly fee"); len(lines) != 1 || lines[0].GetPricingQuantity() != "7" {
			t.Errorf("production rule fee: want one line of 7 (one fee per top-level rule), got %+v", lines)
		}

		derivation, err := costderivation.Load(root, "awswafwebacl")
		if err != nil {
			t.Fatalf("loading the committed derivation: %v", err)
		}
		entries, err := pricebook.Entries(root, "aws")
		if err != nil {
			t.Fatalf("loading the aws price book: %v", err)
		}
		manifest := &wafv1.AwsWafWebAcl{Spec: &wafv1.AwsWafWebAclSpec{
			Region: "us-east-1",
			Rules: []*wafv1.AwsWafWebAclRule{{
				Name: "bot-control",
				Statement: &wafv1.AwsWafWebAclStatement{
					Statement: &wafv1.AwsWafWebAclStatement_ManagedRuleGroup{
						ManagedRuleGroup: &wafv1.AwsWafWebAclManagedRuleGroupStatement{
							Name:       "AWSManagedRulesBotControlRuleSet",
							VendorName: "AWS",
						},
					},
				},
			}},
		}}
		preset, refusal, err := costestimator.Evaluate(manifest, derivation.GetSpec(), entries)
		if err != nil || preset != nil {
			t.Fatalf("Evaluate(bot control): err=%v preset=%+v, want a refusal", err, preset)
		}
		if refusal == nil || !strings.Contains(refusal.Reason, "subscription") {
			t.Errorf("a Bot Control rule must refuse naming the subscription, got %+v", refusal)
		}
	})

	t.Run("awsebsvolume", func(t *testing.T) {
		// The subtract-baseline acceptance oracle: only the excess over
		// gp3's included allotments bills (6,000 - 3,000 IOPS; 250 - 125
		// MiB/s), and the snapshot-restored preset stays honestly absent
		// (its size is the referenced snapshot's, unknowable here).
		presets := generatedPresets(t, summary, "awsebsvolume")
		if _, exists := presets["02-snapshot-restore"]; exists {
			t.Error("the snapshot-restore preset is priced -- its committed size is the referenced snapshot's and must be refused")
		}
		data, ok := presets["01-database-data-volume"]
		if !ok {
			t.Fatal("the data-volume preset is missing")
		}
		if lines := linesForMeter(data, "provisioned IOPS"); len(lines) != 1 || lines[0].GetPricingQuantity() != "3000" {
			t.Errorf("IOPS excess: want one line of 3000 (6000 - 3000), got %+v", lines)
		}
		if lines := linesForMeter(data, "provisioned throughput"); len(lines) != 1 || lines[0].GetPricingQuantity() != "125" {
			t.Errorf("throughput excess: want one line of 125 (250 - 125), got %+v", lines)
		}
	})

	t.Run("awsec2instance", func(t *testing.T) {
		// The refusal-to-surcharge conversion: the dialed-up root volume
		// prices its throughput excess ((1000 - 125) MiB/s) instead of
		// refusing, while its IOPS dial -- unset, under the baseline --
		// honestly contributes no line at all.
		presets := generatedPresets(t, summary, "awsec2instance")
		ml, ok := presets["05-capacity-block-ml"]
		if !ok {
			t.Fatal("the capacity-block preset is missing -- the throughput dial must price its excess, not refuse")
		}
		if lines := linesForMeter(ml, "provisioned throughput"); len(lines) != 1 || lines[0].GetPricingQuantity() != "875" {
			t.Errorf("throughput excess: want one line of 875 (1000 - 125), got %+v", lines)
		}
		if lines := linesForMeter(ml, "provisioned IOPS"); len(lines) != 0 {
			t.Error("an unset IOPS dial is under the included baseline and must contribute no line")
		}
	})

	t.Run("gcpcomputedisk", func(t *testing.T) {
		// The type-keyed capacity lookup plus the Hyperdisk subtractions:
		// each preset's disk type picks its own rate, and only the excess
		// over the included baselines bills.
		presets := generatedPresets(t, summary, "gcpcomputedisk")
		if len(presets) != 3 {
			t.Fatalf("gcpcomputedisk: want all 3 presets priced, got %d", len(presets))
		}
		hyper, ok := presets["03-hyperdisk-high-iops"]
		if !ok {
			t.Fatal("the hyperdisk preset is missing")
		}
		if lines := linesForMeter(hyper, "provisioned IOPS"); len(lines) != 1 || lines[0].GetPricingQuantity() != "3000" {
			t.Errorf("hyperdisk IOPS excess: want one line of 3000 (6000 - 3000), got %+v", lines)
		}
		if lines := linesForMeter(hyper, "provisioned throughput"); len(lines) != 1 || lines[0].GetPricingQuantity() != "150" {
			t.Errorf("hyperdisk throughput excess: want one line of 150 (290 - 140), got %+v", lines)
		}
	})

	t.Run("kuberneteskeycloak", func(t *testing.T) {
		// The spec-declared-defaults fixture: the dev-sandbox preset
		// omits resources entirely, and the modules apply the spec's own
		// (default_container_resources) annotation at deploy time -- the
		// footprint must state exactly those values with their origin
		// named, never a zero reservation and never a refusal.
		presets := generatedPresets(t, summary, "kuberneteskeycloak")
		sandbox, ok := presets["02-dev-sandbox"]
		if !ok {
			t.Fatal("the dev-sandbox preset is missing -- omitted resources must resolve to the spec-declared defaults, not refuse")
		}
		footprint := sandbox.GetCapacityFootprint()
		for _, check := range []struct{ name, got, want string }{
			{"cpu_requests", footprint.GetCpuRequests(), "250m"},
			{"memory_requests", footprint.GetMemoryRequests(), "768Mi"},
			{"cpu_limits", footprint.GetCpuLimits(), "1"},
			{"memory_limits", footprint.GetMemoryLimits(), "1Gi"},
		} {
			if check.got != check.want {
				t.Errorf("dev-sandbox %s: got %q, want %q (the spec's own annotation)", check.name, check.got, check.want)
			}
		}
		if !strings.Contains(footprint.GetBasis(), "spec-declared defaults") {
			t.Errorf("dev-sandbox basis %q does not name the defaults' origin", footprint.GetBasis())
		}
	})

	t.Run("gcpcloudcomposerenvironment", func(t *testing.T) {
		// The starts_with fixture: a manifest pinning a composer-3 image
		// -- undetectable by any structural signal -- must refuse by the
		// version-family prefix. No committed preset pins one, so the pin
		// evaluates the committed rules against a synthetic manifest.
		derivation, err := costderivation.Load(root, "gcpcloudcomposerenvironment")
		if err != nil {
			t.Fatalf("loading the committed derivation: %v", err)
		}
		entries, err := pricebook.Entries(root, "gcp")
		if err != nil {
			t.Fatalf("loading the gcp price book: %v", err)
		}
		manifest := &composerv1.GcpCloudComposerEnvironment{Spec: &composerv1.GcpCloudComposerEnvironmentSpec{
			Region: "us-central1",
			SoftwareConfig: &composerv1.GcpCloudComposerSoftwareConfig{
				ImageVersion: "composer-3-airflow-2.9.3",
			},
		}}
		preset, refusal, err := costestimator.Evaluate(manifest, derivation.GetSpec(), entries)
		if err != nil || preset != nil {
			t.Fatalf("Evaluate(composer-3): err=%v preset=%+v, want a refusal", err, preset)
		}
		if refusal == nil || !strings.Contains(refusal.Reason, "composer-3") {
			t.Errorf("a composer-3-pinned manifest must refuse naming the version family, got %+v", refusal)
		}
	})
}

// generatedPresets parses one component's generated estimate document out
// of the in-memory summary, keyed by preset stem.
func generatedPresets(t *testing.T, summary *Summary, component string) map[string]*costestimatev1.PresetEstimate {
	t.Helper()
	content, ok := summary.Files[filepath.Join("catalog/_pricing/estimates", component+".yaml")]
	if !ok {
		t.Fatalf("no generated estimate for %s", component)
	}
	estimate := &costestimatev1.ComponentCostEstimate{}
	if err := protobufyaml.LoadYamlBytes([]byte(content), estimate); err != nil {
		t.Fatalf("parsing generated estimate for %s: %v", component, err)
	}
	presets := map[string]*costestimatev1.PresetEstimate{}
	for _, preset := range estimate.GetSpec().GetPresets() {
		presets[preset.GetPreset()] = preset
	}
	return presets
}

// linesForMeter returns a preset's line items carrying one meter.
func linesForMeter(preset *costestimatev1.PresetEstimate, meter string) []*costestimatev1.LineItem {
	var lines []*costestimatev1.LineItem
	for _, line := range preset.GetLineItems() {
		if line.GetSkuMeter() == meter {
			lines = append(lines, line)
		}
	}
	return lines
}
