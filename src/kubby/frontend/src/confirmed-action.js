// Keep the confirmation boundary testable without a DOM or Wails runtime.
// The action callback is never evaluated unless the user explicitly approves.
export async function confirmedAction(confirm, action) {
    if (!await confirm()) return false;
    await action();
    return true;
}

export function summarizeLineChanges(diff) {
    return diff.reduce((summary, line) => {
        if (line.t === '+') summary.added++;
        if (line.t === '-') summary.removed++;
        return summary;
    }, { added: 0, removed: 0 });
}
