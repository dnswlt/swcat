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

// Handles clicks on SVG edges.
// Finds the source and target of the given edge in svgMeta and adds
// their IDs as c= query parameters to the URL.
function onClickEdge(edge) {
    if (!svgMeta || !svgMeta.edges) {
        console.error("Metadata missing 'edges' object.");
        return;
    }

    const edgeInfo = svgMeta.edges[edge.id];
    if (!edgeInfo) {
        console.error(`No edge info for edge with ID ${edge.id}`);
        return;
    }

    const { from, to } = edgeInfo;
    if (!from || !to) {
        console.error(`Edge ${edge.id} is missing 'from' or 'to' fields.`);
        return;
    }

    // Build a new URL with two context params (?c=from&c=to).
    const url = new URL(window.location.href);
    url.searchParams.append("c", from);
    url.searchParams.append("c", to);

    window.location.assign(url);
}

// Handles clicks on SVG nodes by looking up the URL to navigate to in 
// svgMeta.routes.
function onClickNode(node, shiftKey) {
    const id = node.id;
    if (!id) return;

    if (shiftKey) {
        if (document.body.dataset.page !== 'graph') {
            // Shift-click only works on the interactive graph page.
            return;
        }
        const url = new URL(window.location);
        url.searchParams.set("q", `rel='${id}'`);
        window.location.href = url;
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

        const edge = e.target.closest(".clickable-edge");
        if (edge) {
            onClickEdge(edge);
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

// Runs all initialization functions relevant for the given page identified by pageId.
async function initPage(pageId) {
    if (['domain', 'system', 'component', 'resource', 'api', 'graph'].includes(pageId)) {
        createTooltip();
        loadSVGMetadata();
        addSVGListener();

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

    // YAML editor
    if (['entity-edit', 'entity-clone'].includes(pageId)) {
        const { initYamlEditor } = await import('./editor.js');
        initYamlEditor();
    }

    if (pageId === 'entity-source') {
        const { initYamlViewer } = await import('./editor.js');
        initYamlViewer();
    }
}

document.addEventListener("DOMContentLoaded", () => {
    initPage(document.body.dataset.page);
});
