import typography from '@tailwindcss/typography';
import defaultTheme from 'tailwindcss/defaultTheme';

/** @type {import('tailwindcss').Config} */
export default {
    content: [
        "./*.html",
        "../templates/*.html",
        "./*.js",
    ],
    safelist: [
        {
            pattern: /graphviz-svg/,
        },
        '#rich-tooltip',
        'tooltip-title',
        'tooltip-attrs',
    ],
    theme: {
        extend: {
            // Preflight applies this to <html>, so the whole UI picks up Inter
            // without touching the templates. The system stack stays behind it
            // as a fallback for the brief window before the webfont loads.
            //
            // Deliberately no fontFeatureSettings here: graphviz measures the
            // graph labels with Inter's default features, so enabling any (the
            // cv* disambiguation variants in particular) would shift advance
            // widths in the browser only, and labels would stop fitting their
            // node boxes.
            fontFamily: {
                sans: ['Inter', ...defaultTheme.fontFamily.sans],
            },
        },
    },
    plugins: [
        typography,
    ],
}