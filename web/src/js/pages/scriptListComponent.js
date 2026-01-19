import Alpine from 'alpinejs';

window.scriptListComponent = function(userId, zone) {
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
      isUserScript: false,
    },
    scripts: [],
    availableZones: [],
    filterType: Alpine.$persist('all').as('script-filter-type').using(sessionStorage),
    showAllZones: Alpine.$persist(false).as('script-show-all-zones').using(sessionStorage),
    searchTerm: Alpine.$persist('').as('script-search-term').using(sessionStorage),
    currentUserId: userId || '',
    currentZone: zone || '',

    async init() {
      await this.getScripts();

      if (window.sseClient) {
        window.sseClient.subscribe('scripts:changed', (payload) => {
          if (payload?.id) this.getScripts(payload.id);
        });

        window.sseClient.subscribe('scripts:deleted', (payload) => {
          this.scripts = this.scripts.filter(x => x.script_id !== payload?.id);
          this.applyFilters();
        });

        window.sseClient.subscribe('reconnected', () => {
          this.getScripts();
        });
      }
    },

    async getScripts(scriptId) {
      const url = scriptId ? `/api/scripts/${scriptId}` : `/api/scripts?all_zones=${this.showAllZones}`;
      const groupsResponse = await fetch('/api/groups');
      const groupsData = groupsResponse.status === 200 ? await groupsResponse.json() : { groups: [] };
      const groupsMap = {};
      groupsData.groups.forEach(g => groupsMap[g.group_id] = g.name);

      // Fetch global scripts
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

              // Collect zones
              if (script.zones && script.zones.length) {
                script.zones.forEach(z => {
                  if (!this.availableZones.includes(z)) {
                    this.availableZones.push(z);
                  }
                });
              }
            });

            // Sort and apply filters after data is loaded
            this.scripts.sort((a, b) => a.name.localeCompare(b.name));
            this.availableZones.sort();
            this.applyFilters();
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {});

      // Fetch user scripts if user has permission
      if (this.currentUserId && !scriptId) {
        await fetch(`/api/scripts?user_id=${this.currentUserId}&all_zones=${this.showAllZones}`, {
          headers: {
            'Content-Type': 'application/json'
          }
        }).then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              data.scripts.forEach(script => {
                script.group_names = [];
                const index = this.scripts.findIndex(s => s.script_id === script.script_id);
                if (index >= 0) {
                  this.scripts[index] = script;
                } else {
                  this.scripts.push(script);
                }

                // Collect zones from user scripts too
                if (script.zones && script.zones.length) {
                  script.zones.forEach(z => {
                    if (!this.availableZones.includes(z)) {
                      this.availableZones.push(z);
                    }
                  });
                }
              });

              // Sort and apply filters after user scripts are loaded
              this.scripts.sort((a, b) => a.name.localeCompare(b.name));
              this.availableZones.sort();
              this.applyFilters();
            });
          } else if (response.status === 401) {
            window.location.href = '/logout';
          }
          this.loading = false;
        }).catch(() => {
          this.loading = false;
        });
      } else {
        this.loading = false;
      }
    },

    createScript(isUserScript = false) {
      this.scriptFormModal.isEdit = false;
      this.scriptFormModal.scriptId = '';
      this.scriptFormModal.isUserScript = isUserScript;
      this.scriptFormModal.show = true;
    },

    editScript(scriptId) {
      const script = this.scripts.find(s => s.script_id === scriptId);
      this.scriptFormModal.isEdit = true;
      this.scriptFormModal.scriptId = scriptId;
      this.scriptFormModal.isUserScript = script && script.user_id ? true : false;
      this.scriptFormModal.show = true;
    },

    canEditScript(script) {
      // User can edit their own scripts or global scripts if they have permission
      if (script.user_id && script.user_id === this.currentUserId) return true;
      if (!script.user_id) return true; // Global scripts visible to all with permission
      return false;
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

    filterChanged() {
      this.$nextTick(() => {
        this.applyFilters();
      });
    },

    showAllZonesChanged() {
      // Reload data from backend when showAllZones changes
      this.getScripts();
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.scripts.forEach(s => {
        let showRow = true;

        // Filter by type
        if (this.filterType === 'global' && s.user_id) showRow = false;
        if (this.filterType === 'user' && s.user_id !== this.currentUserId) showRow = false;

        // Filter by zone (unless showAllZones is true)
        if (!this.showAllZones && this.currentZone) {
          const zones = s.zones || [];
          if (zones.length > 0) {
            // Script has zones - only show if current zone matches
            if (!zones.includes(this.currentZone)) showRow = false;
          }
          // If zones is empty/undefined, script shows in all zones (no restriction)
        }

        // Search term filtering
        if (term.length > 0) {
          const inName = s.name.toLowerCase().includes(term);
          const inDesc = s.description.toLowerCase().includes(term);
          showRow = showRow && (inName || inDesc);
        }

        s.searchHide = !showRow;
      });
    },
  };
}
