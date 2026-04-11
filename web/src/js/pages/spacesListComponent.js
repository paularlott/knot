import Alpine from "alpinejs";
import { popup } from "../popup.js";

// Debounce function to limit rapid calls
function debounce(func, wait) {
  let timeout;
  return function executedFunction(...args) {
    const context = this;
    const later = () => {
      clearTimeout(timeout);
      func.apply(context, args);
    };
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
  };
}

window.spacesListComponent = function (
  userId,
  username,
  forUserId,
  canManageSpaces,
  wildcardDomain,
  zone,
  canTransferSpaces,
  canShareSpaces,
) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  return {
    loading: true,
    spaces: [],
    visibleSpaces: 0,
    // Debounced version of getSpaces for SSE events (500ms debounce)
    debouncedGetSpaces: debounce(function (spaceId) {
      this.getSpaces(spaceId);
    }, 500),
    deleteConfirm: {
      show: false,
      space: {
        space_id: "",
        name: "",
      },
    },
    ceaseShareConfirm: {
      show: false,
      targetUserId: "",
      space: {
        space_id: "",
        name: "",
      },
    },
    chooseUser: {
      toUserId: "",
      toUserUsername: "",
      invalidUser: false,
      show: false,
      isShare: false,
      space: {
        space_id: "",
        name: "",
      },
    },
    spaceDesc: {
      show: false,
      space: {
        name: "",
        description: "",
        note: "",
      },
    },

    showingSpecificUser: userId !== forUserId,
    forUserId:
      userId === forUserId && canManageSpaces
        ? Alpine.$persist(forUserId).as("forUserId").using(sessionStorage)
        : forUserId,
    canManageSpaces,
    canTransferSpaces,
    canShareSpaces,
    users: [],
    forUsersList: [],
    shareUsers: [],
    searchTerm: Alpine.$persist("")
      .as("spaces-search-term")
      .using(sessionStorage),
    quotaComputeLimit: {
      show: false,
      isShared: false,
    },
    quotaStorageLimit: {
      show: false,
      isShared: false,
    },
    badScheduleShow: false,
    showRunningOnly: Alpine.$persist(false)
      .as("spaceFilterRunningOnly")
      .using(sessionStorage),
    showLocalOnly: Alpine.$persist(true)
      .as("spaceFilterLocalOnly")
      .using(sessionStorage),
    showSharedOnly: Alpine.$persist(false)
      .as("spaceFilterSharedOnly")
      .using(sessionStorage),
    showSharedWithMeOnly: Alpine.$persist(false)
      .as("spaceFilterSharedWithMeOnly")
      .using(sessionStorage),
    action: "stop", // 'stop' or 'restart'
    collapsedStacks: {}, // tracks which stacks are collapsed
    stackBusy: {}, // tracks which stacks have an in-progress action
    templateSelector: {
      show: false,
      templates: [],
      groups: [],
      searchTerm: "",
    },
    spaceFormModal: {
      show: false,
      isEdit: false,
      spaceId: "",
      templateId: "",
      forUserId: "",
      forUserUsername: "",
    },
    shareUserIds(space) {
      if (!Array.isArray(space?.shares)) {
        return [];
      }

      return space.shares.filter(
        (shareUserId) => typeof shareUserId === "string" && shareUserId.length,
      );
    },
    firstShareUserId(space) {
      return this.shareUserIds(space)[0] || "";
    },
    isSharedWithViewer(space) {
      return this.shareUserIds(space).includes(this.forUserId);
    },
    isSharedWithCurrentUser(space) {
      return this.shareUserIds(space).includes(userId);
    },
    hasShare(space) {
      return this.shareUserIds(space).length > 0;
    },
    hasSpaceAccessForCurrentUser(space) {
      return space.user_id === userId || this.isSharedWithCurrentUser(space);
    },
    shareBadgeText(space) {
      const shareCount = this.shareUserIds(space).length;
      if (!shareCount) {
        return "";
      }
      if (this.isSharedWithCurrentUser(space)) {
        return `Shared By: ${space.username}`;
      }
      if (shareCount === 1) {
        return "Shared";
      }
      return `Shared (${shareCount})`;
    },

    async init() {
      if (
        this.canManageSpaces ||
        this.canTransferSpaces ||
        this.canShareSpaces
      ) {
        let usersResponse = await fetch("/api/users?state=active", {
          headers: {
            "Content-Type": "application/json",
          },
        });
        let usersList = await usersResponse.json();
        this.users = usersList.users;

        this.forUsersList = [
          { user_id: "", username: "[All Users]" },
          { user_id: userId, username: "[My Spaces]", email: "" },
          ...usersList.users,
        ];

        setTimeout(async () => {
          usersResponse = await fetch("/api/users?state=active&local=true", {
            headers: {
              "Content-Type": "application/json",
            },
          });
          usersList = await usersResponse.json();
          this.shareUsers = usersList.users;

          this.$dispatch("refresh-user-autocompleter");
        }, 0);

        this.$dispatch("refresh-user-autocompleter");
      }

      this.getSpaces();

      // Listen for space-created event
      window.addEventListener("space-created", (e) => {
        this.getSpaces(e.detail.space_id);
      });

      // Subscribe to SSE for real-time updates
      if (window.sseClient) {
        window.sseClient.subscribe("space:changed", (payload) => {
          const sharedWithUserIds = payload?.shared_with_user_ids || [];
          const previousUserIds = payload?.previous_user_ids || [];
          const removedFromCurrentView =
            this.forUserId !== "" &&
            previousUserIds.includes(this.forUserId) &&
            !sharedWithUserIds.includes(this.forUserId) &&
            payload?.user_id !== this.forUserId;

          // Check if space was unshared or transferred away from the user we're viewing
          if (removedFromCurrentView) {
            // Remove the space from the list
            this.spaces = this.spaces.filter((s) => s.space_id !== payload?.id);
            this.searchChanged();
            return;
          }
          // Check if we should fetch/update the space
          if (
            this.forUserId === "" ||
            this.forUserId === payload?.user_id ||
            payload?.user_id === userId ||
            sharedWithUserIds.includes(this.forUserId) ||
            previousUserIds.includes(this.forUserId)
          ) {
            this.debouncedGetSpaces(payload?.id);
          }
        });

        window.sseClient.subscribe("space:deleted", (payload) => {
          if (
            this.forUserId === "" ||
            this.forUserId === payload?.user_id ||
            payload?.user_id === userId
          ) {
            this.spaces = this.spaces.filter((s) => s.space_id !== payload?.id);
            this.searchChanged();
          }
        });

        window.sseClient.subscribe("templates:changed", () => {
          this.getSpaces();
        });

        window.sseClient.subscribe("reconnected", () => {
          this.getSpaces();
        });
      }

      // Refresh uptime displays every 5s
      setInterval(() => {
        this.spaces.forEach((space) => {
          space.uptime = this.formatTimeDiff(space.started_at);
        });
      }, 5000);
    },
    userSearchReset() {
      this.forUserId = userId;
      this.forUsername = "[My Spaces]";
      this.$dispatch("refresh-user-autocompleter");
      this.userChanged();
    },
    userChanged() {
      this.loading = true;

      if (this.forUserId.length === 0) {
        this.forUsername = "";
      } else {
        const user = this.users.find((u) => u.user_id === this.forUserId);
        this.forUsername = user.username;
      }

      this.spaces = [];
      this.getSpaces();
    },
    async getSpaces(spaceId) {
      const url = spaceId
        ? `/api/spaces/${spaceId}`
        : `/api/spaces?user_id=${this.forUserId}`;
      await fetch(url, {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 404 && spaceId) {
            // Space not found (deleted), remove from list
            this.spaces = this.spaces.filter((s) => s.space_id !== spaceId);
            this.searchChanged();
          } else if (response.status === 503 && spaceId) {
            // Space temporarily unavailable, ignore (will be updated via SSE)
          } else if (response.status === 200) {
            let spacesAdded = false;

            response.json().then((data) => {
              const spacesList = spaceId ? [data] : data.spaces;
              spacesList.forEach((space) => {
                // Skip spaces that are deleting and have id = name (already deleted)
                if (space.is_deleting && space.space_id === space.name) {
                  return;
                }

                // If this space isn't in this.spaces then add it
                const existing = this.spaces.find(
                  (s) => s.space_id === space.space_id,
                );
                if (!existing) {
                  space.is_local = space.zone === "" || zone === space.zone;
                  space.shares = space.shares || [];
                  space.uptime = this.formatTimeDiff(space.started_at);
                  space.icon_url_exists = this.imageExists(space.icon_url);

                  this.spaces.push(space);
                  spacesAdded = true;
                }
                // Else update the sharing information
                else {
                  // If space is now deleted (is_deleting && id === name), remove it
                  if (space.is_deleting && space.space_id === space.name) {
                    this.spaces = this.spaces.filter(
                      (s) => s.space_id !== space.space_id,
                    );
                    this.searchChanged();
                    return;
                  }

                  existing.shares = space.shares || [];
                  // Don't update name if space is deleting (backend changes name to ID during deletion)
                  if (!space.is_deleting) {
                    existing.name = space.name;
                  }
                  existing.description = space.description;
                  existing.platform = space.platform;
                  existing.note = space.note;
                  existing.zone = space.zone;
                  existing.node_hostname = space.node_hostname;
                  existing.has_code_server = space.has_code_server;
                  existing.has_ssh = space.has_ssh;
                  existing.has_terminal = space.has_terminal;
                  existing.is_deployed = space.is_deployed;
                  existing.is_pending = space.is_pending;
                  existing.is_deleting = space.is_deleting;
                  existing.update_available = space.update_available;
                  existing.healthy = space.healthy;
                  existing.tcp_ports = space.tcp_ports;
                  existing.http_ports = space.http_ports;
                  existing.has_http_vnc = space.has_http_vnc;
                  existing.has_vscode_tunnel = space.has_vscode_tunnel;
                  existing.vscode_tunnel_name = space.vscode_tunnel_name;
                  existing.sshCmd = `ssh -o ProxyCommand='knot forward ssh ${space.name}' -o StrictHostKeyChecking=no ${username}@knot.${space.name}`;
                  existing.is_local = space.zone === "" || zone === space.zone;
                  existing.has_state = space.has_state;
                  existing.started_at = space.started_at;
                  existing.template_name = space.template_name;
                  existing.stack = space.stack || "";
                  existing.uptime = this.formatTimeDiff(space.started_at);

                  if (existing.icon_url !== space.icon_url) {
                    existing.icon_url = space.icon_url;
                    existing.icon_url_exists = this.imageExists(space.icon_url);
                  }
                }
              });

              // If spaces added then sort them by name
              if (spacesAdded) {
                this.spaces.sort((a, b) => a.name.localeCompare(b.name));
              }

              // Only remove spaces when fetching all (not single space)
              if (!spaceId) {
                this.spaces.forEach((space, index) => {
                  if (!spacesList.find((s) => s.space_id === space.space_id)) {
                    this.spaces.splice(index, 1);
                  }
                });
              }

              // Apply search filter
              this.searchChanged();

              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {
          // Don't logout on network errors - Safari closes connections aggressively
        });
    },
    async imageExists(url) {
      if (!url.length) {
        return false;
      }

      try {
        const response = await fetch(url, { method: "HEAD" });
        return response.ok;
      } catch {
        return false;
      }
    },
    async startSpace(spaceId) {
      const self = this;
      await fetch(`/api/spaces/${spaceId}/start`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Space starting",
              type: "success",
            });
          } else if (response.status === 503) {
            response.json().then((data) => {
              if (data.error === "outside of schedule") {
                self.badScheduleShow = true;
              } else {
                self.$dispatch("show-alert", {
                  msg: `Space could not be started: ${data.error}`,
                  type: "error",
                });
              }
            });
          } else if (response.status === 507) {
            response.json().then((data) => {
              // If compute units exceeded then show the dialog
              if (data.error === "compute unit quota exceeded") {
                const space = self.spaces.find((s) => s.space_id === spaceId);

                self.quotaComputeLimit.isShared =
                  self.isSharedWithViewer(space);
                self.quotaComputeLimit.show = true;
              } else if (data.error === "storage unit quota exceeded") {
                const space = self.spaces.find((s) => s.space_id === spaceId);

                self.quotaStorageLimit.isShared =
                  self.isSharedWithViewer(space);
                self.quotaStorageLimit.show = true;
              } else {
                self.$dispatch("show-alert", {
                  msg: "Space could not be as it has exceeded quota limits.",
                  type: "error",
                });
              }
            });
          } else {
            response
              .json()
              .then((data) => {
                self.$dispatch("show-alert", {
                  msg: `Space could not be started: ${data.error}`,
                  type: "error",
                });
              })
              .catch(() => {
                self.$dispatch("show-alert", {
                  msg: `Space could not be started`,
                  type: "error",
                });
              });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Space could not be started: ${error}`,
            type: "error",
          });
        })
        .finally(() => {
          self.getSpaces(spaceId);
        });
    },
    async stopSpace(spaceId) {
      const self = this;
      await fetch(`/api/spaces/${spaceId}/stop`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Space stopping",
              type: "success",
            });
          } else {
            self.$dispatch("show-alert", {
              msg: "Space could not be stopped",
              type: "error",
            });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Space could not be stopped: ${error}`,
            type: "error",
          });
        })
        .finally(() => {
          self.getSpaces();
        });
    },
    async restartSpace(spaceId) {
      const self = this;
      await fetch(`/api/spaces/${spaceId}/restart`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Space restarting",
              type: "success",
            });
          } else {
            self.$dispatch("show-alert", {
              msg: "Space could not be restarted",
              type: "error",
            });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Space could not be restarted: ${error}`,
            type: "error",
          });
        });
    },
    async deleteSpace(spaceId) {
      const self = this;
      await fetch(`/api/spaces/${spaceId}`, {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Space deleting",
              type: "success",
            });
          } else {
            self.$dispatch("show-alert", {
              msg: "Space could not be deleted",
              type: "error",
            });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Space could not be deleted: ${error}`,
            type: "error",
          });
        });
    },
    openCeaseShareConfirm(space, targetUserId = "") {
      if (!targetUserId && space?.user_id !== userId) {
        targetUserId = userId;
      }
      this.ceaseShareConfirm.show = true;
      this.ceaseShareConfirm.space = space;
      this.ceaseShareConfirm.targetUserId = targetUserId;
    },
    async ceaseSharing(spaceId, shareUserId = "") {
      const self = this;
      const query = shareUserId
        ? `?user_id=${encodeURIComponent(shareUserId)}`
        : "";

      await fetch(`/api/spaces/${spaceId}/share${query}`, {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            const leavingOwnShare =
              self.ceaseShareConfirm.space?.user_id !== userId &&
              (!shareUserId || shareUserId === userId);

            if (leavingOwnShare) {
              self.spaces = self.spaces.filter((s) => s.space_id !== spaceId);
              self.searchChanged();
            } else {
              self.getSpaces(spaceId);
            }

            self.$dispatch("show-alert", {
              msg: "Sharing of Space Stopped",
              type: "success",
            });
          } else {
            response
              .json()
              .then((data) => {
                self.$dispatch("show-alert", {
                  msg: `Could not stop sharing of space: ${data.error}`,
                  type: "error",
                });
              })
              .catch(() => {
                self.$dispatch("show-alert", {
                  msg: "Could not stop sharing of space",
                  type: "error",
                });
              });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Could not stop sharing of space: ${error}`,
            type: "error",
          });
        });
    },
    editSpace(spaceId) {
      const space = this.spaces.find((s) => s.space_id === spaceId);
      this.spaceFormModal.isEdit = true;
      this.spaceFormModal.spaceId = spaceId;
      this.spaceFormModal.templateId = space.template_id;
      this.spaceFormModal.forUserId = space.user_id;
      this.spaceFormModal.forUserUsername = space.username;
      this.spaceFormModal.show = true;
    },
    openWindowForPort(spaceUsername, spaceId, spaceName, port) {
      popup.openPortWindow(
        spaceId,
        wildcardDomain,
        spaceUsername === "" ? username : spaceUsername,
        spaceName,
        port,
      );
    },
    openWindowForVNC(spaceUsername, spaceId, spaceName) {
      popup.openVNC(
        spaceId,
        wildcardDomain,
        spaceUsername === "" ? username : spaceUsername,
        spaceName,
      );
    },
    openCodeServer(spaceId) {
      popup.openCodeServer(spaceId, wildcardDomain);
    },
    openTerminalTunnel(spaceId) {
      popup.openTerminalTunnel(spaceId);
    },
    openVSCodeDev(tunnelName) {
      popup.openVSCodeDev(tunnelName);
    },
    openTerminal(spaceId) {
      popup.openTerminal(spaceId);
    },
    openLogWindow(spaceId) {
      popup.openLogWindow(spaceId);
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // Collect stack names that match the search term
      const matchingStacks = term.length
        ? new Set(this.spaces.filter((s) => s.stack && s.stack.toLowerCase().includes(term)).map((s) => s.stack))
        : new Set();

      this.visibleSpaces = 0;
      this.spaces.forEach((space) => {
        const filterHide =
          (this.showLocalOnly && !space.is_local) ||
          (this.showRunningOnly && !space.is_deployed) ||
          (this.showSharedOnly &&
            (!this.hasShare(space) || this.isSharedWithViewer(space))) ||
          (this.showSharedWithMeOnly &&
            (!this.hasShare(space) || !this.isSharedWithViewer(space)));

        const termHide = term.length
          ? !(
              space.name.toLowerCase().includes(term) ||
              space.template_name.toLowerCase().includes(term) ||
              space.zone.toLowerCase().includes(term) ||
              (space.stack && matchingStacks.has(space.stack))
            )
          : false;

        space.searchHide = filterHide || termHide;

        if (!space.searchHide) {
          this.visibleSpaces++;
        }
      });
    },
    async copyToClipboard(text) {
      await navigator.clipboard.writeText(text);
      this.$dispatch("show-alert", {
        msg: "Copied to clipboard",
        type: "success",
      });
    },
    async transferSpaceTo() {
      const self = this;

      if (this.chooseUser.toUserId === "") {
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
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            shares: self.chooseUser.isShare ? [this.chooseUser.toUserId] : undefined,
            user_id: this.chooseUser.toUserId,
          }),
        },
      )
        .then((response) => {
          if (response.status === 200) {
            if (!self.chooseUser.isShare) {
              // Remove the space from the array
              self.spaces = self.spaces.filter(
                (s) => s.space_id !== self.chooseUser.space.space_id,
              );
            }

            if (self.chooseUser.isShare) {
              self.$dispatch("show-alert", {
                msg: "Space shared",
                type: "success",
              });
            } else {
              self.$dispatch("show-alert", {
                msg: "Space transferred",
                type: "success",
              });
            }
            self.chooseUser.show = false;
            self.getSpaces();
          } else if (response.status === 507) {
            if (self.chooseUser.isShare) {
              self.$dispatch("show-alert", {
                msg: "Space could not be shared as the user has exceeded their quota.",
                type: "error",
              });
            } else {
              self.$dispatch("show-alert", {
                msg: "Space could not be transferred as the user has exceeded their quota.",
                type: "error",
              });
            }
          } else if (response.status === 403) {
            if (self.chooseUser.isShare) {
              self.$dispatch("show-alert", {
                msg: "Space could not be shared as the user is not allowed to use the template.",
                type: "error",
              });
            } else {
              self.$dispatch("show-alert", {
                msg: "Space could not be transferred as the user is not allowed to use the template.",
                type: "error",
              });
            }
          } else {
            response.json().then((data) => {
              if (self.chooseUser.isShare) {
                self.$dispatch("show-alert", {
                  msg: `Space could not be shared: ${data.error}`,
                  type: "error",
                });
              } else {
                self.$dispatch("show-alert", {
                  msg: `Space could not be transferred: ${data.error}`,
                  type: "error",
                });
              }
            });
          }
        })
        .catch((error) => {
          if (self.chooseUser.isShare) {
            self.$dispatch("show-alert", {
              msg: `Space could not be shared: ${error}`,
              type: "error",
            });
          } else {
            self.$dispatch("show-alert", {
              msg: `Space could not be transferred: ${error}`,
              type: "error",
            });
          }
        });
    },

    toggleStack(stackName) {
      this.collapsedStacks[stackName] = !this.collapsedStacks[stackName];
    },
    async _stackAction(stackName, action) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 10 * 60 * 1000);
      try {
        while (true) {
          try {
            const res = await fetch(`/api/spaces/stacks/${encodeURIComponent(stackName)}/${action}`, {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              signal: controller.signal,
            });
            if (res.status === 202) {
              return null;
            }
            const data = await res.json().catch(() => ({}));
            return data.error || `Stack could not be ${action}ed`;
          } catch (e) {
            if (e.name === "AbortError") {
              return `Stack ${action} timed out`;
            }
            await new Promise((resolve) => setTimeout(resolve, 2000));
          }
        }
      } finally {
        clearTimeout(timer);
      }
    },
    async startStack(stackName) {
      this.stackBusy[stackName] = true;
      try {
        const err = await this._stackAction(stackName, "start");
        if (err) {
          this.$dispatch("show-alert", { msg: err, type: "error" });
        } else {
          this.$dispatch("show-alert", { msg: `Stack "${stackName}" started`, type: "success" });
        }
      } finally {
        this.stackBusy[stackName] = false;
      }
    },
    async stopStack(stackName) {
      this.stackBusy[stackName] = true;
      try {
        const err = await this._stackAction(stackName, "stop");
        if (err) {
          this.$dispatch("show-alert", { msg: err, type: "error" });
        } else {
          this.$dispatch("show-alert", { msg: `Stack "${stackName}" stopped`, type: "success" });
        }
      } finally {
        this.stackBusy[stackName] = false;
      }
    },
    async restartStack(stackName) {
      this.stackBusy[stackName] = true;
      try {
        const err = await this._stackAction(stackName, "restart");
        if (err) {
          this.$dispatch("show-alert", { msg: err, type: "error" });
        } else {
          this.$dispatch("show-alert", { msg: `Stack "${stackName}" restarted`, type: "success" });
        }
      } finally {
        this.stackBusy[stackName] = false;
      }
    },
    hasStacks() {
      return this.spaces.some((s) => s.stack && !s.searchHide);
    },
    unstackedVisibleSpaces() {
      return this.spaces.filter((s) => !s.stack && !s.searchHide);
    },
    stackedGroups() {
      const groups = new Map();
      for (const space of this.spaces) {
        if (!space.stack || space.searchHide) continue;
        if (!groups.has(space.stack)) {
          groups.set(space.stack, []);
        }
        groups.get(space.stack).push(space);
      }

      return [...groups.entries()]
        .sort((a, b) => a[0].localeCompare(b[0]))
        .map(([name, spaces]) => ({
          name,
          spaces,
          count: spaces.length,
        }));
    },
    formatTimeDiff(utcTime) {
      // Convert input to Date if not already
      const givenTime = utcTime instanceof Date ? utcTime : new Date(utcTime);
      const currentTime = new Date();

      // Calculate difference in seconds
      const diffSeconds = Math.abs(
        Math.floor((currentTime - givenTime) / 1000),
      );

      // Format based on magnitude
      if (diffSeconds < 60) {
        // Less than a minute: show "<1m"
        return "<1m";
      } else if (diffSeconds < 3600) {
        // Less than an hour: show minutes
        const minutes = Math.floor(diffSeconds / 60);
        return `${minutes}m`;
      } else if (diffSeconds < 86400) {
        // Less than a day: show hours
        const hours = Math.floor(diffSeconds / 3600);
        return `${hours}h`;
      } else {
        // More than a day: show days
        const days = Math.floor(diffSeconds / 86400);
        return `${days}d`;
      }
    },
    isLocalContainer(platform) {
      return platform === "docker";
    },
    doAction(space_id) {
      if (this.action === "stop") {
        this.stopSpace(space_id);
      } else {
        this.restartSpace(space_id);
      }
      this.is_pending = true;
    },
    setAction(act) {
      this.action = act;
    },
    async openTemplateSelector() {
      this.templateSelector.show = true;
      await this.getTemplatesForSelector();
      // Focus the search input after modal transition
      this.$nextTick(() => {
        this.$refs.templateSearchInput?.focus();
      });
    },
    async getTemplatesForSelector() {
      // Fetch groups
      await fetch("/api/groups", {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((groupsList) => {
              this.templateSelector.groups = groupsList.groups;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
            return;
          }
        })
        .catch(() => {
          // Don't logout on network errors
          return;
        });

      // Fetch templates
      await fetch("/api/templates", {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((templateList) => {
              this.templateSelector.templates = templateList.templates;

              this.templateSelector.templates.forEach((template) => {
                template.icon_url_exists = this.imageExists(template.icon_url);

                // Convert group IDs to names
                template.group_names = [];
                template.groups.forEach((groupId) => {
                  this.templateSelector.groups.forEach((group) => {
                    if (group.group_id === groupId) {
                      template.group_names.push(group.name);
                    }
                  });
                });
              });

              // Apply search filter
              this.templateSearchChanged();
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {
          // Don't logout on network errors
        });
    },
    templateSearchChanged() {
      const term = this.templateSelector.searchTerm.toLowerCase();

      this.templateSelector.templates.forEach((template) => {
        // Only show active templates
        let showRow = template.active;

        const zones = template.zones || [];
        if (zones.length > 0) {
          // Hide if any !zone matches the current zone
          const hasNegation = zones.some(
            (z) => z.startsWith("!") && z.substring(1) === zone,
          );
          if (hasNegation) {
            showRow = false;
          } else {
            // If there are any non-negated zones, show only if one matches
            const positiveZones = zones.filter((z) => !z.startsWith("!"));
            if (positiveZones.length > 0) {
              const hasZone = positiveZones.includes(zone);
              showRow = showRow && hasZone;
            }
          }
        }

        // Search term filtering
        if (term.length > 0) {
          const inName = template.name.toLowerCase().includes(term);
          const inDesc = template.description.toLowerCase().includes(term);
          showRow = showRow && (inName || inDesc);
        }

        template.searchHide = !showRow;
      });
    },
    createSpaceFromTemplate(templateId) {
      this.templateSelector.show = false;
      this.spaceFormModal.isEdit = false;
      this.spaceFormModal.spaceId = "";
      this.spaceFormModal.templateId = templateId;
      this.spaceFormModal.forUserId = this.forUserId;
      this.spaceFormModal.forUserUsername = this.forUsername;
      this.spaceFormModal.show = true;
    },
    getDayOfWeek(day) {
      return ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"][day];
    },
    getMaxUptime(maxUptime, maxUptimeUnit) {
      let maxUptimeString = "";
      if (maxUptimeUnit === "minute") {
        maxUptimeString = `${maxUptime} minute${maxUptime > 1 ? "s" : ""}`;
      } else if (maxUptimeUnit === "hour") {
        maxUptimeString = `${maxUptime} hour${maxUptime > 1 ? "s" : ""}`;
      } else if (maxUptimeUnit === "day") {
        maxUptimeString = `${maxUptime} day${maxUptime > 1 ? "s" : ""}`;
      }
      return maxUptimeString;
    },
  };
};
