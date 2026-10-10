// Sortable list tables.
//
// Spread into an Alpine component, then render rows through sorted():
//
//   import { sortable } from '../components/sortable.js';
//   return {
//     ...sortable('groups', {
//       name: (g) => g.name,                                  // default: first key, ascending
//       spaces: { value: (g) => g.max_spaces, dir: 'desc' },  // first click sorts descending
//     }, { key: 'name', dir: 'asc' }),
//     ...
//   };
//
//   <template x-for="g in sorted(groups)" :key="g.group_id">
//   {{ template "sort-th" (map "key" "name" "label" "Name") }}
//
// State:   sortKey, sortDir ('asc' | 'desc')
// Methods: sortBy(key, label)  toggle or switch column, remembered per page
//                              in sessionStorage and announced to screen readers
//          sorted(list[, accessors])  a sorted copy; empty values last, ties by name
//          ariaSort(key)        'ascending' | 'descending' | 'none' for <th aria-sort>
//          sortIcon(key)        'asc' | 'desc' | 'none' for the header icon
//          noMatches(list)      true when the list has rows but search hides all
//          noMatchesText(things) "No groups match “term”." (or "the current filters")
//          clearFilters()       clears the search; pages with filters override it
//          filtersActive()      false; pages with filters override it

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

function valueOf(accessor, item) {
  const fn = typeof accessor === 'function' ? accessor : accessor && accessor.value;
  if (typeof fn === 'function') return fn(item);
  if (typeof accessor === 'string') return item ? item[accessor] : undefined;
  return undefined;
}

function isEmpty(v) {
  return v === null || v === undefined || v === '' || (typeof v === 'number' && Number.isNaN(v));
}

function compare(a, b) {
  if (typeof a === 'boolean') a = a ? 1 : 0;
  if (typeof b === 'boolean') b = b ? 1 : 0;
  if (a instanceof Date) a = a.getTime();
  if (b instanceof Date) b = b.getTime();
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return collator.compare(String(a), String(b));
}

export function sortable(page, accessors, defaults = {}) {
  const storageKey = `knot-sort:${page}`;
  const keys = Object.keys(accessors || {});
  let key = defaults.key || keys[0] || '';
  let dir = defaults.dir || 'asc';
  try {
    const saved = JSON.parse(sessionStorage.getItem(storageKey) || 'null');
    if (saved && keys.includes(saved.key) && (saved.dir === 'asc' || saved.dir === 'desc')) {
      key = saved.key;
      dir = saved.dir;
    }
  } catch (_) { /* storage unavailable */ }

  return {
    sortKey: key,
    sortDir: dir,
    sortAccessors: accessors,

    sortBy(k, label) {
      if (this.sortKey === k) {
        this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
      } else {
        const a = this.sortAccessors[k];
        this.sortKey = k;
        this.sortDir = (a && a.dir) || 'asc';
      }
      try {
        sessionStorage.setItem(storageKey, JSON.stringify({ key: this.sortKey, dir: this.sortDir }));
      } catch (_) { /* storage unavailable */ }
      if (window.knotAnnounce) {
        window.knotAnnounce(`Sorted by ${label || k}, ${this.sortDir === 'asc' ? 'ascending' : 'descending'}.`);
      }
    },

    sorted(list, overrides) {
      if (!Array.isArray(list)) return [];
      const all = overrides ? { ...this.sortAccessors, ...overrides } : this.sortAccessors;
      const accessor = all[this.sortKey];
      const byName = all.name;
      const sign = this.sortDir === 'desc' ? -1 : 1;
      return list
        .map((item, i) => ({ item, i, v: valueOf(accessor, item) }))
        .sort((x, y) => {
          const ex = isEmpty(x.v);
          const ey = isEmpty(y.v);
          if (ex !== ey) return ex ? 1 : -1; // empty values last, either direction
          let c = ex ? 0 : sign * compare(x.v, y.v);
          if (c === 0 && byName && accessor !== byName) {
            const nx = valueOf(byName, x.item);
            const ny = valueOf(byName, y.item);
            if (!isEmpty(nx) && !isEmpty(ny)) c = compare(nx, ny);
          }
          return c || x.i - y.i;
        })
        .map((e) => e.item);
    },

    ariaSort(k) {
      if (this.sortKey !== k) return 'none';
      return this.sortDir === 'asc' ? 'ascending' : 'descending';
    },

    sortIcon(k) {
      return this.sortKey === k ? this.sortDir : 'none';
    },

    noMatches(list) {
      return Array.isArray(list) && list.length > 0 && list.every((i) => i.searchHide);
    },

    noMatchesText(things) {
      const term = String(this.searchTerm || '').trim();
      return term ? `No ${things} match “${term}”.` : `No ${things} match the current filters.`;
    },

    filtersActive() {
      return false;
    },

    clearFilters() {
      this.searchTerm = '';
      if (typeof this.searchChanged === 'function') this.searchChanged();
      const input = document.getElementById('search');
      if (input) input.focus();
    },
  };
}
