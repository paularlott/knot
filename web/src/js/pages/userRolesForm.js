import { validate } from "../validators.js";
import { focus } from "../focus.js";

window.userRolesForm = function (isEdit, roleId) {
  return {
    formData: {
      name: "",
      permissions: [],
      plugin_permissions: [],
    },
    loading: true,
    nameValid: true,
    isEdit,
    stayOnPage: true,
    groupedPermissions: {},
    pluginGroups: {},

    async initData() {
      focus.Element('input[name="name"]');

      // fetch the permission list and the plugin inventory (whose declared
      // permissions feed the Plugin Permissions section) in parallel
      const [permissionsResponse, pluginsResponse] = await Promise.all([
        fetch("/api/permissions", {
          headers: {
            "Content-Type": "application/json",
          },
        }),
        fetch("/api/plugins", {
          headers: {
            "Content-Type": "application/json",
          },
        }),
      ]);
      const permissionsList = await permissionsResponse.json();

      // Group plugin permissions by plugin; only plugins declaring
      // permissions appear.
      this.pluginGroups = {};
      if (pluginsResponse.status === 200) {
        const pluginsList = await pluginsResponse.json();
        (pluginsList.plugins || []).forEach((plugin) => {
          if (plugin.permissions && plugin.permissions.length > 0) {
            this.pluginGroups[plugin.name] = {
              description: plugin.description,
              permissions: plugin.permissions,
            };
          }
        });
      }

      // Group permissions by 'Group' property
      this.groupedPermissions = {};
      permissionsList.permissions.forEach((perm) => {
        if (!this.groupedPermissions[perm.group]) {
          this.groupedPermissions[perm.group] = [];
        }
        this.groupedPermissions[perm.group].push(perm);
      });

      if (isEdit) {
        const roleResponse = await fetch(`/api/roles/${roleId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        });

        if (roleResponse.status !== 200) {
          window.location.href = "/roles";
        } else {
          const role = await roleResponse.json();
          this.formData.name = role.name;
          this.formData.permissions = role.permissions;
          this.formData.plugin_permissions = role.plugin_permissions || [];
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
    togglePluginPermission(permission) {
      if (this.formData.plugin_permissions.includes(permission)) {
        this.formData.plugin_permissions = this.formData.plugin_permissions.filter(
          (p) => p !== permission,
        );
      } else {
        this.formData.plugin_permissions.push(permission);
      }
    },
    togglePermission(permission) {
      if (this.formData.permissions.includes(permission)) {
        this.formData.permissions = this.formData.permissions.filter(
          (p) => p !== permission,
        );
      } else {
        this.formData.permissions.push(permission);
      }
    },
    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      if (err) {
        return;
      }

      this.loading = true;

      await fetch(isEdit ? `/api/roles/${roleId}` : "/api/roles", {
        method: isEdit ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(this.formData),
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Role Updated",
              type: "success",
            });
            self.$dispatch("close-role-form");
          } else if (response.status === 201) {
            self.$dispatch("show-alert", {
              msg: "Role Created",
              type: "success",
            });
            self.$dispatch("close-role-form");
          } else {
            response.json().then((d) => {
              self.$dispatch("show-alert", {
                msg: `Failed to update the role, ${d.error}`,
                type: "error",
              });
            });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Error!<br />${error.message}`,
            type: "error",
          });
        })
        .finally(() => {
          this.loading = false;
        });
    },
    toggleSelectAllPermissions(event) {
      const isChecked = event.target.checked;
      this.formData.permissions = isChecked
        ? Object.keys(this.groupedPermissions)
            .flatMap((group) => this.groupedPermissions[group])
            .map((perm) => perm.id) // or perm.name, depending on your backend
        : [];
    },
  };
};
