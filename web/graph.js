// Controls of the interactive graph page. Every one of them sends the page's
// state to the graph endpoint, so requests are recognized by where they go
// rather than by what triggered them: the search form has no action attribute,
// but its request carries the same state as the entity buttons do.
import { onConfigRequest, samePath } from './url-params.js';

/** The graph endpoint, which the page tells us about. */
function graphURL() {
    return document.querySelector('[data-graph-url]')?.dataset.graphUrl;
}

/** Returns the entity selection after applying an add or remove action. */
export function nextEntities(entities, action, ref) {
    if (action === 'add-entity' && ref && !entities.includes(ref)) {
        return entities.concat(ref);
    }
    if (action === 'remove-entity' && ref) {
        return entities.filter(e => e !== ref);
    }
    return entities;
}

/** Builds the params for a request to the graph endpoint. */
export function graphParams(params, elt) {
    const action = elt.dataset.graphAction;
    const entities = nextEntities(params.getAll('e'), action, elt.dataset.entityRef);

    let query = params.get('q') || '';
    if (!action) {
        // No action means this is the search form submitting itself.
        query = elt.closest('form')?.querySelector('input[name="q"]')?.value ?? query;
    }

    let clusters = params.get('clusters') || '';
    if (action === 'toggle-clusters') {
        clusters = elt.checked ? '1' : '';
    }

    const out = {};
    if (query) out.q = query;
    if (entities.length > 0) out.e = entities;
    if (clusters) out.clusters = clusters;

    // Tell the backend that the entity set changed, rather than just the query.
    if (action === 'add-entity' || action === 'remove-entity' || action === 'toggle-clusters') {
        out.refresh = 'full';
    }

    // "Fully connect" expands the selection server-side and answers with an
    // HX-Redirect, so nothing is changed here — the current state is forwarded.
    if (action === 'fully-connect') {
        out.connect = 'full';
        const maxDepth = document.getElementById('max-depth')?.value;
        if (maxDepth && maxDepth !== '0') {
            out.maxDepth = maxDepth;
        }
    }
    return out;
}

/**
 * Whether an htmx request belongs to the graph page: it is one that goes to the
 * graph endpoint, whatever triggered it.
 */
export function isGraphRequest(event) {
    return samePath(event.detail.path, graphURL());
}

onConfigRequest((elt, event) => isGraphRequest(event), graphParams);
