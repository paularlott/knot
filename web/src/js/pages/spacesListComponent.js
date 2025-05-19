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
    visibleSpaces: 0,
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
      toUserUsername: '',
      invalidUser: false,
      show: false,
      isShare: false,
      space: {
        space_id: '',
        name: '',
      }
    },
    spaceDesc: {
      show: false,
      space: {
        name: '',
        description: '',
        note: '',
      }
    },

    showingSpecificUser: userId !== forUserId,
    forUserId: userId == forUserId && canManageSpaces ? Alpine.$persist(forUserId).as('forUserId').using(sessionStorage) : forUserId,
    canManageSpaces: canManageSpaces,
    canTransferSpaces: canTransferSpaces,
    users: [],
    forUsersList: [],
    shareUsers: [],
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
        let usersResponse = await fetch('/api/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        let usersList = await usersResponse.json();
        this.users = usersList.users;

        this.forUsersList = [{ user_id: '', username: '[All Users]' }, { user_id: userId, username: '[My Spaces]' }, ...usersList.users];

        setTimeout(async () => {
          usersResponse = await fetch('/api/users?state=active&local=true', {
            headers: {
              'Content-Type': 'application/json'
            }
          });
          usersList = await usersResponse.json();
          this.shareUsers = usersList.users;

          this.$dispatch('refresh-user-autocompleter');
        }, 0);

        this.$dispatch('refresh-user-autocompleter');
      }

      this.getSpaces();

      // Start a timer to look for new spaces periodically
      setInterval(async () => {
        this.getSpaces();
      }, 1000);
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

      this.spaces = [];
      this.getSpaces();
    },
    async getSpaces() {
      await fetch('/api/spaces?user_id=' + this.forUserId, {
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
                space.is_local = space.location == '' || location == space.location;
                space.uptime = this.formatTimeDiff(space.started_at);

                this.spaces.push(space);
                spacesAdded = true;
              }
              // Else update the sharing information
              else {
                existing.shared_user_id = space.shared_user_id;
                existing.shared_username = space.shared_username;
                existing.name = space.name;
                existing.description = space.description;
                existing.note = space.note;
                existing.location = space.location;
                existing.has_code_server = space.has_code_server;
                existing.has_ssh = space.has_ssh;
                existing.has_terminal = space.has_terminal;
                existing.is_deployed = space.is_deployed;
                existing.is_pending = space.is_pending;
                existing.is_deleting = space.is_deleting;
                existing.update_available = space.update_available;
                existing.tcp_ports = space.tcp_ports;
                existing.http_ports = space.http_ports;
                existing.has_http_vnc = space.has_http_vnc;
                existing.has_vscode_tunnel = space.has_vscode_tunnel;
                existing.vscode_tunnel_name = space.vscode_tunnel_name;
                existing.sshCmd = "ssh -o ProxyCommand='knot forward ssh " + space.name + "' -o StrictHostKeyChecking=no " + username + "@knot." + space.name;
                existing.is_local = space.location == '' || location == space.location;
                existing.has_state = space.has_state;
                existing.started_at = space.started_at;
                existing.uptime = this.formatTimeDiff(space.started_at);
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
    async startSpace(spaceId) {
      var self = this;
      await fetch(`/api/spaces/${spaceId}/start`, {
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
        self.getSpaces();
      });
    },
    async stopSpace(spaceId) {
      var self = this;
      await fetch(`/api/spaces/${spaceId}/stop`, {
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
        self.getSpaces();
      });
    },
    async deleteSpace(spaceId) {
      let self = this;
      await fetch(`/api/spaces/${spaceId}`, {
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
      await fetch(`/api/spaces/${spaceId}/share`, {
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
    async openWindowForVNC(spaceUsername, spaceId, spaceName) {
      openVNC(spaceId, wildcardDomain, spaceUsername == '' ? username : spaceUsername, spaceName);
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all spaces if name or template name contains the term show; else hide
      this.visibleSpaces = 0;
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

        if(!space.searchHide) {
          this.visibleSpaces++;
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
        this.chooseUser.isShare
          ? `/api/spaces/${this.chooseUser.space.space_id}/share`
          : `/api/spaces/${this.chooseUser.space.space_id}/transfer`,
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
          }

          if(self.chooseUser.isShare) {
            self.$dispatch('show-alert', { msg: "Space shared", type: 'success' });
          }
          else {
            self.$dispatch('show-alert', { msg: "Space transferred", type: 'success' });
          }
          self.chooseUser.show = false;
          self.getSpaces();
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
            if(self.chooseUser.isShare) {
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

    formatTimeDiff(utcTime) {
      // Convert input to Date if not already
      const givenTime = utcTime instanceof Date ? utcTime : new Date(utcTime);
      const currentTime = new Date();

      // Calculate difference in seconds
      const diffSeconds = Math.abs(Math.floor((currentTime - givenTime) / 1000));

      // Format based on magnitude
      if (diffSeconds < 60) {
        // Less than a minute: show seconds
        return `${diffSeconds}s`;
      } else if (diffSeconds < 3600) {
        // Less than an hour: show minutes and seconds
        const minutes = Math.floor(diffSeconds / 60);
        const seconds = diffSeconds % 60;
        return `${minutes}m ${seconds}s`;
      } else if (diffSeconds < 86400) {
        // Less than a day: show hours
        const hours = Math.floor(diffSeconds / 3600);
        return `${hours}h`;
      } else {
        // More than a day: show days
        const days = Math.floor(diffSeconds / 86400);
        return `${days}d`;
      }
    }
  };
}
