# Pack Layout and Where to Find It

The reference pack is one directory tree. When this skill ships
self-contained, the pack travels inside it and sits beside this file in
`components/` -- no repo checkout, network fetch, or marker hunt needed.
Probe for that directory first; when it is absent, the ladder below
resolves the pack from where you are working.

## The shape

```
<pack-root>/
├── _docs/
│   ├── reference-index.md        # root index: provider table with kind,
│   │                             # example, and guide counts (live numbers)
│   ├── reference-commons.md      # the manifest grammar every component
│   │                             # shares + the pack's search grammar
│   ├── reference-graph.yaml      # every foreign-key edge in the catalog
│   └── GUIDE.md                  # catalog-level wisdom: the substitution
│                                 # workflow and verified alternatives
├── _patterns/                    # architecture patterns (README.md lists
│                                 # them; one .md per pattern, each with
│                                 # validated manifests)
├── _pricing/
│   └── estimates/
│       └── <kinddir>.yaml        # GENERATED per-preset monthly estimates
│                                 # (kinddir = kind lowercased, awsalb.yaml);
│                                 # exact decimal strings, price source URL +
│                                 # verification date on every line -- present
│                                 # only for covered components
├── _compliance/
│   ├── controls-catalog.yaml     # the central control catalog: every
│   │                             # control's id, name, and statement
│   └── frameworks/
│       └── <framework>.yaml      # crosswalks (hipaa-security-rule, soc2,
│                                 # fedramp, cis-aws)
│                                 # mapping framework requirements to controls;
│                                 # spec.providers declares provider scope
│                                 # (empty = provider-neutral)
└── <provider>/                   # aws/, gcp/, azure/, kubernetes/, ...
    ├── reference-index.md        # every kind in the provider, one line each
    └── <kind>/
        ├── GUIDE.md              # authored judgment -- present only where
        │                         # wisdom has been written
        ├── cost.yaml             # fact-sheet: billing model, baseline
        │                         # charges, cost drivers, exclusions --
        │                         # present only for covered components
        ├── controls.yaml         # fact-sheet: posture on every catalog
        │                         # control, with evidence
        ├── iac/
        │   └── permissions.yaml  # fact-sheet: least-privilege runner
        │                         # permissions, derived/proven provenance
        └── <api-version>/
            └── reference.md      # the component's complete reference page
```

## Finding the pack, in order

Every probe below stays inside the filesystem your session already grants
-- the working tree, an attached workspace, or a mount your tools handed
you. Never search the wider machine for pack files (home directories --
`~/.stigmer` included -- tool install paths, other checkouts that happen
to exist on the host): a pack that is not reachable inside that boundary
is simply not reachable -- take the fallback below.

1. **Your own skill mount** -- probe for `components/` in the same
   directory as this skill's `SKILL.md`. On a hit it is the pack root:
   the skill and its pack are one artifact, published and versioned
   together, so it is always exactly as fresh as the skill teaching you
   to read it. On a miss, continue down this ladder -- never search the
   host for it.
2. **A Planton open-source repo checkout** -- the pack root is `catalog/`
   under the repo root. Prefer it over the mount only when you are working
   IN the repo (contributing, or reviewing unreleased changes): a checkout
   may carry pages newer than any published skill.
3. **The release artifact** -- every Planton release also publishes the
   pack as one zip:

   ```
   https://downloads.planton.dev/releases/<version>/content/reference-pack.zip
   ```

   Entries carry repo-relative paths, so after extracting into an empty
   directory the pack root is `<dir>/catalog/`. Useful for tools that want
   the pack without the skill; as a mounted agent you should never need it.

## When no pack is reachable

Two per-component fallbacks, in order of preference:

1. **`planton explain <Kind>`** (when the CLI exists where you run) prints
   the same schema-derived facts for one component at a time -- fields,
   validation rules, outputs, outbound references -- fully offline.
2. **Fetch the component's pages into your workspace** (when you have
   network but no CLI): the generated reference page and its neighbors live
   at stable public paths --

   ```
   https://raw.githubusercontent.com/plantonhq/planton/main/catalog/<provider>/<kind>/<api-version>/reference.md
   ```

   Download into the workspace you already own and read there -- the fetch
   stays inside your filesystem boundary, and the page carries the same
   facts the pack ships. The provider index
   (`catalog/<provider>/reference-index.md`), the commons page, and the
   fact-sheet files (`catalog/<provider>/<kind>/cost.yaml`,
   `.../controls.yaml`, `.../iac/permissions.yaml`,
   `catalog/_pricing/estimates/<kinddir>.yaml`,
   `catalog/_compliance/...`) fetch the same way when cost, posture, or
   permission questions arise. Prefer fetching only the pages the answer
   needs over mirroring the tree.

A third rung exists for one question the pack itself cannot settle -- what
a field DOES when it is applied, as opposed to what it is named and how it
is validated: the module source. Every component ships the Terraform
module (`catalog/<provider>/<kind>/iac/tf/`) and the Pulumi module
(`catalog/<provider>/<kind>/iac/pulumi/module/`) that turn its manifest
into cloud resources, and the runner applies the module published for a
release. When a pack page and a live deployment disagree, or a page is
silent on how a value is used, read the module -- from a sparse checkout
pinned to the release whose pack you resolved (the tag in the
release-artifact URL, or of the skills release you installed from), never
quietly from `main`, which may already describe a field the deployment
does not have. The workflow skill's grounding reference carries the exact
checkout commands.

Be honest about what per-component fallbacks cannot give you: full-text
capability search, the inbound `Referenced By` view, the catalog-wide graph,
and the authored guides and patterns (the fetched index partially covers
"what exists"). `planton explain` carries schema facts only -- no prices,
postures, or permissions; those live in the fact-sheet files, so when no
rung reaches them, say the verified cost/posture/permission data is not
reachable rather than reciting figures from memory. When an answer would
have needed any of these, say so instead of approximating.

## Freshness

The pack is regenerated from the schemas and shipped whole: within one
skill mount, one checkout, or one release zip, every page, index, and the
graph are mutually consistent. Never combine files from two different pack
versions in one answer -- resolve one pack root and stay in it. A pack
mounted inside the skill matches the skill's version by construction; only
a repo checkout can legitimately be newer.
