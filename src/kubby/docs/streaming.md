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
| `loglines` | `StartLogStream` goroutine | one owned batch of log lines |
| `logerror` | `StartLogStream` goroutine | an owned stream failure |
| `exec-output` | exec session | terminal output |
| `exec-closed` | exec session | the session ended |
| `portforward-closed` | a tunnel dying | the frontend should drop it |

The frontend listens with `EventsOn(name, handler)`. Anything that opens a stream
must also close it — `closeDrawer()` calls `stopFollow()` and `stopExec()`.

## Log streaming

`StartLogStream` runs a goroutine emitting `loglines`. `StreamLogs` flushes at 64
lines or 40 ms, whichever comes first, so a noisy Pod cannot cross the Wails
bridge and repaint the WebView once per line. **One stream at a time**: each
`StartLogStream` cancels its predecessor, and `App.logCancel` holds the cancel
func behind `logMu`.

Every batch/error carries the frontend-created stream ID. Cancellation cannot
retract an event already crossing the bridge, so the frontend accepts it only
when that ID still owns the open Pod/container. Lines enter a 5,000-line ring
buffer and rendering is coalesced to one animation frame; do not restore
`slice(-5000)` or render inside a per-line listener.

The drawer's Logs tab adds a container picker, a client-side line filter, and
download-to-file. Non-follow reads use `PodLogs(..., tail)`.

## Exec (Terminal)

Exec is a real remote PTY (`TTY: true`) rendered by `@xterm/xterm`. xterm's
`onData` stream goes through `ExecWrite`, so Tab completion, history keys, Ctrl
shortcuts, paste, ANSI colours, cursor motion, and full-screen programs are shell
input/output rather than browser form events. `@xterm/addon-fit` measures the
visible drawer panel and `ExecResize` feeds its columns/rows to client-go's
`TerminalSizeQueue`.

xterm only transports the keys. The selected shell owns line editing: Bash/Zsh
normally interpret Tab and arrow sequences, while a minimal `/bin/sh` may insert a
tab or print `^[[A`. Hidden paths also require their leading dot (`.a<Tab>`), just
as they do in a native terminal. Do not label that transport working as an xterm
failure.

One session at a time. Frontend writes are serialized because Wails calls are
promises and input order must not depend on bridge completion order. The App layer
also owns an exec generation: Stop, drawer close, connection change, or a new Start
invalidates callbacks from the previous session. The session, its terminal-size
queue, and Close paths must remain concurrency-safe.

## Port-forward

`StartPortForward` resolves a **Service to a running Pod** (by selector) before
forwarding — you can forward a Service without knowing which pod backs it.

Unlike the other two, **several tunnels run at once**: `App.pfSessions` is a map
keyed by tunnel. A tunnel dying emits `portforward-closed` so the frontend removes
its row. All tunnels are stopped on disconnect and on cluster switch.

The tunnel registry is application/cluster state, not drawer state. The top bar's
**Tunnels** manager is the global control surface: it hydrates from
`ListPortForwards`, shows the original Pod/Service target, copies or opens the
local endpoint, and can stop one or all tunnels. Starting a tunnel shows a toast
that links back to this manager, so closing a resource drawer never makes a
background session undiscoverable.

The drawer's **Keep running after drawer closes** choice is captured when Start is
clicked. When off, closing or replacing that exact drawer stops its tunnel; a
late Start response is stopped immediately rather than resurrecting it. When on,
the tunnel remains in the global manager. Cluster transitions invalidate pending
starts and stop all registered tunnels. `pfSessions`, its cluster epoch, and
session close are concurrency-safe because teardown also runs from goroutines.

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

The GUI check that actually proves exec interactivity: connect to a shell, type
part of an existing path and press Tab, use Up to recall the command, then run a
cursor-addressing program such as `top` and resize the drawer/window. A headless
bundle check cannot prove those WebView/PTY behaviours.
