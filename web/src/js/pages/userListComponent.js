import Alpine from 'alpinejs';

window.userListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    userFormModal: {
      show: false,
      isEdit: false,
      userId: ''
    },
    deleteConfirm: {
      show: false,
      user: {
        user_id: '',
        username: '',
      }
    },
    stopConfirm: {
      show: false,
      user: {
        user_id: '',
        username: '',
      }
    },
    users: [],
    roles: [],
    groups: [],
    searchTerm: Alpine.$persist('').as('user-search-term').using(sessionStorage),

    async init() {
      await this.getUsers();

      window.addEventListener('close-user-form', () => {
        this.userFormModal.show = false;
        this.getUsers();
      });

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('users:changed', () => {
          this.getUsers();
        });
      }
    },

    async getUsers() {
      await fetch('/api/roles', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((roleList) => {
            this.roles = roleList.roles;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
          return;
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
        return;
      });

      await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((groupList) => {
            this.groups = groupList.groups;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
          return;
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
        return;
      });

      await fetch('/api/users', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((usersList) => {
            this.users = usersList.users;

            this.loading = false;
            this.users.forEach(user => {

              // Make last_login_at human readable data time in the browser's timezone
              if (user.last_login_at) {
                const date = new Date(user.last_login_at);
                user.last_login_at = date.toLocaleString();
              } else {
                user.last_login_at = '-';
              }

              // Convert role IDs to names
              user.role_names = [];
              user.roles.forEach(roleId => {
                this.roles.forEach(role => {
                  if (role.role_id === roleId) {
                    user.role_names.push(role.name);
                  }
                });
              });

              // Convert group IDs to names
              user.group_names = [];
              user.groups.forEach(groupId => {
                this.groups.forEach(group => {
                  if (group.group_id === groupId) {
                    user.group_names.push(group.name);
                  }
                });
              });
            });

            // Apply search filter
            this.searchChanged();
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    createUser() {
      this.userFormModal.isEdit = false;
      this.userFormModal.userId = '';
      this.userFormModal.show = true;
    },
    editUser(userId) {
      this.userFormModal.isEdit = true;
      this.userFormModal.userId = userId;
      this.userFormModal.show = true;
    },
    userSpaces(userId) {
      window.location.href = `/spaces/${userId}`;
    },
    async deleteUser(userId) {
      const self = this;
      await fetch(`/api/users/${userId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "User deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "User could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });

      this.getUsers();
      this.deleteConfirm.show = false
    },
    async stopSpaces(userId) {
      const self = this;
      await fetch(`/api/spaces/${userId}/stop-for-user`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "User spaces stopped", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "User spaces could not be stopped", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });

      this.getUsers();
      this.stopConfirm.show = false
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all users if name or email address contains the term show; else hide
      this.users.forEach(u => {
        if(term.length === 0) {
          u.searchHide = false;
        } else {
          u.searchHide = !(
            u.username.toLowerCase().includes(term) ||
            u.email.toLowerCase().includes(term)
          );
        }
      });
    },
  };
}
