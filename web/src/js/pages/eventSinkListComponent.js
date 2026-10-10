import { sortable } from '../components/sortable.js';
import Alpine from "alpinejs";

window.eventSinkListComponent = function (userId, permissionManageEvents, permissionManageGlobalEvents, isLeafNode) {
  const canAccessOwn = permissionManageEvents || isLeafNode || false;
  const canAccessGlobal = permissionManageGlobalEvents || isLeafNode || false;
  const defaultShowMySinks = canAccessOwn;
  const defaultShowGlobalSinks = canAccessGlobal;

  return {
    ...sortable('event-sinks', {
      name: (s) => s.name,
      owner: (s) => (s.user_id ? 'User' : 'Global'),
      type: (s) => s.sink_type,
      status: (s) => (s.active ? 0 : 1),
    }),
    loading: true,
    confirmDelete(s) {
      window.knotConfirm({
        danger: true,
        message: "Are you sure you want to delete the event sink {name}?",
        name: s.name,
        confirmLabel: "Delete Sink",
        cancelLabel: "Keep Sink",
      }).then((ok) => ok && this.deleteSink(s.event_sink_id));
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
    canAccessOwn: canAccessOwn,
    canAccessGlobal: canAccessGlobal,

    async init() {
      await this.getEventSinks();

      const editId = new URLSearchParams(location.search).get('edit');
      if (editId) {
        const u = new URL(location.href);
        u.searchParams.delete('edit');
        history.replaceState(null, '', u.toString());
        this.editSink(editId);
      }

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
              const sinkList = (sinkId ? [data] : data.event_sinks).filter(
                (sink) => (!sink.user_id ? this.canAccessGlobal : sink.user_id === this.currentUserId && this.canAccessOwn),
              );
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
            window.knotError("delete the event sink", response);
          }
        })
        .catch((err) => window.knotError("delete the event sink", err));
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

    filtersActive() {
      return (this.canAccessOwn && !this.showMySinks) || (this.canAccessGlobal && !this.showGlobalSinks);
    },

    clearFilters() {
      this.searchTerm = "";
      this.showMySinks = this.canAccessOwn;
      this.showGlobalSinks = this.canAccessGlobal;
      this.applyFilters();
      document.getElementById("search")?.focus();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.sinks.forEach((s) => {
        let showRow = true;

        // Filter by owner - show if it matches any enabled filter
        const isGlobal = !s.user_id;
        const isMine = s.user_id === this.currentUserId;
        const matchesFilter =
          (isGlobal && this.canAccessGlobal && this.showGlobalSinks) ||
          (isMine && this.canAccessOwn && this.showMySinks);
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
