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
