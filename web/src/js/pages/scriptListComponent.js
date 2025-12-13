import Alpine from 'alpinejs';

window.scriptListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
    }
  });

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      script: {
        script_id: '',
        name: '',
      }
    },
    scriptFormModal: {
      show: false,
      isEdit: false,
      scriptId: '',
    },
    scripts: [],
    searchTerm: Alpine.$persist('').as('script-search-term').using(sessionStorage),

    async init() {
      await this.getScripts();

      if (window.sseClient) {
        window.sseClient.subscribe('scripts:changed', (payload) => {
          if (payload?.id) this.getScripts(payload.id);
        });

        window.sseClient.subscribe('scripts:deleted', (payload) => {
          this.scripts = this.scripts.filter(x => x.script_id !== payload?.id);
          this.searchChanged();
        });
      }
    },

    async getScripts(scriptId) {
      const url = scriptId ? `/api/scripts/${scriptId}` : '/api/scripts';
      const groupsResponse = await fetch('/api/groups');
      const groupsData = groupsResponse.status === 200 ? await groupsResponse.json() : { groups: [] };
      const groupsMap = {};
      groupsData.groups.forEach(g => groupsMap[g.group_id] = g.name);

      await fetch(url, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((data) => {
            const scriptList = scriptId ? [data] : data.scripts;
            scriptList.forEach(script => {
              script.group_names = (script.groups || []).map(gid => groupsMap[gid] || gid);
              const index = this.scripts.findIndex(s => s.script_id === script.script_id);
              if (index >= 0) {
                this.scripts[index] = script;
              } else {
                this.scripts.push(script);
              }
            });

            this.scripts.sort((a, b) => a.name.localeCompare(b.name));
            this.searchChanged();
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {});
    },

    createScript() {
      this.scriptFormModal.isEdit = false;
      this.scriptFormModal.scriptId = '';
      this.scriptFormModal.show = true;
    },

    editScript(scriptId) {
      this.scriptFormModal.isEdit = true;
      this.scriptFormModal.scriptId = scriptId;
      this.scriptFormModal.show = true;
    },

    async deleteScript(scriptId) {
      await fetch(`/api/scripts/${scriptId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          this.$dispatch('show-alert', { msg: "Script deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          this.$dispatch('show-alert', { msg: "Script could not be deleted", type: 'error' });
        }
      }).catch(() => {});
      this.getScripts();
    },

    searchChanged() {
      const term = this.searchTerm.toLowerCase();
      this.scripts.forEach(s => {
        if (term.length === 0) {
          s.searchHide = false;
        } else {
          s.searchHide = !s.name.toLowerCase().includes(term) && !s.description.toLowerCase().includes(term);
        }
      });
    },
  };
}
