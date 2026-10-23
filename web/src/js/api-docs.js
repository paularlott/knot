// The API reference at /api-docs/: Redoc 3, bundled from node_modules by Vite, renders the OpenAPI document built
// from internal/api/spec/knot-openapi.yaml (/assets/openapi.json). Nothing loads from outside the server.
import { init } from 'redoc/bundles/redoc.standalone.js';
import '../less/api-docs.css';

// Follow the app's light/dark choice (Redoc keeps its own in redoc.colorMode); like the app, dark unless set to light.
try {
  localStorage.setItem('redoc.colorMode', localStorage.getItem('_x_darkMode') === 'false' ? 'light' : 'dark');
} catch { /* storage unavailable: Redoc follows the system setting */ }

init('/assets/openapi.json', {
  disableTelemetry: true, // Redoc would otherwise report page views to Redocly
  router: 'hash', // /api-docs/ is a single static page, so in-page links stay in the fragment
  schemasExpansionLevel: 2,
}, document.getElementById('redoc'));
