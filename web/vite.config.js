import { resolve } from 'path'

export default {
    root: '.',
    base: '/static/dist/',
    build: {
        outDir: '../static/dist',
        emptyOutDir: true,
        manifest: true, // Used by the Go server to find assets
        rollupOptions: {
            input: {
                main: resolve(__dirname, 'main.js')
            },
            output: {
                // Content-hashed names for every output. The Go server reads
                // .vite/manifest.json at startup to resolve logical names
                // (main.js, main.css) to their hashed file paths.
                entryFileNames: '[name]-[hash].js',
                chunkFileNames: '[name]-[hash].js',
                assetFileNames: '[name]-[hash][extname]'
            }
        }
    }
}
