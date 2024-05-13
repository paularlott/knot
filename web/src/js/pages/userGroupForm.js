window.userGroupForm = function(isEdit, groupId) {

  return {
    formData: {
      name: "",
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Group',
    nameValid: true,
    isEdit: isEdit,
    stayOnPage: true,

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

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating group...' : 'Create group...'
      }
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
            if(this.stayOnPage) {
              self.$dispatch('show-alert', { msg: "Group Updated", type: 'success' });
            } else {
              window.location.href = '/groups';
            }
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Group Created", type: 'success' });
            window.location.href = '/groups';
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
          this.buttonLabel = isEdit ? 'Update' : 'Create Group';
          this.loading = false;
        })
    },
  }
}