// The rich tooltips of a diagram. Graphviz renders native <title> tooltips,
// which we strip server-side, so these are the only tooltips in play.
//
// There are two of them. The hover tooltip follows the cursor and cannot be
// entered, so it has to stay short: lists in it are cut off. The pinned tooltip
// belongs to a pinned edge; it stays where it was opened, can be scrolled, and
// shows everything, with links.

/** How many list entries the hover tooltip shows before it refers to pinning. */
export const HOVER_ITEMS = 10;

let tooltip = null;
let pinned = null;

function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

/** Creates the tooltip elements. Does nothing if they already exist. */
export function createTooltip() {
    if (tooltip) return;
    tooltip = document.createElement('div');
    tooltip.id = 'rich-tooltip';
    tooltip.className = 'rich-tooltip';
    document.body.appendChild(tooltip);

    pinned = document.createElement('div');
    pinned.id = 'pinned-tooltip';
    pinned.className = 'rich-tooltip';
    document.body.appendChild(pinned);
}

/**
 * Renders the content of a tooltip for a node or edge, given its metadata
 * entry, or returns '' if the entry has nothing to say. Lists stop after limit
 * entries in all; routes maps entity refs to URLs, which turns entries into
 * links.
 */
export function tooltipContent(info, { limit = Infinity, routes = null } = {}) {
    const title = info?.title || '';
    const attrs = info?.tooltipAttrs || [];
    const sections = info?.sections || [];
    if (!title && attrs.length === 0 && sections.length === 0) return '';

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

    let left = limit;
    let hidden = 0;
    for (const section of sections) {
        const items = section.items || [];
        const shown = items.slice(0, Math.max(0, left));
        left -= shown.length;
        hidden += items.length - shown.length;
        if (shown.length === 0) continue;

        content += '<div class="tooltip-section">';
        content += `<div class="tooltip-section-title">${escapeHTML(section.title)}` +
            ` <span class="tooltip-count">${items.length}</span></div><ul>`;
        for (const item of shown) {
            const label = escapeHTML(item.label);
            const url = routes?.[item.ref];
            content += url ? `<li><a href="${escapeHTML(url)}">${label}</a></li>` : `<li>${label}</li>`;
        }
        content += '</ul></div>';
    }
    if (hidden > 0) {
        content += `<div class="tooltip-more">+${hidden} more · click to show all</div>`;
    }
    return content;
}

/**
 * Shows the hover tooltip for a node or edge, given its metadata entry. An
 * entry with nothing to say shows nothing.
 */
export function showTooltip(info, event) {
    const content = tooltipContent(info, { limit: HOVER_ITEMS });
    if (!content) return;
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
 * Opens the pinned tooltip at the position of event, with the complete content
 * of info and links resolved through routes. Returns whether there was
 * anything to show. It closes along with the pin: a click anywhere outside it,
 * or Escape (see diagram.js).
 */
export function showPinnedTooltip(info, event, routes) {
    const content = tooltipContent(info, { routes });
    if (!content) {
        hidePinnedTooltip();
        return false;
    }
    pinned.innerHTML = content;
    pinned.scrollTop = 0;
    pinned.style.display = 'block';
    placeNextToCursor(pinned, event);
    return true;
}

export function hidePinnedTooltip() {
    if (pinned) pinned.style.display = 'none';
}

/** Reports whether target is inside the pinned tooltip. */
export function inPinnedTooltip(target) {
    return !!pinned?.contains(target);
}

/** Keeps the hover tooltip next to the cursor. */
export function updateTooltipPosition(event) {
    placeNextToCursor(tooltip, event);
}

/**
 * Places el next to the cursor, flipping it to the other side when it would
 * otherwise run past the edge of the viewport.
 */
function placeNextToCursor(el, event) {
    const offset = 15;
    const margin = 8; // keep this much gap from the viewport edge
    const rect = el.getBoundingClientRect();

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

    el.style.left = left + 'px';
    el.style.top = top + 'px';
}
