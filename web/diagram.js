// The SVG diagram embedded in entity pages and the graph page: its metadata
// sidecar, clickable nodes, hoverable and pinnable edges.
//
// The SVG itself comes from the server, either from graphviz or from the
// built-in layout engine. Both label their elements the same way — nodes carry
// the entity ref as their id and a "clickable-node" class, edges are grouped in
// "svg-edge-N" — which is what lets this work against either of them.
import { claim } from './dom.js';
import { createTooltip, hideTooltip, showTooltip, tooltipVisible, updateTooltipPosition } from './tooltip.js';

/** Metadata for the elements of the current diagram, keyed by element id. */
let meta = emptyMeta();

function emptyMeta() {
    return { nodes: {}, edges: {}, clusters: {}, routes: { entities: {} } };
}

/**
 * Reads the JSON sidecar that ships with the SVG. Missing parts are filled in,
 * so callers can look entries up without guarding every level.
 */
export function parseMetadata(text) {
    const raw = JSON.parse(text || '{}');
    return {
        nodes: raw.nodes || {},
        edges: raw.edges || {},
        clusters: raw.clusters || {},
        routes: { entities: raw.routes?.entities || {} },
    };
}

function loadMetadata() {
    const el = document.getElementById('relationships-svg-meta');
    if (!el) {
        meta = emptyMeta();
        return;
    }
    try {
        meta = parseMetadata(el.textContent);
    } catch (e) {
        console.error('Failed to parse relationships metadata JSON:', e);
        meta = emptyMeta();
    }
}

// Searches for entities related to the given entity reference. Only works on
// the graph page, where the search input drives the diagram.
function searchRelatedEntities(entityRef) {
    if (document.body.dataset.page !== 'graph') return;

    const searchInput = document.querySelector('input[name="q"]');
    if (!searchInput) {
        console.error('Search input not found');
        return;
    }
    searchInput.value = `rel='${entityRef}'`;
    const form = searchInput.closest('form');
    if (form) {
        htmx.trigger(form, 'submit');
    }
}

// Toggles entityRef in the e= query param list of the current URL and navigates.
// Used on the component detail page to expand/collapse API consumers/providers.
function toggleExpandedEntity(entityRef) {
    const url = new URL(window.location.href);
    const already = url.searchParams.getAll('e');
    url.searchParams.delete('e');
    if (already.includes(entityRef)) {
        already.filter(e => e !== entityRef).forEach(e => url.searchParams.append('e', e));
    } else {
        already.concat(entityRef).forEach(e => url.searchParams.append('e', e));
    }
    url.hash = 'relationships-svg';
    window.location.href = url.toString();
}

// Handles clicks on diagram nodes: shift-click searches for related entities
// (or expands an API on a component page), a plain click navigates.
function onClickNode(node, shiftKey) {
    const id = node.id;
    if (!id) return;

    if (shiftKey) {
        if (document.body.dataset.page === 'component' && id.startsWith('api:')) {
            toggleExpandedEntity(id);
            return;
        }
        searchRelatedEntities(id);
        return;
    }

    const url = meta.routes.entities[id];
    if (!url) {
        console.warn('No route defined for entity:', id);
        return;
    }
    window.location.href = url;
}

// Id of the edge kept highlighted by a click, or null if none is pinned. The
// highlight itself is the .pinned CSS class; this only records which edge
// carries it, so it can be taken off again.
let pinnedEdgeId = null;

function clearPinnedEdge() {
    if (!pinnedEdgeId) return;
    document.getElementById(pinnedEdgeId)?.classList.remove('pinned');
    pinnedEdgeId = null;
}

// Pins the given edge, or unpins it if it was already pinned. Only one edge is
// pinned at a time.
function togglePinnedEdge(edge) {
    const wasPinned = edge.id === pinnedEdgeId;
    clearPinnedEdge();
    if (!wasPinned && edge.id) {
        edge.classList.add('pinned');
        pinnedEdgeId = edge.id;
    }
}

// Registers the document-level ways of dropping a pinned edge: clicking outside
// the diagram, or pressing Escape. Clicks inside it are handled by the
// container's own listeners, which are replaced along with the diagram.
function initEdgePinning() {
    if (!claim(document.body, 'edgePinningInit')) return;
    document.addEventListener('click', (event) => {
        if (!event.target.closest('#relationships-svg')) {
            clearPinnedEdge();
        }
    });
    document.addEventListener('keydown', (event) => {
        if (event.key === 'Escape') {
            clearPinnedEdge();
        }
    });
}

// Adds the click and hover listeners to the diagram's container. Safe to call
// again after an htmx swap: a container that already carries them is skipped.
function addDiagramListeners() {
    const svg = document.querySelector('#relationships-svg');
    if (!svg || !claim(svg, 'svgListeners')) return;

    svg.addEventListener('click', e => {
        const node = e.target.closest('.clickable-node');
        if (node) {
            onClickNode(node, e.shiftKey);
            return;
        }
        const edge = e.target.closest('g.edge');
        if (edge) {
            togglePinnedEdge(edge);
            return;
        }
        // A click on the diagram background drops the pin.
        clearPinnedEdge();
    });

    svg.addEventListener('mouseover', (event) => {
        // Anywhere on the edge: an external view draws no edge labels, so
        // hovering the line is how an edge is read.
        const edge = event.target.closest('g.edge');
        if (edge) {
            showTooltip(meta.edges[edge.id], event);
            return;
        }
        const node = event.target.closest('.clickable-node');
        if (node) {
            showTooltip(meta.nodes[node.id], event);
        }
    });

    svg.addEventListener('mouseout', (event) => {
        if (event.target.closest('g.edge') || event.target.closest('.clickable-node')) {
            hideTooltip();
        }
    });

    svg.addEventListener('mousemove', (event) => {
        if (tooltipVisible()) {
            updateTooltipPosition(event);
        }
    });
}

/**
 * Wires up the diagram on this page. Idempotent, and called again whenever an
 * htmx swap replaces the diagram — including the first one on the graph page,
 * which starts out without a diagram at all.
 */
export function initDiagram() {
    createTooltip();
    loadMetadata();
    addDiagramListeners();
    initEdgePinning();
}

/** Forgets the pinned edge, whose element is gone after a swap. */
export function resetPinnedEdge() {
    pinnedEdgeId = null;
}
