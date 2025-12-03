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

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('groups:changed', () => {
          this.getGroups();
        });
      }
    },

    async getGroups() {
      await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((groupList) => {
            this.groups = groupList.groups;

            // Apply search filter
            this.searchChanged();

            this.loading = false;
            this.groups.forEach(group => {
              group.showIdPopup = false;
            });
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
