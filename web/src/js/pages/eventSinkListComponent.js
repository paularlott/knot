import Alpine from "alpinejs";

window.eventSinkListComponent = function (userId, permissionManageEvents, permissionManageGlobalEvents, isLeafNode) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  // Default filters: My Sinks ON, Global Sinks OFF (same for leaf and normal mode)
  const defaultShowMySinks = true;
  const defaultShowGlobalSinks = false;

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      sink: {
        event_sink_id: "",
        name: "",
      },
    },
    sinkFormModal: {
      show: false,
      isEdit: false,
      sinkId: "",
      isGlobal: false,
    },
    sinks: [],
    showMySinks: Alpine.$persist(defaultShowMySinks)
      .as("eventsink-show-my-sinks")
      .using(sessionStorage),
    showGlobalSinks: Alpine.$persist(defaultShowGlobalSinks)
      .as("eventsink-show-global-sinks")
      .using(sessionStorage),
    searchTerm: Alpine.$persist("")
      .as("eventsink-search-term")
      .using(sessionStorage),
    currentUserId: userId || "",
    permissionManageEvents: permissionManageEvents || false,
    permissionManageGlobalEvents: permissionManageGlobalEvents || false,
    isLeafNode: isLeafNode || false,

    async init() {
      await this.getEventSinks();

      if (window.sseClient) {
        window.sseClient.subscribe("eventsinks:changed", (payload) => {
          this.getEventSinks(payload?.id);
        });

        window.sseClient.subscribe("eventsinks:deleted", (payload) => {
          this.sinks = this.sinks.filter(
            (x) => x.event_sink_id !== payload?.id,
          );
          this.applyFilters();
        });

        window.sseClient.subscribe("reconnected", () => {
          this.getEventSinks();
        });
      }
    },

    async getEventSinks(sinkId) {
      const url = sinkId
        ? `/api/event-sinks/${sinkId}`
        : `/api/event-sinks`;
      await fetch(url, {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              const sinkList = sinkId ? [data] : data.event_sinks;
              sinkList.forEach((sink) => {
                const index = this.sinks.findIndex(
                  (s) => s.event_sink_id === sink.event_sink_id,
                );
                if (index >= 0) {
                  this.sinks[index] = sink;
                } else {
                  this.sinks.push(sink);
                }
              });

              this.sinks.sort((a, b) => a.name.localeCompare(b.name));
              this.applyFilters();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {});

      this.loading = false;
    },

    createSink(isGlobal = false) {
      this.sinkFormModal.isEdit = false;
      this.sinkFormModal.sinkId = "";
      this.sinkFormModal.isGlobal = isGlobal;
      this.sinkFormModal.show = true;

      // Ensure the relevant filter is enabled so the new sink will be visible
      if (isGlobal) {
        this.showGlobalSinks = true;
      } else {
        this.showMySinks = true;
      }
    },

    editSink(sinkId) {
      const sink = this.sinks.find((s) => s.event_sink_id === sinkId);
      this.sinkFormModal.isEdit = true;
      this.sinkFormModal.sinkId = sinkId;
      this.sinkFormModal.isGlobal =
        sink && !sink.user_id ? true : false;
      this.sinkFormModal.show = true;
    },

    canEditSink(sink) {
      // Global sinks require Manage Global Events permission
      if (!sink.user_id) {
        return this.permissionManageGlobalEvents || this.isLeafNode;
      }
      // Own sinks require Manage Events permission
      if (sink.user_id === this.currentUserId) {
        return this.permissionManageEvents || this.isLeafNode;
      }
      return false;
    },

    canDeleteSink(sink) {
      // In leaf mode, sinks are managed by parent - can't delete
      if (this.isLeafNode) return false;

      if (!sink.user_id) return this.permissionManageGlobalEvents;
      if (sink.user_id === this.currentUserId) return this.permissionManageEvents;
      return false;
    },

    async deleteSink(sinkId) {
      await fetch(`/api/event-sinks/${sinkId}`, {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Event sink deleted",
              type: "success",
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.$dispatch("show-alert", {
              msg: "Event sink could not be deleted",
              type: "error",
            });
          }
        })
        .catch(() => {});
      this.getEventSinks();
    },

    filterChanged() {
      this.$nextTick(() => {
        this.applyFilters();
      });
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.sinks.forEach((s) => {
        let showRow = true;

        // Filter by owner - show if it matches any enabled filter
        const isGlobal = !s.user_id;
        const isMine = s.user_id === this.currentUserId;
        const matchesFilter =
          (isGlobal && this.showGlobalSinks) ||
          (isMine && this.showMySinks);
        if (!matchesFilter) showRow = false;

        // Search term filtering
        if (term.length > 0) {
          const inName = s.name.toLowerCase().includes(term);
          const inDesc = s.description.toLowerCase().includes(term);
          const inEvents = (s.events || []).some((e) =>
            e.toLowerCase().includes(term),
          );
          showRow = showRow && (inName || inDesc || inEvents);
        }

        s.searchHide = !showRow;
      });
    },
  };
};
