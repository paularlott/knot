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
    groups: [],
    searchTerm: Alpine.$persist('').as('group-search-term').using(sessionStorage),

    async init() {
      await this.getGroups();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getGroups();
      }, 3000);
    },

    async getGroups() {
      const response = await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      const groupList = await response.json();
      this.groups = groupList.groups;

      // Apply search filter
      this.searchChanged();

      this.loading = false;
      this.groups.forEach(group => {
        group.showIdPopup = false;
      });
    },
    editGroup(groupId) {
      window.location.href = `/groups/edit/${groupId}`;
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
        } else {
          self.$dispatch('show-alert', { msg: "Group could not be deleted", type: 'error' });
        }
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
