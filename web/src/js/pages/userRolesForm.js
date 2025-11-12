import { validate } from '../validators.js';
import { focus } from '../focus.js';

window.userRolesForm = function(isEdit, roleId) {

  return {
    formData: {
      name: "",
      permissions: []
    },
    loading: true,
    buttonLabel: isEdit ? 'Save Changes' : 'Create Role',
    nameValid: true,
    isEdit,
    stayOnPage: true,
    groupedPermissions: {},

    async initData() {
      focus.Element('input[name="name"]');

      // fetch the permission list
      const response = await fetch('/api/permissions', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      const permissionsList = await response.json();

      // Group permissions by 'Group' property
      this.groupedPermissions = {};
      permissionsList.permissions.forEach(perm => {
        if (!this.groupedPermissions[perm.group]) {
          this.groupedPermissions[perm.group] = [];
        }
        this.groupedPermissions[perm.group].push(perm);
      });

      if(isEdit) {
        const roleResponse = await fetch(`/api/roles/${roleId}`, {
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
      this.nameValid = validate.maxLength(this.formData.name, 64) && validate.required(this.formData.name);
      return this.nameValid;
    },
    togglePermission(permission) {
      if(this.formData.permissions.includes(permission)) {
        this.formData.permissions = this.formData.permissions.filter(p => p !== permission);
      } else {
        this.formData.permissions.push(permission);
      }
    },
    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating role...' : 'Create role...'
      }
      this.loading = true;

      await fetch(isEdit ? `/api/roles/${roleId}` : '/api/roles', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Role Updated", type: 'success' });
            self.$dispatch('close-role-form');
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Role Created", type: 'success' });
            self.$dispatch('close-role-form');
          } else {
            response.json().then((d) => {
              self.$dispatch('show-alert', { msg: `Failed to update the role, ${d.error}`, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Role';
          this.loading = false;
        })
    },
    toggleSelectAllPermissions(event) {
      const isChecked = event.target.checked;
      this.formData.permissions = isChecked
        ? Object.keys(this.groupedPermissions)
            .flatMap(group => this.groupedPermissions[group])
            .map(perm => perm.id) // or perm.name, depending on your backend
        : [];
    }
  }
}