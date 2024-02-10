window.templateListComponent = function() {
  return {
    loading: true,
    deleteConfirm: {
      show: false,
      template: {
        template_id: '',
        name: '',
      }
    },
    templates: [],
    groups: [],
    async getTemplates() {
      this.loading = true;

      const groupsResponse = await fetch('/api/v1/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.groups = await groupsResponse.json();

      const response = await fetch('/api/v1/templates', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.templates = await response.json();

      this.templates.forEach(template => {
        template.showIdPopup = false;

        // Convert group IDs to names
        template.group_names = [];
        template.groups.forEach(groupId => {
          this.groups.forEach(group => {
            if (group.group_id === groupId) {
              template.group_names.push(group.name);
            }
          });
        });
      });

      this.loading = false;
    },
    editTemplate(templateId) {
      window.location.href = `/templates/edit/${templateId}`;
    },
    async deleteTemplate(templateId) {
      var self = this;
      await fetch(`/api/v1/templates/${templateId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Template deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Template could not be deleted", type: 'error' });
        }
      });
      this.getTemplates();
    },
  };
}
