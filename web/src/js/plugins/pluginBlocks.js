// Plugin block renderer: turns validated JSON blocks from a plugin handler
// into DOM using the <template> fragments in plugin-block-templates.tmpl —
// the single source of block presentation. Plugin-supplied strings are only
// ever assigned via textContent (escaping by construction); the html block
// is the one trusted exception (admin-installed markup, rendered raw).
//
// reconcileBlocks() patches an existing region in place, matched by block id
// (falling back to position): same-id charts get a data-only update, blocks
// containing focus are never patched mid-interaction, and popup open state
// survives. Diffing is strictly block-granular — never inside a block.

const PLUGIN_CHART_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#f97316'];

// safeColor gates plugin-supplied colour values before they reach a DOM
// style: hex, rgb()/rgba(), hsl()/hsla() shapes only, anything else drops.
// CSSOM would discard a non-colour assignment anyway (these are property
// assignments, not cssText), but the guard keeps that contract explicit and
// survives a future move to setAttribute or cssText.
const SAFE_COLOR_RE = /^#(?:[0-9a-fA-F]{3,8})$|^rgba?\(\s*[\d.]+%?\s*,\s*[\d.]+%?\s*,\s*[\d.]+%?\s*(?:,\s*[\d.]+\s*)?\)$|^hsla?\(\s*[\d.]+(?:deg|turn)?\s*,\s*[\d.]+%\s*,\s*[\d.]+%\s*(?:,\s*[\d.]+\s*)?\)$/;

function safeColor(value) {
  return typeof value === 'string' && SAFE_COLOR_RE.test(value.trim()) ? value.trim() : '';
}

// badgeClassFor maps well-known state values to badge colours; anything
// unknown gets the neutral blue. Values carry text, so colour is never the
// only signal.
function badgeClassFor(value) {
  const v = String(value).toLowerCase();
  if (['running', 'healthy', 'ok', 'active', 'alive', 'success'].includes(v)) {
    return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200';
  }
  if (['stopped', 'inactive', 'unknown'].includes(v)) {
    return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
  }
  if (['pending', 'creating', 'starting', 'waiting'].includes(v)) {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200';
  }
  if (['deleting', 'failed', 'error', 'down', 'dead'].includes(v)) {
    return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200';
  }
  return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200';
}

const ALERT_STYLES = {
  info: {
    class: 'bg-blue-50 border-blue-200 text-blue-800 dark:bg-blue-900/40 dark:border-blue-800 dark:text-blue-200',
    icon: 'M11.25 11.25l.041-.02a.75.75 0 0 1 1.063.852l-.708 2.836a.75.75 0 0 0 1.063.853l.041-.021M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9-3.75h.008v.008H12V8.25Z',
  },
  success: {
    class: 'bg-green-50 border-green-200 text-green-800 dark:bg-green-900/40 dark:border-green-800 dark:text-green-200',
    icon: 'M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
  },
  warning: {
    class: 'bg-amber-50 border-amber-200 text-amber-800 dark:bg-amber-900/40 dark:border-amber-800 dark:text-amber-200',
    icon: 'M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z',
  },
  error: {
    class: 'bg-red-50 border-red-200 text-red-800 dark:bg-red-900/40 dark:border-red-800 dark:text-red-200',
    icon: 'm9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
  },
};

let pbFormSeq = 1;

function tpl(id) {
  const template = document.getElementById(id);
  return template.content.firstElementChild.cloneNode(true);
}

// q finds a slot that may be the cloned root itself or a descendant of it
// (querySelector alone misses roots, and several templates carry their slot
// attribute on the root element).
function q(root, selector) {
  if (root.matches(selector)) return root;
  return root.querySelector(selector);
}

function chartTheme() {
  const dark = document.documentElement.classList.contains('dark');
  return {
    grid: dark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)',
    tick: dark ? '#9ca3af' : '#6b7280',
  };
}

function chartSummary(block) {
  const parts = (block.datasets || []).map((ds) => {
    const data = ds.data || [];
    if (!data.length) return `${ds.name}: no data`;
    const min = Math.min(...data);
    const max = Math.max(...data);
    return `${ds.name} ${min}–${max}`;
  });
  return `${block.chart_type} chart: ${parts.join(', ')} over ${block.labels.length} points`;
}

function chartDatasets(block) {
  const pie = block.chart_type === 'doughnut' || block.chart_type === 'pie';
  return (block.datasets || []).map((ds, i) => {
    let backgroundColor = ds.color || PLUGIN_CHART_COLORS[i % PLUGIN_CHART_COLORS.length];
    // Pies colour per slice: one dataset colour would paint every slice
    // the same. An array colour overrides the palette point by point.
    if (pie) {
      backgroundColor = Array.isArray(ds.color)
        ? ds.color
        : ds.data.map((_, j) => PLUGIN_CHART_COLORS[j % PLUGIN_CHART_COLORS.length]);
    }
    return {
      label: ds.name,
      data: ds.data,
      borderColor: pie ? '#ffffff' : (ds.color || PLUGIN_CHART_COLORS[i % PLUGIN_CHART_COLORS.length]),
      backgroundColor,
      tension: 0.3,
      fill: block.chart_type === 'line' ? false : undefined,
      borderWidth: pie ? 2 : 2,
    };
  });
}

// initPluginChart builds (or updates) the chart.js instance on a canvas.
export function initPluginChart(canvas, block) {
  const theme = chartTheme();
  const config = {
    type: block.chart_type,
    data: { labels: block.labels, datasets: chartDatasets(block) },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      plugins: { legend: { labels: { color: theme.tick } } },
      scales: block.chart_type === 'doughnut' || block.chart_type === 'pie' ? {} : {
        x: { grid: { color: theme.grid }, ticks: { color: theme.tick } },
        y: { grid: { color: theme.grid }, ticks: { color: theme.tick } },
      },
    },
  };
  canvas.parentElement.style.height = `${block.height || 240}px`;
  canvas.setAttribute('role', 'img');
  canvas.setAttribute('aria-label', chartSummary(block));
  const existing = window.Chart ? window.Chart.getChart(canvas) : null;
  if (existing) {
    existing.data.labels = config.data.labels;
    existing.data.datasets = config.data.datasets;
    existing.options.plugins.legend.labels.color = theme.tick;
    if (config.options.scales.x) {
      existing.options.scales.x = config.options.scales.x;
      existing.options.scales.y = config.options.scales.y;
    }
    existing.update();
    return;
  }
  new window.Chart(canvas, config); // eslint-disable-line no-new
}

// renderBlock builds one block's DOM. The returned element carries
// data-block-type / data-block-id for reconciliation and event delegation.
// buildFormFields constructs a form's field controls. Autocompleters are
// plain inputs with a suggestion listbox (combobox pattern); dynamic
// options are fetched by the page runtime via the hooks callback.
function buildFormFields(form, block, hooks) {
  const wrap = q(form, '[data-fields]');
  wrap.innerHTML = '';
  (block.fields || []).forEach((field) => {
    if (field.type === 'hidden') {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = field.name;
      input.value = field.value || '';
      wrap.appendChild(input);
      return;
    }
    const row = tpl('pb-form-field');
    row.dataset.fieldType = field.type;
    q(row, '[data-label]').textContent = field.label;
    q(row, '[data-label]').setAttribute('for', `plugin-field-${field.name}`);
    const control = q(row, '[data-control]');
    const inputId = `plugin-field-${field.name}`;
    let input;
    if (field.type === 'select') {
      input = document.createElement('select');
      input.name = field.name;
      input.className = 'form-field';
      input.id = inputId;
      (field.options || []).forEach((option) => {
        const el = document.createElement('option');
        el.value = option;
        el.textContent = option;
        if (option === field.value) el.selected = true;
        input.appendChild(el);
      });
    } else if (field.type === 'autocomplete') {
      const ac = tpl('pb-autocomplete');
      input = q(ac, '[data-ac-input]');
      input.name = field.name;
      input.id = inputId;
      input.value = field.value || '';
      if (field.placeholder) input.placeholder = field.placeholder;
      const list = q(ac, '[data-ac-list]');
      const listId = `plugin-list-${field.name}`;
      list.id = listId;
      input.setAttribute('role', 'combobox');
      input.setAttribute('aria-autocomplete', 'list');
      input.setAttribute('aria-expanded', 'false');
      input.setAttribute('aria-controls', listId);
      input.dataset.pbOptions = JSON.stringify(field.options || []);
      input.dataset.pbDynamic = field.dynamic_options ? '1' : '';
      if (hooks && hooks.onDynamicOptions && field.dynamic_options) hooks.onDynamicOptions(input, field);
      control.appendChild(ac);
    } else {
      input = document.createElement('input');
      input.type = field.type === 'number' ? 'number' : 'text';
      input.name = field.name;
      input.id = inputId;
      input.value = field.value || '';
      if (field.placeholder) input.placeholder = field.placeholder;
      input.className = 'form-field';
    }
    if (field.type !== 'autocomplete') control.appendChild(input);
    wrap.appendChild(row);
  });
}

export function renderBlock(block, hooks) {
  let node;
  switch (block.type) {
    case 'text': {
      node = tpl('pb-text');
      node.textContent = block.text;
      break;
    }
    case 'stat': {
      node = tpl('pb-stat');
      q(node, '[data-label]').textContent = block.label;
      const value = q(node, '[data-value]');
      value.textContent = block.value;
      value.title = block.value;
      if (block.accent) value.style.color = safeColor(block.accent);
      if (block.unit) q(node, '[data-unit]').textContent = block.unit;
      if (block.delta) {
        const delta = q(node, '[data-delta]');
        const down = !!block.down;
        delta.textContent = `${down ? '▼' : '▲'} ${block.delta}`;
        delta.classList.add(down ? 'text-red-600' : 'text-green-600', down ? 'dark:text-red-400' : 'dark:text-green-400');
      }
      break;
    }
    case 'bar': {
      node = tpl('pb-bar');
      q(node, '[data-label]').textContent = block.label;
      const pct = Math.max(0, Math.min(100, (block.value / block.max) * 100));
      q(node, '[data-reading]').textContent =
        `${block.value.toFixed(1)} / ${block.max.toFixed(1)}${block.unit ? ` ${block.unit}` : ''}`;
      const track = q(node, '[data-track]');
      track.setAttribute('aria-valuenow', pct.toFixed(0));
      const fill = q(node, '[data-fill]');
      fill.style.width = `${pct.toFixed(1)}%`;
      fill.style.backgroundColor = safeColor(block.color);
      fill.setAttribute('aria-label', block.label);
      break;
    }
    case 'table': {
      node = tpl('pb-table');
      const head = q(node, '[data-head]').insertRow();
      (block.columns || []).forEach((col) => {
        const th = document.createElement('th');
        th.scope = 'col';
        th.className = 'px-4 py-3 text-xs font-medium tracking-wider text-left uppercase';
        th.textContent = col.label;
        head.appendChild(th);
      });
      const body = q(node, '[data-body]');
      (block.rows || []).forEach((row) => {
        const tr = body.insertRow();
        tr.className = 'bg-white border-b dark:bg-gray-800 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-600/10';
        block.columns.forEach((col, index) => {
          const td = tr.insertCell();
          td.className = 'px-4 py-3 align-top text-gray-700 dark:text-gray-200';
          const value = row[col.key];
          if (col.badge) {
            td.classList.add('whitespace-nowrap');
            const badge = document.createElement('span');
            badge.className = `inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${badgeClassFor(value)}`;
            badge.textContent = value;
            td.appendChild(badge);
          } else {
            td.textContent = value;
            td.title = value;
            if (index === 0) td.classList.add('font-medium', 'text-gray-900', 'dark:text-white');
            if (typeof value === 'string' && value.length > 48) td.classList.add('max-w-[22rem]', 'truncate', 'block');
          }
        });
      });
      break;
    }
    case 'chart': {
      node = tpl('pb-chart');
      // The chart config rides on the canvas for the runtime to initialize.
      const canvas = q(node, '[data-canvas]');
      canvas.dataset.pbChart = JSON.stringify(block);
      break;
    }
    case 'form': {
      node = tpl('pb-form');
      buildFormFields(node, block, hooks);
      q(node, '[data-submit]').textContent = block.submit || 'Apply';
      if (!block.auto_submit) node.setAttribute('data-plugin-auto-wait', '1');
      const submitBtn = q(node, '[data-submit]');
      const cancelBtn = q(node, '[data-cancel]');
      if (block.auto_submit) submitBtn.remove();
      if (!block.cancel) cancelBtn.remove();
      else cancelBtn.textContent = block.cancel;
      break;
    }
    case 'markdown': {
      node = tpl('pb-markdown');
      q(node, '[data-html]').innerHTML = block.html; // trusted: server-rendered from plugin markdown
      break;
    }
    case 'html': {
      node = tpl('pb-html');
      q(node, '[data-html]').innerHTML = block.html; // trusted: admin-installed
      break;
    }
    case 'error':
    default: {
      node = tpl('pb-error');
      node.textContent = block.message || `unknown block type "${block.type}"`;
      break;
    }
  }
  node.dataset.blockType = block.type;
  if (block.id) node.dataset.blockId = block.id;
  // The region is a responsive grid: stats are its cells (the KPI row) and
  // popup buttons line up as an actions row; everything else spans the
  // full row.
  if (block.type !== 'stat' && block.type !== 'popup') node.classList.add('col-span-full');
  return node;
}

// restoreFocus replaces `old` with `next`, moving focus along if the user
// was inside the old node (matched by element name + name/id attributes).
function replacePreservingFocus(old, next) {
  const active = document.activeElement;
  const hadFocus = old.contains(active);
  let restore = null;
  if (hadFocus && active !== old) {
    restore = {
      tag: active.tagName,
      name: active.getAttribute('name'),
      type: active.getAttribute('type'),
    };
  }
  old.replaceWith(next);
  if (restore) {
    let candidate = null;
    if (restore.name) candidate = next.querySelector(`[name="${CSS.escape(restore.name)}"]`);
    if (!candidate && restore.tag === 'BUTTON') candidate = next.querySelector('button');
    if (!candidate) candidate = next;
    try { candidate.focus({ preventScroll: true }); } catch (e) { /* focus optional */ }
  }
}

