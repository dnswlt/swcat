// What a page gets is decided by the markup it contains, not by which page it
// is: a page that starts rendering a diagram, or an editor, needs no change
// here. Only genuinely page-specific behavior (see body.dataset.page in
// diagram.js) still asks what page it is on.
import { claim } from './dom.js';
import { initAllPopovers } from './popover.js';
import { initDiagram, resetPinnedEdge } from './diagram.js';

// Wires up a page's diagram, plus the events that replace or invalidate it.
function initDiagramPage() {
    initDiagram();

    if (!claim(document.body, 'diagramPageInit')) return;

    // Reload the page after plugins have completed successfully.
    document.body.addEventListener('pluginsSuccess', () => {
        setTimeout(() => location.reload(), 1500);
    });

    // A swap can bring in markup the page did not have (it is triggered by a
    // HX-Trigger-After-Swap response header): the first diagram on the graph
    // page, which only arrives once a search has been made, or the chips of an
    // external view that was opened from the internal one. So the features are
    // scanned for again rather than only the diagram being re-wired. Every one
    // of them is safe to run twice.
    // The listener hands back the promise so that a caller — a test, for now —
    // can wait for the features to have started. The DOM itself ignores it.
    document.body.addEventListener('svgUpdated', () => {
        resetPinnedEdge();
        return initFeatures();
    });
}

// Renders the JSON annotations shown read-only on entity pages.
async function initJsonViewers() {
    const { initJsonViewer } = await import('./editor.js');
    document.querySelectorAll('.json-viewer').forEach(viewer => initJsonViewer(viewer.id));
}

/** Each entry is the markup a feature needs, and how to start it. */
export const FEATURES = [
    ['[popover]', () => initAllPopovers()],
    // A diagram, or the graph page's container, which only fills with one once
    // a search has been made.
    ['#relationships-svg, #graph-container', () => initDiagramPage()],
    ['[data-chip-action]', () => import('./relationships.js')],
    ['#graph-container', () => import('./graph.js')],
    ['.json-viewer', () => initJsonViewers()],
    ['#yaml-editor', async () => (await import('./editor.js')).initYamlEditor()],
    ['#yaml-viewer', async () => (await import('./editor.js')).initYamlViewer()],
];

/**
 * Starts every feature whose markup is present. Safe to call again whenever the
 * page changes: each feature ignores what it has already wired up.
 */
export async function initFeatures() {
    for (const [selector, init] of FEATURES) {
        if (document.querySelector(selector)) {
            await init();
        }
    }
}
