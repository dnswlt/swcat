// Controls of a relationship view — the external view of a system or of a
// domain. Both are described by two params:
//   s=<entity-ref>...  the selected neighbors: systems on a system page,
//                      domains on a domain page. With nothing selected, every
//                      neighbor is covered.
//   detail=<level>     how far into a neighbor the view goes.
import { onConfigRequest } from './url-params.js';

/** Returns the selection after clicking `ref`, given what is selected now. */
export function nextSelection(selected, ref, extend) {
    const isSelected = selected.includes(ref);
    if (extend) {
        // Cmd/Ctrl-click adds to or removes from the selection.
        return isSelected ? selected.filter(s => s !== ref) : selected.concat(ref);
    }
    // A plain click narrows to this one, and clicking the only selected one
    // clears the selection, which covers every neighbor again.
    return isSelected && selected.length === 1 ? [] : [ref];
}

/** Builds the params for a click on a chip or on the detail control. */
export function relationshipParams(params, elt, event) {
    const action = elt.dataset.chipAction;
    if (action === 'select') {
        const extend = event?.detail?.triggeringEvent?.metaKey ||
            event?.detail?.triggeringEvent?.ctrlKey;
        const selection = nextSelection(params.getAll('s'), elt.dataset.entityRef, extend);
        return { s: selection, detail: params.getAll('detail') };
    }
    if (action === 'detail') {
        // Always explicit: the default level differs per view and may change,
        // and a shared link should keep showing what its author saw.
        return { s: params.getAll('s'), detail: [elt.dataset.detail] };
    }
    return { s: params.getAll('s'), detail: params.getAll('detail') };
}

onConfigRequest(
    (elt) => elt.dataset.chipAction === 'detail' ||
        (elt.dataset.chipAction === 'select' && !!elt.dataset.entityRef),
    relationshipParams,
);
