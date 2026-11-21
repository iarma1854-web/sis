# SIS — Text Analyzer (Checkpoint 2: Concurrent Processing)

In-memory Go application. A **task** is a unit of work: analyze a piece of text
and produce statistics (words, unique words, runes, lines).
Checkpoint 2 adds asynchronous processing with a **worker pool** and a **bounded queue**.

## Run

    go run .
    go test ./...
    go test -race ./...
    go vet ./...

## How it works

1. `Create(input)` validates the input (empty / whitespace-only → `ErrEmptyInput`),
   stores a new task in memory with a unique ID, state `Pending`.
2. `Start(workers, queueCap)` launches a fixed pool of worker goroutines and creates
   the bounded queue (`chan int` of task IDs, capacity `queueCap`).
   `main.go` uses **3 workers** and a queue capacity of **5**.
3. `Submit(id)` puts a `Pending` task into the queue, sets it to `Queued` and
   **returns immediately** (it does not wait for processing).
4. Each worker loops `for id := range queue`: it atomically claims the task
   (`Queued → Processing`), runs the `Processor` **without holding any lock**, then stores
   the result and sets `Completed` (or `Failed`). Workers are reused for many tasks.
5. `Get` / `List` can be called at any time, also while tasks are being processed.
6. `Process(id)` (Checkpoint 1) still exists: it processes a `Pending` task synchronously.

```
 Create ──> tasks map (mutex) <───────────── Get / List
               ^                                  
 Submit ──> [ bounded queue of IDs ] ──> Worker 1 ─┐
               (cap = queueCap)      ──> Worker 2 ─┼─> update task under mutex
                                     ──> Worker 3 ─┘
```

## Task lifecycle

    Pending ──> Queued ──> Processing ──> Completed
       │                       └────────> Failed
       └──(sync Process)──> Processing

| From       | To                    |
|------------|-----------------------|
| Pending    | Queued, Processing (sync `Process` only) |
| Queued     | Processing            |
| Processing | Completed, Failed     |
| Completed  | — (terminal)          |
| Failed     | — (terminal)          |

Any other transition (e.g. submitting a `Queued`, `Processing` or `Completed` task, or
calling `Process` on a `Queued` task) returns `ErrInvalidTransition`. No retries are supported.

## Full queue

`Submit` uses a **non-blocking send**. If the queue is full it returns `ErrQueueFull`;
the task stays `Pending` (never marked `Queued`/`Processing`) and can be submitted again later.
No goroutines are spawned to bypass the limit.

## Synchronization

- One `sync.RWMutex` (`Service.mu`) protects the `tasks` map, `nextID`, the pool flags
  and **all fields of every `Task`** (protecting the map alone is not enough).
  `Get`/`List` use `RLock` and return **copies**, so callers never touch internal data.
- Check-and-change operations are atomic: `begin` checks the state and moves the task to
  `Processing` under one lock, so two workers can never take the same task.
- The lock is never held while the `Processor` runs, so workers process in parallel.
- Channel ownership: the queue is created in `Start` and closed **only** in `Shutdown`,
  under `mu`. `Submit` checks `stopped` and sends under the same `mu`, so sending on a closed
  channel is impossible. The send is non-blocking, so holding the lock cannot deadlock
  with workers (which need the lock to update tasks).

## Shutdown

`Shutdown()` sets `stopped` (new `Submit` → `ErrShutdown`), closes the queue, and waits
(`sync.WaitGroup`) for the workers. Workers exit only when the queue is closed **and drained**,
so every accepted task is processed before `Shutdown` returns. Calling it again is a safe
no-op (it just waits again). `Submit`/`Shutdown` before `Start` → `ErrNotRunning`.
The pool cannot be restarted after shutdown (`Start` → `ErrAlreadyStarted`).

## Tests

`task/service_test.go` — Checkpoint 1 tests (unchanged).
`task/pool_test.go` — startup, async submit, many tasks with per-task result checks,
real overlap of two tasks (gate processor, no sleeps), concurrency ≤ worker count,
full queue, duplicate/invalid submit, failure → `Failed`, concurrent Create/Submit/Get/List,
rejection after shutdown, idempotent shutdown, draining of accepted tasks,
concurrent Submit + Shutdown (no lost tasks, no panic).
