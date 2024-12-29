window.spacesListComponent = function(userId, username, forUserId, canManageSpaces, wildcardDomain, location) {

  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

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
    showingSpecificUser: userId !== forUserId,
    forUserId: userId == forUserId && canManageSpaces ? Alpine.$persist(forUserId).as('forUserId').using(sessionStorage) : forUserId,
    canManageSpaces: canManageSpaces,
    users: [],
    searchTerm: Alpine.$persist('').as('spaces-search-term').using(sessionStorage),
    quotaComputeLimitShow: false,
    quotaStorageLimitShow: false,
    badScheduleShow: false,

    async init() {
      if(this.canManageSpaces) {
        const usersResponse = await fetch('/api/v1/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        let usersList = await usersResponse.json();
        this.users = usersList.users;

        // Look through the users list and change the one that matches the userId to be Your spaces
        this.users.forEach(user => {
          if(user.user_id === userId) {
            user.username = "My Spaces";
          }
        });
      }

      this.getSpaces(true);

      // Start a timer to look for new spaces periodically
      setInterval(async () => {
        this.getSpaces(false);
      }, 5000);
    },
    async userChanged() {
      this.loading = true;

      if(this.forUserId.length == 0) {
        this.forUsername = "";
      } else {
        const user = this.users.find(user => user.user_id === this.forUserId);
        this.forUsername = user.username;
      }

      this.getSpaces(true);
    },
    async getSpaces(replaceAll) {
      // Clear all timers if replacing all
      if(replaceAll) {
        Object.keys(this.timerIDs).forEach((key) => {
          clearInterval(this.timerIDs[key]);
        });
        this.timerIDs = [];
        this.spaces = [];
        this.loading = true;
      }

      await fetch('/api/v1/spaces?user_id=' + this.forUserId, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if(response.status === 200) {
          var spacesAdded = false;

          response.json().then((spacesList) => {
            spacesList.spaces.forEach(async space => {
              // If this space isn't in this.spaces then add it
              if(!this.spaces.find(s => s.space_id === space.space_id)) {
                // Setup the available services
                space.update_available = false;
                space.is_deployed = false;
                space.is_pending = false;
                space.is_deleting = false;
                space.has_code_server = false;
                space.has_ssh = false;
                space.has_terminal = false;
                space.has_http_vnc = false;
                space.tcp_ports = [];
                space.http_ports = [];
                space.is_local = space.location == '' || location == space.location;
                space.has_vscode_tunnel = false;
                space.vscode_tunnel_name = '';

                this.spaces.push(space);
                spacesAdded = true;

                // Lookup the space and update it's state
                const s2 = this.spaces.find(s2 => s2.space_id === space.space_id);
                await this.fetchServiceState(s2);
                this.timerIDs[s2.space_id] = setInterval(async () => {
                  this.fetchServiceState(s2);
                }, 3000);
              }
            });

             // If spaces added then sort them by name
            if(spacesAdded) {
              this.spaces.sort((a, b) => (a.name > b.name) ? 1 : -1);
            }

            // Apply search filter
            this.searchChanged();

            this.loading = false;
          });

        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch((error) => {
        window.location.href = '/logout';
      });
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
            space.is_deleting = serviceState.is_deleting;
            space.update_available = serviceState.update_available;
            space.tcp_ports = serviceState.tcp_ports;
            space.http_ports = serviceState.http_ports;
            space.has_http_vnc = serviceState.has_http_vnc;
            space.has_vscode_tunnel = serviceState.has_vscode_tunnel;
            space.vscode_tunnel_name = serviceState.vscode_tunnel_name;
            space.sshCmd = "ssh -o ProxyCommand='knot forward ssh " + serviceState.name + "' -o StrictHostKeyChecking=no " + username + "@knot." + serviceState.name;
            space.is_local = space.location == '' || location == space.location;
          });
        } else if (response.status === 401) {
          window.location.href = '/login?redirect=' + window.location.pathname;
        } else if (response.status === 404) {
          // Remove the space from the array
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
        } else if(response.status === 503) {
          response.json().then((data) => {
            if(data.error == 'outside of schedule') {
              self.badScheduleShow = true;
            } else {
              self.$dispatch('show-alert', { msg: "Space could not be started: " + data.error, type: 'error' });
            }
          });
        } else if(response.status === 507) {
          response.json().then((data) => {
            // If compute units exceeded then show the dialog
            if(data.error == 'compute unit quota exceeded') {
              self.quotaComputeLimitShow = true;
            } else if(data.error == 'storage unit quota exceeded') {
              self.quotaStorageLimitShow = true;
            } else {
              self.$dispatch('show-alert', { msg: "Space could not be as it has exceeded quota limits.", type: 'error' });
            }
          });
        } else {
          response.json().then((data) => {
            self.$dispatch('show-alert', { msg: "Space could not be started: " + data.error, type: 'error' });
          });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Space could not be started: " + error, type: 'error' });
      }).finally(() => {
        const space = this.spaces.find(space => space.space_id === spaceId);
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
          self.$dispatch('show-alert', { msg: "Space deleting", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Space could not be deleted", type: 'error' });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Space could not be deleted: " + error, type: 'error' });
      }).finally(() => {
        const space = this.spaces.find(space => space.space_id === spaceId);
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
    async copyToClipboard(text) {
      await navigator.clipboard.writeText(text);
      this.$dispatch('show-alert', { msg: "Copied to clipboard", type: 'success' });
    },
  };
}
