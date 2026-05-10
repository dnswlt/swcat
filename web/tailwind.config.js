import typography from '@tailwindcss/typography';

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
        extend: {},
    },
    plugins: [
        typography,
    ],
}