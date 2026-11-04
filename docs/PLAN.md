---
PLAN: "refactor: project dates in the admin panel on the admin's own clock (time.MachineZone)"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan — `modules/panel/projects.go` without the removed `time.FormatDateTime`

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/time@v0.6.3
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`, `go mod tidy` after `go get`.

## Why

`webtyp.com/time` v0.6.3 removed every implicit local time. The panel shows the administrator when a project was
created; it is read on the administrator's own computer, so the zone is `time.MachineZone()`, stated explicitly.

## APIs used (time v0.6.3)

| Symbol | Signature |
|---|---|
| `time.MachineZone` | `func MachineZone() Zone` |
| `(Zone) DateTime` | `func (z Zone) DateTime(u Unix) string` — `"YYYY-MM-DD HH:MM"` |

## Stage 1

`modules/panel/projects.go:137` `time.FormatDateTime(sec)` → `time.MachineZone().DateTime(sec)`, with the Spanish
comment "el reloj de quien administra: lo lee en su propio equipo".

## Delivery check

1. `gotest` passes (a PR that does not compile, or whose tests could not run, is a failed delivery).
2. `grep -rn -E "time\.(FormatDate|FormatTime|FormatDateTime|IsToday)\(" --include=*.go .` → empty.

Commit, push, open the PR. Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
