window.spacesListComponent = function(userId, username, forUserId, forUserUsername, canManageSpaces, wildcardDomain) {
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
    forUsername: forUserUsername,
    canManageSpaces: canManageSpaces,
    createURL: "",
    users: [],
    async init() {
      if(this.canManageSpaces) {
        const usersResponse = await fetch('/api/v1/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        this.users = await usersResponse.json();
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
      this.createURL = '/spaces/create' + (this.forUserId.length && this.forUserId != userId ? '/' + this.forUserId : '');

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
      this.spaces = await response.json();
      this.spaces.forEach(space => {
        space.starting = false;
        space.stopping = false;
        space.deleting = false;
        space.showMenu = false;
        space.showIdPopup = false;
        space.showSSHPopup = false;
        space.showPortMenu = false;
        space.sshCmd = "ssh -o ProxyCommand='knot forward ssh %h' -o StrictHostKeyChecking=no " + username + "@" + space.name;
        this.timerIDs[space.space_id] = setInterval(async () => {
          await this.fetchServiceState(space);
        }, 5000);
      });
      this.loading = false;
    },
    async fetchServiceState(space, resetStateFlags = false) {
      await fetch(`/api/v1/spaces/${space.space_id}/service-state`, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((serviceState) => {
            space.has_code_server = serviceState.has_code_server;
            space.has_ssh = serviceState.has_ssh;
            space.has_terminal = serviceState.has_terminal;
            space.is_deployed = serviceState.is_deployed;
            space.update_available = serviceState.update_available;
            space.tcp_ports = serviceState.tcp_ports;
            space.http_ports = serviceState.http_ports;
            space.has_http_vnc = serviceState.has_http_vnc;

            if (resetStateFlags) {
              space.starting = false;
              space.stopping = false;
              space.deleting = false;
            }
          });
        } if (response.status === 401) {
          window.location.href = '/login?redirect=' + window.location.pathname;
        } else {
          space.has_code_server = space.has_ssh = space.has_terminal = false;
          space.tcp_ports = space.http_ports = [];

          // If 404 then remove the space from the array
          if (response.status === 404) {
            this.spaces = this.spaces.filter(s => s.space_id !== space.space_id);

            // Clear the timer
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
          self.$dispatch('show-alert', { msg: "Space started", type: 'success' });
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
          self.$dispatch('show-alert', { msg: "Space stopped", type: 'success' });
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
    }
  };
}
