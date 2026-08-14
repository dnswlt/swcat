// Configures htmx requests for the system view's controls, which build the
// query params from the current URL state and the action being performed.
//
// The external view is described by two params:
//   s=<system-ref>...  the selected neighbors, shown with their parts. With
//                      nothing selected, every neighbor is shown as one box.
//   detail=<level>     how far into a system the view goes.
document.body.addEventListener('htmx:configRequest', (event) => {
    const elt = event.detail.elt;

    const chipAction = elt.getAttribute('data-chip-action');
    const systemRef = elt.getAttribute('data-system-ref');

    // Every control needs an action; only the system chips act on one system.
    if (!chipAction || (chipAction === 'select' && !systemRef)) {
        return;
    }

    // Clone the params of the page we are on, then apply this control's change.
    const newParams = new URLSearchParams(window.location.search);

    if (chipAction === 'select') {
        const selected = newParams.getAll('s');
        const isSelected = selected.includes(systemRef);
        // Cmd/Ctrl-click extends the selection; a plain click replaces it, and
        // clicking the only selected chip clears it (showing all systems again).
        const extend = event.detail.triggeringEvent?.metaKey || event.detail.triggeringEvent?.ctrlKey;
        let next;
        if (extend) {
            next = isSelected ? selected.filter(s => s !== systemRef) : selected.concat(systemRef);
        } else {
            next = isSelected && selected.length === 1 ? [] : [systemRef];
        }
        newParams.delete('s');
        next.forEach(s => newParams.append('s', s));
    } else if (chipAction === 'detail') {
        const detail = elt.getAttribute('data-detail');
        if (detail === 'apis') {
            newParams.delete('detail'); // the default needs no parameter
        } else {
            newParams.set('detail', detail);
        }
    }

    // Send both params on every request: leaving one out would silently reset
    // that half of the view.
    // Use arrays for multi-valued params; Object.fromEntries would drop duplicates.
    event.detail.parameters['s'] = newParams.getAll('s');
    event.detail.parameters['detail'] = newParams.getAll('detail');
});
