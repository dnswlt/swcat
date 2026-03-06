// State management: URL is the single source of truth
// We read from URL params, not from DOM state

/**
 * Gets all currently selected entity refs from URL
 * @returns {string[]} Array of entity refs
 */
function getSelectedEntities() {
    const url = new URL(window.location);
    return url.searchParams.getAll('e');
}

/**
 * Gets current search query from URL
 * @returns {string} Current query
 */
function getSearchQuery() {
    const url = new URL(window.location);
    return url.searchParams.get('q') || '';
}

/**
 * Gets current clusters setting from URL
 * @returns {string} '1' if clusters enabled, '' otherwise
 */
function getClusters() {
    const url = new URL(window.location);
    return url.searchParams.get('clusters') || '';
}

/**
 * Configures htmx requests to dynamically build query params based on current URL state
 * and the action being performed (add/remove entity).
 *
 * This handler intercepts ALL requests to /ui/graph to ensure query params are always
 * built from the URL (single source of truth), avoiding stale hidden input values.
 */
document.body.addEventListener('htmx:configRequest', (event) => {
    const elt = event.detail.elt;
    const path = event.detail.path;

    // Only intercept requests to the graph endpoint
    // Handles both /ui/graph and /ui/ref/<ref>/-/graph
    if (!path.endsWith('/graph') && !path.match(/\/graph\?/)) {
        return;
    }

    const action = elt.getAttribute('data-graph-action');
    const entityRef = elt.getAttribute('data-entity-ref');

    // Get current state from URL
    let selectedEntities = getSelectedEntities();
    let query = getSearchQuery();
    let clusters = getClusters();

    // Apply action (if any)
    if (action === 'add-entity' && entityRef) {
        // Add entity if not already present
        if (!selectedEntities.includes(entityRef)) {
            selectedEntities.push(entityRef);
        }
    } else if (action === 'remove-entity' && entityRef) {
        // Remove entity
        selectedEntities = selectedEntities.filter(e => e !== entityRef);
    } else if (action === 'toggle-clusters') {
        clusters = elt.checked ? '1' : '';
    } else if (!action) {
        // This is a search request - read query from the form input
        const form = elt.closest('form');
        if (form) {
            const queryInput = form.querySelector('input[name="q"]');
            if (queryInput) {
                query = queryInput.value || '';
            }
        }
    }

    // Build query params for the request
    // Clear existing params and rebuild from computed state
    event.detail.parameters = {};

    if (query) {
        event.detail.parameters['q'] = query;
    }

    // Add all selected entities
    if (selectedEntities.length > 0) {
        event.detail.parameters['e'] = selectedEntities;
    }

    if (clusters) {
        event.detail.parameters['clusters'] = clusters;
    }

    // Signal to backend that this is an entity change operation (not just search)
    if (action === 'add-entity' || action === 'remove-entity' || action === 'toggle-clusters') {
        event.detail.parameters['refresh'] = 'full';
    }
});

// Note: svgUpdated event is already handled in main.js (initPage function)
// for all pages that render SVGs, including the graph page.
