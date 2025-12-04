import Alpine from 'alpinejs';

window.groupListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      group: {
        group_id: '',
        name: '',
      }
    },
    groupFormModal: {
      show: false,
      isEdit: false,
      groupId: '',
    },
    groups: [],
    searchTerm: Alpine.$persist('').as('group-search-term').using(sessionStorage),

    async init() {
      await this.getGroups();

      // Subscribe to SSE for real-time updates
      if (window.sseClient) {
        window.sseClient.subscribe('groups:changed', (payload) => {
          if (payload?.id) this.getGroups(payload.id);
        });

        window.sseClient.subscribe('groups:deleted', (payload) => {
          this.groups = this.groups.filter(g => g.group_id !== payload?.id);
          this.searchChanged();
        });
      }
    },

    async getGroups(groupId) {
      const url = groupId ? `/api/groups/${groupId}` : '/api/groups';
      await fetch(url, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((data) => {
            const groupList = groupId ? [data] : data.groups;

            groupList.forEach(group => {
              group.showIdPopup = false;
              const index = this.groups.findIndex(g => g.group_id === group.group_id);
              if (index >= 0) {
                this.groups[index] = group;
              } else {
                this.groups.push(group);
              }
            });

            this.groups.sort((a, b) => {
              return a.name.localeCompare(b.name);
            });

            // Apply search filter
            this.searchChanged();

            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    createGroup() {
      this.groupFormModal.isEdit = false;
      this.groupFormModal.groupId = '';
      this.groupFormModal.show = true;
    },
    editGroup(groupId) {
      this.groupFormModal.isEdit = true;
      this.groupFormModal.groupId = groupId;
      this.groupFormModal.show = true;
    },
    loadGroups() {
      this.getGroups();
    },
    async deleteGroup(groupId) {
      const self = this;
      await fetch(`/api/groups/${groupId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Group deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Group could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
      this.getGroups();
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all groups if name contains the term show; else hide
      this.groups.forEach(g => {
        if(term.length === 0) {
          g.searchHide = false;
        } else {
          g.searchHide = !g.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
