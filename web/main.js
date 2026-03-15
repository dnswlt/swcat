import './style.css'; // Make sure Tailwind CSS gets included by vite.

// Metadata parsed from the JSON sidecar returned together with the SVG graph.
let svgMeta = {};
// <div> element used to display a tooltip when hovering over SVG elements.
let tooltip = null;

function createTooltip() {
    tooltip = document.createElement('div');
    tooltip.id = 'rich-tooltip';
    document.body.appendChild(tooltip);
}

function showTooltip(edgeId, event) {
    if (!svgMeta.edges) return;
    const edgeInfo = svgMeta.edges[edgeId];
    if (!edgeInfo || !edgeInfo.tooltipAttrs) return;

    let content = '';

    edgeInfo.tooltipAttrs.forEach(attr => {
        if (attr.Key) {
            content += `<p><strong>${attr.Key}:</strong> ${attr.Value}</p>`;
        } else {
            content += `<p>${attr.Value}</p>`;
        }
    });

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

// Handles clicks on SVG nodes by looking up the URL to navigate to in
// svgMeta.routes.
function onClickNode(node, shiftKey) {
    const id = node.id;
    if (!id) return;

    if (shiftKey) {
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
        const label = event.target.closest('g.edge text');
        if (label) {
            const edgeGroup = label.closest('g.edge');
            if (edgeGroup && edgeGroup.id) {
                showTooltip(edgeGroup.id, event);
            }
        }
    });

    svg.addEventListener('mouseout', (event) => {
        const label = event.target.closest('g.edge text');
        if (label) {
            hideTooltip();
        }
    });

    svg.addEventListener('mousemove', (event) => {
        if (tooltip.style.display === 'block') {
            updateTooltipPosition(event);
        }
    });
}

// Positions the session popover above the session button when it opens.
function initSessionPopover() {
    const btn = document.getElementById('session-btn');
    const popover = document.getElementById('session-popover');
    if (!btn || !popover) return;

    popover.addEventListener('beforetoggle', (e) => {
        if (e.newState === 'open') {
            const rect = btn.getBoundingClientRect();
            popover.style.bottom = `${window.innerHeight - rect.top + 4}px`;
            popover.style.top = 'unset';
            popover.style.right = `${window.innerWidth - rect.right}px`;
            popover.style.left = 'unset';
        }
    });

    popover.addEventListener('toggle', (e) => {
        if (e.newState === 'open') {
            document.getElementById('session-prefix-input')?.focus();
        }
    });
}

// Positions the plugin popover below the plugin button when it opens.
function initPluginPopover() {
    const btn = document.getElementById('plugin-btn');
    const popover = document.getElementById('plugin-popover');
    if (!btn || !popover) return;

    popover.addEventListener('beforetoggle', (e) => {
        if (e.newState === 'open') {
            const rect = btn.getBoundingClientRect();
            popover.style.top = `${rect.bottom + 4}px`;
            popover.style.left = `${rect.left}px`;
        }
    });
}

// Positions the documents popover below the docs button when it opens.
function initDocsPopover() {
    const btn = document.getElementById('docs-btn');
    const popover = document.getElementById('docs-popover');
    if (!btn || !popover) return;

    popover.addEventListener('beforetoggle', (e) => {
        if (e.newState === 'open') {
            const rect = btn.getBoundingClientRect();
            popover.style.top = `${rect.bottom + 4}px`;
            popover.style.left = `${rect.left}px`;
        }
    });
}

// Runs all initialization functions relevant for the given page identified by pageId.
async function initPage(pageId) {
    initSessionPopover();

    if (['domain', 'system', 'component', 'resource', 'api', 'graph'].includes(pageId)) {
        createTooltip();
        loadSVGMetadata();
        addSVGListener();
        initPluginPopover();

        // Reload the page after plugins have completed successfully.
        document.body.addEventListener("pluginsSuccess", () => {
            setTimeout(() => location.reload(), 1500);
        });

        // Re-run listener registration and SVG metadata parsing after an SVG update
        // (triggered by a HX-Trigger-After-Swap response header).
        document.body.addEventListener("svgUpdated", () => {
            loadSVGMetadata();
            addSVGListener();
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

    if (pageId === 'documents') {
        initDocsPopover();
    }
}

document.addEventListener("DOMContentLoaded", () => {
    initPage(document.body.dataset.page);
});
