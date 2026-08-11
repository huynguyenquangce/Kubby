const DEFAULT_MAX_WORK = 4_000_000;

// A line-oriented Myers bisect diff. Its two frontier arrays are O(n + m),
// unlike the former LCS matrix which retained O(n*m) integers. Pathological
// inputs stop at a deterministic work budget and fall back to a valid (though
// deliberately non-minimal) delete/add block instead of freezing the WebView.
export function lineDiff(aText, bText, { maxWork = DEFAULT_MAX_WORK } = {}) {
    const a = String(aText).split('\n');
    const b = String(bText).split('\n');
    const out = [];
    const budget = { remaining: Math.max(1, maxWork) };
    diffRange(a, b, out, budget);
    return out;
}

function diffRange(a, b, out, budget) {
    let prefix = 0;
    while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix]) prefix++;
    for (let i = 0; i < prefix; i++) out.push({ t: ' ', l: a[i] });

    let aEnd = a.length;
    let bEnd = b.length;
    while (aEnd > prefix && bEnd > prefix && a[aEnd - 1] === b[bEnd - 1]) {
        aEnd--;
        bEnd--;
    }

    const aMiddle = a.slice(prefix, aEnd);
    const bMiddle = b.slice(prefix, bEnd);
    diffMiddle(aMiddle, bMiddle, out, budget);

    for (let i = aEnd; i < a.length; i++) out.push({ t: ' ', l: a[i] });
}

function diffMiddle(a, b, out, budget) {
    if (a.length === 0) {
        for (const line of b) out.push({ t: '+', l: line });
        return;
    }
    if (b.length === 0) {
        for (const line of a) out.push({ t: '-', l: line });
        return;
    }

    const split = bisect(a, b, budget);
    if (!split || (split.x === 0 && split.y === 0)
        || (split.x === a.length && split.y === b.length)) {
        for (const line of a) out.push({ t: '-', l: line });
        for (const line of b) out.push({ t: '+', l: line });
        return;
    }

    diffRange(a.slice(0, split.x), b.slice(0, split.y), out, budget);
    diffRange(a.slice(split.x), b.slice(split.y), out, budget);
}

function bisect(a, b, budget) {
    const n = a.length;
    const m = b.length;
    const maxD = Math.ceil((n + m) / 2);
    const offset = maxD;
    const length = 2 * maxD + 1;
    const forward = new Int32Array(length);
    const reverse = new Int32Array(length);
    forward.fill(-1);
    reverse.fill(-1);
    forward[offset + 1] = 0;
    reverse[offset + 1] = 0;

    const delta = n - m;
    const overlapOnForward = delta % 2 !== 0;
    let forwardStart = 0;
    let forwardEnd = 0;
    let reverseStart = 0;
    let reverseEnd = 0;

    for (let d = 0; d < maxD; d++) {
        if (--budget.remaining < 0) return null;

        for (let k = -d + forwardStart; k <= d - forwardEnd; k += 2) {
            if (--budget.remaining < 0) return null;
            const index = offset + k;
            let x;
            if (k === -d || (k !== d && forward[index - 1] < forward[index + 1])) {
                x = forward[index + 1];
            } else {
                x = forward[index - 1] + 1;
            }
            let y = x - k;
            while (x < n && y < m && a[x] === b[y]) {
                x++;
                y++;
                if (--budget.remaining < 0) return null;
            }
            forward[index] = x;
            if (x > n) forwardEnd += 2;
            else if (y > m) forwardStart += 2;
            else if (overlapOnForward) {
                const reverseIndex = offset + delta - k;
                if (reverseIndex >= 0 && reverseIndex < length && reverse[reverseIndex] !== -1) {
                    const reverseX = n - reverse[reverseIndex];
                    if (x >= reverseX) return { x, y };
                }
            }
        }

        for (let k = -d + reverseStart; k <= d - reverseEnd; k += 2) {
            if (--budget.remaining < 0) return null;
            const index = offset + k;
            let x;
            if (k === -d || (k !== d && reverse[index - 1] < reverse[index + 1])) {
                x = reverse[index + 1];
            } else {
                x = reverse[index - 1] + 1;
            }
            let y = x - k;
            while (x < n && y < m && a[n - x - 1] === b[m - y - 1]) {
                x++;
                y++;
                if (--budget.remaining < 0) return null;
            }
            reverse[index] = x;
            if (x > n) reverseEnd += 2;
            else if (y > m) reverseStart += 2;
            else if (!overlapOnForward) {
                const forwardIndex = offset + delta - k;
                if (forwardIndex >= 0 && forwardIndex < length && forward[forwardIndex] !== -1) {
                    const forwardX = forward[forwardIndex];
                    const forwardK = delta - k;
                    const forwardY = forwardX - forwardK;
                    const reverseX = n - x;
                    if (forwardX >= reverseX) return { x: forwardX, y: forwardY };
                }
            }
        }
    }
    return null;
}
