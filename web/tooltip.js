// The rich tooltip shown when hovering nodes and edges of a diagram. Graphviz
// renders native <title> tooltips, which we strip server-side, so this is the
// only tooltip in play.

let tooltip = null;

function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

/** Creates the tooltip element. Does nothing if it already exists. */
export function createTooltip() {
    if (tooltip) return;
    tooltip = document.createElement('div');
    tooltip.id = 'rich-tooltip';
    document.body.appendChild(tooltip);
}

/**
 * Shows the tooltip for a node or edge, given its metadata entry. An entry with
 * neither a title nor attributes has nothing to say, so nothing is shown.
 */
export function showTooltip(info, event) {
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

export function hideTooltip() {
    if (tooltip) tooltip.style.display = 'none';
}

export function tooltipVisible() {
    return tooltip?.style.display === 'block';
}

/**
 * Places the tooltip next to the cursor, flipping it to the other side when it
 * would otherwise run past the edge of the viewport.
 */
export function updateTooltipPosition(event) {
    const offset = 15;
    const margin = 8; // keep this much gap from the viewport edge
    const rect = tooltip.getBoundingClientRect();

    // Horizontal: place to the right of the cursor, but flip to the left if it
    // would overflow the visible right edge of the viewport.
    let left = event.pageX + offset;
    if (event.clientX + offset + rect.width > window.innerWidth - margin) {
        left = event.pageX - offset - rect.width;
    }
    // Clamp so we never push past the left edge either.
    left = Math.max(window.scrollX + margin, left);

    // Vertical: same flip logic against the bottom edge.
    let top = event.pageY + offset;
    if (event.clientY + offset + rect.height > window.innerHeight - margin) {
        top = event.pageY - offset - rect.height;
    }
    top = Math.max(window.scrollY + margin, top);

    tooltip.style.left = left + 'px';
    tooltip.style.top = top + 'px';
}
