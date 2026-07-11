import Alpine from "alpinejs";

window.commandListComponent = function (userId, zone, permissionManageCommands, permissionManageOwnCommands, isLeafNode) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search")?.focus();
    }
  });

  const canAccessOwn = permissionManageOwnCommands || isLeafNode || false;
  const canAccessGlobal = permissionManageCommands || isLeafNode || false;
  const defaultShowMy = canAccessOwn;
  const defaultShowGlobal = canAccessGlobal;

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      command: {
        command_id: "",
        name: "",
      },
    },
    commandFormModal: {
      show: false,
      isEdit: false,
      commandId: "",
      isUserCommand: false,
    },
    commands: [],
    availableZones: [],
    showMyCommands: Alpine.$persist(defaultShowMy).as("cmd-show-my").using(sessionStorage),
    showGlobalCommands: Alpine.$persist(defaultShowGlobal).as("cmd-show-global").using(sessionStorage),
    showLocalCommands: Alpine.$persist(true).as("cmd-show-local").using(sessionStorage),
    showAllZones: Alpine.$persist(false).as("cmd-show-all-zones").using(sessionStorage),
    searchTerm: Alpine.$persist("").as("cmd-search-term").using(sessionStorage),
    currentUserId: userId || "",
    currentZone: zone || "",
    permissionManageCommands: permissionManageCommands || false,
    permissionManageOwnCommands: permissionManageOwnCommands || false,
    canAccessOwn,
    canAccessGlobal,
    isLeafNode: isLeafNode || false,

    async init() {
      await this.getCommands();
      window.sseClient?.subscribe('slashcommands:changed', () => this.getCommands(null, true));
      window.sseClient?.subscribe('slashcommands:deleted', () => this.getCommands(null, true));
    },

    async getCommands(commandId = null, silent = false) {
      if (!silent) this.loading = true;

      const params = new URLSearchParams();
      if (this.showAllZones) params.set("all_zones", "true");

      await fetch(`/api/command?${params.toString()}`, { headers: { "Content-Type": "application/json" } })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              const commandList = data.commands || [];
              commandList.forEach((cmd) => {
                const index = this.commands.findIndex((c) => c.command_id === cmd.command_id);
                if (index >= 0) this.commands[index] = cmd;
                else this.commands.push(cmd);
                if (cmd.zones && cmd.zones.length) {
                  cmd.zones.forEach((z) => { if (!this.availableZones.includes(z)) this.availableZones.push(z); });
                }
              });
              this.commands.sort((a, b) => a.name.localeCompare(b.name));
              this.availableZones.sort();
              this.applyFilters();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {});

      if (this.canAccessOwn && this.currentUserId && !commandId) {
        const p = new URLSearchParams(params);
        p.set("user_id", this.currentUserId);
        await fetch(`/api/command?${p.toString()}`, { headers: { "Content-Type": "application/json" } })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                (data.commands || []).forEach((cmd) => {
                  const index = this.commands.findIndex((c) => c.command_id === cmd.command_id);
                  if (index >= 0) this.commands[index] = cmd;
                  else this.commands.push(cmd);
                  if (cmd.zones && cmd.zones.length) {
                    cmd.zones.forEach((z) => { if (!this.availableZones.includes(z)) this.availableZones.push(z); });
                  }
                });
                this.commands.sort((a, b) => a.name.localeCompare(b.name));
                this.availableZones.sort();
                this.applyFilters();
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
            this.loading = false;
          })
          .catch(() => { this.loading = false; });
      } else {
        this.loading = false;
      }
    },

    createCommand(isUserCommand = false) {
      this.commandFormModal.isEdit = false;
      this.commandFormModal.commandId = "";
      this.commandFormModal.isUserCommand = isUserCommand;
      this.commandFormModal.show = true;
      if (isUserCommand) this.showMyCommands = true;
      else this.showGlobalCommands = true;
    },

    editCommand(commandId) {
      const cmd = this.commands.find((c) => c.command_id === commandId);
      this.commandFormModal.isEdit = true;
      this.commandFormModal.commandId = commandId;
      this.commandFormModal.isUserCommand = cmd && cmd.user_id ? true : false;
      this.commandFormModal.show = true;
    },

    canEditCommand(cmd) {
      if (this.isLeafNode) return true;
      if (cmd.user_id && cmd.user_id === this.currentUserId) return true;
      if (!cmd.user_id) return this.permissionManageCommands;
      return false;
    },

    canActuallyEditCommand(cmd) {
      if (cmd.is_managed) return false;
      if (cmd.user_id && cmd.user_id === this.currentUserId) return true;
      if (!cmd.user_id) return this.permissionManageCommands;
      return false;
    },

    canDeleteCommand(cmd) {
      if (this.isLeafNode) return false;
      if (cmd.user_id && cmd.user_id === this.currentUserId) return true;
      if (!cmd.user_id) return this.permissionManageCommands;
      return false;
    },

    async deleteCommand(commandId) {
      await fetch(`/api/command/${commandId}`, { method: "DELETE", headers: { "Content-Type": "application/json" } })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", { msg: "Command deleted", type: "success" });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.$dispatch("show-alert", { msg: "Command could not be deleted", type: "error" });
          }
        })
        .catch(() => {});
      this.getCommands();
    },

    filterChanged() {
      this.$nextTick(() => this.applyFilters());
    },

    showAllZonesChanged() {
      this.getCommands();
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.commands.forEach((c) => {
        let showRow = true;
        const isGlobal = !c.user_id;
        const isMine = c.user_id === this.currentUserId;
        const matchesFilter = (isGlobal && this.canAccessGlobal && this.showGlobalCommands) || (isMine && this.canAccessOwn && this.showMyCommands);
        if (!matchesFilter) showRow = false;
        if (this.isLeafNode && this.showLocalCommands && c.is_managed) showRow = false;
        if (!this.showAllZones && this.currentZone) {
          const zones = c.zones || [];
          if (zones.length > 0 && !zones.includes(this.currentZone)) showRow = false;
        }
        if (term.length > 0) {
          const inName = (c.name || "").toLowerCase().includes(term);
          const inDesc = (c.description || "").toLowerCase().includes(term);
          showRow = showRow && (inName || inDesc);
        }
        c.searchHide = !showRow;
      });
    },
  };
};
