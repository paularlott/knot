import {defineConfig} from 'vite';
import {resolve} from 'path';
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/assets/",
  plugins: [
    tailwindcss(),
  ],
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
        knot: resolve(import.meta.dirname, 'web/src/js/knot.js'),
        meshanimation: resolve(import.meta.dirname, 'web/src/js/mesh-animation.js'),
        nunito: resolve(import.meta.dirname, 'web/src/less/nunito.less'),
      },
    },
    css: {
      postcss: {
        plugins: [tailwindcss()],
      },
      preprocessorOptions: {
        less: {
          javascriptEnabled: true,
        },
      },
    },
  }
});
