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

**RESOLVED (human, 2026-07-31): the fork KEEPS `spectro/` and lands it as PICK-RESOLVE, then re-runs
`spectro/run.sh` in that same resolution.** `e0764bbd` + `e2bc5177` are pinned `F4` in
`../decisions/known-commits.tsv` and listed in `../decisions/resolve-commits.txt`.

**Why the fork keeps it — palette does not consume it.** `vendorcrd-sync` syncs
`config/vendorcrd/<p>` from the **upstream release manifests**, fetched over `net/http` from a release
URL ("No shelling, no git" — its README). All six URLs in `cmd/vendorcrd-sync/providers.yaml` point at
`kubernetes-sigs`, and this fork publishes **no release assets**, so the URL cannot be re-pointed at it.
The Spectro deltas reach palette as palette's **own kustomize overlay patches**
(`config/vendorcrd/vsphere/global/kustomization.yaml` pins the image and sets `--webhook-port=9443`).
So the fork's contribution to palette is its **code** (`9767d35a`) and its **image** — never these files.

**Why NOT `REGENERATE` (corrected — this was initially got wrong).** `REGENERATE` cherry-picks nothing,
and **`spectro/` does not exist in `v1.16.1`** (`git ls-tree -d v1.16.1 spectro/` → empty). Of the 10
files in `e0764bbd` only **2** are generated (`spectro/generated/core-{base,global}.yaml`); the other
**8** — `spectro/run.sh` plus the 7 `spectro/core/{base,global}/*.yaml` kustomize **inputs** — are
hand-written fork source. So a `REGENERATE` decision would have left the branch with no `spectro/` dir
at all, dropping the generator itself and making "regenerate via `run.sh`" impossible. It also would
have emitted **no** Regression-Watch row (engine: `REGEN` at :267 vs `MIXED` at :270), hiding the loss.
Additionally no Makefile target invokes `run.sh`, so the `REGENERATE` reason string "regenerate via
make" would have been false and its regen-pending INCOMPLETE undischargeable.

**Resolution contract:** land all 10 files, then re-run `cd spectro && ./run.sh` inside the same
PICK-RESOLVE commit so the 2 generated outputs are v1beta2-correct rather than v1beta1-stale.
Verified all five kustomization bases (`config/default/crd`, `config/govmomi/webhook`, `config/manager`,
`config/certmanager`, `config/base`) still exist in `v1.16.1`, so `run.sh` works once the code compiles.

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

**RESOLVED (round 2): put `1.26.5` in `.github/workflows/main.yml`, not the Makefile — P3c cannot reach
it there, and no conf change is needed.** P3c only rewrites `$BUILD_VARS_MAKEFILE`. A workflow `env:`
value also *beats* a Makefile `?=` (make sets `$(origin VAR)` to `environment`, so `?=` no-ops), and the
Makefile already forwards it via `BUILD_ARGS --build-arg BUILDER_GOLANG_VERSION`. The whole delta is
exactly 4 lines — `GO_VERSION: 1.26.5` and `BUILDER_GOLANG_VERSION: 1.26.5` in each of the two Build
Image steps (normal + FIPS).

⚠️ **Note the round-2 delta ACTIVATED this hazard.** Previously the target Makefile had no
`BUILDER_GOLANG_VERSION` at all (only `GO_VERSION ?= 1.25.9`), so P3c printed "keys absent" and no-op'd.
Now that `4677a04e` lands and introduces the key, P3c **will** rewrite it to 1.25.0 after every commit.

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

---

## D-6 — Protocol U's IMAGE stage (Patch H) names a workflow this fork does not have

The documented command is `gh workflow run spectro-release.yaml --ref spectro-v<target>-master`.
**`spectro-release.yaml` has never existed in this fork** (`git log --all --diff-filter=A --
"**/spectro-release.yaml"` → empty). The fork's release workflow is `.github/workflows/main.yml`
(`name: Release`, `workflow_dispatch`) and it takes **required** inputs the documented command omits:

| input | required | notes |
|--|--|--|
| `release_version` | yes | default `0.0.0` |
| `rel_type` | yes | choice: `release` \| `rc` |

Two consequences: (1) the IMAGE stage will fail as documented and needs the real invocation; (2) a
repo-specific workflow filename is hardcoded in shared orchestration logic — it belongs in conf as a
`RELEASE_WORKFLOW` key alongside `BUILD_VARS_*`. Note `main.yml` only exists on the upgrade branch
*because* `4677a04e` lands it, so the IMAGE stage is downstream of that resolution succeeding.

---

## Regression Watch — two High risks require explicit human sign-off

`--check-regwatch` blocks execute on any `High`. Both are force-picked commits whose *resolution
quality* is the risk; neither failure mode is caught by Tier-1 or Tier-2.

### `9767d35a` — High — palette's two-pod model

`v1.16.1:main.go:236` still defaults `--webhook-port` to **9443**. Palette's
`newns/kustomization.yaml` passes no `--webhook-port`, and `base/core-base.yaml` has **no**
serving-cert mount (only `global/core-global.yaml:7343` does). So:

- lose the `9443 → 0` default flip ⇒ every tenant `capv-controller-manager` **CrashLoops** (it tries
  to serve webhooks without a cert).
- lose the if/else gating ⇒ the webhook pod (`--leader-elect=false`) *also* runs every controller ⇒
  **duplicate reconcile**.

`main.go` went 496 → 752 lines between fork and target, webhook types were renamed, and
`setupClusterCache`'s signature changed — so this is a genuine re-authoring, not a merge.
**Sign-off asks:** read the resolved `main.go` and confirm both (a) the `--webhook-port` default is 0
and (b) the webhook-vs-controller gating is intact.

### `3670335d` — High — a product-semantics decision, not a merge

The fork behavior is genuinely absent from the target (`v1.16.1:vspherevm_controller.go:239` returns
unconditionally), **but the fork's own implementation is latently broken**: its NotFound branches only
log, then fall through to `Get(Name: vsphereDeploymentZone.Spec.FailureDomain)` on a zero-valued
struct — an empty name — which errors anyway. So the "upgrade-stuck" fix likely never worked on that
path. Any faithful port must therefore *invent* semantics, and the coherent one (proceed with
`vsphereFailureDomain == nil`) makes datacenter/datastore/folder/pool overrides silently skip ⇒
**wrong VM placement instead of a loud error**. `*string → string` in v1beta2 as well.
**Sign-off asks:** decide the intended behavior. "Tolerate a missing DeploymentZone" and "place the VM
without its failure domain" are different product decisions, and the fork's code achieved neither.

---

## Human sign-offs on the two High-risk resolutions (2026-07-31)

Both risks are downgraded **H → M** in `../decisions/regwatch-risk.tsv`. The rationale is *not* that the
risk shrank — it is that the H asked for something unverifiable before execute. Both original H's were
"confirm the resolved code does X", and the resolved code does not exist until execute runs. The
sign-offs below convert each into (a) a **binding resolution constraint** for the conflict-resolver and
(b) a **hard Gate-2 check** on real code. Neither may be waived by the DOD waiver path.

### SO-1 — `9767d35a` Controller/Webhook separation: preserve the logic exactly

**Instruction:** keep the commit's logic **identical**. Resolve conflicts manually against v1.16.1's
restructured `main.go` (496 → 752 lines, webhook types renamed, `setupClusterCache` signature changed).
Re-author the placement; do **not** re-design the behavior.

Resolution constraints — the resolved `main.go` MUST satisfy all three:
1. `fs.IntVar(&webhookOpts.Port, "webhook-port", ...)` defaults to **`0`**, not 9443.
2. **Every** webhook registration is inside `if webhookOpts.Port != 0 { … }` — all six
   (`VSphereClusterTemplate`, `VSphereMachine`, `VSphereMachineTemplate`, `VSphereVM`,
   `VSphereDeploymentZone`, `VSphereFailureDomain`) — **and `setupChecks(mgr)` with them.**
3. clusterCache is enabled only when the webhook port **is** 0 (per the commit's own message,
   "enabled clusterCache only when webhook port is 0").

**Hard Gate-2 checks** (both, on the resolved diff — neither is reachable by Tier-1 or Tier-2, which
never start two pods with palette's overlays):
- `grep -n '"webhook-port"' main.go` → the default literal is `0`.
- The webhook block and `setupChecks` are gated; the global pod (`--webhook-port=9443
  --leader-elect=false`) must NOT reach controller setup, or it double-reconciles against the
  leader-electing tenant pods.

### SO-2 — `3670335d` upgrade-stuck fix: Option A

**Decision: Option A — tolerate a missing `VSphereDeploymentZone` for DELETION only.** Port the
*intent*, not the diff; the fork's own implementation never worked (see D-1 note below).

Required behavior:

| condition | behavior |
|--|--|
| `Get(VSphereDeploymentZone)` → NotFound **and** `!vsphereVM.GetDeletionTimestamp().IsZero()` | proceed; **skip the `VSphereFailureDomain` lookup** and leave `vsphereFailureDomain` nil |
| `Get(VSphereDeploymentZone)` → NotFound **and** the VM is **not** being deleted | **return the error** (creation still fails loudly) |
| any other error | return the error |

Why the skip is mandatory: the fork's version tolerated NotFound but then fell through to
`Get(Name: vsphereDeploymentZone.Spec.FailureDomain)` on a zero-valued struct — an **empty name** —
which errors anyway. So the tolerance branch never reached a successful reconcile and the fix was a
no-op on that path. Tolerating the first Get without also skipping the second reproduces that bug.

⚠️ **Accepted trade-off, recorded deliberately.** Option A **drops the fork's second branch**
(`else if apierrors.IsNotFound(err)` → `"ignoring vspheredeploymentzone not found, might be worker
vm"`). A non-deleting VM with a missing zone therefore still errors. If the original stuck upgrade
involved non-deleting worker VMs, Option A may not fully cover it — flagged to the approver, who chose
A over Option B ("place the VM without its failure domain"), because B skips
datacenter/datastore/folder/resource-pool overrides **silently**, risking wrong VM placement with no
error. Revisit only with evidence from a real stuck upgrade.

Note `machine.Spec.FailureDomain` is `*string` in v1beta1 and `string` in v1beta2, so the
`*failureDomain` dereferences change shape as part of the axis-2 migration regardless.
