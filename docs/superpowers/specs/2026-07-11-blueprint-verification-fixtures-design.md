# Blueprint Verification Fixtures Design

## Goal

Create a set of real, visually inspectable blueprint files that cover the current top-level `nodes/*.json` library and exercise nested sequence, loop, branch, array, function, variable, return, and timer behavior. The same fixture set will later drive Go parser and execution comparisons against independent Go reference algorithms.

## Delivery Phases

1. Generate and visually inspect the fixture blueprints.
2. Add Go tests that load each fixture and compare its output with a same-input Go reference implementation.
3. Run deterministic randomized input cases through both implementations and compare outputs.
4. Summarize parser, execution, display, and output mismatches with proposed fixes.
5. Implement and verify approved fixes.

Only phase 1 begins after this design is approved. Each later phase requires the prior phase's acceptance result.

## Fixture Location

All files live in `examples/verification-blueprints/` so they can be opened directly in the editor and loaded by Go tests without duplicated copies.

```text
examples/verification-blueprints/
  README.md
  coverage.json
  01_legacy_all_nodes_showcase.vgf
  02_control_flow_maze.obp
  03_array_data_lab.obp
  04_deterministic_algorithm.obp
  05_function_orchestrator.obp
  06_timer_lifecycle.obp
  functions/
    10_score_kernel.obpf
    11_array_fold_and_format.obpf
    12_nested_control_function.obpf
    13_local_state_isolation.obpf
```

`README.md` records the purpose, inputs, expected visible outputs, and opening order. `coverage.json` maps every top-level node schema to one or more fixture files and marks whether the node is structural-only, synchronously executed, or asynchronously executed.

## Blueprint Set

### 01 Legacy All Nodes Showcase

Legacy `.vgf` with integer, array, and timer entrances. It uses legacy-compatible classes and ports, functionally grouped by input, arithmetic, arrays, control flow, events, debug, and return output. It is the primary visual import/export fixture and includes at least one executable route for each entrance.

### 02 Control Flow Maze

Native `.obp` that uses `Sequence` to fan into several deterministic paths. The main path nests `Foreach`, `ForeachIntArray`, `While`, and `ForLoopBreak`, then uses greater-than, less-than, equal, `BoolIf`, `RangeCompare`, `EqualSwitch`, and deterministic `Probability`. It returns a stable trace string and integer aggregate.

### 03 Array Data Lab

Native `.obp` covering integer and string array creation, append, indexed reads, length, membership, string split, cast, variables, and return nodes. Fixed arrays such as `[3, 1, 4, 1, 5]` make expected results easy to inspect.

### 04 Deterministic Algorithm

Native `.obp` with a concrete score calculation. It combines entry values, arithmetic, absolute subtraction, division, modulo, range classification, switch classification, and deterministic random values (`min == max`). It returns score, remainder, category, and branch trace.

### 10 Score Kernel Function

Complex `.obpf` with function entry/return, multiple typed parameters and outputs, local variables, arithmetic, nested conditions, range comparison, switch selection, and a deterministic probability branch. It returns numeric score and category text.

### 11 Array Fold And Format Function

Complex `.obpf` with local accumulator variables, nested integer-array and counted loops, array read/append/length/membership operations, string processing, casts, and multiple outputs. It calculates a weighted checksum and a formatted summary.

### 12 Nested Control Function

Complex `.obpf` that emphasizes nested `Sequence`, loop, branch, range, switch, and break paths. It produces a deterministic trace and aggregate, so control-flow regressions are visible in the output order.

### 13 Local State Isolation Function

Complex `.obpf` that uses local Get/Set variables, local arrays, loops, branches, and return values. The orchestrator invokes it twice with the same input; both calls must yield the same output and must not alter a same-named caller variable.

### 05 Function Orchestrator

Native `.obp` that calls all four function files, including nested calls, passes results into later functions, branches on returned values, and emits every final output. This is the primary function loading, binding, local-state, and return-flow fixture.

### 06 Timer Lifecycle

Native `.obp` with normal and timer entrances, `CreateTimer`, `CloseTimer`, debug output, a function call, and return nodes. Later tests use a controllable Go timer module to verify creation, firing, cancellation, and release behavior without wall-clock races.

## Determinism Rules

- Random nodes use equal minimum and maximum values.
- Probability nodes use `0` or `10000` only.
- All arrays, strings, and entry arguments are fixed in phase 1.
- Each execution output connects to at most one successor; fixture design must not rely on the known legacy exec fan-out overwrite behavior.
- All graph and function outputs are explicit return nodes, not logger-only observations.

## Future Go Verification

Phase 2 adds a root legacy fixture test and an engine fixture test. Every executable fixture gets an independent plain-Go reference function. Tests load the same on-disk fixture files, supply the same arguments to the engine, and compare ordered return values, strings, variables where relevant, and controlled timer events.

Phase 3 adds seeded random case generation. Generated values stay within safe domains: positive divisors, bounded loop counts, arrays with bounded lengths, and deterministic random/probability configuration. Failures record fixture path, seed, input, expected output, actual output, and trace data where available.

## Acceptance for Phase 1

- Every listed blueprint opens in the editor without known nodes becoming hidden placeholders.
- Ports, labels, connections, groups, functions, and entry/return nodes are visually understandable.
- `coverage.json` lists every top-level schema from `nodes/*.json`.
- The files are syntactically valid in their respective `.vgf`, `.obp`, and `.obpf` formats.
- No Go execution-comparison tests are added until visual inspection is accepted.
