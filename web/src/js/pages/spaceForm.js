import { validate } from '../validators.js';
import { focus } from '../focus.js';

window.spaceForm = function(isEdit, spaceId, userId, preferredShell, forUserId, forUserUsername, templateId) {
  return {
    iconList: [],
    formData: {
      name: "",
      description: "",
      icon_url: "",
      template_id: templateId,
      shell: preferredShell,
      user_id: forUserId,
      alt_names: [],
      custom_fields: [],
      created_at: "",
      created_at_formatted: "",
      selected_node_id: ""
    },
    template_id: templateId,
    template: {
      custom_fields: []
    },
    isManual: false,
    loading: true,
    buttonLabel: isEdit ? 'Save Changes' : 'Create Space',
    buttonLabelWorking: isEdit ? 'Saving...' : 'Creating...',
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    volume_size_valid: {},
    volume_size_label: {},
    isEdit,
    stayOnPage: true,
    altNameValid: [],
    descValid: true,
    startOnCreate: true,
    saving: false,
    quotaStorageLimitShow: false,
    availableNodes: [],
    loadingNodes: false,

    formatCreatedAt() {
      return this.formData.created_at_formatted || '';
    },

    formatNodeLabel(node) {
      return `${node.hostname} (${node.running_spaces} running / ${node.total_spaces} total)`;
    },

    async initData() {
      const iconsResponse = await fetch('/api/icons', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      if (iconsResponse.status === 200) {
        const icons = await iconsResponse.json();
        this.iconList.push(...icons);
      }

      focus.Element('input[name="name"]');

      if(isEdit) {
        const spaceResponse = await fetch(`/api/spaces/${spaceId}`, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (spaceResponse.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const space = await spaceResponse.json();

          this.formData.name = space.name;
          this.formData.description = space.description;
          this.formData.template_id = this.template_id = space.template_id;
          this.formData.shell = space.shell;
          this.formData.icon_url = space.icon_url;
          this.formData.custom_fields = space.custom_fields;
          this.formData.created_at = space.created_at;
          this.formData.created_at_formatted = space.created_at_formatted;

          if(space.user_id !== userId) {
            this.formData.user_id = space.user_id;
            this.forUsername = space.username;
          } else {
            this.formData.user_id = "";
            this.forUsername = "";
          }

          // Set the alt names and mark all as valid
          this.formData.alt_names = space.alt_names ? space.alt_names : [];
          this.altNameValid = [];
          for (let i = 0; i < this.formData.alt_names.length; i++) {
            this.altNameValid.push(true);
          }
        }
      }

      const templatesResponse = await fetch('/api/templates/'+(isEdit ? this.formData.template_id : templateId), {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.template = await templatesResponse.json();

      // Initialize custom fields array immediately to prevent Alpine errors
      if (this.template.custom_fields && this.template.custom_fields.length > 0) {
        // If editing, preserve existing values; if creating, initialize with empty strings
        const existingFields = isEdit ? this.formData.custom_fields : [];
        this.formData.custom_fields = this.template.custom_fields.map(field => {
          return {
            name: field.name,
            value: existingFields.find(f => f.name === field.name)?.value || ''
          };
        });
      } else {
        this.formData.custom_fields = [];
      }

      // Get if the template is manual
      this.isManual = this.template ? this.template.platform === 'manual' : false;
      this.startOnCreate = !this.isManual;

      if(!isEdit) {
        this.formData.icon_url = this.template.icon_url;
      }

      // Fetch available nodes for local container templates
      if(!isEdit && this.template && this.template.platform !== 'manual' && this.template.platform !== 'nomad') {
        this.loadingNodes = true;
        const nodesResponse = await fetch('/api/templates/'+templateId+'/nodes', {
          headers: {
            'Content-Type': 'application/json'
          }
        });
        if (nodesResponse.status === 200) {
          this.availableNodes = await nodesResponse.json();
          if (this.availableNodes.length === 1) {
            this.formData.selected_node_id = this.availableNodes[0].node_id;
          }
        }
        this.loadingNodes = false;
      }

      this.loading = false;
    },
    addAltName() {
      this.altNameValid.push(true);
      this.formData.alt_names.push('');
    },
    removeAltName(index) {
      this.formData.alt_names.splice(index, 1);
      this.altNameValid.splice(index, 1);
    },
    checkName() {
      this.nameValid = validate.name(this.formData.name);
      return this.nameValid;
    },
    checkAltName(index) {
      if(index >= 0 && index < this.formData.alt_names.length) {
        let isValid = validate.name(this.formData.alt_names[index]) && this.formData.alt_names[index] !== this.formData.name;

        // If valid then check for duplicate extra name
        if(isValid) {
          for (let i = 0; i < this.formData.alt_names.length; i++) {
            if(i !== index && this.formData.alt_names[i] === this.formData.alt_names[index]) {
              isValid = false;
              break;
            }
          }
        }

        this.altNameValid[index] = isValid;
        return isValid;
      } else {
        return false;
      }
    },
    checkDesc() {
      this.descValid = this.formData.description.length <= 1024;
      return this.descValid;
    },
    submitData() {
      let err = false;
      const self = this;

      self.saving = true;
      self.stayOnPage = false;
      err = !this.checkName() || err;
      err = !this.checkDesc() || err;

      // Remove the blank alt names
      for (let i = this.formData.alt_names.length - 1; i >= 0; i--) {
        if(this.formData.alt_names[i] === '') {
          this.formData.alt_names.splice(i, 1);
          this.altNameValid.splice(i, 1);
        }
      }

      // Check the alt names
      for (let i = 0; i < this.formData.alt_names.length; i++) {
        err = !this.checkAltName(i) || err;
      }

      if(err) {
        self.saving = false;
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating space...' : 'Creating space...'
      }
      this.loading = true;

      fetch(isEdit ? `/api/spaces/${spaceId}` : '/api/spaces', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Space updated", type: 'success' });
            self.$dispatch('close-space-form');
          } else if (response.status === 201) {
            self.$dispatch('close-space-form');

            // If start on create
            if (this.startOnCreate) {
              response.json().then((data) => {
                fetch(`/api/spaces/${data.space_id}/start`, {
                  method: 'POST',
                  headers: {
                    'Content-Type': 'application/json'
                  }
                }).then((response2) => {
                  if (response2.status === 200) {
                    self.$dispatch('show-alert', { msg: "Space started", type: 'success' });
                  } else {
                    response2.json().then((d) => {
                      self.$dispatch('show-alert', { msg: `Failed to start space, ${d.error}`, type: 'error' });
                    });
                  }
                }).catch((error) => {
                  self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
                }).finally(() => {
                  self.$dispatch('close-space-form');
                })
              });
            }
          } else if (response.status === 507) {
            self.quotaStorageLimitShow = true;
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: (isEdit ? "Failed to update space, " : "Failed to create space, ") + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Space';
          this.loading = false;
        })

      self.saving = false;
    },
  }
}
