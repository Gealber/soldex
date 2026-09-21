# Working on soldex with an AI assistant

soldex is swap math. Every number it returns is one somebody prices a trade on,
so a confident wrong number is worse than no number. That single fact drives most
of what follows.

This file is the working agreement for anyone — human or model — changing this
repo. It is written from what actually went wrong here, not from general
principles.

## Scope

Models, math and quotes. No RPC, no instruction builders, no strategy. Callers
fetch state and build transactions; soldex turns state into a number.

If a change needs an account fetched mid-quote, the design is wrong. Take the
state as an argument.

## The rules that matter

### Verify against the chain, not the SDK

An IDL, an SDK or a repo commit tells you what someone intended. The deployed
program tells you what happens. Read offsets off live accounts and check the
values make sense as the field you think they are.

This is not pedantry. A program ID recalled from memory here returned
`NOT FOUND`, and a "verified" set of offsets turned out to have been read from an
account census that silently included a second account type.

### Check your sample before you believe your result

Filter by discriminator *and* `dataSize`. Programs own several account types and
mixing them turns every field into noise.

A scan of Raydium CP-Swap once "falsified" a correct set of offsets because the
program owns roughly as many `ObservationState` accounts as pools. The offsets
were right; the sample was wrong. A clean boolean that turns noisy when you widen
the sample is the tell.

### Prove the test fails with the fix disabled

Write the fix, write the test, then break the fix and watch the test fail. A test
that passes both ways tests nothing.

State the number: "reverting this reads 500000000 instead of 59999936" is
evidence. "Tests pass" is not.

### A number that does not move proves nothing

Before claiming a feature is verified, disable it and re-run. If the output is
unchanged, that run said nothing about it.

The Raydium CLMM differential check matched exactly on the first pool tried — on
which the volatility accumulator was zero, so the dynamic fee contributed nothing
and the match was vacuous. `TestCLMMChainVectorFeaturesAreLive` exists so that
cannot happen quietly again.

Related: a deep pool cannot distinguish fee-on-input from fee-on-output. The
curve is near-linear there and both give the same answer. Test where the
difference exists.

### Refuse rather than guess

When a mode, layout or formula is not established, return an error. Do not fall
back to a plausible value.

`ErrUnsupportedBaseFeeMode` exists because the DAMM v2 market-cap scheduler's
stepping formula is still unknown: two plausible readings were tested against
real swaps and *both* were falsified. Shipping either would have replaced a loud
gap with a quiet wrong number on 1,588 pools.

Say "I don't know" and stop. Record what was ruled out so the next attempt starts
from the negative result instead of re-deriving it.

### Make the wrong call impossible, not documented

If a caller can leave a fee at zero, one eventually will, and a zero fee
over-states every output. The `From*` constructors exist because a consumer left
`BaseFeeNumerator` on a TODO and quoted a whole venue with no fee.

Prefer deriving a value over accepting one. Prefer an error over a zero default.

### Re-measure populations

"Rare enough to skip" ages badly. Raydium CLMM's `fee_on` went from 794 pools to
9,706 in six weeks. Counts in comments are a snapshot, not a constant — if a
decision rests on one, measure it again.

## Style

Commits: `venue: imperative summary`, then a body explaining *why*, with the
numbers that justify it. Look at `git log` before writing one.

Comments: short. Explain why something is not obvious, not what the code does. No
dates unless the date identifies a program deployment or a frozen snapshot —
provenance dates are noise to the next reader.

Gate before every commit:

    gofmt -l . && go vet ./... && go test ./... && go test -race ./...

## Working in steps

Long changes go in a plan file with checkboxes, outside the repo, one step per
iteration, with a log appended as each lands. It survives a lost session and it
makes a blocked step visible instead of silently skipped.

Two things that kept it honest here: a step that cannot be finished stays
unticked with the evidence recorded, and a step whose finding contradicts the
plan gets the plan corrected rather than the finding shaded.

## Local tools

`opt/` is gitignored. Each tool is its own module with
`replace github.com/Gealber/soldex => ../../`, because these tools check the
*working tree* against chain — a tagged release would verify the wrong code.
Credentials live at `opt/` root and never leave it.

Consumers are different: they import a tagged version. No local `replace` in a
consumer, or you are testing against code nobody else has.

## Things worth knowing before changing quote math

- `models.DAMMPool.TradingFeeNumerator` is the CLIFF, not the live fee. Resolve
  with `CurrentBaseFeeNumerator`.
- Pump-AMM quote reserves are not the vault balance. Price through
  `EffectiveQuoteReserve`.
- Raydium CP-Swap and Pump both take a TOTAL fee rate that includes a creator
  component. Passing the trade fee alone under-charges.
- A `models` field that decodes as zero on a short account is a real cohort, not
  a bug. Account layouts here grow by appending, and every old cohort stays live.
