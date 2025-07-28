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
    roles: [],
    searchTerm: Alpine.$persist('').as('role-search-term').using(sessionStorage),

    async init() {
      await this.getRoles();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getRoles();
      }, 3000);
    },

    async getRoles() {
      const response = await fetch('/api/roles', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      const roleList = await response.json();
      roleList.roles.sort((a, b) => (a.name > b.name) ? 1 : -1);
      this.roles = roleList.roles

      // Apply search filter
      this.searchChanged();

      this.loading = false;
      this.roles.forEach(role => {
        role.showIdPopup = false;
      });
    },
    editRole(roleId) {
      window.location.href = `/roles/edit/${roleId}`;
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
        } else {
          self.$dispatch('show-alert', { msg: "Role could not be deleted", type: 'error' });
        }
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
