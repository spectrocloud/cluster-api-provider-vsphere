# capv upgrade ledger

## Cycle: spectro-master (CAPI v1.9.1 / v1beta1) → v1.16.1 (CAPI v1.13.1 / v1beta2)

| field | value |
|--|--|
| frozen source S | `9d84d6ac` (`origin/spectro-master`) |
| target T | `v1.16.1` @ `d345703d` |
| fork-point F | `00df580a` |
| new branch | `spectro-v1.16.1-master` |
| axis | **2 — contract migration v1beta1 → v1beta2** (engine flags, does NOT auto-perform) |
| n-3 | **NEEDS-HOP(gap=4)** — overridden, see W-1 |
| core baseline injected | `v1.13.3` / `v1beta2` — see D-4 for the precise caveat |

Gate-1 review is on the plan PR (branch `capi-plan-v1.16.1`). This file records the waivers and the
open decisions; the per-commit decisions live in `../decisions/`.

---

## W-1 — n-3 waiver: going direct to v1.16.1, no intermediate hop

The engine reports `NEEDS-HOP(gap=4)` (CAPI v1.9.1 → v1.13.1 is four minors and crosses the
contract). **Decision: go direct.** Recorded here because `summary.json` still carries the
unqualified `NEEDS-HOP` verdict and an undocumented override is indistinguishable from a bypass.

Rationale (as verified, not as originally argued):

- The candidate intermediate, CAPV `v1.15.3`, is CAPI `v1.12.5` / contract **`v1beta1`** — confirmed
  in its `go.mod` and `metadata.yaml`. The contract flip lands exactly at CAPV **1.16**.
- `v1.15.3` has **no `api/` directory**, so a hop would *not* pay the `apis/` → `api/` relocation.
  It defers the single largest risk rather than reducing it, at 2× pipeline cost.
- The genuine fork surface is ~9 commits, all already routed to human review.
- The earlier argument ("palette's base branch is already v1beta2, so a v1beta1 intermediate can't
  integrate") is **not** the load-bearing reason — a hop branch need never be palette-integrated.
  It is a fork-side reconcile-risk question, and the point above is what answers it.

**Compensating controls required before Gate 2** (this waiver is conditional on them):
- Tier-2 CRD schema diff and conversion fuzz treated as **hard** gates, not advisory.
- CBT green for `CLOUD=vsphere` against a palette dev build.
- No `INCOMPLETE(*)` remaining on the code PR.

---

## Open decisions — Gate 1 must settle these

### D-1 — Who owns the generated Spectro manifests? (blocks `e0764bbd`, `e2bc5177`)

Both commits are pinned `F4` → NEEDS-DECISION in `../decisions/known-commits.tsv` so neither can be
auto-picked. `spectro/generated/core-global.yaml` alone is ~6k lines of **CAPI-v1.9-era (v1beta1)**
CRDs + webhook config. Replaying that onto a v1beta2 base is a stale-schema regression, and Spec
Patch F says these are regenerated, never cherry-picked.

Two viable paths:
1. **Palette-owned (Patch F).** Add a `vsphere` provider entry to `palette/cmd/vendorcrd-sync/providers.yaml`,
   regenerate `palette/config/vendorcrd/vsphere` from v1beta2, and stop carrying `spectro/generated`
   in the fork. Also resolves D-3.
2. **Fork-owned.** Carry `spectro/` forward and regenerate in-fork against the v1beta2 base; palette's
   `config/vendorcrd/vsphere` stays hand-maintained (D-3 then persists).

### D-2 — What Go version does the branch land on?

The fork deliberately runs **Go 1.26.5** (CVE-driven, feeds a FIPS build). Three interacting facts:

- `179c310f` is a pure `1.26.4 → 1.26.5` edit of `Makefile` + `.github/workflows/main.yml`. It does
  **not** touch `go.mod`.
- `bcb615aa` — skip-listed for CVE reasons — is what set `1.26.4` *and* `go.mod`'s `go 1.26.0`.
  With it skipped, `179c310f` has **no anchor and cannot apply**. (The skip-list's original rationale
  for retaining `179c310f`, "the fork is ahead on go.mod", is therefore void.)
- `4677a04e` introduces `BUILDER_GOLANG_VERSION ?= 1.23`; the target Makefile has `GO_VERSION ?= 1.25.9`
  and no `BUILDER_GOLANG_VERSION` at all.
- The engine's P3c build-vars refresh (**execute-only**, so invisible in the plan) does
  `s/^(BUILDER_GOLANG_VERSION…=).*/\1${TGT_GO}/` → would force **1.25.0**.

So with no action the branch lands on 1.23 or 1.25.0, silently reverting a security bump.
To land 1.26.5 all three must happen: skip `179c310f` as unanchored, set `GO_VERSION` +
`BUILDER_GOLANG_VERSION` to `1.26.5` explicitly while resolving `4677a04e`, and prevent P3c from
overwriting it (`BUILD_VARS_GO_KEY`).

### D-3 — Palette cannot consume this provider yet

`palette/cmd/vendorcrd-sync/providers.yaml` lists only `capi`, `aws`, `azure`, `apache-cloudstack` —
**no `vsphere`** — although `palette/config/vendorcrd/vsphere/{base,global,newns}` exists as a
hand-maintained tree. `vendorcrd-sync sync|verify --provider vsphere` therefore cannot run, so Patch F's
mandatory post-conditions cannot be verified in vendorcrd output. Palette's `go.mod` also has no
vSphere provider require/replace, so Patch I reduces to the image pin + vendorcrd for this provider.

### D-4 — Core-first is a tracked dependency, not a completed prerequisite

The baseline was injected as `CAPI_BASELINE_VERSION=v1.13.3`, but palette pins
`github.com/spectrocloud-public/cluster-api v1.13.3-**beta1**` — a pre-release — on
`spectro-capx-v1beta2`, an unmerged branch. No core plan PR or `luma/approved` evidence exists.

Assessment: the **contract** half (`v1beta2`) is independently corroborated by `v1.16.1:metadata.yaml`
and does not depend on palette. The **version** half is satisfied only by an unreleased beta on an
unmerged branch, and `v1.13.3-beta1 ≥ v1.13.1` (the target's own requirement). Treat as: Gate 2
blocked on the core release landing + `spectro-capx-v1beta2` merging.

### D-5 — Axis-2 migration: scope known, owner not assigned

The engine flags the contract migration and deliberately does not perform it. Scope for capv:

1. **Import relocation** — `apis/v1beta1` + `apis/vmware/v1beta1` (fork) → `api/govmomi/v1beta2` +
   `api/supervisor/v1beta2` (target). Confirmed: fork has `apis/`, target has `api/`.
2. **Condition API re-authoring** — target sites now dual-write `deprecatedv1beta1conditions.MarkFalse(...)`
   **and** `conditions.Set(..., metav1.Condition{...})`. Any fork hunk touching a condition site must be
   re-authored, not replayed (`8366ec8e` was exactly such a site and is now skip-listed).
3. **Manifest regeneration** against v1beta2 CRDs — this is D-1.
4. **CRD storage-version migration + conversion webhooks** — target-owned, not fork-owned.

Confirmed good news: **none of the auto-PICKed commits introduce broken imports** — `9c3c9344` edits
existing files only, `e5d72bb3` is a workflow. The axis-2 build risk is concentrated entirely in the
NEEDS-DECISION resolutions.

---

## Outstanding conf defects (`conf/capi-upgrade.conf`)

Fixed this cycle:
- `MANDATORY_PATTERNS` / `GENERATED_PATHS` were **unquoted** — the file is `.`-sourced, so values
  containing `|` and spaces were parsed as command pipelines. `MANDATORY_PATTERNS` collapsed to
  `"Added"` and `GENERATED_PATHS` to `"config/"`, silently degrading the classifier.

Still wrong — mitigated by ledger pins, but the durable fix belongs in conf:
- **`GENERATED_PATHS`** says `spectro/base|spectro/global`; real paths are `spectro/core/base`,
  `spectro/core/global`, `spectro/generated`. Root cause of D-1's auto-PICK.
- **`BUILD_VARS_TAG_KEY=TAG`** — this fork's `TAG ?= dev` carries no version; the version-bearing key
  is `DEV_TAG ?= v1.12.0-spectro-${SPECTRO_VERSION}`. As configured, a v1.16.1 build is tagged
  **v1.12.0**, and that tag is what Patch I pins into palette.
- **`MANDATORY_PATTERNS`** is CAPG-derived: `Added Spectro Manifests` matches nothing in CAPV, and
  `exclusive webhook` does not match `9767d35a` "Controller and Webhook separation" — the single most
  palette-critical fork commit. Pinned `F4` as mitigation.
- **`CRD_API_PATHS`** default assumes `api/`; this fork uses `apis/`, so the Patch L parity flag is
  latent-dead. Verified no commit in `S ^T` currently touches `apis/*types.go`, so no effect on this
  plan — a future-correctness fix (`api/` → `apis?/`, preserving the conversion/config-crd clauses).
- **`F4_HOTSPOTS`** is empty. Per the SKILL this is safe-by-default (unknowns route to NEEDS-DECISION,
  and 10 did). Populating it would add precision, not safety.
