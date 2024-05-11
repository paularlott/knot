window.spacesListComponent = function(userId, username, forUserId, canManageSpaces, wildcardDomain, location) {
  return {
    loading: true,
    spaces: [],
    timerIDs: [],
    deleteConfirm: {
      show: false,
      space: {
        space_id: '',
        name: '',
      }
    },
    forUserId: forUserId,
    canManageSpaces: canManageSpaces,
    users: [],
    searchTerm: '',
    async init() {
      if(this.canManageSpaces) {
        const usersResponse = await fetch('/api/v1/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        usersList = await usersResponse.json();
        this.users = usersList.users;
      }

      this.getSpaces();
    },
    async userChanged() {
      this.loading = true;

      if(this.forUserId.length == 0) {
        this.forUsername = "";
      } else {
        const user = this.users.find(user => user.user_id === this.forUserId);
        this.forUsername = user.username;
      }

      this.getSpaces();
    },
    async getSpaces() {
      // Clear all timers
      Object.keys(this.timerIDs).forEach((key) => {
        clearInterval(this.timerIDs[key]);
      });
      this.timerIDs = [];

      const response = await fetch('/api/v1/spaces?user_id=' + this.forUserId, {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      spacesList = await response.json();
      this.spaces = spacesList.spaces;

      this.spaces.forEach(space => {
        space.showMenu = false;
        space.showIdPopup = false;
        space.showSSHPopup = false;
        space.showPortMenu = false;

        // Setup the available services
        space.update_available = false;
        space.is_deployed = false;
        space.is_pending = false;
        space.has_code_server = false;
        space.has_ssh = false;
        space.has_terminal = false;
        space.has_http_vnc = false;
        space.tcp_ports = [];
        space.http_ports = [];
        space.is_local = space.location == '' || location == space.location;

        if(space.is_local) {
          this.fetchServiceState(space, true);

          this.timerIDs[space.space_id] = setInterval(async () => {
            await this.fetchServiceState(space);
          }, 5000);
        }
      });
      this.loading = false;
    },
    async fetchServiceState(space) {
      await fetch(`/api/v1/spaces/${space.space_id}/service-state`, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((serviceState) => {
            space.name = serviceState.name;
            space.location = serviceState.location;
            space.has_code_server = serviceState.has_code_server;
            space.has_ssh = serviceState.has_ssh;
            space.has_terminal = serviceState.has_terminal;
            space.is_deployed = serviceState.is_deployed;
            space.is_pending = serviceState.is_pending;
            space.update_available = serviceState.update_available;
            space.tcp_ports = serviceState.tcp_ports;
            space.http_ports = serviceState.http_ports;
            space.has_http_vnc = serviceState.has_http_vnc;
            space.sshCmd = "ssh -o ProxyCommand='knot forward ssh %h' -o StrictHostKeyChecking=no " + username + "@" + serviceState.name;
            space.is_local = space.location == '' || location == space.location;

            // If space is not local then stop the timer
            if (!space.is_local) {
              clearInterval(this.timerIDs[space.space_id]);
              delete this.timerIDs[space.space_id];
            }
          });
        } else if (response.status === 401) {
          window.location.href = '/login?redirect=' + window.location.pathname;
        } else {
          space.has_code_server = space.has_ssh = space.has_terminal = false;
          space.tcp_ports = space.http_ports = [];

          // If 404 then remove the space from the array
          this.spaces = this.spaces.filter(s => s.space_id !== space.space_id);

          // If time exists for the space then clear it
          if (this.timerIDs[space.space_id]) {
            clearInterval(this.timerIDs[space.space_id]);
            delete this.timerIDs[space.space_id];
          }
        }
      });
    },
    async startSpace(spaceId) {
      var self = this;
      await fetch(`/api/v1/spaces/${spaceId}/start`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Space starting", type: 'success' });
        } else {
          response.json().then((data) => {
            self.$dispatch('show-alert', { msg: "Space could not be started: " + data.error, type: 'error' });
          });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Space could not be started: " + error, type: 'error' });
      }).finally(() => {
        const space = this.spaces.find(space => space.space_id === spaceId);
        space.showMenu = false;
        this.fetchServiceState(space, true);
      });
    },
    async stopSpace(spaceId) {
      var self = this;
      await fetch(`/api/v1/spaces/${spaceId}/stop`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Space stopping", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Space could not be stopped", type: 'error' });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Space could not be stopped: " + error, type: 'error' });
      }).finally(() => {
        const space = this.spaces.find(space => space.space_id === spaceId);
        space.showMenu = false;
        this.fetchServiceState(space, true);
      });
    },
    async deleteSpace(spaceId) {
      var self = this;
      await fetch(`/api/v1/spaces/${spaceId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Space deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Space could not be deleted", type: 'error' });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Space could not be deleted: " + error, type: 'error' });
      }).finally(() => {
        const space = this.spaces.find(space => space.space_id === spaceId);
        space.showMenu = false;
        this.getSpaces();
      });
    },
    editSpace(spaceId) {
      window.location.href = `/spaces/edit/${spaceId}`;
    },
    async openWindowForPort(spaceId, spaceName, port) {
      openPortWindow(spaceId, wildcardDomain, username, spaceName, port);
    },
    async openWindowForVNC(spaceId, spaceName) {
      openVNC(spaceId, wildcardDomain, username, spaceName);
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all spaces if name or template name contains the term show; else hide
      this.spaces.forEach(space => {
        if(term.length == 0) {
          space.searchHide = false;
        } else {
          space.searchHide = !(
            space.name.toLowerCase().includes(term) ||
            space.template_name.toLowerCase().includes(term) ||
            space.location.toLowerCase().includes(term)
          );
        }
      });
    },
  };
}
