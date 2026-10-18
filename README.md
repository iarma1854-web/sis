# SIS — Text Analyzer (Checkpoint 1)

In-memory Go application. A **task** is a unit of work: analyze a piece of text
and produce statistics (words, unique words, runes, lines).

## Run

    go run .
    go test ./...

## Task lifecycle

    Pending ──> Processing ──> Completed
                    └────────> Failed

Allowed transitions:

| From       | To                    |
|------------|-----------------------|
| Pending    | Processing            |
| Processing | Completed, Failed     |
| Completed  | — (terminal)          |
| Failed     | — (terminal)          |

Any other transition (e.g. processing a Completed task again) returns
`ErrInvalidTransition`.

Note: processing is synchronous, so `Processing` exists only inside a single
`Process` call. It will become observable when concurrency is added later.

## Validation

Input that is empty or whitespace-only is rejected with `ErrEmptyInput`.

## Design

- `Task` — ID, input, state, result. All methods use pointer receivers.
- `Processor` interface — processing logic; `TextAnalyzer` is the implementation.
- `Service` — in-memory storage (`map[int]*Task`), create / get / process / list.
- `Get` and `List` return copies, so callers cannot corrupt stored tasks.