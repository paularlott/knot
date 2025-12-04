import Alpine from 'alpinejs';

window.rolesListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      role: {
        role_id: '',
        name: '',
      }
    },
    roleFormModal: {
      show: false,
      isEdit: false,
      roleId: '',
    },
    roles: [],
    searchTerm: Alpine.$persist('').as('role-search-term').using(sessionStorage),

    async init() {
      await this.getRoles();

      // Subscribe to SSE for real-time updates
      if (window.sseClient) {
        window.sseClient.subscribe('roles:changed', (payload) => {
          if (payload?.id) this.getRoles(payload.id);
        });

        window.sseClient.subscribe('roles:deleted', (payload) => {
          this.roles = this.roles.filter(x => x.role_id !== payload?.id);
          this.searchChanged();
        });
      }
    },

    async getRoles(roleId) {
      const url = roleId ? `/api/roles/${roleId}` : '/api/roles';
      await fetch(url, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((data) => {
            const roleList = roleId ? [data] : data.roles;

            roleList.forEach(role => {
              role.showIdPopup = false;
              const index = this.roles.findIndex(r => r.role_id === role.role_id);
              if (index >= 0) {
                this.roles[index] = role;
              } else {
                this.roles.push(role);
              }
            });

            this.roles.sort((a, b) => {
              return a.name.localeCompare(b.name);
            });

            // Apply search filter
            this.searchChanged();

            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    createRole() {
      this.roleFormModal.isEdit = false;
      this.roleFormModal.roleId = '';
      this.roleFormModal.show = true;
    },
    editRole(roleId) {
      this.roleFormModal.isEdit = true;
      this.roleFormModal.roleId = roleId;
      this.roleFormModal.show = true;
    },
    loadRoles() {
      this.getRoles();
    },
    async deleteRole(roleId) {
      const self = this;
      await fetch(`/api/roles/${roleId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Role deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Role could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
      this.getRoles();
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all groups if name contains the term show; else hide
      this.roles.forEach(r => {
        if(term.length === 0) {
          r.searchHide = false;
        } else {
          r.searchHide = !r.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
