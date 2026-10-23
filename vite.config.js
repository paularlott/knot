import {defineConfig} from 'vite';
import {resolve} from 'path';
import {execFileSync} from 'node:child_process';
import tailwindcss from "@tailwindcss/vite";

const here = import.meta.dirname;

// Redoc's "API docs by Redocly" badge loads the Redocly logo from cdn.redoc.ly. The skin (web/src/less/api-docs.css)
// hides the badge, but a hidden image is still fetched, so point it at a blank one: the API reference makes no outside
// request. The build fails if Redoc changes this, so the plugin is reviewed on a Redoc upgrade.
const blank = 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>');
const redocLogos = ['https://cdn.redoc.ly/redoc/logo-mini-full.svg', 'https://cdn.redoc.ly/redoc/logo-mini.svg'];
const redocOffline = {
  name: 'redoc-offline',
  transform(code, id) {
    if (!id.includes('/node_modules/redoc/bundles/')) return;
    let out = code;
    for (const url of redocLogos) {
      if (!out.includes(url)) this.error(`redoc no longer references ${url}: review the redoc-offline plugin`);
      out = out.replaceAll(url, blank);
    }
    return {code: out, map: null};
  },
};

// openapi.json, served at /assets/openapi.json for the API reference: Redocly CLI bundles the description and any file
// it references into one JSON document.
const openapiYaml = resolve(here, 'internal/api/spec/knot-openapi.yaml');
const openapiJson = {
  name: 'openapi-json',
  buildStart() { this.addWatchFile(openapiYaml); },
  generateBundle() {
    const redocly = resolve(here, 'node_modules/.bin/redocly');
    const json = execFileSync(redocly, ['bundle', openapiYaml, '--ext', 'json'], {
      cwd: here, encoding: 'utf8', maxBuffer: 64 << 20, stdio: ['ignore', 'pipe', 'inherit'],
      env: {...process.env, REDOCLY_TELEMETRY: 'off', REDOCLY_SUPPRESS_UPDATE_NOTICE: 'true'},
    });
    const spec = JSON.parse(json);
    delete spec.info?.['x-logo']; // the reference shows no logo, as before Redoc 3 (which has no hideLogo option)
    this.emitFile({type: 'asset', fileName: 'openapi.json', source: JSON.stringify(spec)});
  },
};

export default defineConfig({
  base: "/assets/",
  plugins: [
    tailwindcss(),
    redocOffline,
    openapiJson,
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
        vnc: resolve(import.meta.dirname, 'web/src/js/vnc.js'),
        nunito: resolve(import.meta.dirname, 'web/src/less/nunito.less'),
        apidocs: resolve(import.meta.dirname, 'web/src/js/api-docs.js'),
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
