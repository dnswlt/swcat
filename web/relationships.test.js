import { test } from 'node:test';
import assert from 'node:assert/strict';

// The module registers an htmx listener as soon as it loads, so a document has
// to exist first — hence the dynamic import. A stub is enough: these tests are
// about the params, not about the DOM.
globalThis.document = { body: { addEventListener() { } } };
const { nextSelection, relationshipParams } = await import('./relationships.js');

function chip(ref, { meta = false } = {}) {
    return [
        { dataset: { chipAction: 'select', entityRef: ref } },
        { detail: { triggeringEvent: { metaKey: meta, ctrlKey: false } } },
    ];
}

test('a plain click narrows to one neighbor', () => {
    assert.deepEqual(nextSelection([], 'domain:iam'), ['domain:iam']);
    assert.deepEqual(nextSelection(['domain:shop'], 'domain:iam'), ['domain:iam']);
});

test('clicking the only selected neighbor covers all of them again', () => {
    assert.deepEqual(nextSelection(['domain:iam'], 'domain:iam'), []);
});

test('cmd-click extends and removes', () => {
    assert.deepEqual(nextSelection(['domain:iam'], 'domain:shop', true),
        ['domain:iam', 'domain:shop']);
    assert.deepEqual(nextSelection(['domain:iam', 'domain:shop'], 'domain:iam', true),
        ['domain:shop']);
});

test('selecting keeps the detail level', () => {
    const params = new URLSearchParams('?detail=domains');
    assert.deepEqual(relationshipParams(params, ...chip('domain:iam')),
        { s: ['domain:iam'], detail: ['domains'] });
});

test('changing the level keeps the selection and is always explicit', () => {
    const params = new URLSearchParams('?s=domain:iam');
    const elt = { dataset: { chipAction: 'detail', detail: 'systems' } };
    // Even the default level goes into the URL, so a shared link keeps showing
    // what its author saw.
    assert.deepEqual(relationshipParams(params, elt, {}),
        { s: ['domain:iam'], detail: ['systems'] });
});
