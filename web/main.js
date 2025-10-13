import './style.css'; // Make sure Tailwind CSS gets included by vite.

let svgMeta = {};

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

    // Build a new URL with two context params (?context=from&context=to).
    const url = new URL(window.location.href);
    url.searchParams.append("c", from);
    url.searchParams.append("c", to);

    window.location.assign(url);
}

function onClickNode(node) {
    const id = node.id;
    if (!id) return;

    // Expects the node ID to be the fully qualified entity reference.
    const parts = id.split(":");
    if (parts.length !== 2) return;

    const kind = parts[0];
    const name = parts[1];

    if (!kind || !name) return;

    let path = "";
    switch (kind) {
        case "component":
            path = "/ui/components/";
            break;
        case "resource":
            path = "/ui/resources/";
            break;
        case "api":
            path = "/ui/apis/";
            break;
        case "system":
            path = "/ui/systems/";
            break;
        case "group":
            path = "/ui/groups/";
            break;
        case "domain":
            path = "/ui/domains/";
            break;
        default:
            console.log(`Unhandled kind ${kind} in SVG.`);
            return;
    }

    window.location.href = path + encodeURIComponent(name);
}

function addSVGListener() {
    const svg = document.querySelector("#relationships-svg");
    if (!svg) return;

    svg.addEventListener("click", e => {
        const node = e.target.closest(".clickable-node");
        if (node) {
            onClickNode(node);
            return;
        }

        const edge = e.target.closest(".clickable-edge");
        if (edge) {
            onClickEdge(edge);
            return;
        }

    });
}

async function initPage(pageId) {
    if (['domain', 'system', 'component', 'resource', 'api', 'group'].includes(pageId)) {
        loadSVGMetadata();
        addSVGListener();
    }
    if (['entity-edit', 'entity-clone'].includes(pageId)) {
        const { initEditor } = await import('./editor.js');
        initEditor();
    }
}

document.addEventListener("DOMContentLoaded", () => {
    initPage(document.body.dataset.page);
});
