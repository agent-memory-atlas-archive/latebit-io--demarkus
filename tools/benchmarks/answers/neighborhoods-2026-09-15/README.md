# Cached neighborhood token evaluation

Pre-merge evaluation for graph maintainers: completed cohorts show no token-efficiency improvement, and neighborhood navigation benefits remain unmeasured because readers did not use graph tools.

## Comparison boundary

Measured after commit: `f4852aa536bbab77be71c79172ba578761945ce5`, on `fix/graph-neighborhood-performance`. The freshly paired synthetic before arm uses the production MCP from `ede2b80243a1867aa438bf7962046a916e93a5fc`, before cached neighborhoods. Both arms use identical current server, proxy, runner, fixture, scorer, and reader policy. This comparison covers the neighborhood feature plus its performance fix, not the optimization alone relative to its parent.

Independent latebit/music controls compare existing immutable v4 baselines with current production binaries. Their `scoped-direct-read-v1` profile excludes graph tools; changes are ordinary-retrieval regression observations, not measurements of neighborhood effectiveness. Original reports and v1-v4 dataset inputs remain unchanged.

Settings: OpenCode 1.18.30, `openai/gpt-6-astra`, variant `low`, eight reader iterations, three-minute per-question timeout, two fresh sessions for each of eight tasks. Synthetic uses `section-first`; independent controls use `section-first-outcome-v2`. A temporary npm installation supplied pinned OpenCode because the workstation version had advanced to 1.18.31.

## Results

| Cohort | Supported correct before/after | Total model tokens before/after | Tokens/correct before/after | Change |
|---|---:|---:|---:|---:|
| Synthetic, graph tools available | 16/16 → 16/16 | 89,953 → 95,014 | 5,622.06 → 5,938.38 | +5.63% |
| Latebit v4, direct-only | 6/16 → 6/16 | 112,614 → 114,864 | 18,769 → 19,144 | +2.00% |
| Music v4, direct-only | 14/16 → 14/16 | 89,372 → 91,028 | 6,383.71 → 6,502 | +1.85% |

All three strict comparisons pass input/settings compatibility and complete usage/lifecycle checks. None has an improved point estimate or a supported-correct task/category regression. These are two-repeat observations, not statistical significance claims.

| Cohort | Result tokens before/after | Model turns before/after | MCP calls before/after | Protocol requests before/after |
|---|---:|---:|---:|---:|
| Synthetic | 3,082 → 3,183 | 52 → 53 | 38 → 39 | 56 → 56 |
| Latebit v4 | 13,866 → 13,855 | 63 → 64 | 53 → 54 | 74 → 74 |
| Music v4 | 9,385 → 13,552 | 54 → 54 | 41 → 40 | 61 → 58 |

Latebit scoring dimensions are unchanged: answer correctness 8/16, evidence sufficiency 12/16, citation validity 14/16. Music answer correctness and evidence sufficiency remain 14/16 and 16/16, but citation validity falls from 15/16 to 14/16. Unchanged composite accuracy does not erase that diagnostic regression. No semantic rescoring was performed for this run.

## Interpretation and merge gate

Both synthetic arms exposed graph tools, but all observed calls were lookup/fetch. Therefore this cohort measures the reader-visible tool surface and ordinary retrieval without demonstrating a graph-navigation benefit. For matching q1 traces, three model turns consume 5,257 tokens before and 5,437 after, with identical 241 result tokens. The 180-token increase is consistent with added schema overhead; aggregate differences also include reader path variation. CPU allocation savings are not LLM token savings.

The token-efficiency gate is not met. Do not claim that neighborhood benefits offset the mechanical explore/restore costs. Before advancing on that claim, freeze a corpus-confined graph-enabled independent profile and rerun both comparison arms with identical profile, tasks, scoring, model settings, and explicit cold/warm graph preparation. Verify actual graph-tool use; availability alone does not exercise the feature. Keep frozen v4 as a separate control. A usability-only acceptance would be an explicit tradeoff, not a measured token win.

## Retained attempts and accounting

Raw runs remain ignored under `tools/benchmarks/artifacts/`:

- `answers-neighborhood-before-synthetic-2026-09-15/`: interrupted first attempt; eight answers passed with 45,039 known tokens, then q1 repeat 2 emitted no events and exited with status 1. The outer command reached its 20-minute deadline before report finalization. No report or complete usage claim; startup/cleanup stall cause remains unresolved.
- `answers-neighborhood-before-smoke-2026-09-15/`: successful q1 diagnostic, 5,257 tokens; not a baseline.
- `answers-neighborhood-before-synthetic-r2-2026-09-15/`: complete synthetic before arm.
- `answers-neighborhood-after-synthetic-2026-09-15/`: complete synthetic after arm.
- `answers-neighborhood-after-latebit-v4-2026-09-15/`: complete independent control.
- `answers-neighborhood-after-music-v4-2026-09-15/`: complete independent control.

This evaluation spent 441,155 known model tokens across complete, diagnostic, and interrupted attempts. The interrupted attempt has unknown usage completeness, so this is a lower bound rather than a complete cost claim. Cohort ratios above charge all completed answers, including incorrect ones; incomplete runs stay separate and visible.

## Reproduction

Build `make answer-bench` at the measured after commit. Build the before MCP from the pinned before commit in a separate worktree. Set `PINNED_OPENCODE` to a verified 1.18.30 executable and `BEFORE_MCP` to that worktree's built MCP. Use new output directories for every rerun; never overwrite the retained paths.

```bash
tools/bin/demarkus-answer-bench -opencode "$PINNED_OPENCODE" -mcp-bin "$BEFORE_MCP" \
  -out "$NEW_BEFORE_OUTPUT" -model openai/gpt-6-astra -variant low \
  -reader-policy section-first -repeats 2 -steps 8 -timeout 3m
tools/bin/demarkus-answer-bench -opencode "$PINNED_OPENCODE" \
  -out "$NEW_AFTER_OUTPUT" -model openai/gpt-6-astra -variant low \
  -reader-policy section-first -repeats 2 -steps 8 -timeout 3m
tools/bin/demarkus-answer-bench -opencode "$PINNED_OPENCODE" \
  -corpus tools/benchmarks/corpora/latebit-2026-09-14/restored \
  -questions tools/benchmarks/corpora/latebit-2026-09-14-v4 -origin mark://latebit \
  -out "$NEW_LATEBIT_OUTPUT" -model openai/gpt-6-astra -variant low \
  -reader-policy section-first-outcome-v2 -repeats 2 -steps 8 -timeout 3m
tools/bin/demarkus-answer-bench -opencode "$PINNED_OPENCODE" \
  -corpus tools/benchmarks/corpora/music-2026-09-14/restored \
  -questions tools/benchmarks/corpora/music-2026-09-14-v4 -origin mark://music \
  -out "$NEW_MUSIC_OUTPUT" -model openai/gpt-6-astra -variant low \
  -reader-policy section-first-outcome-v2 -repeats 2 -steps 8 -timeout 3m
```

Run sequentially: each cohort owns fixture port 16319. Allow at least the full scheduled timeout plus setup/cleanup in any enclosing command deadline.

Metrics-only reports in this directory preserve implementation hashes, fixture identities, all attempts, usage, scoring dimensions, and source-report digests without private retrieved content:

- [Synthetic before](synthetic-before.json) and [after](synthetic-after.json).
- [Latebit before](latebit-v4-before.json), exported from retained `answers-latebit-v4-rerun3-2026-09-15/report.json`, and [after](latebit-v4-after.json).
- [Music before](music-v4-before.json), exported from retained `answers-music-v4-rerun1-2026-09-15/report.json`, and [after](music-v4-after.json).

See also [answer evaluation](../README.md) and [mechanical results](../../graph/cached-neighborhoods-2026-09-15.md).
