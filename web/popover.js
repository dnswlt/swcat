// Positioning for native popovers, which the UA stylesheet centers in the
// viewport; these are anchored to the button that opens them instead.
import { claim } from './dom.js';

// Placement strategies. Each returns the CSS positioning props needed to anchor
// one corner of the popover to one corner of its trigger, given the trigger's
// bounding rect and a gap (in px).
//
// Naming follows CSS Anchor / Floating UI: the value is "<side>-<alignment>",
// where <side> is which side of the trigger the popover sits on, and
// <alignment> is which edge of the trigger the popover lines up with.
// Coordinates are page-relative (CSS fixed/absolute against viewport edges).
const PLACEMENTS = {
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
//   data-popover-placement (default "bottom-start"): see PLACEMENTS
//   data-popover-offset    (default 4): gap in px between trigger and popover
//   data-popover-focus     (optional): CSS selector to focus when opened
//
// The trigger is located via [popovertarget="<popover-id>"].
function initPopover(popover) {
    const btn = document.querySelector(`[popovertarget="${popover.id}"]`);
    if (!btn || !claim(popover, 'popoverInit')) return;

    const placementKey = popover.dataset.popoverPlacement || 'bottom-start';
    const placement = PLACEMENTS[placementKey];
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

/**
 * Initializes every [popover] element within the given root (defaults to the
 * whole document). Idempotent per element, so calling this repeatedly after
 * htmx swaps only wires up popovers that have actually been (re)inserted.
 */
export function initAllPopovers(root = document) {
    root.querySelectorAll('[popover]').forEach(initPopover);
}
