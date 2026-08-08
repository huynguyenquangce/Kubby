# Streaming: logs, exec, port-forward

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `logstream.go`, `exec.go`, `portforward.go`. Grouped together because all
three hold **long-lived connections** and all three push to the frontend via
**Wails events** rather than return values.

## The shared rule

> A 30 s timeout kills all three. `Cluster.Clientset` has one; `Cluster.Stream`
> and `Cluster.Rest` do not. Log following must use `Stream`; exec and
> port-forward must use `Rest`.

This is the single most repeated mistake in this area. See
[cluster-clients.md](cluster-clients.md).

## Wails events

| Event | Emitted by | Meaning |
|---|---|---|
| `logline` | `StartLogStream` goroutine | one log line |
| `exec-output` | exec session | terminal output |
| `exec-closed` | exec session | the session ended |
| `portforward-closed` | a tunnel dying | the frontend should drop it |

The frontend listens with `EventsOn(name, handler)`. Anything that opens a stream
must also close it — `closeDrawer()` calls `stopFollow()` and `stopExec()`.

## Log streaming

`StartLogStream` runs a goroutine emitting `logline`. **One stream at a time**:
each `StartLogStream` calls `StopLogStream` first, and `App.logCancel` holds the
cancel func.

The drawer's Logs tab adds a container picker, a client-side line filter, and
download-to-file. Non-follow reads use `PodLogs(..., tail)`.

## Exec (Terminal)

**Line-mode, TTY off** — deliberately.

Output goes to a plain `<pre>`, so a PTY's ANSI and cursor escapes would render as
garbage. The trade-off is that full-screen TUIs (`vi`, `top`) do not work, and that
is accepted. **Do not flip `TTY: true`** without also adding a real terminal
emulator (xterm.js); doing one without the other produces an unreadable panel.

One session at a time. stdin goes through `ExecWrite`.

## Port-forward

`StartPortForward` resolves a **Service to a running Pod** (by selector) before
forwarding — you can forward a Service without knowing which pod backs it.

Unlike the other two, **several tunnels run at once**: `App.pfSessions` is a map
keyed by tunnel. A tunnel dying emits `portforward-closed` so the frontend removes
its row. All tunnels are stopped on disconnect and on cluster switch.

Local port `0` asks the OS to pick a free port.

## Verify

Streaming is the hardest thing to check from a CLI, but all three are reachable:

```powershell
go run ./cmd/kubby-cli logs <pod> -n <ns> --tail 100
go run ./cmd/kubby-cli exec <pod> -n <ns> -- "ls -la /"
go run ./cmd/kubby-cli port-forward <pod> -n <ns> --remote 8080 --local 0 --hold 30
```

The port-forward check that actually proves it: forward something with an HTTP
endpoint (`coredns:9153` works on a bare cluster) and `curl` it while the tunnel is
held.
