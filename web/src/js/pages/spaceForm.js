window.spaceForm = function(isEdit, spaceId, userId, preferredShell, forUserId, forUserUsername, templateId) {
  return {
    formData: {
      name: "",
      template_id: "",
      shell: preferredShell,
      user_id: forUserId,
      alt_names: [],
    },
    templates: [],
    template_id: templateId,
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Space',
    buttonLabelWorking: isEdit ? 'Updating...' : 'Creating...',
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    volume_size_valid: {},
    volume_size_label: {},
    isEdit: isEdit,
    stayOnPage: true,
    altNameValid: [],
    startOnCreate: true,
    saving: false,

    async initData() {
      focusElement('input[name="name"]');

      // Fetch the available templates
      const templatesResponse = await fetch('/api/templates', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      let templateList = await templatesResponse.json();
      this.templates = templateList.templates;

      if(isEdit) {
        const spaceResponse = await fetch('/api/spaces/' + spaceId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (spaceResponse.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const space = await spaceResponse.json();

          this.formData.name = space.name;
          this.formData.template_id = this.template_id = space.template_id;
          this.formData.shell = space.shell;

          if(space.user_id != userId) {
            this.formData.user_id = space.user_id;
            this.forUsername = space.username;
          } else {
            this.formData.user_id = "";
            this.forUsername = "";
          }

          // Set the alt names and mark all as valid
          this.formData.alt_names = space.alt_names ? space.alt_names : [];
          this.altNameValid = [];
          for (var i = 0; i < this.formData.alt_names.length; i++) {
            this.altNameValid.push(true);
          }
        }
      } else {
        if(templateId == '') {
          this.formData.template_id = this.template_id = document.querySelector('#template option:first-child').value;
        } else {
          this.formData.template_id = this.template_id;
        }
      }

      // Fetch the template to get the volume sizes
      const templateResponse = await fetch('/api/templates/' + this.template_id, {
        headers: {
          'Content-Type': 'application/json'
        }
      });

      this.loading = false;
    },
    async addAltName() {
      this.altNameValid.push(true);
      this.formData.alt_names.push('');
    },
    async removeAltName(index) {
      this.formData.alt_names.splice(index, 1);
      this.altNameValid.splice(index, 1);
    },
    checkName() {
      return this.nameValid = validate.name(this.formData.name);
    },
    checkAltName(index) {
      if(index >= 0 && index < this.formData.alt_names.length) {
        var isValid = validate.name(this.formData.alt_names[index]) && this.formData.alt_names[index] !== this.formData.name;

        // If valid then check for duplicate extra name
        if(isValid) {
          for (var i = 0; i < this.formData.alt_names.length; i++) {
            if(i !== index && this.formData.alt_names[i] === this.formData.alt_names[index]) {
              isValid = false;
              break;
            }
          }
        }

        return this.altNameValid[index] = isValid;
      } else {
        return false;
      }
    },
    submitData() {
      var err = false,
          self = this;

      self.saving = true;
      err = !this.checkName() || err;

      // Remove the blank alt names
      for (var i = this.formData.alt_names.length - 1; i >= 0; i--) {
        if(this.formData.alt_names[i] === '') {
          this.formData.alt_names.splice(i, 1);
          this.altNameValid.splice(i, 1);
        }
      }

      // Check the alt names
      for (var i = 0; i < this.formData.alt_names.length; i++) {
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

      fetch(isEdit ? '/api/spaces/' + spaceId : '/api/spaces', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            if(this.stayOnPage) {
              self.$dispatch('show-alert', { msg: "Space updated", type: 'success' });
            } else {
              window.location.href = '/spaces';
              return;
            }
          } else if (response.status === 201) {

            // If start on create
            if (this.startOnCreate) {
              response.json().then((data) => {
                fetch('/api/spaces/' + data.space_id + '/start', {
                  method: 'POST',
                  headers: {
                    'Content-Type': 'application/json'
                  }
                }).then((response) => {
                  if (response.status === 200) {
                    self.$dispatch('show-alert', { msg: "Space started", type: 'success' });
                  } else {
                    response.json().then((data) => {
                      self.$dispatch('show-alert', { msg: "Failed to start space, " + data.error, type: 'error' });
                    });
                  }
                }).catch((error) => {
                  self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
                }).finally(() => {
                  window.location.href = '/spaces';
                })
              });
            } else {
              window.location.href = '/spaces';
            }
          } else if (response.status === 507) {
            self.$dispatch('show-alert', { msg: "Failed to create space, limit reached", type: 'error' });
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: (isEdit ? "Failed to update space, " : "Failed to create space, ") + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Space';
          this.loading = false;
        })

      self.saving = false;
    },
  }
}
