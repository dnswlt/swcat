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
    ],
    theme: {
        extend: {},
    },
    plugins: [
        typography,
    ],
}