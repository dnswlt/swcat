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
        'rich-tooltip',
        '#rich-tooltip',
        '#pinned-tooltip',
        'tooltip-title',
        'tooltip-attrs',
        'tooltip-section',
        'tooltip-section-title',
        'tooltip-count',
        'tooltip-more',
    ],
    theme: {
        extend: {},
    },
    plugins: [
        typography,
    ],
}