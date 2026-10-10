import { validate } from "../validators.js";
import { focus } from "../focus.js";

window.userGroupForm = function (isEdit, groupId) {
  return {
    formData: {
      name: "",
      max_spaces: 0,
      compute_units: 0,
      storage_units: 0,
      max_tunnels: 0,
      file_storage_mb: 0,
      max_buckets: 0,
    },
    loading: true,
    nameValid: true,
    maxSpacesValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
    maxTunnelsValid: true,
    fileStorageValid: true,
    maxBucketsValid: true,
    isEdit,
    stayOnPage: true,

    async initData() {
      focus.Element('input[name="name"]');

      if (isEdit) {
        const groupResponse = await fetch(`/api/groups/${groupId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        });

        if (groupResponse.status !== 200) {
          window.location.href = "/groups";
        } else {
          const group = await groupResponse.json();

          this.formData.name = group.name;
          this.formData.max_spaces = group.max_spaces;
          this.formData.compute_units = group.compute_units;
          this.formData.storage_units = group.storage_units;
          this.formData.max_tunnels = group.max_tunnels;
          this.formData.file_storage_mb = group.file_storage_mb || 0;
          this.formData.max_buckets = group.max_buckets || 0;
        }
      }

      this.loading = false;
    },
    checkName() {
      this.nameValid =
        validate.maxLength(this.formData.name, 64) &&
        validate.required(this.formData.name);
      return this.nameValid;
    },
    checkMaxSpaces() {
      this.maxSpacesValid = validate.isNumber(
        this.formData.max_spaces,
        0,
        10000,
      );
      return this.maxSpacesValid;
    },
    checkComputeUnits() {
      this.computeUnitsValid = validate.isNumber(
        this.formData.compute_units,
        0,
        Infinity,
      );
      return this.computeUnitsValid;
    },
    checkStorageUnits() {
      this.storageUnitsValid = validate.isNumber(
        this.formData.storage_units,
        0,
        Infinity,
      );
      return this.storageUnitsValid;
    },
    checkMaxTunnels() {
      this.maxTunnelsValid = validate.isNumber(
        this.formData.max_tunnels,
        0,
        100,
      );
      return this.maxTunnelsValid;
    },
    checkFileStorage() {
      this.fileStorageValid = validate.isNumber(
        this.formData.file_storage_mb,
        0,
        4294967295,
      );
      return this.fileStorageValid;
    },
    checkMaxBuckets() {
      this.maxBucketsValid = validate.isNumber(
        this.formData.max_buckets,
        0,
        4294967295,
      );
      return this.maxBucketsValid;
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkMaxSpaces() || err;
      err = !this.checkComputeUnits() || err;
      err = !this.checkStorageUnits() || err;
      err = !this.checkMaxTunnels() || err;
      err = !this.checkFileStorage() || err;
      err = !this.checkMaxBuckets() || err;
      if (err) {
        focus.firstInvalid(this.$root);
        return;
      }

      this.loading = true;

      const data = {
        name: this.formData.name,
        max_spaces: parseInt(this.formData.max_spaces),
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
        max_tunnels: parseInt(this.formData.max_tunnels),
        file_storage_mb: parseInt(this.formData.file_storage_mb),
        max_buckets: parseInt(this.formData.max_buckets),
      };

      await fetch(isEdit ? `/api/groups/${groupId}` : "/api/groups", {
        method: isEdit ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(data),
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Group Updated",
              type: "success",
            });
            self.$dispatch("close-group-form");
          } else if (response.status === 201) {
            self.$dispatch("show-alert", {
              msg: "Group Created",
              type: "success",
            });
            self.$dispatch("close-group-form");
          } else {
            window.knotError(isEdit ? "save the group" : "create the group", response);
          }
        })
        .catch((error) => {
          window.knotError(isEdit ? "save the group" : "create the group", error);
        })
        .finally(() => {
          this.loading = false;
        });
    },
  };
};
