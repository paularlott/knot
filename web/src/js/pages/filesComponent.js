// Files page: buckets, a browser for the files in a bucket, uploads,
// a small-text-file editor, sharing and (for file storage managers)
// transfers. Everything goes through the /api/files API.

const TEXT_EDIT_LIMIT = 1024 * 1024;

const TEXT_EXTENSIONS = new Set([
  'txt', 'md', 'markdown', 'toml', 'yaml', 'yml', 'json', 'jsonc', 'ini', 'cfg', 'conf', 'env',
  'properties', 'xml', 'csv', 'tsv', 'log', 'sh', 'bash', 'zsh', 'fish', 'py', 'js', 'mjs', 'ts',
  'go', 'rs', 'rb', 'php', 'java', 'kt', 'c', 'h', 'cpp', 'hpp', 'cs', 'sql', 'html', 'htm', 'css',
  'scss', 'less', 'tf', 'hcl', 'nomad', 'dockerfile', 'gitignore', 'editorconfig', 'service',
]);

function encodeKey(key) {
  return key.split('/').map(encodeURIComponent).join('/');
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

// request is fetch for the files API; an expired session goes to the
// login page, as on every other page.
async function request(url, opts) {
  const response = await fetch(url, opts);
  if (response.status === 401) window.location.href = '/logout';
  return response;
}

function jsonHeaders() {
  return { 'Content-Type': 'application/json', 'Accept': 'application/json' };
}

async function apiError(response) {
  try {
    const data = await response.json();
    if (data && data.error) return data.error;
  } catch (e) { /* not JSON */ }
  return `request failed (${response.status})`;
}

function formatBytes(n) {
  if (!n) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return (i === 0 ? v : v.toFixed(1)) + ' ' + units[i];
}

// Buckets are named "<username>--<name>"; the API gives each a display_name,
// the short name for the caller's own buckets and the full name for others'.
// API calls always use the full name.
function shortName(name) {
  const i = name.indexOf('--');
  return i < 0 ? name : name.slice(i + 2);
}

// Fresh dialog state.
const emptyShareForm = () => ({ type: 'user', query: '', id: '', write: false, open: false, active: -1, error: '' });
const emptyTransfer = (show = false, bucket = null) => ({ show, bucket, id: '', username: '', query: '', open: false, active: -1, force: false, error: '', saving: false });
const emptyEditor = (fields = {}) => ({ show: false, key: '', content: '', original: '', etag: '', isNew: false, loading: false, saving: false, error: '', ...fields });

window.filesComponent = function (canOwn, isAdmin, canShare, canTransfer) {
  return {
    canOwn,
    isAdmin,
    canShare,
    canTransfer,
    loading: true,
    view: 'buckets',
    announce: '',

    // Buckets
    buckets: [],
    showAll: false,
    searchTerm: '',
    usage: null,

    // Browser
    bucket: null,
    prefix: '',
    folders: [],
    files: [],
    next: '',
    truncated: false,
    listing: false,
    dragOver: false,
    uploads: [],

    // Dialogs
    createModal: { show: false, name: '', error: '', saving: false },
    bucketQuotaModal: false,
    deleteBucketModal: { show: false, bucket: null, error: '', saving: false },
    shareModal: { show: false, bucket: null, error: '', saving: false, showAddForm: false, removeGrant: null, form: emptyShareForm() },
    transferModal: emptyTransfer(),
    deleteFileModal: { show: false, entry: null, isFolder: false, error: '', saving: false },
    editor: emptyEditor(),
    newFileModal: { show: false, name: '', error: '' },
    shareUsers: [],
    shareGroups: [],
    shareTargetsLoaded: false,

    async init() {
      window.addEventListener('popstate', () => this.loadFromLocation());
      document.addEventListener('keydown', (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'k' && this.view === 'buckets') {
          e.preventDefault();
          document.getElementById('bucket-search')?.focus();
        }
      });
      await this.loadFromLocation();

      // Other people's changes, and other servers', arrive as they happen.
      if (window.sseClient) {
        window.sseClient.subscribe('files:changed', (payload) => this.filesChanged(payload));
        window.sseClient.subscribe('reconnected', () => this.filesChanged(null));
      }
    },

    // filesChanged gathers the buckets that changed and, once they have
    // settled, refreshes what is on screen: the bucket list, or the open
    // bucket and the folder being browsed.
    changedIds: new Set(),
    changedAll: false,
    changedTimer: null,

    filesChanged(payload) {
      const ids = payload && payload.bucket_ids;
      if (ids && ids.length) ids.forEach(id => this.changedIds.add(id));
      else this.changedAll = true;
      clearTimeout(this.changedTimer);
      this.changedTimer = setTimeout(() => this.applyFilesChanged(), 500);
    },

    async applyFilesChanged() {
      if (this.listing || this.loading) {
        // Something is already being loaded; look again once it has.
        this.changedTimer = setTimeout(() => this.applyFilesChanged(), 500);
        return;
      }
      const all = this.changedAll;
      const ids = this.changedIds;
      this.changedAll = false;
      this.changedIds = new Set();

      if (this.view === 'buckets') {
        await this.loadBuckets();
        return;
      }
      if (this.view === 'browse' && this.bucket && (all || ids.has(this.bucket.id))) {
        const b = await this.refreshBucket(this.bucket.name);
        if (!b) {
          // Deleted, or no longer shared with this user.
          this.alert('This bucket is no longer available.', 'error');
          await this.showBuckets();
          return;
        }
        await this.listFiles(false);
      }
    },

    say(message) {
      // Re-set so repeated identical messages are announced again.
      this.announce = '';
      this.$nextTick(() => { this.announce = message; });
    },

    // The shared alert renders HTML and is itself a live region, so escape
    // names (other users choose them) and don't announce twice.
    alert(msg, type = 'success') {
      this.$dispatch('show-alert', { msg: escapeHTML(msg), type });
    },

    formatBytes,

    // Focus a dialog's main field once the dialog's focus trap has taken
    // effect (it focuses the first focusable element, the close button).
    focusSoon(id) {
      setTimeout(() => document.getElementById(id)?.focus(), 250);
    },

    formatDate(value) {
      if (!value) return '';
      const d = new Date(value);
      return isNaN(d) ? '' : d.toLocaleString();
    },

    // yourAccess describes the caller's own relationship to a bucket: owner,
    // a share (and through what), or "Manager" when they reach it only as a
    // file storage manager.
    yourAccess(b) {
      if (!b.via) return 'Manager';
      if (b.via === 'owner') return 'Owner';
      const via = { user: '', group: ' · group', all: ' · all users' }[b.via] || '';
      return this.accessLabel(b.granted) + via;
    },

    yourAccessClass(b) {
      if (!b.via) return 'app-badge-purple';
      if (b.via === 'owner') return 'app-badge-success';
      return b.granted === 'write' ? 'app-badge-info' : 'app-badge-neutral';
    },

    accessLabel(access) {
      return { owner: 'Owner', write: 'Read & write', read: 'Read only' }[access] || access;
    },

    // ---------------------------------------------------------------
    // Navigation
    // ---------------------------------------------------------------

    async loadFromLocation() {
      const params = new URLSearchParams(location.search);
      const bucket = params.get('bucket');
      if (bucket) {
        await this.openBucket(bucket, params.get('prefix') || '', false);
      } else {
        this.view = 'buckets';
        this.bucket = null;
        await this.loadBuckets();
      }
      this.loading = false;
    },

    pushLocation() {
      const u = new URL(location.href);
      u.search = '';
      if (this.view === 'browse' && this.bucket) {
        u.searchParams.set('bucket', this.bucket.name);
        if (this.prefix) u.searchParams.set('prefix', this.prefix);
      }
      history.pushState(null, '', u.toString());
    },

    async showBuckets() {
      this.view = 'buckets';
      this.bucket = null;
      this.pushLocation();
      await this.loadBuckets();
      this.$nextTick(() => document.getElementById('files-heading')?.focus());
    },

    get breadcrumbs() {
      const crumbs = [];
      if (!this.prefix) return crumbs;
      let acc = '';
      for (const part of this.prefix.split('/').filter(Boolean)) {
        acc += part + '/';
        crumbs.push({ name: part, prefix: acc });
      }
      return crumbs;
    },

    // ---------------------------------------------------------------
    // Buckets
    // ---------------------------------------------------------------

    async loadBuckets() {
      const response = await request('/api/files/buckets' + (this.showAll ? '?all=true' : ''), { headers: jsonHeaders() });
      if (!response.ok) {
        this.alert(await apiError(response), 'error');
        return;
      }
      const data = await response.json();
      this.buckets = data.buckets || [];
      await this.loadUsage();
    },

    get filteredBuckets() {
      const term = this.searchTerm.trim().toLowerCase();
      if (!term) return this.buckets;
      return this.buckets.filter(b => b.name.includes(term) || (b.owner_name || '').toLowerCase().includes(term));
    },

    label(b) {
      return b ? (b.display_name || b.name) : '';
    },

    // Owners share and transfer their own buckets with the matching
    // permission; file storage administrators manage every bucket.
    canShareBucket(b) {
      return !!b && (this.isAdmin || (b.via === 'owner' && this.canShare));
    },
    canTransferBucket(b) {
      return !!b && (this.isAdmin || (b.via === 'owner' && this.canTransfer));
    },

    // The name the bucket will have once the selected user owns it.
    get transferNewName() {
      const m = this.transferModal;
      if (!m.bucket) return '';
      return (m.username ? m.username.toLowerCase() : '<new owner>') + '--' + shortName(m.bucket.name);
    },

    get usageText() {
      if (!this.usage) return '';
      const used = formatBytes(this.usage.used_bytes);
      const buckets = `${this.usage.buckets}${this.usage.max_buckets ? ' of ' + this.usage.max_buckets : ''} ${this.usage.buckets === 1 && !this.usage.max_buckets ? 'bucket' : 'buckets'}`;
      if (!this.usage.quota_bytes) return `${used} used in your buckets · ${buckets}`;
      return `${used} of ${formatBytes(this.usage.quota_bytes)} used (${this.usagePercent}%) · ${buckets}`;
    },

    get usagePercent() {
      if (!this.usage || !this.usage.quota_bytes) return 0;
      return Math.min(100, Math.round(this.usage.used_bytes * 100 / this.usage.quota_bytes));
    },

    async loadUsage() {
      const usage = await request('/api/files/usage', { headers: jsonHeaders() });
      this.usage = usage.ok ? await usage.json() : null;
    },

    get atBucketLimit() {
      return !!(this.usage && this.usage.max_buckets && this.usage.buckets >= this.usage.max_buckets);
    },

    // Check the bucket limit up front so nobody fills in the form only to
    // be refused.
    async openCreate() {
      await this.loadUsage();
      if (this.atBucketLimit) {
        this.bucketQuotaModal = true;
        return;
      }
      this.createModal = { show: true, name: '', error: '', saving: false };
      this.focusSoon('bucket-name');
    },

    async createBucket() {
      const name = this.createModal.name.trim();
      if (!/^[a-z0-9][a-z0-9-]{1,28}[a-z0-9]$/.test(name) || name.includes('--')) {
        this.createModal.error = 'Use 3-30 lowercase letters, digits and hyphens, starting and ending with a letter or digit, without --.';
        return;
      }
      this.createModal.saving = true;
      const response = await request('/api/files/buckets', { method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ name }) });
      this.createModal.saving = false;
      if (!response.ok) {
        this.createModal.error = await apiError(response);
        return;
      }
      this.createModal.show = false;
      this.alert(`Bucket ${name} created`);
      await this.loadBuckets();
    },

    confirmDeleteBucket(b) {
      this.deleteBucketModal = { show: true, bucket: b, error: '', saving: false };
    },

    async deleteBucket() {
      const b = this.deleteBucketModal.bucket;
      this.deleteBucketModal.saving = true;
      const response = await request(`/api/files/buckets/${encodeURIComponent(b.name)}?force=true`, { method: 'DELETE', headers: jsonHeaders() });
      this.deleteBucketModal.saving = false;
      if (!response.ok) {
        this.deleteBucketModal.error = await apiError(response);
        return;
      }
      this.deleteBucketModal.show = false;
      this.alert(`Bucket ${this.label(b)} deleted`);
      if (this.view === 'browse') {
        await this.showBuckets();
      } else {
        await this.loadBuckets();
      }
    },

    // ---------------------------------------------------------------
    // Sharing and transfer
    // ---------------------------------------------------------------

    async loadShareTargets() {
      if (this.shareTargetsLoaded) return;
      const response = await request('/api/files/share-targets', { headers: jsonHeaders() });
      if (response.ok) {
        const data = await response.json();
        this.shareUsers = data.users || [];
        this.shareGroups = data.groups || [];
        this.shareTargetsLoaded = true;
      }
    },

    async refreshBucket(name) {
      const response = await request(`/api/files/buckets/${encodeURIComponent(name)}`, { headers: jsonHeaders() });
      if (!response.ok) return null;
      const b = await response.json();
      const i = this.buckets.findIndex(x => x.name === name);
      if (i >= 0) this.buckets[i] = b;
      if (this.bucket && this.bucket.name === name) this.bucket = b;
      return b;
    },

    async openShare(b) {
      await this.loadShareTargets();
      const fresh = await this.refreshBucket(b.name) || b;
      this.shareModal = { show: true, bucket: fresh, error: '', saving: false, showAddForm: false, removeGrant: null, form: emptyShareForm() };
    },

    openAddShare() {
      this.shareModal.form = emptyShareForm();
      this.shareModal.showAddForm = true;
      this.focusSoon('share-target');
    },

    setShareType(type) {
      const f = this.shareModal.form;
      f.type = type;
      f.query = '';
      f.id = '';
      f.error = '';
      f.active = -1;
      if (type !== 'all') this.focusSoon('share-target');
    },

    grantLabel(g) {
      if (g.type === 'all') return 'All users';
      return g.name || g.id;
    },

    grantTypeLabel(g) {
      return { user: 'User', group: 'Group', all: 'Everyone' }[g.type] || g.type;
    },

    // Combobox options: users or groups matching the typed text, leaving out
    // those the bucket is already shared with.
    shareOptions() {
      const f = this.shareModal.form;
      const q = f.query.trim().toLowerCase();
      const granted = new Set((this.shareModal.bucket?.grants || []).filter(g => g.type === f.type).map(g => g.id));
      const all = f.type === 'group'
        ? this.shareGroups.map(g => ({ id: g.group_id, name: g.name }))
        : this.shareUsers.filter(u => !u.self).map(u => ({ id: u.user_id, name: u.username }));
      return all.filter(o => !granted.has(o.id) && (!q || o.name.toLowerCase().includes(q))).slice(0, 50);
    },

    // Only users who may own buckets can receive one, the current owner
    // aside; an administrator can take a bucket back themselves.
    transferOptions() {
      const m = this.transferModal;
      const q = m.query.trim().toLowerCase();
      return this.shareUsers
        .filter(u => u.can_own && u.user_id !== m.bucket?.owner_id && (!q || u.username.toLowerCase().includes(q)))
        .slice(0, 50)
        .map(u => ({ id: u.user_id, name: u.username }));
    },

    // Shared keyboard handling for both comboboxes.
    comboKeydown(event, state, options, select) {
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        state.open = true;
        state.active = options.length ? (state.active + 1) % options.length : -1;
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        state.open = true;
        state.active = options.length ? (state.active <= 0 ? options.length - 1 : state.active - 1) : -1;
      } else if (event.key === 'Enter') {
        if (state.open && state.active >= 0 && options[state.active]) {
          event.preventDefault();
          select(options[state.active]);
        }
      } else if (event.key === 'Escape' && state.open) {
        event.preventDefault();
        event.stopPropagation();
        state.open = false;
      }
    },

    comboInput(state) {
      state.id = '';
      state.open = true;
      state.active = -1;
    },

    selectShareOption(o) {
      const f = this.shareModal.form;
      f.id = o.id;
      f.query = o.name;
      f.open = false;
      f.error = '';
    },

    selectTransferOption(o) {
      const m = this.transferModal;
      m.id = o.id;
      m.username = o.name;
      m.query = o.name;
      m.open = false;
      m.error = '';
    },

    // resolveTyped accepts a name typed in full without picking it from the list.
    resolveTyped(query, options) {
      const q = query.trim().toLowerCase();
      return options.find(o => o.name.toLowerCase() === q) || null;
    },

    async addShare() {
      const m = this.shareModal;
      const f = m.form;
      let name = '';
      if (f.type !== 'all') {
        if (!f.id) {
          const match = this.resolveTyped(f.query, this.shareOptions());
          if (match) this.selectShareOption(match);
        }
        if (!f.id) {
          f.error = f.type === 'group' ? 'Choose a group from the list.' : 'Choose a user from the list.';
          return;
        }
        name = f.id;
      }
      const label = f.type === 'all' ? 'all users' : f.query;
      if (await this.sendGrant({ type: f.type, name, access: f.write ? 'write' : 'read' }, f)) {
        m.showAddForm = false;
        this.say(`Shared ${this.label(m.bucket)} with ${label}, ${f.write ? 'read and write' : 'read only'}`);
      }
    },

    async toggleGrantWrite(g) {
      const write = g.access !== 'write';
      if (await this.sendGrant({ type: g.type, name: g.id, access: write ? 'write' : 'read' }, this.shareModal)) {
        this.say(`${this.grantLabel(g)} now has ${write ? 'read and write' : 'read only'} access`);
      }
    },

    async sendGrant(req, errorTarget) {
      const m = this.shareModal;
      m.saving = true;
      errorTarget.error = '';
      const response = await request(`/api/files/buckets/${encodeURIComponent(m.bucket.name)}/share`, { method: 'POST', headers: jsonHeaders(), body: JSON.stringify(req) });
      m.saving = false;
      if (!response.ok) {
        errorTarget.error = await apiError(response);
        return false;
      }
      m.bucket = await response.json();
      await this.refreshBucket(m.bucket.name);
      return true;
    },

    async removeGrant() {
      const m = this.shareModal;
      const g = m.removeGrant;
      m.removeGrant = null;
      m.saving = true;
      m.error = '';
      const response = await request(`/api/files/buckets/${encodeURIComponent(m.bucket.name)}/unshare`, { method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ type: g.type, name: g.id }) });
      m.saving = false;
      if (!response.ok) {
        m.error = await apiError(response);
        return;
      }
      m.bucket = await response.json();
      await this.refreshBucket(m.bucket.name);
      this.say(`Stopped sharing ${this.label(m.bucket)} with ${this.grantLabel(g)}`);
    },

    async openTransfer(b) {
      await this.loadShareTargets();
      this.transferModal = emptyTransfer(true, b);
      this.focusSoon('transfer-user');
    },

    async transferBucket() {
      const m = this.transferModal;
      if (!m.id) {
        const match = this.resolveTyped(m.query, this.transferOptions());
        if (match) this.selectTransferOption(match);
      }
      if (!m.id) { m.error = 'Choose the new owner from the list.'; return; }
      m.saving = true;
      const response = await request(`/api/files/buckets/${encodeURIComponent(m.bucket.name)}/transfer`, { method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ user: m.id, force: this.isAdmin && m.force }) });
      m.saving = false;
      if (!response.ok) {
        m.error = await apiError(response);
        return;
      }
      m.show = false;
      this.alert(`Bucket ${this.label(m.bucket)} transferred to ${m.username} as ${this.transferNewName}`);
      await this.loadBuckets();
    },

    // ---------------------------------------------------------------
    // Browsing
    // ---------------------------------------------------------------

    get canWrite() {
      return this.bucket && (this.bucket.access === 'owner' || this.bucket.access === 'write');
    },

    async openBucket(name, prefix = '', push = true) {
      const response = await request(`/api/files/buckets/${encodeURIComponent(name)}`, { headers: jsonHeaders() });
      if (!response.ok) {
        this.alert(await apiError(response), 'error');
        this.view = 'buckets';
        await this.loadBuckets();
        return;
      }
      this.bucket = await response.json();
      this.view = 'browse';
      await this.openPrefix(prefix, push);
    },

    async openPrefix(prefix, push = true) {
      this.prefix = prefix;
      if (push) this.pushLocation();
      await this.listFiles(false);
      this.$nextTick(() => document.getElementById('files-heading')?.focus());
    },

    async listFiles(more) {
      this.listing = true;
      const q = new URLSearchParams({ delimiter: '/', limit: '500' });
      if (this.prefix) q.set('prefix', this.prefix);
      if (more && this.next) q.set('after', this.next);
      const response = await request(`/api/files/list/${encodeURIComponent(this.bucket.name)}?${q}`, { headers: jsonHeaders() });
      this.listing = false;
      if (!response.ok) {
        this.alert(await apiError(response), 'error');
        return;
      }
      const data = await response.json();
      const folders = (data.prefixes || []).map(p => ({ prefix: p, name: p.slice(this.prefix.length).replace(/\/$/, '') }));
      const files = (data.objects || []).map(o => ({ ...o, name: o.key.slice(this.prefix.length) }));
      this.folders = more ? this.folders.concat(folders) : folders;
      this.files = more ? this.files.concat(files) : files;
      this.next = data.next || '';
      this.truncated = !!data.is_truncated;
      if (!more) {
        const count = this.folders.length + this.files.length;
        this.say(`${this.label(this.bucket)}${this.prefix ? ' / ' + this.prefix : ''}: ${count} ${count === 1 ? 'item' : 'items'}`);
      }
    },

    objectUrl(key) {
      return `/api/files/objects/${encodeURIComponent(this.bucket.name)}/${encodeKey(key)}`;
    },

    downloadUrl(f) {
      return this.objectUrl(f.key);
    },

    isEditable(f) {
      if (f.size > TEXT_EDIT_LIMIT) return false;
      const type = (f.content_type || '').toLowerCase();
      if (type.startsWith('text/') || /(json|yaml|toml|xml|javascript|x-sh)/.test(type)) return true;
      const name = f.name.toLowerCase();
      const ext = name.includes('.') ? name.split('.').pop() : name;
      return TEXT_EXTENSIONS.has(ext) || name.startsWith('.');
    },

    confirmDeleteFile(entry, isFolder) {
      this.deleteFileModal = { show: true, entry, isFolder, error: '', saving: false };
    },

    async deleteKeys(keys) {
      for (const key of keys) {
        const response = await request(this.objectUrl(key), { method: 'DELETE', headers: jsonHeaders() });
        if (!response.ok && response.status !== 404) {
          throw new Error(await apiError(response));
        }
      }
    },

    async deleteFile() {
      const m = this.deleteFileModal;
      m.saving = true;
      m.error = '';
      try {
        let keys = [m.entry.key];
        if (m.isFolder) {
          keys = [];
          let after = '';
          for (;;) {
            const q = new URLSearchParams({ prefix: m.entry.prefix, limit: '1000' });
            if (after) q.set('after', after);
            const response = await request(`/api/files/list/${encodeURIComponent(this.bucket.name)}?${q}`, { headers: jsonHeaders() });
            if (!response.ok) throw new Error(await apiError(response));
            const data = await response.json();
            keys.push(...(data.objects || []).map(o => o.key));
            if (!data.is_truncated) break;
            after = data.next;
          }
        }
        await this.deleteKeys(keys);
      } catch (e) {
        m.saving = false;
        m.error = e.message;
        return;
      }
      m.saving = false;
      m.show = false;
      this.alert(m.isFolder ? `Folder ${m.entry.name} deleted` : `${m.entry.name} deleted`);
      await this.listFiles(false);
    },

    // ---------------------------------------------------------------
    // Uploads
    // ---------------------------------------------------------------

    chooseFiles() {
      this.$refs.fileInput.value = '';
      this.$refs.fileInput.click();
    },

    chooseFolder() {
      this.$refs.folderInput.value = '';
      this.$refs.folderInput.click();
    },

    filesPicked(event) {
      const files = Array.from(event.target.files || []).map(f => ({ file: f, path: f.webkitRelativePath || f.name }));
      this.upload(files);
    },

    async dropped(event) {
      this.dragOver = false;
      if (!this.canWrite) return;
      const items = Array.from(event.dataTransfer.items || []);
      const collected = [];
      const entries = items.map(i => i.webkitGetAsEntry && i.webkitGetAsEntry()).filter(Boolean);
      if (entries.length) {
        for (const entry of entries) {
          await this.walkEntry(entry, '', collected);
        }
      } else {
        for (const f of Array.from(event.dataTransfer.files || [])) {
          collected.push({ file: f, path: f.name });
        }
      }
      this.upload(collected);
    },

    walkEntry(entry, base, out) {
      return new Promise((resolve) => {
        if (entry.isFile) {
          entry.file((f) => { out.push({ file: f, path: base + f.name }); resolve(); }, () => resolve());
          return;
        }
        if (!entry.isDirectory) { resolve(); return; }
        const reader = entry.createReader();
        const readBatch = () => {
          reader.readEntries(async (batch) => {
            if (!batch.length) { resolve(); return; }
            for (const child of batch) {
              await this.walkEntry(child, base + entry.name + '/', out);
            }
            readBatch();
          }, () => resolve());
        };
        readBatch();
      });
    },

    async upload(items) {
      if (!items.length) return;
      const queued = items.map(i => ({ id: Math.random().toString(36).slice(2), name: i.path, file: i.file, progress: 0, status: 'queued', error: '' }));
      this.uploads = queued.concat(this.uploads.filter(u => u.status === 'uploading' || u.status === 'queued'));
      this.say(`Uploading ${queued.length} ${queued.length === 1 ? 'file' : 'files'}`);

      let ok = 0;
      for (const u of queued) {
        const item = this.uploads.find(x => x.id === u.id);
        if (await this.uploadOne(item)) ok++;
      }
      const failed = queued.length - ok;
      this.alert(failed ? `${ok} uploaded, ${failed} failed` : `${ok} ${ok === 1 ? 'file' : 'files'} uploaded`, failed ? 'error' : 'success');
      await this.listFiles(false);
      setTimeout(() => { this.uploads = this.uploads.filter(u => u.status !== 'done'); }, 4000);
    },

    uploadOne(item) {
      return new Promise((resolve) => {
        const key = this.prefix + item.name;
        const xhr = new XMLHttpRequest();
        xhr.open('PUT', this.objectUrl(key));
        xhr.setRequestHeader('Content-Type', item.file.type || 'application/octet-stream');
        xhr.setRequestHeader('Accept', 'application/json');
        xhr.setRequestHeader('X-Knot-Mtime', String(item.file.lastModified / 1000));
        item.status = 'uploading';
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) item.progress = Math.round(e.loaded * 100 / e.total);
        };
        xhr.onload = () => {
          if (xhr.status === 401) {
            window.location.href = '/logout';
            return;
          }
          if (xhr.status >= 200 && xhr.status < 300) {
            item.status = 'done';
            item.progress = 100;
            resolve(true);
            return;
          }
          let msg = `failed (${xhr.status})`;
          try { msg = JSON.parse(xhr.responseText).error || msg; } catch (e) { /* not JSON */ }
          item.status = 'error';
          item.error = msg;
          resolve(false);
        };
        xhr.onerror = () => {
          item.status = 'error';
          item.error = 'network error';
          resolve(false);
        };
        xhr.send(item.file);
      });
    },

    // ---------------------------------------------------------------
    // Editor
    // ---------------------------------------------------------------

    async openEditor(f) {
      this.editor = emptyEditor({ show: true, key: f.key, loading: true });
      const response = await request(this.downloadUrl(f));
      if (!response.ok) {
        this.editor.loading = false;
        this.editor.error = await apiError(response);
        return;
      }
      const text = await response.text();
      this.editor.loading = false;
      if (text.includes('\u0000')) {
        this.editor.error = 'This file is not text and cannot be edited here. Download it instead.';
        return;
      }
      this.editor.content = text;
      this.editor.original = text;
      this.editor.etag = response.headers.get('ETag') || '';
      this.focusSoon('editor-content');
    },

    openNewFile() {
      this.newFileModal = { show: true, name: '', error: '' };
      this.focusSoon('new-file-name');
    },

    startNewFile() {
      const name = this.newFileModal.name.trim().replace(/^\/+/, '');
      if (!name || name.split('/').some(p => p === '' || p === '.' || p === '..')) {
        this.newFileModal.error = 'Enter a file name, optionally with folders, e.g. app/settings.toml.';
        return;
      }
      this.newFileModal.show = false;
      this.editor = emptyEditor({ show: true, key: this.prefix + name, isNew: true });
      this.focusSoon('editor-content');
    },

    get editorDirty() {
      return this.editor.content !== this.editor.original;
    },

    closeEditor() {
      if (this.editorDirty && !this.editor.saving && !confirm('Discard your changes?')) return;
      this.editor.show = false;
    },

    async saveEditor() {
      const e = this.editor;
      e.saving = true;
      e.error = '';
      const headers = { 'Accept': 'application/json', 'Content-Type': 'text/plain; charset=utf-8', 'X-Knot-Mtime': String(Date.now() / 1000) };
      if (e.isNew) headers['If-None-Match'] = '*';
      else if (e.etag) headers['If-Match'] = e.etag;
      const response = await request(this.objectUrl(e.key), { method: 'PUT', headers, body: e.content });
      e.saving = false;
      if (response.status === 412) {
        e.error = e.isNew
          ? 'A file with this name already exists. Choose another name or edit the existing file.'
          : 'Someone else changed this file since you opened it. Copy your changes, reopen the file and apply them again.';
        return;
      }
      if (!response.ok) {
        e.error = await apiError(response);
        return;
      }
      const info = await response.json();
      e.etag = `"${info.etag}"`;
      e.original = e.content;
      e.isNew = false;
      e.show = false;
      this.alert(`${e.key.slice(this.prefix.length) || e.key} saved`);
      await this.listFiles(false);
    },
  };
};
