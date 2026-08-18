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
| `exec-output` | exec session | terminal output carrying its frontend session ID |
| `exec-closed` | exec session | the owned session ended |
| `portforward-closed` | a tunnel dying | the frontend should drop the key only for its exact connection ID |

The frontend listens with `EventsOn(name, handler)`. Anything that opens a stream
must also close it — `closeDrawer()` calls `stopFollow()` and `stopExec()`.

## Log streaming

`StartLogStream` runs a goroutine emitting `loglines`. `StreamLogs` flushes at
most 512 lines once per 40 ms, capping enqueue pressure on Wails' asynchronous
WebView event queue at 25 events/s. Once a batch is full, the reader applies
backpressure until the next tick instead of emitting another event immediately.
**One stream at a time**: each
`StartLogStream` cancels its predecessor, and `App.logCancel` holds the cancel
func behind `logMu`.

Every batch/error carries the frontend-created stream ID. Cancellation cannot
retract an event already crossing the bridge, so the frontend accepts it only
when that ID still owns the open Pod/container. Lines enter a 5,000-line ring
buffer and rendering is coalesced to one animation frame. Normal following
appends text-node chunks and trims only the oldest chunks; a full `<pre>` rebuild
is reserved for static loads and filter changes. Do not restore `slice(-5000)`,
per-line bridge events, or whole-history repainting for every live batch.

The drawer's Logs tab adds a container picker, a client-side line filter, and
download-to-file. Drawer tabs load on first selection rather than fetching every
hidden tab on open, and Logs/Terminal share the same owned `PodContainers`
promise. Non-follow reads use `PodLogs(..., tail)`.

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

Opening the Terminal tab auto-attaches after its owned container lookup finishes.
The default shell choice is `auto`: the backend probes `/bin/bash`,
`/usr/bin/bash`, `/bin/ash`, then `/bin/sh` and opens the PTY with the first one
that executes successfully. This probing and PTY creation stay behind the single
`StartExec` bound call, which returns the resolved shell for truthful UI status.
Users can still select an explicit shell when diagnosing an unusual image; an
explicit choice is not silently replaced. A container or shell change reconnects
once under the same drawer ownership. A normal shell exit or a failed discovery
does not enter a retry loop — the user gets the preserved output and a Retry
control. Distroless containers may legitimately have no supported shell.

One session at a time. Frontend writes are serialized because Wails calls are
promises and input order must not depend on bridge completion order. The App layer
also owns an exec generation: Stop, drawer close, connection change, or a new Start
invalidates callbacks from the previous session. The session, its terminal-size
queue, and Close paths must remain concurrency-safe. Output/closed events also
carry the frontend-created session ID; the WebView accepts only the exact current
ID, because a generation check before `EventsEmit` cannot retract an event already
crossing the bridge.

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
clicked. A frontend operation ID is registered in the backend before startup I/O.
When Keep running is off, closing or replacing that exact drawer cancels the
pending operation immediately as well as stopping an active tunnel; a late Start
response is torn down rather than resurrecting it. When on,
the tunnel remains in the global manager. Cluster transitions invalidate pending
starts and stop all registered tunnels. `pfSessions`, its cluster epoch, and
session close are concurrency-safe because teardown also runs from goroutines.
Tunnel keys and close events carry the stable connection ID so an event from A
cannot remove a same-shaped tunnel rendered for B.

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

The GUI check that actually proves exec interactivity: open a Pod's Terminal tab
and confirm it attaches without a Connect click, type part of an existing path
and press Tab, use Up to recall the command, then run a
cursor-addressing program such as `top` and resize the drawer/window. A headless
bundle check cannot prove those WebView/PTY behaviours.
