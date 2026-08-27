# Confirmation: what the code does, and where it departs from the paper

The technical paper's section 5.1 confirms a site once **100% of the DAG's
current vertices confirm it**, directly or indirectly. This document records how
that is implemented, one correction to an earlier misreading, and one measured
departure that needs a decision.

## The rule as implemented

Every tip owns a bit in a vector. Each tracked site carries one bit per tip that
confirms it, set when that tip is created. Marking walks backwards from a new
site's approval targets and stops as soon as it meets a site that already carries
the bit — if a site has it, so do all of its ancestors, because bits are always
set over a complete backward closure. A site is confirmed when its bit count
reaches the threshold, and sites are bucketed by bit count so the check is a
lookup rather than a scan.

Two properties bound the work:

- **Confirmation is closed downwards.** If the tips that confirm a site also
  confirm its ancestors, the ancestors are confirmed too. Marking can stop at a
  confirmed site, and confirmed sites can leave the tracker. What is held in
  memory is the frontier, not the ledger.
- **A tip that is never approved** would otherwise hold its bit forever and stop
  anything newer from reaching the threshold. `dag.tiptimeout` drops such a tip
  from the denominator while leaving it selectable, so its transaction is not
  stranded.

## Correction: a tip is a site nothing approves

This was implemented first as "a site stops being a tip once `dag.approvetx`
other sites reference it", reusing the setting that says how many sites a *new*
site approves. That was a misreading. A tip is a vertex nothing points at — the
ordinary meaning — and the two quantities are unrelated.

The consequence was not cosmetic. Keeping partly approved sites in the
denominator also kept them *selectable*, and since the forward walk stops at the
first site with room for another approval, those were the sites selection kept
landing on. Genuinely new tips were almost never approved, so they never left the
denominator, and the tip set grew linearly with the ledger while confirmation
stopped. Measured over 6000 inserts with four concurrent publishers: **10 sites
confirmed under the old reading, 5980 under the corrected one.**

`dag.approvetx` now means only what it says — how many sites a new site approves
— and is read only by tip selection.

## Departure: `dag.confirmshare`

Even with the corrected tip definition, the literal 100% rule does not converge
once several sites choose their approvals against the same view of the graph.
That is not an edge case; it is what every real network does, because a site
published by one node is not visible to the others until it has propagated.

Measured, 6000 inserts per setting, `dag.tiptimeout=0` so the liveness valve
cannot be what makes it pass:

| Concurrent publishers | `confirmshare=1000` (100%) | `confirmshare=667` (⅔) |
| --------------------- | -------------------------- | ---------------------- |
| 1                     | 100.0%                     | 100.0%                 |
| 4                     | 99.7%                      | 99.8%                  |
| 16                    | **0.0%**                   | 98.1%                  |
| 64                    | **0.0%**                   | 32.5%                  |

The reason is structural. The 100% rule needs *every* live tip to cover a site.
The tip set grows with concurrency, and while a site waits for the last few
stragglers, new tips keep arriving and joining the denominator. Below 100% the
rule does not wait for the stragglers, and it converges.

`dag.confirmshare` is the share of live tips required, in permille. **The default
is 1000 — the paper's literal rule — so nothing changes without a decision.** The
node warns at start-up when it is set to 1000, with these numbers, because a
default that cannot confirm anything beyond a handful of publishers is worth
saying out loud.

### Recommendation

Set `dag.confirmshare: 667` for any network with more than a few publishing
nodes. Two thirds is not arbitrary: it is the same fraction as the validator
quorum the commit-transaction consensus uses, so a site is confirmed under
exactly the share of the graph that a commit transaction needs of the validator
set.

This is a protocol-level decision and it belongs to whoever owns the
specification. The alternatives considered:

- **Keep 100% and lean on `dag.tiptimeout`.** This is what the code did before
  the share existed, and it does work — but it makes a liveness safeguard the
  mechanism confirmation depends on, ties confirmation latency to wall-clock
  timing rather than to the graph, and leaves the denominator size dependent on
  load. `grape_tips_expired_total` exists to show how often it fires; a steady
  rate means the valve is carrying the rule.
- **Confirm against the tip set as of the previous commit transaction** rather
  than against the current one. A fixed denominator per epoch converges by
  construction and fits the five-second commit cadence. This is more invasive
  and has not been implemented or measured.

## What to watch in production

| Metric | What a bad reading means |
| ------ | ------------------------ |
| `grape_confirm_tips` | The denominator. Growing steadily with the ledger is the failure above. |
| `grape_confirm_active_sites` | The frontier. Should be bounded by confirmation latency, not by ledger size. |
| `grape_tips_expired_total` | The liveness valve firing. A steady rate means selection is not reaching the tips it should. |
| `grape_walk_steps` | Zero-step walks mean selection has degenerated to returning its own starting point. |
| `grape_selection_fallbacks_total` | Selections that fell back to a uniform pick, so with no bias applied. |

## Tests

- `TestTrackerAgreesWithTheDefinition` — the incremental tracker against a
  brute-force oracle after every insert, across several seeds in both
  directions.
- `TestConfirmationConvergesUnderConcurrentArrival` — the table above, as
  assertions, at both settings. The test that neither earlier design survived.
- `TestAnApprovalIsCountedOnce`, `TestDuplicateApprovalTargetsCountOnce` — an
  approval counts once, at the point a site becomes part of the graph.
- `TestAbandonedTipStopsHoldingUpConfirmation` — the timeout valve.
