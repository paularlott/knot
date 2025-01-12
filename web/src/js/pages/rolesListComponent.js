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
      this.getRoles();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getRoles();
      }, 3000);
    },

    async getRoles() {
      const response = await fetch('/api/v1/roles', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      let roleList = await response.json();
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
      let self = this;
      await fetch(`/api/v1/roles/${roleId}`, {
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
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all groups if name contains the term show; else hide
      this.roles.forEach(r => {
        if(term.length == 0) {
          r.searchHide = false;
        } else {
          r.searchHide = !r.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
