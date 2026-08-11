// Async calls through the Wails bridge cannot be cancelled reliably.  Instead,
// every response carries the UI state that owns it; a response may render only
// while that state is still current.
export function resourceKey(ref = {}) {
    return [ref.kind ?? '', ref.namespace ?? '', ref.name ?? ''].join('\u0000');
}

export function createRequestScopes() {
    let connectionEpoch = 0;
    let viewEpoch = 0;
    let drawerEpoch = 0;
    let modalEpoch = 0;

    return {
        connectionChanged() {
            connectionEpoch++;
            viewEpoch++;
            drawerEpoch++;
            modalEpoch++;
        },

        connectionToken() {
            return connectionEpoch;
        },

        isCurrentConnection(token) {
            return token === connectionEpoch;
        },

        beginView(view, namespace) {
            return { connectionEpoch, epoch: ++viewEpoch, view, namespace };
        },

        isCurrentView(scope, view, namespace) {
            return !!scope
                && scope.connectionEpoch === connectionEpoch
                && scope.epoch === viewEpoch
                && scope.view === view
                && scope.namespace === namespace;
        },

        openDrawer(ref) {
            return {
                connectionEpoch,
                epoch: ++drawerEpoch,
                resourceKey: resourceKey(ref),
                ref: { ...ref },
            };
        },

        closeDrawer() {
            drawerEpoch++;
        },

        isCurrentDrawer(scope, ref) {
            return !!scope
                && scope.connectionEpoch === connectionEpoch
                && scope.epoch === drawerEpoch
                && scope.resourceKey === resourceKey(ref);
        },

        drawerOwnerKey(scope) {
            if (!scope) return '';
            return `${scope.connectionEpoch}:${scope.epoch}:${scope.resourceKey}`;
        },

        openModal(ownerKey) {
            return { connectionEpoch, epoch: ++modalEpoch, ownerKey: String(ownerKey ?? '') };
        },

        closeModal() {
            modalEpoch++;
        },

        isCurrentModal(scope, ownerKey = scope?.ownerKey) {
            return !!scope
                && scope.connectionEpoch === connectionEpoch
                && scope.epoch === modalEpoch
                && scope.ownerKey === String(ownerKey ?? '');
        },
    };
}
