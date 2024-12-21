window.userGroupForm = function(isEdit, groupId) {

  return {
    formData: {
      name: "",
      max_spaces: 0,
      compute_units: 0,
      storage_units: 0,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Group',
    nameValid: true,
    maxSpacesValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
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
          this.formData.max_spaces = group.max_spaces;
          this.formData.compute_units = group.compute_units;
          this.formData.storage_units = group.storage_units;
        }
      }

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.maxLength(this.formData.name, 64) && validate.required(this.formData.name);
    },
    checkMaxSpaces() {
      return this.maxSpacesValid = validate.isNumber(this.formData.max_spaces, 0, 10000);
    },
    checkComputeUnits() {
      return this.computeUnitsValid = validate.isNumber(this.formData.compute_units, 0, Infinity);
    },
    checkStorageUnits() {
      return this.storageUnitsValid = validate.isNumber(this.formData.storage_units, 0, Infinity);
    },

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkMaxSpaces() || err;
      err = !this.checkComputeUnits() || err;
      err = !this.checkStorageUnits() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating group...' : 'Create group...'
      }
      this.loading = true;

      data = {
        name: this.formData.name,
        max_spaces: parseInt(this.formData.max_spaces),
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
      }

      fetch(isEdit ? '/api/v1/groups/' + groupId : '/api/v1/groups', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
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