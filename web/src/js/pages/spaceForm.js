window.spaceForm = function(isEdit, spaceId, userId, preferredShell, forUserId, forUserUsername) {
  return {
    formData: {
      name: "",
      template_id: "",
      agent_url: "",
      shell: preferredShell,
      user_id: forUserId,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update Space' : 'Create Space',
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    async initData() {
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
          this.formData.template_id = space.template_id;
          this.formData.agent_url = space.agent_url;
          this.formData.shell = space.shell;

          if(space.user_id != userId) {
            this.formData.user_id = space.user_id;
            this.forUsername = space.username;
          } else {
            this.formData.user_id = "";
            this.forUsername = "";
          }
        }
      } else {
        this.formData.template_id = document.querySelector('#template option:first-child').value;
      }

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.name(this.formData.name);
    },
    checkAddress() {
      if(this.formData.template_id == "00000000-0000-0000-0000-000000000000") {
        return this.addressValid = validate.uri(this.formData.agent_url);
      }
      return true;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkAddress() || err;
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
