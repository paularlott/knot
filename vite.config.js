import {defineConfig} from 'vite';
import {resolve} from 'path';

export default defineConfig({
  base: "/assets/",
  build: {
    chunkSizeWarningLimit: 1024 * 1024, // 1MB
    outDir: "./web/public_html/assets/",
    manifest: true,
    rollupOptions: {
      output: {
        format: 'es',
        strict: false,
        entryFileNames: "js/[name].js",
        chunkFileNames: "js/[name].js",
        assetFileNames: "css/[name].[ext]",
        dir: 'web/public_html/assets/',
      },
      input: {
        app: resolve(__dirname, 'web/src/js/app.js'),
        terminal: resolve(__dirname, 'web/src/js/terminal.js'),
        nunito: resolve(__dirname, 'web/src/less/nunito.less'),
      },
    }
  }
});
