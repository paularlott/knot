window.userListComponent = function() {
  return {
    loading: true,
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
    searchTerm: '',

    async getUsers() {
      this.loading = true;
      const rolesResponse = await fetch('/api/v1/roles', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.roles = await rolesResponse.json();

      const groupsResponse = await fetch('/api/v1/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      groupList = await groupsResponse.json();
      this.groups = groupList.groups;

      const response = await fetch('/api/v1/users', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      usersList = await response.json();
      this.users = usersList.users;

      this.loading = false;
      this.users.forEach(user => {
        user.showIdPopup = false;
        user.showMenu = false;

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
            if (role.id_role === roleId) {
              user.role_names.push(role.role_name);
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
    },
    editUser(userId) {
      window.location.href = `/users/edit/${userId}`;
    },
    userSpaces(userId) {
      window.location.href = `/spaces/${userId}`;
    },
    async deleteUser(userId) {
      var self = this;
      await fetch(`/api/v1/users/${userId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "User deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "User could not be deleted", type: 'error' });
        }
      });

      this.getUsers();
      this.deleteConfirm.show = false
    },
    async stopSpaces(userId) {
      var self = this;
      await fetch(`/api/v1/spaces/stop-for-user/${userId}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "User spaces stopped", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "User spaces could not be stopped", type: 'error' });
        }
      });

      this.getUsers();
      this.stopConfirm.show = false
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all users if name or email address contains the term show; else hide
      this.users.forEach(u => {
        if(term.length == 0) {
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
