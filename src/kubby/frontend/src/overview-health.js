export function overviewHealthModel({ total, errored = [], nodes = null }) {
    if (total === null) {
        return {
            tone: 'unavailable',
            icon: '?',
            title: 'Cluster health unavailable',
            summary: 'Pod status could not be loaded. Other successful sections remain live.',
            ratio: '– / –',
        };
    }

    const unhealthyPods = errored?.length ?? 0;
    const healthyPods = Math.max(total - unhealthyPods, 0);
    const ratio = `${healthyPods} / ${total}`;
    const unhealthyNodes = Array.isArray(nodes)
        ? nodes.filter((node) => node?.ready === false || (node?.pressure?.length ?? 0) > 0)
        : [];

    if (unhealthyPods > 0) {
        const percent = total > 0 ? Math.round((healthyPods / total) * 100) : 0;
        return {
            tone: percent >= 90 ? 'warning' : 'error',
            icon: '!',
            title: `${unhealthyPods} pod${unhealthyPods === 1 ? ' needs' : 's need'} attention`,
            summary: unhealthyNodes.length > 0
                ? 'Workload and infrastructure health are degraded. Open an issue below for live evidence.'
                : 'Workload health is degraded. Open an issue below for live evidence.',
            ratio,
        };
    }

    if (unhealthyNodes.length > 0) {
        return {
            tone: 'warning',
            icon: '!',
            title: `${unhealthyNodes.length} node${unhealthyNodes.length === 1 ? ' needs' : 's need'} attention`,
            summary: 'All Pods are ready, but cluster infrastructure is degraded. Review Node status below.',
            ratio,
        };
    }

    if (nodes === null) {
        return {
            tone: 'warning',
            icon: '!',
            title: total === 0 ? 'Workload health ready' : 'Workloads healthy',
            summary: total === 0
                ? 'No workloads are running. Node health could not be loaded.'
                : 'No active Pod failures were reported, but Node health could not be loaded.',
            ratio,
        };
    }

    return {
        tone: 'ok',
        icon: '✓',
        title: total === 0 ? 'Cluster ready' : 'Cluster healthy',
        summary: total === 0
            ? 'No workloads are running yet. Cluster infrastructure is available below.'
            : 'All workloads and Nodes are ready. No active Pod failures were reported.',
        ratio,
    };
}
