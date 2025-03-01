import { space } from "postcss/lib/list";

window.spacesListComponent = function(userId, username, forUserId, canManageSpaces, wildcardDomain, location, canTransferSpaces, canShareSpaces) {

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
    ceaseShareConfirm: {
      show: false,
      space: {
        space_id: '',
        name: '',
      }
    },
    chooseUser: {
      toUserId: '',
      invalidUser: false,
      show: false,
      isShare: false,
      space: {
        space_id: '',
        name: '',
      }
    },
    showingSpecificUser: userId !== forUserId,
    forUserId: userId == forUserId && canManageSpaces ? Alpine.$persist(forUserId).as('forUserId').using(sessionStorage) : forUserId,
    canManageSpaces: canManageSpaces,
    canTransferSpaces: canTransferSpaces,
    users: [],
    forUsersList: [],
    searchTerm: Alpine.$persist('').as('spaces-search-term').using(sessionStorage),
    quotaComputeLimitShow: false,
    quotaStorageLimitShow: false,
    badScheduleShow: false,
    showRunningOnly:Alpine.$persist(false).as('spaceFilterRunningOnly').using(sessionStorage),
    showLocalOnly:Alpine.$persist(true).as('spaceFilterLocalOnly').using(sessionStorage),
    showSharedOnly:Alpine.$persist(false).as('spaceFilterSharedOnly').using(sessionStorage),
    showSharedWithMeOnly:Alpine.$persist(false).as('spaceFilterSharedWithMeOnly').using(sessionStorage),

    async init() {
      if(this.canManageSpaces || this.canTransferSpaces || this.canShareSpaces) {
        const usersResponse = await fetch('/api/v1/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        let usersList = await usersResponse.json();
        this.users = usersList.users;

        this.forUsersList = [{ user_id: '', username: '[All Users]' }, { user_id: userId, username: '[My Spaces]' }, ...usersList.users];

        this.$dispatch('refresh-user-autocompleter');
      }

      this.getSpaces(true);

      // Start a timer to look for new spaces periodically
      setInterval(async () => {
        this.getSpaces(false);
      }, 3000);
    },
    async userSearchReset() {
      this.forUserId = userId;
      this.forUsername = '[My Spaces]';
      this.$dispatch('refresh-user-autocompleter');
      this.userChanged();
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
              const existing = this.spaces.find(s => s.space_id === space.space_id);
              if(!existing) {
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
                space.has_state = false;

                this.spaces.push(space);
                spacesAdded = true;

                // Lookup the space and update it's state
                const s2 = this.spaces.find(s2 => s2.space_id === space.space_id);
                await this.fetchServiceState(s2);
                this.timerIDs[s2.space_id] = setInterval(async () => {
                  this.fetchServiceState(s2);
                }, 2000);
              }
              // Else update the sharing information
              else {
                existing.shared_user_id = space.shared_user_id;
                existing.shared_username = space.shared_username;
              }
            });

             // If spaces added then sort them by name
            if(spacesAdded) {
              this.spaces.sort((a, b) => (a.name > b.name) ? 1 : -1);
            }

            // Look through the list of spaces and remove any that are not in the spacesList
            this.spaces.forEach((space, index) => {
              if(!spacesList.spaces.find(s => s.space_id === space.space_id)) {
                this.spaces.splice(index, 1);
              }
            });

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
            space.has_state = serviceState.has_state;
          });
        } else if (response.status === 401) {
          window.location.href = '/login?redirect=' + window.location.pathname;
        } else if (response.status === 404) {
          // Remove the space from the array
          this.spaces = this.spaces.filter(s => s.space_id !== space.space_id);

          // If time exists for the space then clear it
          if (this.timerIDs[space.space_id]) {
            clearInterval(this.timerIDs[space.space_id]);
            this.timerIDs[space.space_id] = null;
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
        this.fetchServiceState(space);
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
        this.fetchServiceState(space);
      });
    },
    async deleteSpace(spaceId) {
      let self = this;
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
      });
    },
    async ceaseSharing(spaceId) {
      let self = this;
      await fetch(`/api/v1/spaces/${spaceId}/share`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Sharing of Space Stopped", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Could not stop sharing of space", type: 'error' });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Could not stop sharing of space: " + error, type: 'error' });
      });
    },
    editSpace(spaceId) {
      window.location.href = `/spaces/edit/${spaceId}`;
    },
    async openWindowForPort(spaceUsername, spaceId, spaceName, port) {
      openPortWindow(spaceId, wildcardDomain, spaceUsername == '' ? username : spaceUsername, spaceName, port);
    },
    async openWindowForVNC(spaceId, spaceName) {
      openVNC(spaceId, wildcardDomain, username, spaceName);
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all spaces if name or template name contains the term show; else hide
      this.spaces.forEach(space => {
        if(term.length == 0) {
          space.searchHide =
            (this.showLocalOnly && !space.is_local) ||
            (this.showRunningOnly && !space.is_deployed) ||
            (this.showSharedOnly && (space.shared_user_id == "" || space.shared_user_id == this.forUserId)) ||
            (this.showSharedWithMeOnly && (space.shared_user_id == "" || space.shared_user_id != this.forUserId));
        } else {
          space.searchHide = !(
            space.name.toLowerCase().includes(term) ||
            space.template_name.toLowerCase().includes(term) ||
            space.location.toLowerCase().includes(term)
          ) ||
          (this.showLocalOnly && !space.is_local) ||
          (this.showRunningOnly && !space.is_deployed) ||
          (this.showSharedOnly && (space.shared_user_id == "" || space.shared_user_id == this.forUserId)) ||
          (this.showSharedWithMeOnly && (space.shared_user_id == "" || space.shared_user_id != this.forUserId));
        }
      });
    },
    async copyToClipboard(text) {
      await navigator.clipboard.writeText(text);
      this.$dispatch('show-alert', { msg: "Copied to clipboard", type: 'success' });
    },
    async transferSpaceTo() {
      let self = this;

      if(this.chooseUser.toUserId == '') {
        this.chooseUser.invalidUser = true;
        return;
      }

      this.chooseUser.invalidUser = false;

      // Transfer the space to the new user
      await fetch(
        this.isShare
          ? `/api/v1/spaces/${this.chooseUser.space.space_id}/transfer`
          : `/api/v1/spaces/${this.chooseUser.space.space_id}/share`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          user_id: this.chooseUser.toUserId
        })
      }).then((response) => {
        if (response.status === 200) {

          if(!self.chooseUser.isShare) {
            // Remove the space from the array
            self.spaces = self.spaces.filter(s => s.space_id !== self.chooseUser.space.space_id);

            // If time exists for the space then clear it
            if (self.timerIDs[self.chooseUser.space.space_id]) {
              clearInterval(self.timerIDs[self.chooseUser.space.space_id]);
              self.timerIDs[self.chooseUser.space.space_id] = null;
            }
          }

          if(self.chooseUser.isShare) {
            self.$dispatch('show-alert', { msg: "Space shared", type: 'success' });
          }
          else {
            self.$dispatch('show-alert', { msg: "Space transferred", type: 'success' });
          }
          this.chooseUser.show = false;
        } else if(response.status === 507) {
          if(self.chooseUser.isShare) {
            self.$dispatch('show-alert', { msg: "Space could not be shared as the user has exceeded their quota.", type: 'error' });
          }
          else {
            self.$dispatch('show-alert', { msg: "Space could not be transferred as the user has exceeded their quota.", type: 'error' });
          }
        } else if(response.status === 403) {
          if(self.chooseUser.isShare) {
            self.$dispatch('show-alert', { msg: "Space could not be shared as the user is not allowed to use the template.", type: 'error' });
          }
          else {
            self.$dispatch('show-alert', { msg: "Space could not be transferred as the user is not allowed to use the template.", type: 'error' });
          }
        } else {
          response.json().then((data) => {
            if(self.isShare) {
              self.$dispatch('show-alert', { msg: "Space could not be shared: " + data.error, type: 'error' });
            }
            else {
              self.$dispatch('show-alert', { msg: "Space could not be transferred: " + data.error, type: 'error' });
            }
          });
        }
      }).catch((error) => {
        if(self.chooseUser.isShare) {
          self.$dispatch('show-alert', { msg: "Space could not be shared: " + error, type: 'error' });
        }
        else {
          self.$dispatch('show-alert', { msg: "Space could not be transferred: " + error, type: 'error' });
        }
      });
    },
  };
}
