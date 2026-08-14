export class LineRingBuffer {
    constructor(capacity) {
        if (!Number.isInteger(capacity) || capacity < 1) throw new Error('capacity must be a positive integer');
        this.capacity = capacity;
        this.values = new Array(capacity);
        this.start = 0;
        this.length = 0;
    }

    clear() {
        this.values = new Array(this.capacity);
        this.start = 0;
        this.length = 0;
    }

    replace(lines) {
        this.clear();
        this.pushMany(lines);
    }

    pushMany(lines) {
        for (const line of lines ?? []) {
            if (this.length < this.capacity) {
                this.values[(this.start + this.length) % this.capacity] = String(line);
                this.length++;
            } else {
                this.values[this.start] = String(line);
                this.start = (this.start + 1) % this.capacity;
            }
        }
    }

    toArray() {
        const out = new Array(this.length);
        for (let i = 0; i < this.length; i++) {
            out[i] = this.values[(this.start + i) % this.capacity];
        }
        return out;
    }
}

// Keeps the live log <pre> incremental. Replacing its complete text on every
// batch makes layout cost grow with retained history; text-node chunks let a
// follow stream append new work and discard only the oldest chunks instead.
export class IncrementalLogView {
    constructor(view, capacity, createTextNode = (text) => document.createTextNode(text)) {
        if (!view) throw new Error('view is required');
        if (!Number.isInteger(capacity) || capacity < 1) throw new Error('capacity must be a positive integer');
        this.view = view;
        this.capacity = capacity;
        this.createTextNode = createTextNode;
        this.chunks = [];
        this.lineCount = 0;
        this.empty = true;
    }

    replace(lines, emptyText = '(no logs)') {
        const normalized = Array.from(lines ?? [], String).slice(-this.capacity);
        this.view.textContent = '';
        this.chunks = [];
        this.lineCount = 0;
        this.empty = normalized.length === 0 || (normalized.length === 1 && normalized[0] === '');
        if (this.empty) {
            this.view.textContent = emptyText;
            return;
        }
        this.appendChunk(normalized);
    }

    append(lines) {
        const normalized = Array.from(lines ?? [], String);
        if (normalized.length === 0) return;
        if (normalized.length >= this.capacity) {
            this.replace(normalized.slice(-this.capacity));
            return;
        }
        if (this.empty) {
            this.view.textContent = '';
            this.empty = false;
        }
        this.appendChunk(normalized);
        this.trimOldest(this.lineCount - this.capacity);
    }

    appendChunk(lines) {
        const prefix = this.lineCount > 0 ? '\n' : '';
        const node = this.createTextNode(prefix + lines.join('\n'));
        this.view.appendChild(node);
        this.chunks.push({ node, count: lines.length });
        this.lineCount += lines.length;
    }

    trimOldest(excess) {
        while (excess > 0 && this.chunks.length > 0) {
            const first = this.chunks[0];
            if (excess >= first.count) {
                excess -= first.count;
                this.lineCount -= first.count;
                this.view.removeChild(first.node);
                this.chunks.shift();
                this.stripFirstSeparator();
                continue;
            }

            let cut = -1;
            for (let i = 0; i < excess; i++) cut = first.node.data.indexOf('\n', cut + 1);
            first.node.data = cut >= 0 ? first.node.data.slice(cut + 1) : '';
            first.count -= excess;
            this.lineCount -= excess;
            excess = 0;
        }
    }

    stripFirstSeparator() {
        const first = this.chunks[0];
        if (first?.node.data.startsWith('\n')) first.node.data = first.node.data.slice(1);
    }
}

// Coalesce any number of incoming batches into at most one render per frame.
export function createFrameScheduler(render, schedule = requestAnimationFrame, cancel = cancelAnimationFrame) {
    let handle = null;
    return {
        request() {
            if (handle !== null) return;
            handle = schedule(() => {
                handle = null;
                render();
            });
        },
        flush() {
            if (handle !== null) {
                cancel(handle);
                handle = null;
            }
            render();
        },
        cancel() {
            if (handle === null) return;
            cancel(handle);
            handle = null;
        },
    };
}
