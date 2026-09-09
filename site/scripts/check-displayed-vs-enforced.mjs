/**
 * The displayed-vs-enforced guard: what the pricing page displays must agree
 * with what the platform enforces, mechanically, at build time.
 *
 * Three laws:
 *
 * 1. VOCABULARY -- every `entitlementKey` in src/data/value-matrix.ts must
 *    be a registered platform entitlement key (the pinned list below
 *    mirrors the platform's registered vocabulary). A typo'd or invented
 *    key fails the build.
 *
 * 2. SOLD-WHEN-CLAIMED -- any key on a row displayed as INCLUDED on some
 *    plan must appear in the authored catalog's entitlement contents, read
 *    live from the sibling planton-platform checkout. A capability the
 *    pricing page claims as included that no offer actually sells is
 *    display lying about enforcement. Rows shown only as "Coming Soon" or
 *    prose text announce decided placement and are exempt.
 *
 * 3. LIMITS -- every seat count and day count the site states must equal
 *    the number the platform enforces: the free tier's seats and the
 *    license sizes' seats from the authored catalog, the evaluation's days
 *    from its catalog row, and the self-hosted community edition's seats
 *    from the edition's published entitlement set (the platform's own test
 *    pins that file to the binary). The numbers are read from
 *    src/data/pricing.ts -- the site's single pricing-truth module, which
 *    every card, matrix cell, and sentence reads -- so pinning the module
 *    pins every surface. A drift here once lived for weeks: the community
 *    ceiling changed in the platform and the page kept the old number.
 *
 * The platform half needs the planton-platform checkout beside this repo
 * (or PLANTON_PLATFORM_DIR); when it is absent (e.g. website-only CI) that
 * half is SKIPPED with a loud notice -- the authoritative gate is the local
 * `make build`, which runs where both checkouts exist.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';
import { parseAllDocuments } from 'yaml';

const GUARD = 'displayed-vs-enforced guard';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const matrixPath = path.join(repoRoot, 'src', 'data', 'value-matrix.ts');
const pricingPath = path.join(repoRoot, 'src', 'data', 'pricing.ts');

// Mirrors the platform's registered entitlement vocabulary (the entitlements
// knowledge article is the canonical home; the platform's catalog gate test
// carries the same pin). Extend ONLY when a key registers there first.
const REGISTERED_KEYS = new Set([
  'sso',
  'scim',
  'audit_export',
  'air_gap',
  'custom_module_registry',
  'deployment_safety',
  'access_transparency',
  'custom_roles',
  'impact_analysis',
]);

function fail(message) {
  console.error(`\u2717 ${GUARD}: ${message}`);
  process.exitCode = 1;
}

// ---------------------------------------------------------------------------
// Laws 1 + 2: the matrix's gated rows against the registered vocabulary and
// the authored catalog's feature keys.
// ---------------------------------------------------------------------------

/** Extract gated rows (entitlementKey + whether any cell renders included). */
function extractGatedRows(sourceText) {
  const source = ts.createSourceFile('value-matrix.ts', sourceText, ts.ScriptTarget.Latest, true);
  const rows = [];

  function cellRendersIncluded(node) {
    // A cell is "included" when it is the YES constant or an inline
    // `{ kind: 'included' }` literal; the `everywhere` preset is all-YES.
    if (ts.isIdentifier(node)) {
      return node.text === 'YES' || node.text === 'everywhere';
    }
    if (ts.isObjectLiteralExpression(node)) {
      return node.properties.some(
        (p) =>
          ts.isPropertyAssignment(p) &&
          p.name.getText(source) === 'kind' &&
          ts.isStringLiteralLike(p.initializer) &&
          p.initializer.text === 'included',
      );
    }
    return false;
  }

  function visit(node) {
    if (ts.isObjectLiteralExpression(node)) {
      let entitlementKey;
      let anyIncluded = false;
      for (const prop of node.properties) {
        if (!ts.isPropertyAssignment(prop)) continue;
        const name = prop.name.getText(source);
        if (name === 'entitlementKey' && ts.isStringLiteralLike(prop.initializer)) {
          entitlementKey = prop.initializer.text;
        }
        if (name === 'cells') {
          if (ts.isIdentifier(prop.initializer)) {
            anyIncluded = cellRendersIncluded(prop.initializer);
          } else if (ts.isObjectLiteralExpression(prop.initializer)) {
            anyIncluded = prop.initializer.properties.some(
              (cell) => ts.isPropertyAssignment(cell) && cellRendersIncluded(cell.initializer),
            );
          }
        }
      }
      if (entitlementKey) {
        rows.push({ entitlementKey, anyIncluded });
      }
    }
    ts.forEachChild(node, visit);
  }

  visit(source);
  return rows;
}

// ---------------------------------------------------------------------------
// Law 3: the pricing-truth module's numbers against the platform's.
// ---------------------------------------------------------------------------

/**
 * The exported numeric constants of pricing.ts, read from the source: a
 * `export const NAME = <number>` yields a number, `= [<number>, ...] as const`
 * yields an array. Anything else (derived values, strings, objects) is
 * deliberately not a pin target -- the numbers this law pins are authored
 * literals, one per fact.
 */
function extractNumericExports(sourceText) {
  const source = ts.createSourceFile('pricing.ts', sourceText, ts.ScriptTarget.Latest, true);
  const values = new Map();

  function numberOf(node) {
    if (ts.isNumericLiteral(node)) return Number(node.text);
    return undefined;
  }

  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    const exported = statement.modifiers?.some((m) => m.kind === ts.SyntaxKind.ExportKeyword);
    if (!exported) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || !declaration.initializer) continue;
      let initializer = declaration.initializer;
      if (ts.isAsExpression(initializer)) initializer = initializer.expression;
      const scalar = numberOf(initializer);
      if (scalar !== undefined) {
        values.set(declaration.name.text, scalar);
        continue;
      }
      if (ts.isArrayLiteralExpression(initializer)) {
        const items = initializer.elements.map(numberOf);
        if (items.every((n) => n !== undefined)) values.set(declaration.name.text, items);
      }
    }
  }
  return values;
}

/** Every authored offer row, parsed. */
function readCatalog(catalogDir) {
  const offers = [];
  for (const file of fs.readdirSync(catalogDir).filter((f) => f.endsWith('.yaml'))) {
    const content = fs.readFileSync(path.join(catalogDir, file), 'utf8');
    for (const doc of parseAllDocuments(content)) {
      const offer = doc.toJS();
      if (offer?.spec) offers.push({ file, ...offer });
    }
  }
  return offers;
}

/** The union of feature keys the authored catalog actually sells/grants. */
function catalogFeatureUnion(offers) {
  const union = new Set();
  for (const offer of offers) {
    for (const feature of offer.spec?.entitlements?.features ?? []) union.add(feature);
  }
  return union;
}

const sameNumbers = (a, b) => JSON.stringify(a) === JSON.stringify(b);

/**
 * The limit pins: each names the site constant, where the platform states
 * the same fact, and both values -- so a failure says exactly which file to
 * edit on which side.
 */
function checkLimits(pricing, offers, communityEditionPath) {
  const pin = (constant, platformSource, enforced) => {
    const displayed = pricing.get(constant);
    if (displayed === undefined) {
      fail(`src/data/pricing.ts no longer exports a literal ${constant} -- the limits law lost its pin target`);
      return;
    }
    if (enforced === undefined) {
      fail(`${platformSource} states no value for the fact ${constant} displays -- the platform side of the pin is gone`);
      return;
    }
    if (!sameNumbers(displayed, enforced)) {
      fail(
        `src/data/pricing.ts says ${constant} = ${JSON.stringify(displayed)}; ` +
          `the platform's ${platformSource} enforces ${JSON.stringify(enforced)} -- ` +
          `the page would display a number the platform does not enforce; change both together`,
      );
    }
  };

  const byProduct = (product) => offers.filter((o) => o.spec.product === product);

  // The hosted free tier bounds seats only; its row is the one plan_default.
  const [freeTier] = byProduct('plan_default');
  pin('FREE_TIER_SEATS', `catalog row ${freeTier?.file ?? '(no plan_default row)'} (spec.entitlements.limits.seats)`,
    freeTier?.spec?.entitlements?.limits?.seats);

  // The license sizes: every sold license row's seat ceiling, smallest first.
  const licenseRows = byProduct('license');
  pin('SELF_HOSTED_LICENSE_SEAT_CEILINGS',
    `catalog rows ${licenseRows.map((o) => o.file).join(', ') || '(no license rows)'} (spec.licenseTerms.seatLimit)`,
    licenseRows.length > 0
      ? licenseRows.map((o) => o.spec?.licenseTerms?.seatLimit).sort((a, b) => a - b)
      : undefined);

  // The evaluation key's term, from the one claimable license row.
  const [evaluation] = byProduct('license_grant');
  pin('EVALUATION_DAYS', `catalog row ${evaluation?.file ?? '(no license_grant row)'} (spec.licenseTerms.termDays)`,
    evaluation?.spec?.licenseTerms?.termDays);

  // The self-hosted community edition's ceiling, from its published set.
  let communitySeats;
  if (fs.existsSync(communityEditionPath)) {
    const [doc] = parseAllDocuments(fs.readFileSync(communityEditionPath, 'utf8'));
    communitySeats = doc?.toJS()?.limits?.seats;
  }
  pin('COMMUNITY_SEAT_LIMIT',
    `published community edition ${path.relative(path.dirname(path.dirname(communityEditionPath)), communityEditionPath)} (limits.seats)`,
    communitySeats);

  // Deliberately not pinned: the enterprise packages' seat ceilings. No
  // catalog row carries them yet (the enterprise offer is authored at the
  // enterprise rehearsal, after which they join this law).
}

// ---------------------------------------------------------------------------
// Run.
// ---------------------------------------------------------------------------

const gatedRows = extractGatedRows(fs.readFileSync(matrixPath, 'utf8'));
if (gatedRows.length === 0) {
  fail('no entitlementKey rows found in value-matrix.ts -- the extractor or the data module changed shape');
}

// Law 1: vocabulary.
for (const { entitlementKey } of gatedRows) {
  if (!REGISTERED_KEYS.has(entitlementKey)) {
    fail(
      `'${entitlementKey}' is not a registered platform entitlement key -- ` +
        `register it in the platform's entitlements article first, then extend this pin`,
    );
  }
}

// Laws 2 + 3 read the live platform checkout. repoRoot is site/ inside the
// open-source repo; the platform checkout is a sibling of the REPO, hence
// two hops up.
const platformDir = process.env.PLANTON_PLATFORM_DIR ?? path.resolve(repoRoot, '..', '..', 'planton-platform');
const catalogDir = path.join(platformDir, 'product/apis/ai/planton/billing/offer/v1alpha1/assets/catalog');
const communityEditionPath = path.join(
  platformDir,
  'product/apis/ai/planton/licensing/entitlements/v1alpha1/assets/community-edition/self-hosted.yaml',
);

if (!fs.existsSync(catalogDir)) {
  console.warn(
    `\u26a0 ${GUARD}: sibling planton-platform checkout not found at ${platformDir} -- ` +
      `SKIPPING the displayed-vs-sold and limits checks (vocabulary law still enforced). ` +
      `The authoritative run is the local make build beside the platform checkout.`,
  );
} else {
  const offers = readCatalog(catalogDir);

  // Law 2: included-when-sold.
  const sold = catalogFeatureUnion(offers);
  for (const { entitlementKey, anyIncluded } of gatedRows) {
    if (anyIncluded && !sold.has(entitlementKey)) {
      fail(
        `the pricing matrix displays '${entitlementKey}' as INCLUDED, but no authored catalog row ` +
          `sells or grants it -- either stamp the key onto the offer's entitlement contents in the ` +
          `platform's catalog, or render the row as Coming Soon`,
      );
    }
  }

  // Law 3: limits.
  checkLimits(extractNumericExports(fs.readFileSync(pricingPath, 'utf8')), offers, communityEditionPath);
}

if (process.exitCode !== 1) {
  const checked = gatedRows.map((r) => r.entitlementKey).join(', ');
  console.log(`\u2713 ${GUARD}: ${gatedRows.length} gated rows agree with the platform (${checked})`);
  if (fs.existsSync(catalogDir)) {
    console.log(
      `\u2713 ${GUARD}: displayed limits agree with the platform ` +
        `(FREE_TIER_SEATS, SELF_HOSTED_LICENSE_SEAT_CEILINGS, EVALUATION_DAYS, COMMUNITY_SEAT_LIMIT)`,
    );
  }
}
