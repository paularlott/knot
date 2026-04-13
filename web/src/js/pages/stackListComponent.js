import Alpine from "alpinejs";

window.stackListComponent = function (userId, zone, permissionManageStackDefinitions, isLeafNode, permissionUseSpaces) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  const defaultShowMyDefs = true;
  const defaultShowGlobalDefs = false;

  return {
    loading: true,
    definitions: [],
    showMyDefs: Alpine.$persist(defaultShowMyDefs)
      .as("stack-show-my-defs")
      .using(sessionStorage),
    showGlobalDefs: Alpine.$persist(defaultShowGlobalDefs)
      .as("stack-show-global-defs")
      .using(sessionStorage),
    showAllZones: Alpine.$persist(false)
      .as("stack-show-all-zones")
      .using(sessionStorage),
    searchTerm: Alpine.$persist("")
      .as("stack-search-term")
      .using(sessionStorage),

    // Modals
    deleteDefConfirm: { show: false, def: {} },
    createStackModal: { show: false, def: null, prefix: "", name: "", error: "", creating: false },

    // Context
    currentUserId: userId || "",
    currentZone: zone || "",
    permissionManageStackDefinitions: permissionManageStackDefinitions || false,
    isLeafNode: isLeafNode || false,
    permissionUseSpaces: permissionUseSpaces || false,

    async init() {
      await this.getDefinitions();

      if (window.sseClient) {
        window.sseClient.subscribe("stack-definitions:changed", (payload) => {
          if (payload?.id) this.getDefinitions(payload.id);
        });

        window.sseClient.subscribe("stack-definitions:deleted", (payload) => {
          this.definitions = this.definitions.filter(
            (d) => d.stack_definition_id !== payload?.id,
          );
          this.applyFilters();
        });

        window.sseClient.subscribe("reconnected", () => {
          this.getDefinitions();
        });
      }
    },

    async getDefinitions(defId) {
      const url = defId
        ? `/api/stack-definitions/${defId}`
        : `/api/stack-definitions?all_zones=${this.showAllZones}`;

      const groupsResponse = await fetch("/api/groups");
      const groupsData =
        groupsResponse.status === 200
          ? await groupsResponse.json()
          : { groups: [] };
      const groupsMap = {};
      groupsData.groups.forEach((g) => (groupsMap[g.group_id] = g.name));

      await fetch(url, {
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              const list = defId ? [data] : data.stack_definitions || [];
              list.forEach((def) => {
                def.group_names = (def.groups || []).map(
                  (gid) => groupsMap[gid] || gid,
                );
                const index = this.definitions.findIndex(
                  (d) => d.stack_definition_id === def.stack_definition_id,
                );
                if (index >= 0) {
                  this.definitions[index] = def;
                } else {
                  this.definitions.push(def);
                }
              });

              this.definitions.sort((a, b) => a.name.localeCompare(b.name));
              this.applyFilters();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {});

      // Fetch user definitions
      if (this.currentUserId && !defId) {
        await fetch(
          `/api/stack-definitions?user_id=${this.currentUserId}&all_zones=${this.showAllZones}`,
          { headers: { "Content-Type": "application/json" } },
        )
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                const list = data.stack_definitions || [];
                list.forEach((def) => {
                  def.group_names = (def.groups || []).map(
                    (gid) => groupsMap[gid] || gid,
                  );
                  const index = this.definitions.findIndex(
                    (d) => d.stack_definition_id === def.stack_definition_id,
                  );
                  if (index >= 0) {
                    this.definitions[index] = def;
                  } else {
                    this.definitions.push(def);
                  }
                });
                this.definitions.sort((a, b) => a.name.localeCompare(b.name));
                this.applyFilters();
              });
            }
            this.loading = false;
          })
          .catch(() => {
            this.loading = false;
          });
      } else {
        this.loading = false;
      }
    },

    canUseDef(def) {
      if (!this.permissionUseSpaces) return false;
      if (!def.active) return false;
      return true;
    },

    canDeleteDef(def) {
      if (this.isLeafNode) return false;
      if (def.user_id && def.user_id === this.currentUserId) return true;
      if (!def.user_id) return this.permissionManageStackDefinitions;
      return false;
    },

    openCreateStack(def) {
      this.createStackModal = {
        show: true,
        def: def,
        prefix: "",
        name: "",
        error: "",
        creating: false,
      };
    },

    async createStackFromDef() {
      const modal = this.createStackModal;
      if (!modal.prefix || !modal.def) return;

      const stackName = modal.name || modal.prefix;
      const prefix = modal.prefix;
      const spaces = modal.def.spaces || [];

      if (spaces.length === 0) {
        modal.error = "This definition has no spaces.";
        return;
      }

      modal.creating = true;
      modal.error = "";

      const created = [];

      try {
        // Pass 1: Create all spaces
        for (const comp of spaces) {
          const spaceName = prefix + "-" + comp.name;
          const body = {
            name: spaceName,
            template_id: comp.template_id,
            stack: stackName,
            description: comp.description || "",
            shell: comp.shell || "",
            custom_fields: (comp.custom_fields || []).map((cf) => ({
              name: cf.name,
              value: cf.value,
            })),
          };
          if (comp.startup_script_id) {
            body.startup_script_id = comp.startup_script_id;
          }

          const res = await fetch("/api/spaces", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
          });

          if (res.status === 401) {
            window.location.href = "/logout";
            return;
          }
          if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            throw new Error(
              `Failed to create space "${spaceName}": ${err.error || res.statusText}`,
            );
          }

          const data = await res.json();
          created.push({ key: comp.name, id: data.space_id, comp });
        }

        // Build key-to-ID map
        const keyToID = {};
        for (const s of created) {
          keyToID[s.key] = s.id;
        }

        // Pass 2: Set dependencies
        for (const s of created) {
          if (!s.comp.depends_on || s.comp.depends_on.length === 0) continue;
          const depIDs = s.comp.depends_on
            .map((k) => keyToID[k])
            .filter(Boolean);
          if (depIDs.length > 0) {
            const spaceName = prefix + "-" + s.key;
            await fetch(`/api/spaces/${s.id}`, {
              method: "PUT",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                name: spaceName,
                stack: stackName,
                depends_on: depIDs,
              }),
            });
          }
        }

        // Pass 3: Apply port forwards
        for (const s of created) {
          if (!s.comp.port_forwards || s.comp.port_forwards.length === 0)
            continue;
          const forwards = [];
          for (const pf of s.comp.port_forwards) {
            if (!keyToID[pf.to_space]) continue;
            forwards.push({
              local_port: pf.local_port,
              space: prefix + "-" + pf.to_space,
              remote_port: pf.remote_port,
              persistent: true,
            });
          }
          if (forwards.length > 0) {
            await fetch(`/space-io/${s.id}/port/apply`, {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ forwards }),
            });
          }
        }

        modal.show = false;
        this.$dispatch("show-alert", {
          msg: `Stack "${stackName}" created with ${created.length} space(s)`,
          type: "success",
        });
      } catch (err) {
        // Cleanup created spaces on failure
        for (const s of created) {
          await fetch(`/api/spaces/${s.id}`, { method: "DELETE" }).catch(
            () => {},
          );
        }
        modal.error = err.message;
      } finally {
        modal.creating = false;
      }
    },

    async deleteDefinition(defId) {
      await fetch(`/api/stack-definitions/${defId}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Stack definition deleted",
              type: "success",
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.$dispatch("show-alert", {
              msg: "Stack definition could not be deleted",
              type: "error",
            });
          }
        })
        .catch(() => {});
      this.getDefinitions();
    },

    showAllZonesChanged() {
      this.getDefinitions();
    },

    filterChanged() {
      this.$nextTick(() => this.applyFilters());
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.definitions.forEach((d) => {
        let showRow = true;

        const isGlobal = !d.user_id;
        const isMine = d.user_id === this.currentUserId;
        const matchesFilter =
          (isGlobal && this.showGlobalDefs) || (isMine && this.showMyDefs);
        if (!matchesFilter) showRow = false;

        if (!this.showAllZones && this.currentZone) {
          const zones = d.zones || [];
          if (zones.length > 0 && !zones.includes(this.currentZone))
            showRow = false;
        }

        if (term.length > 0) {
          const haystack = (
            d.name +
            " " +
            d.description +
            " " +
            (d.group_names || []).join(" ")
          ).toLowerCase();
          if (!haystack.includes(term)) showRow = false;
        }

        d.searchHide = !showRow;
      });
    },
  };
};
