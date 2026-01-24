export function addEntityToGraph(ref) {
    const url = new URL(window.location);
    // Check if already present to avoid duplicates
    const current = url.searchParams.getAll('e');
    if (!current.includes(ref)) {
        url.searchParams.append('e', ref);
        window.location = url;
    }
}

export function removeEntityFromGraph(ref) {
    const url = new URL(window.location);
    const current = url.searchParams.getAll('e');
    url.searchParams.delete('e');
    for (const e of current) {
        if (e !== ref) {
            url.searchParams.append('e', e);
        }
    }
    window.location = url;
}

// Attach functions to window so they can be called from inline onclick handlers
window.addEntityToGraph = addEntityToGraph;
window.removeEntityFromGraph = removeEntityFromGraph;
