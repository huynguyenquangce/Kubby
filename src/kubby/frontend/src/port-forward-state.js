function sameOwner(forward, ref) {
    return !!forward && !!ref
        && forward.kind === ref.kind
        && forward.namespace === ref.namespace
        && forward.name === ref.name;
}

export function upsertForward(forwards, incoming) {
    if (!incoming?.key) return [...forwards];
    const next = forwards.filter((forward) => forward.key !== incoming.key);
    next.push(incoming);
    return next;
}

export function removeForward(forwards, key) {
    return forwards.filter((forward) => forward.key !== key);
}

export function forwardsToStopOnDrawerClose(forwards, ref) {
    return forwards
        .filter((forward) => !forward.keepRunning && sameOwner(forward, ref))
        .map((forward) => forward.key);
}

export function shouldRetainStartedForward({ drawerStillOwnsRequest, keepRunning }) {
    return !!keepRunning || !!drawerStillOwnsRequest;
}

export function canApplyForwardHydration({ connectionCurrent, requestedVersion, currentVersion }) {
    return !!connectionCurrent && requestedVersion === currentVersion;
}
