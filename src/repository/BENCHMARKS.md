# Cleanup capacity

Run from the project root:

```bash
go test ./src/repository -run '^$' -bench '^BenchmarkCleanupCapacity$' -benchtime=1x -count=3 -args -cleanup-tickets=10000000 -cleanup-seconds=1
```

`cleanup-tickets` sets the initial queue size (default: 1 million).
`cleanup-seconds` sets the time budget for each scan, in whole seconds
(default: 1). Use the same value as your deployment's `clearMaxSeconds`.
Each iteration prepares a fresh queue outside the measurement. `-benchtime=1x`
runs one scan per sample; `-count=3` provides three independent samples.

Scenarios:

- `active`: all tickets remain active; cleanup traverses the queue and updates positions.
- `expired`: all tickets have expired; cleanup also removes nodes and map entries.

Metrics:

- `tickets/op`: tickets actually processed per scan, including those removed.
- `removed/op`: tickets removed per scan.
- `timeouts/op`: fraction of scans that reached the time limit. With `-benchtime=1x`,
  `1` means time ran out and `0` means the queue ended first.
- `tickets/s`: tickets processed divided by the measured elapsed time.
- `ns/op`: scan duration, in nanoseconds.

**To determine how many tickets fit within the time budget, use `tickets/op`
from a sample with `timeouts/op=1`.** If `timeouts/op` is zero, increase
`cleanup-tickets`: the sample only confirms that this queue fits within the
budget. Do not extrapolate `tickets/s` as though it were a measurement that
reached the timeout.

The count excludes the ticket at which the timer interrupts execution, even if
its position was already written before the check. The statistics reuse the
surviving ticket count and the change in queue size; no new counter is
incremented per ticket in the production code path.

The measurement uses the actual cleanup routine with its mutex, timer, list,
and map. Keys are unique, contain `KEY_SIZE` characters, and are prepared
without randomness because key generation is not part of cleanup. Persistence
and logging are disabled; there are no concurrent HTTP requests. These results
therefore do not represent disk costs, contention with clients, or every
possible distribution of active and expired tickets.

Large queues require memory proportional to the number of tickets. Run without
`-race` or other simultaneous benchmarks, and monitor memory pressure and swap
usage. The limit is checked between tickets, so it is not a hard deadline:
runtime pauses, scheduling, and the work on a single ticket can exceed it.
