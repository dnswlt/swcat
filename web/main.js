import './style.css'; // Make sure Tailwind CSS gets included by vite.

function addSVGListener() {
    const svg = document.querySelector("#relationships-svg");
    if (!svg) return;

    svg.addEventListener("click", e => {
        const el = e.target.closest(".clickable-node");
        if (!el) return;

        const id = el.id;
        if (!id) return;

        const parts = id.split(":");
        if (parts.length !== 2) return;

        const kind = parts[0];
        const name = parts[1];

        if (!kind || !name) return;

        let path = "";
        switch (kind) {
            case "component":
                path = "/ui/components/";
                break;
            case "resource":
                path = "/ui/resources/";
                break;
            case "api":
                path = "/ui/apis/";
                break;
            case "system":
                path = "/ui/systems/";
                break;
            case "group":
                path = "/ui/groups/";
                break;
            case "domain":
                path = "/ui/domains/";
                break;
            default:
                console.log(`Unhandled kind ${kind} in SVG.`);
                return;
        }

        window.location.href = path + encodeURIComponent(name);
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
    case 'system':
    case 'apis':
    case 'api':
    case 'resources':
    case 'resource':
    case 'groups':
    case 'group':
        initPage();
        break;
    default:
        console.error(`Unhandled page in init: ${page}`);
        break;
}
