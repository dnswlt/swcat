// Configures htmx requests for the controls of a relationship view — the
// external view of a system or of a domain — which build the query params from
// the current URL state and the action being performed.
//
// An external view is described by two params:
//   s=<entity-ref>...  the selected neighbors: systems on a system page,
//                      domains on a domain page. With nothing selected, every
//                      neighbor is covered.
//   detail=<level>     how far into a neighbor the view goes.
document.body.addEventListener('htmx:configRequest', (event) => {
    const elt = event.detail.elt;

    const chipAction = elt.getAttribute('data-chip-action');
    const entityRef = elt.getAttribute('data-entity-ref');

    // Every control needs an action; only the chips act on one entity.
    if (!chipAction || (chipAction === 'select' && !entityRef)) {
        return;
    }

    // Clone the params of the page we are on, then apply this control's change.
    const newParams = new URLSearchParams(window.location.search);

    if (chipAction === 'select') {
        const selected = newParams.getAll('s');
        const isSelected = selected.includes(entityRef);
        // Cmd/Ctrl-click extends the selection; a plain click replaces it, and
        // clicking the only selected chip clears it (showing all systems again).
        const extend = event.detail.triggeringEvent?.metaKey || event.detail.triggeringEvent?.ctrlKey;
        let next;
        if (extend) {
            next = isSelected ? selected.filter(s => s !== entityRef) : selected.concat(entityRef);
        } else {
            next = isSelected && selected.length === 1 ? [] : [entityRef];
        }
        newParams.delete('s');
        next.forEach(s => newParams.append('s', s));
    } else if (chipAction === 'detail') {
        // Always explicit: the default level differs per view and may change,
        // and a shared link should keep showing what its author saw.
        newParams.set('detail', elt.getAttribute('data-detail'));
    }

    // Send both params on every request: leaving one out would silently reset
    // that half of the view.
    // Use arrays for multi-valued params; Object.fromEntries would drop duplicates.
    event.detail.parameters['s'] = newParams.getAll('s');
    event.detail.parameters['detail'] = newParams.getAll('detail');
});
