import Alpine from 'alpinejs';

window.templateListComponent = function(canManageSpaces, zone) {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    showAll: Alpine.$persist(false).as('templates-show-all').using(sessionStorage),
    showInactive: Alpine.$persist(false).as('templates-show-inactive').using(sessionStorage),
    zone,
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
    canManageSpaces,
    users: [],
    searchTerm: Alpine.$persist('').as('template-search-term').using(sessionStorage),

    async init() {
      await this.getTemplates();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getTemplates();
      }, 3000);
    },

    async getTemplates() {
      if(this.canManageSpaces) {
        await fetch('/api/users?state=active', {
          headers: {
            'Content-Type': 'application/json'
          }
        }).then((response) => {
          if (response.status === 200) {
            response.json().then((usersList) => {
              this.users = usersList.users;
            });
          } else if (response.status === 401) {
            window.location.href = '/logout';
            return;
          }
        }).catch(() => {
          window.location.href = '/logout';
          return;
        });
      }

      await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((groupsList) => {
            this.groups = groupsList.groups;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
          return;
        }
      }).catch(() => {
        window.location.href = '/logout';
        return;
      });

      await fetch('/api/templates', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((templateList) => {
            this.templates = templateList.templates;

            this.templates.forEach(template => {
              template.showIdPopup = false;
              template.icon_url_exists = this.imageExists(template.icon_url);

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
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        window.location.href = '/logout';
      });
    },
    async imageExists(url) {
      if (!url.length) {
        return false;
      }

      try {
        const response = await fetch(url, { method: 'HEAD' });
        return response.ok;
      } catch {
        return false;
      }
    },
    editTemplate(templateId) {
      window.location.href = `/templates/edit/${templateId}`;
    },
    duplicateTemplate(templateId) {
      window.location.href = `/templates/edit/${templateId}#duplicate`;
    },
    createSpaceFromTemplate(templateId) {
      window.location.href = `/spaces/create/${templateId}`;
    },
    async deleteTemplate(templateId) {
      const self = this;
      await fetch(`/api/templates/${templateId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Template deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Template could not be deleted", type: 'error' });
        }
      }).catch(() => {
        window.location.href = '/logout';
      });
      this.getTemplates();
    },
    async createSpaceAs() {
      if(this.chooseUser.forUserId === '') {
        this.chooseUser.invalidUser = true;
        return;
      }

      this.chooseUser.invalidUser = this.chooseUser.invalidTemplate = false;

      // Get the list of templates
      await fetch(`/api/templates?user_id=${this.chooseUser.forUserId}`, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((templates) => {

            // If the selected template is in the list then created
            const template = templates.templates.find(t => t.template_id === this.chooseUser.template.template_id);
            if(template) {
              this.chooseUser.show = false;
              window.location.href = `/spaces/create/${this.chooseUser.template.template_id}/${this.chooseUser.forUserId}`;
            } else {
              this.chooseUser.invalidTemplate = true;
            }
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        window.location.href = '/logout';
      });
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      this.templates.forEach(template => {
        // Default: show if active or showInactive is true
        let showRow = template.active || this.showInactive;

        // Zone filtering (unless showAll)
        if (!this.showAll) {
          const zones = template.zones || [];
          if (zones.length > 0) {
            // Hide if any !zone matches the current zone
            const hasNegation = zones.some(z => z.startsWith('!') && z.substring(1) === this.zone);
            if (hasNegation) {
              showRow = false;
            } else {
              // If there are any non-negated zones, show only if one matches
              const positiveZones = zones.filter(z => !z.startsWith('!'));
              if (positiveZones.length > 0) {
                const hasZone = positiveZones.includes(this.zone);
                showRow = showRow && hasZone;
              }
            }
          }
          // If zones is empty, showRow remains unchanged (no restriction)
        }

        // Search term filtering
        if (term.length > 0) {
          const inName = template.name.toLowerCase().includes(term);
          const inDesc = template.description.toLowerCase().includes(term);
          showRow = showRow && (inName || inDesc);
        }

        template.searchHide = !showRow;
      });
    },
    getDayOfWeek(day) {
      return ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'][day];
    },
    getMaxUptime(maxUptime, maxUptimeUnit) {
      let maxUptimeString = '';
      if(maxUptimeUnit === 'minute') {
        maxUptimeString = `${maxUptime} minute${maxUptime > 1 ? 's' : ''}`;
      } else if(maxUptimeUnit === 'hour') {
        maxUptimeString = `${maxUptime} hour${maxUptime > 1 ? 's' : ''}`;
      } else if(maxUptimeUnit === 'day') {
        maxUptimeString = `${maxUptime} day${maxUptime > 1 ? 's' : ''}`;
      }
      return maxUptimeString;
    }
  };
}
