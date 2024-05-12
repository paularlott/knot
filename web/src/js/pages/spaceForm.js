window.spaceForm = function(isEdit, spaceId, userId, preferredShell, forUserId, forUserUsername, templateId) {
  return {
    formData: {
      name: "",
      template_id: "",
      agent_url: "",
      shell: preferredShell,
      user_id: forUserId,
      volume_sizes: {},
      alt_names: [],
    },
    templates: [],
    template_id: templateId,
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Space',
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    volume_sizes: [],
    volume_size_valid: {},
    volume_size_label: {},
    isEdit: isEdit,
    stayOnPage: true,
    hasEditableVolumeSizes: false,
    altNameValid: [],

    async initData() {
      var self = this;

      focusElement('input[name="name"]');

      // Fetch the available templates
      const templatesResponse = await fetch('/api/v1/templates', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      templateList = await templatesResponse.json();
      this.templates = templateList.templates;

      if(isEdit) {
        const spaceResponse = await fetch('/api/v1/spaces/' + spaceId, {
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
          this.formData.agent_url = space.agent_url;
          this.formData.shell = space.shell;
          this.formData.volume_sizes = space.volume_sizes;

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
      const templateResponse = await fetch('/api/v1/templates/' + this.template_id, {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      await templateResponse.json().then(data => {
        self.volume_sizes = data.volume_sizes ? data.volume_sizes : [];

        for (var i = 0; i < self.volume_sizes.length; i++) {
          self.volume_size_valid[self.volume_sizes[i].id] = true;

          var n = self.volume_sizes[i].name.replace(/\$\{\{.*\.space\.id.*\}\}/, '00000000-0000-0000-0000-000000000000');
          self.volume_size_label[self.volume_sizes[i].id] = {
            name: n,
            label: n.replace(/\$\{\{.*\.space\.name.*\}\}/, self.formData.name)
          }

          if(self.volume_sizes[i].capacity_min !== self.volume_sizes[i].capacity_max) {
            self.hasEditableVolumeSizes = true;
          }

          // If size not defined the use the min from self.volume_size[i].capacity_min
          if(self.formData.volume_sizes[self.volume_sizes[i].id] === undefined) {
            self.formData.volume_sizes[self.volume_sizes[i].id] = self.volume_sizes[i].capacity_min;
          }
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
      // Update the labels
      for (var i = 0; i < this.volume_sizes.length; i++) {
        this.volume_size_label[this.volume_sizes[i].id].label = this.volume_size_label[this.volume_sizes[i].id].name.replace(/\$\{\{.*\.space\.name.*\}\}/g, this.formData.name);
      }

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
    checkAddress() {
      if(this.formData.template_id == "00000000-0000-0000-0000-000000000000") {
        return this.addressValid = validate.uri(this.formData.agent_url);
      }
      return true;
    },
    checkVolumeSize(id) {
      var volume = this.volume_sizes.find(volume => volume.id === id);
      this.formData.volume_sizes[id] = parseInt(this.formData.volume_sizes[id]);
      return this.volume_size_valid[id] = this.formData.volume_sizes[id] >= volume.capacity_min && this.formData.volume_sizes[id] <= volume.capacity_max;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkAddress() || err;

      // Check the sizes of all the volumes
      for (var i = 0; i < this.volume_sizes.length; i++) {
        err = !this.checkVolumeSize(this.volume_sizes[i].id) || err;
      }

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
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating space...' : 'Creating space...'
      }
      this.loading = true;

      fetch(isEdit ? '/api/v1/spaces/' + spaceId : '/api/v1/spaces', {
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
            }
          } else if (response.status === 201) {
            window.location.href = '/spaces' + (this.forUserId ? '/' + this.forUserId : '');
          } else if (response.status === 507) {
            self.$dispatch('show-alert', { msg: "Failed to create space, storage limit reached", type: 'error' });
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
    },
  }
}
