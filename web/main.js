import './style.css'; // Make sure Tailwind CSS gets included by vite.

function addSVGListener() {
    const svg = document.querySelector("#relationships-svg");
    if (!svg) return;

    svg.addEventListener("click", e => {
        const el = e.target.closest("[id]");
        if (!el) return;

        const query = new URLSearchParams(window.location.search);
        query.set("highlight", el.id);

        htmx.ajax("GET",
            window.location.pathname + "?" + query.toString(),
            { target: "#component-view", swap: "innerHTML", pushUrl: true });
    });
}

function initPage() {
    document.addEventListener("DOMContentLoaded", () => {
        addSVGListener();
    });
}

const page = document.body.dataset.page;
switch (page) {
    case 'components':
    case 'component':
    case 'systems':
    case 'apis':
        initPage();
        break;
    default:
        console.error(`Unhandled page in init: ${page}`);
        break;
}
