export function validTerminalSize(cols, rows) {
    if (!Number.isInteger(cols) || !Number.isInteger(rows) || cols < 1 || rows < 1) return null;
    return { cols, rows };
}

// Wails calls return promises. Serialising them preserves control-sequence and
// paste order even when the bridge resolves individual writes at different times.
export function createSerialWriter(write, onError = () => {}) {
    let tail = Promise.resolve();
    return (data) => {
        const operation = tail.then(() => write(data));
        tail = operation.catch((err) => onError(err));
        return tail;
    };
}
