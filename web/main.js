import './style.css'; // Make sure Tailwind CSS gets included by vite.
import './custom-content.css';

// Self-hosted Noto Sans, latin + latin-ext only. Used solely by the SVG graph
// (via .graphviz-svg text) — node labels render at 400, edge labels at 400
// italic. The rest of the UI uses the system font stack, so no other weights
// are needed here.
import '@fontsource/noto-sans/latin-400.css';
import '@fontsource/noto-sans/latin-400-italic.css';
import '@fontsource/noto-sans/latin-ext-400.css';
import '@fontsource/noto-sans/latin-ext-400-italic.css';

import { initFeatures } from './features.js';

document.addEventListener('DOMContentLoaded', initFeatures);
