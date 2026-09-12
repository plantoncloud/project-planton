# Component Grounding

How to discover cloud resource kinds and learn the exact field names, types,
validation rules, and stack outputs before writing a single line of template
YAML. Grounding is offline-first: you never need a control plane or login to
author a chart correctly.

## Two instruments, one division of work

- **The multi-cloud-catalog skill is the research layer.** For discovery
  ("what exists for this provider?"), component depth (required fields,
  outputs, a validated example), wiring questions in BOTH directions ("what
  can reference this?"), and per-component judgment, read the catalog
  skill's reference pack — its own SKILL.md names which file answers which
  question and in what order. Whole-catalog questions that would take a
  dozen serial lookups resolve there in one or two file reads.
- **`planton explain` is the drill-down and recovery oracle.** Reach for it
  to drill one field's exact contract, to match a BUILD ERROR's field path
  to the schema (never guess a correction), and as the complete grounding
  path whenever no catalog pack is reachable — it is compiled into the CLI
  and works fully offline.

**On the platform-tools arm (no CLI), the same division holds with one
substitution**: the drill-down instrument is the component's own generated
reference page, read from a pack in your tree or fetched into your workspace
(the catalog skill's pack-layout reference names the sources — the fetched
page carries the same schema-derived facts the explain report renders). The
platform's catalog tools (`search_catalog_components`,
`get_catalog_component`) answer DISCOVERY — what exists, by which name —
never field-level facts: they return catalog entries, not schemas. What
exists is deliberately not the same question as what the user's
organization has ENABLED: discovery serves the full release catalog on
purpose, and `get_catalog_availability` answers what the organization's
catalog policy disables — the check-design-disclose law in
`catalog-availability.md` governs when each question matters. And the
compile loop doubles as the recovery oracle exactly as on the CLI arm: the
build report's issues carry the schema's own verdicts on every field path
you got wrong.

**Research never justifies leaving your filesystem boundary.** Probe for
the catalog pack only inside the tree you were given — the workspace and
whatever your session already mounts. Your skill mount (the `.stigmer/`
folder beside your workspace) holds skill files only — finding it never
licenses a search of the host's `.stigmer` home. Never hunt the wider
machine for pack files: no searching home directories (`~/.stigmer`
included), tool install paths, or other checkouts that happen to exist on
the host. A pack that is not reachable inside the boundary IS the no-pack
case — take the explain path without commentary and move on.

## The explain toolchain

| Step | Command | When |
|------|---------|------|
| Discover kinds | `planton explain --list` | You need a PascalCase kind name (e.g. `AwsVpc`) and the pack is not reachable |
| Read a kind's schema | `planton explain AwsVpc` | Before writing or fixing any manifest of that kind |
| Drill into one field | `planton explain aws-vpc.spec.instanceTenancy` | You need a field's allowed values or exact contract |
| JSON for parsing | `planton explain AwsVpc -o json` | When you need to programmatically scan fields or outputs |
| Validate a draft manifest | `planton validate draft.yaml` | After writing one resource in isolation (offline) |
| Compile the whole chart | `planton chart build <dir> -o json` | After every edit batch (needs control plane) |

`planton explain` is fully offline — the definitions AND their documentation
are compiled into the CLI binary. It is the authoritative grounding surface
for chart authors, and it covers platform API resources too (`planton explain
infra-chart` is the chart manifest's own schema, with fields the tooling
assembles labeled so you never hand-author them).
(`planton cloud-resource schema <Kind>` still works as an alias that renders
the same reference for cloud resource kinds.)

## Reading an explain report

Example (abbreviated):

```text
KIND:    AwsVpc
VERSION: aws.planton.dev/v1alpha1

DESCRIPTION:
  AwsVpcSpec defines an Amazon VPC ...

MANIFEST:
  apiVersion: aws.planton.dev/v1alpha1
  kind: AwsVpc
  metadata:
    name: <resource-name>
  spec:
    <the fields below>

RULES:
  - exactly one of cidrBlock or an IPAM-based source must be set

SPEC:
  cidrBlock <string>
    The primary IPv4 CIDR block for the VPC ...

  instanceTenancy <enum>
    values: default, dedicated

OUTPUTS (reference as valueFrom fieldPath: status.outputs.<name>):
  vpcId <string>
    The VPC's identifier ...
```

What each section means:

- **MANIFEST** — the envelope skeleton: exactly what the top level of your
  YAML must look like.
- **SPEC** — every field you can write under `spec:` in YAML. The name is
  the exact key (protojson camelCase — `cidrBlock`, not `cidr_block`).
  `-required-` means the build fails if you omit it. `values:` lists the only
  legal enum spellings — use them exactly as shown. Fields typed
  `string | valueFrom` accept either a `{value: <literal>}` or a
  cross-resource reference block (see `dependencies.md`); a bare string does
  not parse. Fields marked `(assembled)` or `(computed)` are filled by
  tooling or the platform — never write them by hand.
- **RULES** — cross-field constraints. Read these before combining fields in
  non-obvious ways.
- **OUTPUTS** — what this kind publishes after deployment. These are the
  leaves you use in `fieldPath: status.outputs.<name>` when wiring another
  resource to this one.

When the build rejects a field, re-run `planton explain <Kind>` (or drill
straight to the reported path: `planton explain <Kind>.spec.<field>`) and
match the error message's field path to the report — never guess a
correction. Manifest load errors themselves point back at `planton explain`
with the exact path to look up.

## Deeper grounding (when the explain report is not enough)

All of these live in the open-source repo `github.com/plantonhq/planton`.
Read them from a local clone when one exists; otherwise fetch raw files
directly, e.g.
`https://raw.githubusercontent.com/plantonhq/planton/main/catalog/aws/awsvpc/v1alpha1/spec.proto`
(prefix each path below with
`https://raw.githubusercontent.com/plantonhq/planton/main/`). No clone and
no network? The explain report alone is sufficient for correct composition.

1. **The generated reference page** — the pack page itself:
   `catalog/<provider>/<component>/<api-version>/reference.md`. On the
   platform-tools arm this fetch is not "deeper" grounding — it IS the
   grounding path (the explain report's equal, rendered from the same
   schemas). Fetch it into your workspace and read it there.
2. **Official presets** — worked, valid manifests for common shapes:
   `catalog/<provider>/<component>/presets/` (at the component root).
   Read a preset when you need a realistic starting point for a kind you
   have not authored before.
3. **Catalog pages** — the human-oriented doc at each component's root:
   `catalog/<provider>/<component>/catalog.md`. Good for topology context
   ("this resource usually sits behind a VPC") but the explain report is
   the field-name authority.
4. **Chart fleet** — production charts that wire many kinds together:
   `charts/<provider>/<chart-name>/` in the same repo.
   Copy wiring patterns from a chart that already solves a similar topology.
5. **Proto source** — when you need edge cases beyond the rendered report:
   `catalog/<provider>/<component>/v1alpha1/spec.proto` and
   `…/v1alpha1/outputs.proto`. Same field names and the same comments — the
   explain report is generated from these files, so the report normally
   suffices.
6. **The module that applies it** — the last rung, and the only absolute
   truth: the Terraform module at `catalog/<provider>/<component>/iac/tf/`
   and the Pulumi module at `…/iac/pulumi/module/` are the code that turns
   the manifest into cloud resources. Read them when the question is what a
   field DOES rather than what it is named — which provider argument it
   feeds, what a blank value falls back to, which output is written from
   which resource attribute, whether two fields interact. The explain
   report and the reference page describe the contract; the module is the
   contract's implementation, and when the two disagree the module is what
   will run. Take a sparse checkout rather than the whole repository:

   ```
   git clone --depth 1 --filter=blob:none --sparse --branch <release> \
     https://github.com/plantonhq/planton <dir>
   git -C <dir> sparse-checkout set catalog/<provider>/<component>
   ```

   `<release>` is the version whose pack you resolved — the tag in the
   release-artifact URL, or the tag of the skills release you installed
   from — because the runner applies the module BINARY published for that
   release, not `main`. When the release is not knowable, read `main` and
   say so: a module newer than the one that will run can describe a field
   the deployment does not have yet.

## Composition doctrine (per-component knowledge)

You do **not** install hundreds of component skills — one workflow skill
(this one) plus the multi-cloud-catalog research skill cover the whole
catalog. Component facts and per-component wisdom live in the catalog
skill's pack and are READ AT ANSWER TIME, never recalled from memory —
whatever you remember about a component's schema is stale by construction.
When no pack is reachable, the explain report plus a preset and a fleet
example for the kind is the complete degradation path.

## What not to use

- **Search or catalog tools/APIs** for field names — they return metadata
  and links, not inline schemas. They answer "what exists"; the field
  authority is the explain report (CLI arm) or the component's reference
  page (platform-tools arm).
- **Guessing from AWS/Azure/GCP docs** — Planton kinds wrap provider APIs with
  their own field names and validation. Always ground from the explain report
  or the reference page.
- **Web UIs** — the file-based instruments above are the contract this skill
  is written against; a screenshot of a console is never a schema.
