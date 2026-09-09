/**
 * The displayed-vs-enforced guard: the pricing value matrix's entitlement
 * keys must agree with the platform's authored offer catalog, mechanically.
 *
 * Two laws:
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
 * The catalog half needs the planton-platform checkout beside this repo
 * (or PLANTON_PLATFORM_DIR); when it is absent (e.g. website-only CI) that
 * half is SKIPPED with a loud notice -- the authoritative gate is the local
 * `make build`, which runs where both checkouts exist.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';
import { parseAllDocuments } from 'yaml';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const matrixPath = path.join(repoRoot, 'src', 'data', 'value-matrix.ts');

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
]);

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

/** The union of feature keys the authored catalog actually sells/grants. */
function catalogFeatureUnion(catalogDir) {
  const union = new Set();
  for (const file of fs.readdirSync(catalogDir).filter((f) => f.endsWith('.yaml'))) {
    const content = fs.readFileSync(path.join(catalogDir, file), 'utf8');
    for (const doc of parseAllDocuments(content)) {
      const offer = doc.toJS();
      const features = offer?.spec?.entitlements?.features ?? [];
      for (const feature of features) union.add(feature);
    }
  }
  return union;
}

function fail(message) {
  console.error(`\u2717 entitlement-keys guard: ${message}`);
  process.exitCode = 1;
}

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

// Law 2: included-when-sold, against the live authored catalog.
// repoRoot is site/ inside the open-source repo; the platform checkout is a
// sibling of the REPO, hence two hops up.
const platformDir = process.env.PLANTON_PLATFORM_DIR ?? path.resolve(repoRoot, '..', '..', 'planton-platform');
const catalogDir = path.join(
  platformDir,
  'product/apis/ai/planton/billing/offer/v1alpha1/assets/catalog',
);
if (!fs.existsSync(catalogDir)) {
  console.warn(
    `\u26a0 entitlement-keys guard: sibling planton-platform checkout not found at ${platformDir} -- ` +
      `SKIPPING the displayed-vs-sold check (vocabulary law still enforced). ` +
      `The authoritative run is the local make build beside the platform checkout.`,
  );
} else {
  const sold = catalogFeatureUnion(catalogDir);
  for (const { entitlementKey, anyIncluded } of gatedRows) {
    if (anyIncluded && !sold.has(entitlementKey)) {
      fail(
        `the pricing matrix displays '${entitlementKey}' as INCLUDED, but no authored catalog row ` +
          `sells or grants it -- either stamp the key onto the offer's entitlement contents in the ` +
          `platform's catalog, or render the row as Coming Soon`,
      );
    }
  }
}

if (process.exitCode !== 1) {
  const checked = gatedRows.map((r) => r.entitlementKey).join(', ');
  console.log(`\u2713 entitlement-keys guard: ${gatedRows.length} gated rows agree with the platform (${checked})`);
}
