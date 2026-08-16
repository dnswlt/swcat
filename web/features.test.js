import { test } from 'node:test';
import assert from 'node:assert/strict';

// A page is described here by the selectors it matches. Features that need a
// real DOM are not exercised; what is under test is which of them start, and
// when. Returns the handlers registered on document.body, so the events htmx
// would fire can be fired here.
function stubDocument(present) {
    const handlers = {};
    const element = () => ({ dataset: {}, style: {}, addEventListener: () => { } });
    globalThis.document = {
        body: {
            dataset: {},
            addEventListener: (type, fn) => { handlers[type] = fn; },
            appendChild: () => { },
        },
        addEventListener: () => { },
        createElement: element,
        getElementById: () => null,
        querySelector: (sel) => (present.includes(sel) ? element() : null),
        querySelectorAll: () => [],
    };
    return handlers;
}

const DIAGRAM = '#relationships-svg, #graph-container';
const CHIPS = '[data-chip-action]';

test('a page without a diagram starts nothing', async () => {
    const handlers = stubDocument([]);
    const { initFeatures } = await import('./features.js');
    await initFeatures();
    assert.deepEqual(Object.keys(handlers), []);
});

test('a diagram page listens for swaps', async () => {
    const handlers = stubDocument([DIAGRAM]);
    const { initFeatures } = await import('./features.js');
    await initFeatures();
    assert.ok(handlers.svgUpdated, 'a diagram page has to hear about swapped-in markup');
});

// The internal view of a system or domain has no chips at all: they arrive with
// the swap that switches to the external view. Scanning for features again on
// that swap is what makes them work.
test('controls that arrive with a swap are started then', async () => {
    const handlers = stubDocument([DIAGRAM]);
    const { initFeatures } = await import('./features.js');
    await initFeatures();

    // The external view has been swapped in, chips and all.
    globalThis.document.querySelector = (sel) =>
        ([DIAGRAM, CHIPS].includes(sel) ? { dataset: {}, addEventListener: () => { } } : null);
    const configured = [];
    globalThis.document.body.addEventListener = (type) => configured.push(type);

    await handlers.svgUpdated();

    assert.ok(configured.includes('htmx:configRequest'),
        'the chips need their request handler to be registered');
});
