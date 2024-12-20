window.templateListComponent = function(canManageSpaces) {
  return {
    loading: true,
    deleteConfirm: {
      show: false,
      template: {
        template_id: '',
        name: '',
      }
    },
    chooseUser: {
      forUserId: '',
      invalidUser: false,
      invalidTemplate: false,
      show: false,
      template: {
        template_id: '',
        name: '',
      }
    },
    templates: [],
    groups: [],
    canManageSpaces: canManageSpaces,
    users: [],
    searchTerm: Alpine.$persist('').as('template-search-term').using(sessionStorage),

    async init() {
      this.getTemplates();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getTemplates();
      }, 5000);
    },

    async getTemplates() {
      if(this.canManageSpaces) {
        const usersResponse = await fetch('/api/v1/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        let usersList = await usersResponse.json();
        this.users = usersList.users;
      }

      const groupsResponse = await fetch('/api/v1/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      groupsList = await groupsResponse.json();
      this.groups = groupsList.groups;

      const response = await fetch('/api/v1/templates', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      templateList = await response.json();
      this.templates = templateList.templates;

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

      // Apply search filter
      this.searchChanged();

      this.loading = false;
    },
    editTemplate(templateId) {
      window.location.href = `/templates/edit/${templateId}`;
    },
    createSpaceFromTemplate(templateId) {
      window.location.href = `/spaces/create/${templateId}`;
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
    async createSpaceAs() {
      if(this.chooseUser.forUserId == '') {
        this.chooseUser.invalidUser = true;
        return;
      }

      this.chooseUser.invalidUser = this.chooseUser.invalidTemplate = false;

      // Get the list of templates
      await fetch('/api/v1/templates?user_id=' + this.chooseUser.forUserId, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((templates) => {

            // If the selected template is in the list then created
            var template = templates.templates.find(template => template.template_id === this.chooseUser.template.template_id);
            if(template) {
              this.chooseUser.show = false;
              window.location.href = `/spaces/create/${this.chooseUser.template.template_id}/${this.chooseUser.forUserId}`;
            } else {
              this.chooseUser.invalidTemplate = true;
              return;
            }
          });
        }
      });
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all templates if name or description contains the term show; else hide
      this.templates.forEach(template => {
        if(term.length == 0) {
          template.searchHide = false;
        } else {
          template.searchHide = !(
            template.name.toLowerCase().includes(term) ||
            template.description.toLowerCase().includes(term)
          );
        }
      });
    },
  };
}
