window.userRolesForm = function(isEdit, roleId) {

  return {
    formData: {
      name: "",
      permissions: []
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Role',
    nameValid: true,
    isEdit: isEdit,
    stayOnPage: true,
    permissions: [],

    async initData() {
      focusElement('input[name="name"]');

      // fetch the permission list
      const response = await fetch('/api/v1/permissions', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.permissions = await response.json();

      if(isEdit) {
        const roleResponse = await fetch('/api/v1/roles/' + roleId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (roleResponse.status !== 200) {
          window.location.href = '/roles';
        } else {
          const role = await roleResponse.json();
          this.formData.name = role.name;
          this.formData.permissions = role.permissions;
        }
      }

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.maxLength(this.formData.name, 64) && validate.required(this.formData.name);
    },
    togglePermission(permission) {
      if(this.formData.permissions.includes(permission)) {
        this.formData.permissions = this.formData.permissions.filter(p => p !== permission);
      } else {
        this.formData.permissions.push(permission);
      }
    },
    async submitData() {
      let err = false,
          self = this;
      err = !this.checkName() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating role...' : 'Create role...'
      }
      this.loading = true;

      fetch(isEdit ? '/api/v1/roles/' + roleId : '/api/v1/roles', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            if(this.stayOnPage) {
              self.$dispatch('show-alert', { msg: "Role Updated", type: 'success' });
            } else {
              window.location.href = '/roles';
            }
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Role Created", type: 'success' });
            window.location.href = '/roles';
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: "Failed to update the role, " + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Role';
          this.loading = false;
        })
    },
  }
}