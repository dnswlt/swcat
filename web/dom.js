/**
 * Marks an element as wired up for the given feature, returning false if it
 * already was. htmx swaps put fresh, unmarked elements in place, so those get
 * wired up again, while a repeated call on the same element does nothing —
 * listeners are never stacked twice on one element.
 */
export function claim(el, feature) {
    if (el.dataset[feature] === '1') return false;
    el.dataset[feature] = '1';
    return true;
}
