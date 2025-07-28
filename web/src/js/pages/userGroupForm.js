import { validate } from '../validators.js';
import { focus } from '../focus.js';

window.userGroupForm = function(isEdit, groupId) {

  return {
    formData: {
      name: "",
      max_spaces: 0,
      compute_units: 0,
      storage_units: 0,
      max_tunnels: 0,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Group',
    nameValid: true,
    maxSpacesValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
    maxTunnelsValid: true,
    isEdit,
    stayOnPage: true,

    async initData() {
      focus.Element('input[name="name"]');

      if(isEdit) {
        const groupResponse = await fetch(`/api/groups/${groupId}`, {
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
          this.formData.max_tunnels = group.max_tunnels;
        }
      }

      this.loading = false;
    },
    checkName() {
      this.nameValid = validate.maxLength(this.formData.name, 64) && validate.required(this.formData.name);
      return this.nameValid;
    },
    checkMaxSpaces() {
      this.maxSpacesValid = validate.isNumber(this.formData.max_spaces, 0, 10000);
      return this.maxSpacesValid;
    },
    checkComputeUnits() {
      this.computeUnitsValid = validate.isNumber(this.formData.compute_units, 0, Infinity);
      return this.computeUnitsValid;
    },
    checkStorageUnits() {
      this.storageUnitsValid = validate.isNumber(this.formData.storage_units, 0, Infinity);
      return this.storageUnitsValid;
    },
    checkMaxTunnels() {
      this.maxTunnelsValid = validate.isNumber(this.formData.max_tunnels, 0, 100);
      return this.maxTunnelsValid;
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkMaxSpaces() || err;
      err = !this.checkComputeUnits() || err;
      err = !this.checkStorageUnits() || err;
      err = !this.checkMaxTunnels() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating group...' : 'Create group...'
      }
      this.loading = true;

      const data = {
        name: this.formData.name,
        max_spaces: parseInt(this.formData.max_spaces),
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
        max_tunnels: parseInt(this.formData.max_tunnels),
      }

      await fetch(isEdit ? `/api/groups/${groupId}` : '/api/groups', {
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
            response.json().then((d) => {
              self.$dispatch('show-alert', { msg: `Failed to update the group, ${d.error}`, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Group';
          this.loading = false;
        })
    },
  }
}