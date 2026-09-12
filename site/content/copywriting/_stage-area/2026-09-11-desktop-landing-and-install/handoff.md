# Handoff — Planton Desktop landing page and install page

**Date:** 2026-09-11
**Approved content:** `draft-1.md` in this folder (no `preview-N.html` was produced: the same session that wrote the copy implements it, so the rendered page is the preview).

## Routes

The pages live at `/features/desktop` (landing) and `/features/desktop/download` (install). The short paths `/desktop` and `/download` the draft names are the intended final homes; the edge routes them to the console today, so nothing under them reaches the site.

## Overview

planton.ai has had no page for Planton Desktop since the in-repo website was retired on 2026-08-14 and planton.ai came home under `site/` four days later. The desktop app is public — a signed, notarized macOS build, Windows and Linux installers, a live Homebrew cask — and the only place the website mentions it is a copy-only tile on the home page. Two pages fix that: a landing page at `/desktop` that makes the case, and an install page at `/download` that gets a visitor from click to first launch.

## Objective

A developer who lands from LinkedIn, Reddit, or X can answer in two seconds whether they can run the whole platform on their laptop, for free, from the tool they already use — and then install it without guessing anything.

## The argument (read before touching a word)

The page does **not** argue "convenience without losing control" as its thesis. That is the mission and the contrast with platforms that hide the cloud, but the reader we want already has convenience and control: a coding agent and a cloud CLI. The page argues **what Planton adds to the agent the reader already uses** — a typed catalog instead of memory, verification before creation, a record instead of a transcript, secrets the agent never reads, push-to-deploy, templates that compose, and a team path that redoes nothing — and it concedes plainly that for a one-off bucket the agent alone wins. Control is the second beat. Competitors are described, never named.

## Component mapping (paths only)

- `src/app/(root)/download/page.tsx` — the install page (session 1)
- `src/components/product/desktop/` — section components for both pages, same shape as `product/runner/` and `product/cli/`
- `src/data/desktop-download.ts` — the single source for download URLs, platform facts, and the platform detector
- `src/app/(root)/desktop/page.tsx` — the landing page (session 2)
- `packages/website-shell/src/data/navigation.ts` and `header/` — nav, footer, header link
- `src/components/landing-page/v4-2026-08-17-1700/ThreeWaysToRun.tsx` — the home tile's link
- `src/app/(focus)/desktop/open/page.tsx` — the fallback link to `/download`
- `public/sitemap.xml` — hand-maintained; add both routes

## Content guidance

- Every sentence that states what Planton **is** reads from `src/data/positioning.ts`. The umbrella sentence appears once. Each hub's analogy appears only inside that hub's section.
- Every price reads a constant from `src/data/pricing.ts`; every catalog count reads `src/data/platform-stats.ts`.
- Title Case for headings, buttons, and nav; sentence case for prose. Real em dashes. No temporal language ("recently", "we just").
- Numbers on the page are measured numbers with a caption saying where. The two numbers nobody measured — total disk footprint and cold first-run time — are not on the page.
- The Windows card renders only while its `available` flag is true; the flag is set from what the published installer's version resource actually says.
- Things the page must not claim: an MCP server on the laptop (it is hosted), NATS (retired), a monthly free AI allowance (not live), dollar savings, any competitor by name.

## Verification expectations

`make -C site build` green; every download link answers 200; the promoted card matches the browser's platform under three user-agent overrides; the `design-reviewer` verdict is PASS for each page before it is called done.
