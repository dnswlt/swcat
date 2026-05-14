import './style.css'; // Make sure Tailwind CSS gets included by vite.
import './custom-content.css';

// Self-hosted Noto Sans, latin + latin-ext only. Used solely by the SVG graph
// (via .graphviz-svg text) — node labels render at 400, edge labels at 400
// italic. The rest of the UI uses the system font stack, so no other weights
// are needed here.
import '@fontsource/noto-sans/latin-400.css';
import '@fontsource/noto-sans/latin-400-italic.css';
import '@fontsource/noto-sans/latin-ext-400.css';
import '@fontsource/noto-sans/latin-ext-400-italic.css';

// Metadata parsed from the JSON sidecar returned together with the SVG graph.
let svgMeta = {};
// <div> element used to display a tooltip when hovering over SVG elements.
let tooltip = null;

function createTooltip() {
    tooltip = document.createElement('div');
    tooltip.id = 'rich-tooltip';
    document.body.appendChild(tooltip);
}

function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

function showTooltip(info, event) {
    if (!info) return;
    const title = info.title || '';
    const attrs = info.tooltipAttrs || [];
    if (!title && attrs.length === 0) return;

    let content = '';
    if (title) {
        content += `<div class="tooltip-title">${escapeHTML(title)}</div>`;
    }
    if (attrs.length > 0) {
        content += '<dl class="tooltip-attrs">';
        attrs.forEach(attr => {
            content += `<dt>${escapeHTML(attr.Key)}</dt><dd>${escapeHTML(attr.Value)}</dd>`;
        });
        content += '</dl>';
    }

    tooltip.innerHTML = content;
    tooltip.style.display = 'block';
    updateTooltipPosition(event);
}

function hideTooltip() {
    tooltip.style.display = 'none';
}

function updateTooltipPosition(event) {
    tooltip.style.left = (event.pageX + 15) + 'px';
    tooltip.style.top = (event.pageY + 15) + 'px';
}

// Fetches the #relationships-svg-meta <script> element, parses its content as JSON,
// and stores the result in the svgMeta global variable.
function loadSVGMetadata() {
    const metaElem = document.getElementById("relationships-svg-meta");
    if (!metaElem) {
        console.warn("Document has no #relationships-svg-meta. Edge metadata disabled.");
        return;
    }

    try {
        svgMeta = JSON.parse(metaElem.textContent || "{}");
    } catch (e) {
        console.error("Failed to parse relationships metadata JSON:", e);
        svgMeta = {};
    }
}

// Searches for entities related to the given entity reference.
// Only works on the graph page - updates the search input and triggers htmx.
function searchRelatedEntities(entityRef) {
    if (document.body.dataset.page !== 'graph') {
        // Shift-click only works on the interactive graph page
        return;
    }

    const searchInput = document.querySelector('input[name="q"]');
    if (!searchInput) {
        console.error('Search input not found');
        return;
    }

    // Update search input with relation query
    searchInput.value = `rel='${entityRef}'`;

    // Trigger htmx on the form to perform the search
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

// Handles clicks on SVG nodes by looking up the URL to navigate to in
// svgMeta.routes.
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

    if (!svgMeta || !svgMeta.routes) {
        console.error("Cannot process node click: missing svgMeta.routes");
        return;
    }

    const url = svgMeta.routes.entities[id];
    if (!url) {
        console.warn("No route defined for entity:", id);
        return;
    }
    window.location.href = url;
}

// Adds all relevant listeners to the top-level SVG element
// (clicking, hovering).
function addSVGListener() {
    const svg = document.querySelector("#relationships-svg");
    if (!svg) return;

    svg.addEventListener("click", e => {
        const node = e.target.closest(".clickable-node");
        if (node) {
            onClickNode(node, e.shiftKey);
            return;
        }
    });

    svg.addEventListener('mouseover', (event) => {
        const edgeLabel = event.target.closest('g.edge text');
        if (edgeLabel) {
            const edgeGroup = edgeLabel.closest('g.edge');
            const edgeInfo = edgeGroup && svgMeta.edges && svgMeta.edges[edgeGroup.id];
            if (edgeInfo) showTooltip(edgeInfo, event);
            return;
        }
        const node = event.target.closest('.clickable-node');
        if (node) {
            const nodeInfo = svgMeta.nodes && svgMeta.nodes[node.id];
            if (nodeInfo) showTooltip(nodeInfo, event);
        }
    });

    svg.addEventListener('mouseout', (event) => {
        if (event.target.closest('g.edge text') || event.target.closest('.clickable-node')) {
            hideTooltip();
        }
    });

    svg.addEventListener('mousemove', (event) => {
        if (tooltip.style.display === 'block') {
            updateTooltipPosition(event);
        }
    });
}

// Placement strategies for popovers. Each returns the CSS positioning props
// needed to anchor one corner of the popover to one corner of its trigger,
// given the trigger's bounding rect and a gap (in px).
//
// Naming follows CSS Anchor / Floating UI: the value is "<side>-<alignment>",
// where <side> is which side of the trigger the popover sits on, and
// <alignment> is which edge of the trigger the popover lines up with.
// Coordinates are page-relative (CSS fixed/absolute against viewport edges).
const POPOVER_PLACEMENTS = {
    'bottom-start': (rect, gap) => ({
        top: `${rect.bottom + gap}px`,
        left: `${rect.left}px`,
    }),
    'bottom-end': (rect, gap) => ({
        top: `${rect.bottom + gap}px`,
        right: `${window.innerWidth - rect.right}px`,
    }),
    'top-start': (rect, gap) => ({
        bottom: `${window.innerHeight - rect.top + gap}px`,
        left: `${rect.left}px`,
    }),
    'top-end': (rect, gap) => ({
        bottom: `${window.innerHeight - rect.top + gap}px`,
        right: `${window.innerWidth - rect.right}px`,
    }),
};

// Wires up positioning and optional on-open behavior for a single popover.
// Reads declarative configuration from data-* attributes on the popover element:
//   data-popover-placement (default "bottom-start"): see POPOVER_PLACEMENTS
//   data-popover-offset    (default 4): gap in px between trigger and popover
//   data-popover-focus     (optional): CSS selector to focus when opened
//
// The trigger is located via [popovertarget="<popover-id>"]. Re-running on the
// same element is a no-op, so this is safe to call after htmx swaps that may
// replace the popover element with a fresh copy (which clears the flag).
function initPopover(popover) {
    if (popover.dataset.popoverInit === '1') return;
    const btn = document.querySelector(`[popovertarget="${popover.id}"]`);
    if (!btn) return;
    popover.dataset.popoverInit = '1';

    const placementKey = popover.dataset.popoverPlacement || 'bottom-start';
    const placement = POPOVER_PLACEMENTS[placementKey];
    if (!placement) {
        console.warn(`Unknown popover placement "${placementKey}" on #${popover.id}`);
        return;
    }
    const offset = Number(popover.dataset.popoverOffset || 4);
    const focusSel = popover.dataset.popoverFocus;

    popover.addEventListener('beforetoggle', (e) => {
        if (e.newState !== 'open') return;
        const pos = placement(btn.getBoundingClientRect(), offset);
        // Reset all sides first so a previous placement can't leak through
        // (e.g. if the placement attribute is ever swapped at runtime).
        Object.assign(popover.style, {
            top: 'unset', bottom: 'unset', left: 'unset', right: 'unset',
            ...pos,
        });
    });

    if (focusSel) {
        popover.addEventListener('toggle', (e) => {
            if (e.newState === 'open') {
                document.querySelector(focusSel)?.focus();
            }
        });
    }
}

// Initializes every [popover] element within the given root (defaults to the
// whole document). Idempotent per element, so calling this repeatedly after
// htmx swaps will only wire up popovers that have actually been (re)inserted.
function initAllPopovers(root = document) {
    root.querySelectorAll('[popover]').forEach(initPopover);
}

// Runs all initialization functions relevant for the given page identified by pageId.
async function initPage(pageId) {
    initAllPopovers();

    if (['domain', 'system', 'component', 'resource', 'api', 'graph'].includes(pageId)) {
        createTooltip();
        loadSVGMetadata();
        addSVGListener();

        // Reload the page after plugins have completed successfully.
        document.body.addEventListener("pluginsSuccess", () => {
            setTimeout(() => location.reload(), 1500);
        });

        // Re-run listener registration and SVG metadata parsing after an SVG update
        // (triggered by a HX-Trigger-After-Swap response header). Also re-init
        // popovers: OOB swaps may have replaced the trigger/popover elements
        // (e.g. #connect-popover lives inside the swapped #graph-container).
        document.body.addEventListener("svgUpdated", () => {
            loadSVGMetadata();
            addSVGListener();
            initAllPopovers();
        });

        // JSON viewer to render JSON annotations.
        const jsonViewers = document.querySelectorAll('.json-viewer');
        if (jsonViewers.length > 0) {
            const { initJsonViewer } = await import('./editor.js');
            jsonViewers.forEach(viewer => {
                initJsonViewer(viewer.id);
            });
        }
    }

    if (pageId === 'graph') {
        await import('./graph.js');
    }

    if (pageId === 'system') {
        await import('./system.js');
    }

    // YAML editor
    if (['entity-edit', 'entity-clone'].includes(pageId)) {
        const { initYamlEditor, initJsonViewer } = await import('./editor.js');
        initYamlEditor();

        // JSON viewer to render JSON annotations if present (e.g. read-only ExtensionsJSON)
        const jsonViewers = document.querySelectorAll('.json-viewer');
        jsonViewers.forEach(viewer => {
            initJsonViewer(viewer.id);
        });
    }

    if (pageId === 'entity-source') {
        const { initYamlViewer, initJsonViewer } = await import('./editor.js');
        initYamlViewer();

        // JSON viewer to render JSON annotations if present (e.g. read-only ExtensionsJSON)
        const jsonViewers = document.querySelectorAll('.json-viewer');
        jsonViewers.forEach(viewer => {
            initJsonViewer(viewer.id);
        });
    }
}

document.addEventListener("DOMContentLoaded", () => {
    initPage(document.body.dataset.page);
});
