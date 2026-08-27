// Own one in-flight result by a stable UI key. Selecting a different key
// invalidates old work; selecting the same key preserves it across a remount
// such as closing and reopening the same resource drawer.
export function createKeyedRequestOwner() {
    let currentKey = '';
    let generation = 0;

    return {
        select(key) {
            const next = String(key ?? '');
            if (next === currentKey) return false;
            currentKey = next;
            generation++;
            return true;
        },

        begin(key) {
            const requestKey = String(key ?? '');
            if (requestKey !== currentKey) throw new Error('Request key is not selected.');
            return { key: requestKey, generation: ++generation };
        },

        isCurrent(token) {
            return !!token
                && token.key === currentKey
                && token.generation === generation;
        },
    };
}
