window.spaceForm = function(isEdit, spaceId, userId, preferredShell, forUserId, forUserUsername, templateId) {
  return {
    formData: {
      name: "",
      template_id: "",
      agent_url: "",
      shell: preferredShell,
      user_id: forUserId,
      volume_size: {}
    },
    template_id: templateId,
    loading: true,
    buttonLabel: isEdit ? 'Update Space' : 'Create Space',
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    volume_size: [],
    volume_size_valid: {},
    volume_size_label: {},
    isEdit: isEdit,
    hasEditableVolumeSizes: false,

    async initData() {
      var self = this;

      focusElement('input[name="name"]');

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
          this.formData.volume_size = space.volume_size;

          if(space.user_id != userId) {
            this.formData.user_id = space.user_id;
            this.forUsername = space.username;
          } else {
            this.formData.user_id = "";
            this.forUsername = "";
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
        self.volume_size = data.volume_sizes ? data.volume_sizes : [];

        for (var i = 0; i < self.volume_size.length; i++) {
          self.volume_size_valid[self.volume_size[i].id] = true;

          var n = self.volume_size[i].name.replace(/\$\{\{.*\.space\.id.*\}\}/, '00000000-0000-0000-0000-000000000000');
          self.volume_size_label[self.volume_size[i].id] = {
            name: n,
            label: n.replace(/\$\{\{.*\.space\.name.*\}\}/, self.formData.name)
          }

          if(self.volume_size[i].capacity_min !== self.volume_size[i].capacity_max) {
            self.hasEditableVolumeSizes = true;
          }

          // If size not defined the use the min from self.volume_size[i].capacity_min
          if(self.formData.volume_size[self.volume_size[i].id] === undefined) {
            self.formData.volume_size[self.volume_size[i].id] = self.volume_size[i].capacity_min;
          }
        }
      });

      this.loading = false;
    },
    checkName() {
      // Update the labels
      for (var i = 0; i < this.volume_size.length; i++) {
        this.volume_size_label[this.volume_size[i].id].label = this.volume_size_label[this.volume_size[i].id].name.replace(/\$\{\{.*\.space\.name.*\}\}/g, this.formData.name);
      }

      return this.nameValid = validate.name(this.formData.name);
    },
    checkAddress() {
      if(this.formData.template_id == "00000000-0000-0000-0000-000000000000") {
        return this.addressValid = validate.uri(this.formData.agent_url);
      }
      return true;
    },
    checkVolumeSize(id) {
      var volume = this.volume_size.find(volume => volume.id === id);
      this.formData.volume_size[id] = parseInt(this.formData.volume_size[id]);
      return this.volume_size_valid[id] = this.formData.volume_size[id] >= volume.capacity_min && this.formData.volume_size[id] <= volume.capacity_max;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkAddress() || err;

      // Check the sizes of all the volumes
      for (var i = 0; i < this.volume_size.length; i++) {
        err = !this.checkVolumeSize(this.volume_size[i].id) || err;
      }

      if(err) {
        return;
      }

      this.buttonLabel = isEdit ? 'Updating space...' : 'Creating space...'
      this.loading = true;

      fetch(isEdit ? '/api/v1/spaces/' + spaceId : '/api/v1/spaces', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Space updated", type: 'success' });
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
          this.buttonLabel = isEdit ? 'Update Space' : 'Create Space';
          this.loading = false;
        })
    },
  }
}
