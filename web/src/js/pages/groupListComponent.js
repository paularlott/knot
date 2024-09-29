window.groupListComponent = function() {
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

    async getGroups() {
      this.loading = true;
      const response = await fetch('/api/v1/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      groupList = await response.json();
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
      var self = this;
      await fetch(`/api/v1/groups/${groupId}`, {
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
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all groups if name contains the term show; else hide
      this.groups.forEach(g => {
        if(term.length == 0) {
          g.searchHide = false;
        } else {
          g.searchHide = !g.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
