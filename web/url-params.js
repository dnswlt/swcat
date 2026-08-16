// Query params for htmx requests are rebuilt from the URL on every request, so
// the URL stays the single source of truth and no stale hidden input or DOM
// state can contradict it.

/** Returns the query params of the page currently shown. */
export function currentParams() {
    return new URLSearchParams(window.location.search);
}

/**
 * Registers a builder for the htmx requests that `matches` accepts.
 *
 * The builder receives the current URL params and the element that triggered
 * the request, and returns the complete set of params to send — whatever htmx
 * collected itself is replaced, so a request carries exactly the state the
 * builder decided on.
 *
 * @param {(elt: Element, event: CustomEvent) => boolean} matches
 * @param {(params: URLSearchParams, elt: Element, event: CustomEvent) => Record<string, string[]>} build
 */
export function onConfigRequest(matches, build) {
    document.body.addEventListener('htmx:configRequest', (event) => {
        const elt = event.detail.elt;
        if (!matches(elt, event)) return;
        event.detail.parameters = build(currentParams(), elt, event);
    });
}

/** Compares two URLs by path, ignoring their query strings. */
export function samePath(a, b) {
    if (!a || !b) return false;
    const origin = window.location.origin;
    return new URL(a, origin).pathname === new URL(b, origin).pathname;
}
