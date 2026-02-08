// Configures htmx requests for system chips to dynamically build query params
// based on current URL state and the chip action being performed.
document.body.addEventListener('htmx:configRequest', (event) => {
    const elt = event.detail.elt;

    // Only intercept requests from system chips
    const chipAction = elt.getAttribute('data-chip-action');
    const systemRef = elt.getAttribute('data-system-ref');

    if (!chipAction || !systemRef) {
        return;
    }

    // Get current URL params
    const url = new URL(window.location);
    const params = url.searchParams;

    // Clone current params
    const newParams = new URLSearchParams(params);

    // Apply chip action - each chip manages only its own parameters
    if (chipAction === 'solo') {
        // o=<me>: show only this system (exclusive)
        newParams.delete('o');
        newParams.delete('x');
        newParams.set('o', systemRef);
    } else if (chipAction === 'toggle-detail') {
        // c=<me>: toggle detail view
        const contextSystems = newParams.getAll('c');
        if (contextSystems.includes(systemRef)) {
            // Remove this system from context
            newParams.delete('c');
            contextSystems.filter(s => s !== systemRef).forEach(s => newParams.append('c', s));
        } else {
            // Add this system to context
            newParams.append('c', systemRef);
        }
    } else if (chipAction === 'toggle-exclude') {
        // Toggle exclusion/inclusion
        const onlySystems = newParams.getAll('o');

        if (onlySystems.length > 0) {
            // We're in "only" mode - toggle by adding/removing from o= list
            if (onlySystems.includes(systemRef)) {
                // Remove from "only" list (exclude it)
                newParams.delete('o');
                onlySystems.filter(s => s !== systemRef).forEach(s => newParams.append('o', s));
            } else {
                // Add to "only" list (include it)
                newParams.append('o', systemRef);
            }
        } else {
            // We're in "exclude" mode - toggle by adding/removing from x= list
            const excludedSystems = newParams.getAll('x');
            if (excludedSystems.includes(systemRef)) {
                // Remove from exclusions (include it)
                newParams.delete('x');
                excludedSystems.filter(s => s !== systemRef).forEach(s => newParams.append('x', s));
            } else {
                // Add to exclusions
                newParams.append('x', systemRef);
            }
        }
    }

    // Update htmx parameters with our computed ones.
    // Use arrays for multi-valued params; Object.fromEntries would drop duplicates.
    event.detail.parameters['o'] = newParams.getAll('o');
    event.detail.parameters['x'] = newParams.getAll('x');
    event.detail.parameters['c'] = newParams.getAll('c');
});
