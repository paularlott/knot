// Build with: npx mix --production

let mix = require('laravel-mix');

mix.less('web/src/less/app.less', 'web/public_html/app.css')
  .less('web/src/terminal/terminal.less', 'web/public_html/terminal.css')
  .js('web/src/js/app.js', 'web/public_html/app.js')
  .js('web/src/js/terminal.js', 'web/public_html/terminal.js');
