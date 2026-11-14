import { test } from 'node:test';
import assert from 'node:assert/strict';
import { HOVER_ITEMS, tooltipContent } from './tooltip.js';

function apis(n) {
    const items = Array.from({ length: n }, (_, i) => ({ ref: `api:a${i}`, label: `a${i}` }));
    return { title: 'x → y', sections: [{ title: 'APIs', items }] };
}

const count = (html, s) => html.split(s).length - 1;

test('an entry with nothing to say renders nothing', () => {
    assert.equal(tooltipContent({}), '');
    assert.equal(tooltipContent(undefined), '');
});

test('the hover tooltip cuts long lists short and says how many are left', () => {
    const html = tooltipContent(apis(HOVER_ITEMS + 3), { limit: HOVER_ITEMS });
    assert.equal(count(html, '<li>'), HOVER_ITEMS);
    assert.match(html, /\+3 more/);
    // The count is of the whole list, not of what is shown.
    assert.match(html, new RegExp(`<span class="tooltip-count">${HOVER_ITEMS + 3}</span>`));
});

test('a list that fits is not cut', () => {
    const html = tooltipContent(apis(HOVER_ITEMS), { limit: HOVER_ITEMS });
    assert.equal(count(html, '<li>'), HOVER_ITEMS);
    assert.doesNotMatch(html, /more/);
});

test('the limit applies across sections', () => {
    const info = {
        sections: [
            { title: 'APIs', items: [{ ref: 'api:a', label: 'a' }, { ref: 'api:b', label: 'b' }] },
            { title: 'Resources', items: [{ ref: 'resource:r', label: 'r' }] },
        ],
    };
    const html = tooltipContent(info, { limit: 2 });
    assert.doesNotMatch(html, /Resources/);
    assert.match(html, /\+1 more/);
});

test('the pinned tooltip shows everything, linked where a route is known', () => {
    const html = tooltipContent(apis(30), { routes: { 'api:a1': '/ui/apis/a1' } });
    assert.equal(count(html, '<li'), 30);
    assert.match(html, /<a href="\/ui\/apis\/a1">a1<\/a>/);
    assert.doesNotMatch(html, /more/);
});

test('labels are escaped', () => {
    const html = tooltipContent({ sections: [{ title: '<b>', items: [{ ref: 'x', label: '<i>' }] }] });
    assert.doesNotMatch(html, /<b>|<i>/);
});
