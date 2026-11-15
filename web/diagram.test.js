import { test } from 'node:test';
import assert from 'node:assert/strict';
import { toggleExpanded } from './diagram.js';

const page = 'http://localhost/ui/components/c';

test('shift-clicking an API expands it, keeping the others', () => {
    const url = toggleExpanded(`${page}?e=api:a`, 'api:b');
    assert.deepEqual(url.searchParams.getAll('e'), ['api:a', 'api:b']);
});

test('shift-clicking an expanded API collapses it', () => {
    const url = toggleExpanded(`${page}?e=api:a&e=api:b`, 'api:a');
    assert.deepEqual(url.searchParams.getAll('e'), ['api:b']);
});

test('the fragment is dropped: the diagram is replaced in place', () => {
    assert.equal(toggleExpanded(`${page}#relationships-svg`, 'api:a').hash, '');
});
