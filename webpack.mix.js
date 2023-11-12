// Build with: npx mix --production

let mix = require('laravel-mix');

mix.less('web/src/less/app.less', 'web/public_html/app.css')
  .js('web/src/js/app.js', 'web/public_html/app.js');
