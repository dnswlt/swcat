
import { updateNavbarLinks } from './helpers.js';

function loadStationInfo(station_abbr) {
    htmx.ajax('GET', `/stations/${station_abbr}/info`, {
        target: "#station-info-container",
        swap: 'innerHTML'
    });
}

// Initialization function for the map page.
export function initMapPage() {
    document.addEventListener("DOMContentLoaded", () => {
        const DEFAULT_COLOR = "#DA291C";
        const HIGHLIGHT_COLOR = "#000000";

        // prev/next buttons for the date picker
        

        // Every marker is a <g>
        const markers = document.querySelectorAll('g[id^="station-marker-"]');

        markers.forEach(marker => {
            marker.addEventListener("click", () => {
                const station = marker.id.replace("station-marker-", "");

                // Reset all shapes
                document.querySelectorAll(".marker-shape")
                    .forEach(shape => shape.setAttribute("fill", DEFAULT_COLOR));

                // Highlight the shape inside this marker
                const shape = marker.querySelector(".marker-shape");
                if (shape) shape.setAttribute("fill", HIGHLIGHT_COLOR);

                loadStationInfo(station);

                // Update nav links and URL
                const params = new URLSearchParams(window.location.search);
                params.set("station", station);
                updateNavbarLinks(params)
                const path = window.location.pathname;
                const newUrl = `${path}?${params.toString()}`;
                window.history.replaceState({}, '', newUrl)
            });
        });

        // On page load: update navbar using current + saved state
        const currentParams = new URLSearchParams(window.location.search);
        updateNavbarLinks(currentParams);
        const station = currentParams.get("station");
        if (station) {
            loadStationInfo(station);
        }
    });
}