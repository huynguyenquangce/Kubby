# Developer doc branches

These files are the **branches** of [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

Start at the skeleton. It holds what is true across the whole app and routes you to
the one branch you need — the map lives there so there is only ever one copy of it
to keep current.

Each branch is self-contained: what it owns, the decisions behind it, the traps that
have already caused bugs, and how to verify a change to it.

Repository-level product and workflow documents deliberately stay in the root
[`docs/`](../../../docs/) directory. In particular,
[`docs/BUILD.md`](../../../docs/BUILD.md) is the only build procedure; feature
branches should link to it rather than copy dependency or build commands.
