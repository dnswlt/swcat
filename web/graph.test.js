import { test } from 'node:test';
import assert from 'node:assert/strict';

// The module registers an htmx listener as soon as it loads, so a document has
// to exist first — hence the dynamic import.
let graphContainer = null;
globalThis.document = {
    body: { addEventListener() { } },
    querySelector: (sel) => (sel === '[data-graph-url]' ? graphContainer : null),
    getElementById: () => null,
};
globalThis.window = { location: { origin: 'http://localhost:9191' } };
const { graphParams, isGraphRequest, nextEntities } = await import('./graph.js');

const noAction = { dataset: {}, closest: () => null };

test('adding an entity is idempotent', () => {
    assert.deepEqual(nextEntities([], 'add-entity', 'system:a'), ['system:a']);
    assert.deepEqual(nextEntities(['system:a'], 'add-entity', 'system:a'), ['system:a']);
});

test('removing an entity leaves the others', () => {
    assert.deepEqual(nextEntities(['system:a', 'system:b'], 'remove-entity', 'system:a'),
        ['system:b']);
});

test('an entity change carries the query and asks for a full refresh', () => {
    const params = new URLSearchParams('?q=dispo&e=system:a');
    const elt = { dataset: { graphAction: 'add-entity', entityRef: 'system:b' }, closest: () => null };
    assert.deepEqual(graphParams(params, elt),
        { q: 'dispo', e: ['system:a', 'system:b'], refresh: 'full' });
});

test('a search reads the query from its form and keeps the entities', () => {
    const params = new URLSearchParams('?q=old&e=system:a');
    const elt = {
        dataset: {},
        closest: () => ({ querySelector: () => ({ value: 'new query' }) }),
    };
    assert.deepEqual(graphParams(params, elt), { q: 'new query', e: ['system:a'] });
});

test('empty params are left out entirely', () => {
    assert.deepEqual(graphParams(new URLSearchParams(''), noAction), {});
});

test('toggling clusters follows the checkbox', () => {
    const on = { dataset: { graphAction: 'toggle-clusters' }, checked: true, closest: () => null };
    assert.deepEqual(graphParams(new URLSearchParams('?clusters=1'), on),
        { clusters: '1', refresh: 'full' });

    const off = { dataset: { graphAction: 'toggle-clusters' }, checked: false, closest: () => null };
    assert.deepEqual(graphParams(new URLSearchParams('?clusters=1'), off), { refresh: 'full' });
});

test('only requests to the graph endpoint are intercepted', () => {
    graphContainer = { dataset: { graphUrl: '/ui/graph' } };
    assert.equal(isGraphRequest({ detail: { path: '/ui/graph' } }), true);
    // The search form appends its query to the same endpoint.
    assert.equal(isGraphRequest({ detail: { path: '/ui/graph?q=dispo' } }), true);
    assert.equal(isGraphRequest({ detail: { path: '/ui/systems/dispo' } }), false);
    // A page that renders no graph never matches, whatever it requests.
    graphContainer = null;
    assert.equal(isGraphRequest({ detail: { path: '/ui/graph' } }), false);
});

test('the endpoint is read from the page, not guessed', () => {
    // Catalogs served under a prefix have a different graph path.
    graphContainer = { dataset: { graphUrl: '/ui/ref/main/-/graph' } };
    assert.equal(isGraphRequest({ detail: { path: '/ui/ref/main/-/graph?e=system:a' } }), true);
    assert.equal(isGraphRequest({ detail: { path: '/ui/graph' } }), false);
});
