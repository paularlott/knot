window.userGroupForm = function(isEdit, groupId) {

  return {
    formData: {
      name: "",
    },
    loading: true,
    buttonLabel: isEdit ? 'Update Group' : 'Create Group',
    nameValid: true,

    async initData() {
      focusElement('input[name="name"]');

      if(isEdit) {
        const groupResponse = await fetch('/api/v1/groups/' + groupId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (groupResponse.status !== 200) {
          window.location.href = '/groups';
        } else {
          const group = await groupResponse.json();

          this.formData.name = group.name;
        }
      }

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.maxLength(this.formData.name, 64) && validate.required(this.formData.name);
    },

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      if(err) {
        return;
      }

      this.buttonLabel = isEdit ? 'Updating group...' : 'Create group...'
      this.loading = true;

      fetch(isEdit ? '/api/v1/groups/' + groupId : '/api/v1/groups', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Group Updated", type: 'success' });
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Group Created", type: 'success' });
            response.json().then(function(data) {
              window.location.href = '/groups/edit/' + data.group_id;
            });
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: "Failed to update the group, " + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update Group' : 'Create Group';
          this.loading = false;
        })
    },
  }
}