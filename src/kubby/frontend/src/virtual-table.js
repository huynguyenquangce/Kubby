const DEFAULT_ROW_HEIGHT = 48;
const DEFAULT_OVERSCAN = 10;
const VIRTUALIZE_AT = 80;

const states = new Map();

function scrollOwner(body) {
    return body.closest('.content') || document.scrollingElement;
}

function makeSpacer(body, height) {
    const tr = document.createElement('tr');
    tr.className = 'virtual-spacer';
    tr.setAttribute('aria-hidden', 'true');
    tr.setAttribute('role', 'presentation');
    const td = document.createElement('td');
    td.colSpan = body.closest('table')?.querySelectorAll('thead th').length || 1;
    td.style.height = `${Math.max(0, height)}px`;
    tr.appendChild(td);
    return tr;
}

function render(state) {
    const { body, filtered, rowHeight, overscan } = state;
    const table = body.closest('table');
    const owner = scrollOwner(body);
    if (!table || !owner) return;

    const useWindow = filtered.length >= VIRTUALIZE_AT;
    let start = 0;
    let end = filtered.length;
    if (useWindow) {
        const ownerRect = owner.getBoundingClientRect();
        const bodyTop = body.getBoundingClientRect().top - ownerRect.top + owner.scrollTop;
        const visibleTop = Math.max(0, owner.scrollTop - bodyTop);
        start = Math.max(0, Math.floor(visibleTop / rowHeight) - overscan);
        end = Math.min(filtered.length, Math.ceil((visibleTop + owner.clientHeight) / rowHeight) + overscan);

        const activeRow = document.activeElement?.closest?.('tr');
        const activeIndex = filtered.indexOf(activeRow);
        if (activeIndex >= 0) {
            start = Math.min(start, activeIndex);
            end = Math.max(end, activeIndex + 1);
        }
    }

    const fragment = document.createDocumentFragment();
    if (useWindow && start > 0) fragment.appendChild(makeSpacer(body, start * rowHeight));
    for (let i = start; i < end; i++) {
        const row = filtered[i];
        row.hidden = false;
        row.setAttribute('aria-rowindex', String(i + 2));
        fragment.appendChild(row);
    }
    if (useWindow && end < filtered.length) fragment.appendChild(makeSpacer(body, (filtered.length - end) * rowHeight));
    body.replaceChildren(fragment);
    table.setAttribute('aria-rowcount', String(filtered.length + 1));
    state.start = start;
    state.end = end;

    if (!state.filtering && state.hasMore && end >= filtered.length - 20) state.onNearEnd?.();
}

function schedule(state) {
    if (state.frame) return;
    state.frame = requestAnimationFrame(() => {
        state.frame = 0;
        render(state);
    });
}

export function setVirtualRows(body, rows, options = {}) {
    clearVirtualRows(body);
    const owner = scrollOwner(body);
    const state = {
        body,
        rows: [...rows],
        filtered: [...rows],
        rowHeight: options.rowHeight || DEFAULT_ROW_HEIGHT,
        overscan: options.overscan || DEFAULT_OVERSCAN,
        onNearEnd: options.onNearEnd || null,
        hasMore: !!options.hasMore,
        filtering: false,
        frame: 0,
        start: 0,
        end: 0,
        owner,
    };
    state.onScroll = () => schedule(state);
    owner?.addEventListener('scroll', state.onScroll, { passive: true });
    states.set(body, state);
    render(state);
}

export function appendVirtualRows(body, rows, { hasMore } = {}) {
    const state = states.get(body);
    if (!state) {
        setVirtualRows(body, rows, { hasMore });
        return;
    }
    state.rows.push(...rows);
    state.filtered.push(...rows);
    state.hasMore = !!hasMore;
    render(state);
}

export function updateVirtualPaging(body, { hasMore, onNearEnd } = {}) {
    const state = states.get(body);
    if (!state) return;
    state.hasMore = !!hasMore;
    if (onNearEnd) state.onNearEnd = onNearEnd;
}

export function filterVirtualRows(body, predicate) {
    const state = states.get(body);
    if (!state) return null;
    state.filtering = !!predicate;
    state.filtered = predicate ? state.rows.filter(predicate) : [...state.rows];
    render(state);
    return { shown: state.filtered.length, total: state.rows.length };
}

export function sortVirtualRows(body, compare) {
    const state = states.get(body);
    if (!state) return false;
    state.rows.sort(compare);
    const visible = new Set(state.filtered);
    state.filtered = state.rows.filter((row) => visible.has(row));
    render(state);
    return true;
}

export function allTableRows(body) {
    return states.get(body)?.rows ?? [...body.querySelectorAll('tr:not(.virtual-spacer)')];
}

export function clearVirtualRows(body) {
    const state = states.get(body);
    if (state) {
        state.owner?.removeEventListener('scroll', state.onScroll);
        if (state.frame) cancelAnimationFrame(state.frame);
        states.delete(body);
    }
    body.closest('table')?.removeAttribute('aria-rowcount');
    body.replaceChildren();
}

export function clearVirtualSelections() {
    for (const state of states.values()) {
        for (const row of state.rows) {
            row.classList.remove('selected');
            const checkbox = row.querySelector('.row-check');
            if (checkbox) checkbox.checked = false;
        }
    }
}
