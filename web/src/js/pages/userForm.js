window.userForm = function(isEdit, userId, isProfile) {
  var entity = isProfile ? 'Profile' : 'User';

  return {
    roles: [],
    groups: [],
    formData: {
      username: "",
      email: "",
      password: "",
      password_confirm: "",
      service_password: "",
      preferred_shell: "",
      ssh_public_key: "",
      github_username: "",
      timezone: "",
      active: true,
      max_spaces: 0,
      compute_units: 0,
      storage_units: 0,
      max_tunnels: 0,
      roles: [],
      groups: [],
      totp_secret: "",
    },
    last_login_at: "",
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create ' + entity,
    stayOnPage: true,
    isEdit: isEdit,
    usernameValid: true,
    emailValid: true,
    passwordValid: true,
    confirmPasswordValid: true,
    servicePasswordValid: true,
    shellValid: true,
    tzValid: true,
    maxSpacesValid: true,
    githubUsernameValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
    maxTunnelsValid: true,
    showTOTP: false,
    resetConfirmShow: false,

    async initUsers() {
      focusElement('input[name="username"]');

      const rolesResponse = await fetch('/api/roles', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      let roleList = await rolesResponse.json();
      this.roles = roleList.roles;

      const groupsResponse = await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      groupsList = await groupsResponse.json();
      this.groups = groupsList.groups;

      if(isEdit) {
        const userResponse = await fetch('/api/users/' + userId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (userResponse.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const user = await userResponse.json();
          this.formData.username = user.username;
          this.formData.email = user.email;
          this.formData.preferred_shell = user.preferred_shell;
          this.formData.ssh_public_key = user.ssh_public_key;
          this.formData.github_username = user.github_username;
          this.formData.active = user.active;
          this.formData.max_spaces = user.max_spaces;
          this.formData.compute_units = user.compute_units;
          this.formData.storage_units = user.storage_units;
          this.formData.max_tunnels = user.max_tunnels;
          this.formData.roles = user.roles;
          this.formData.groups = user.groups;
          this.formData.timezone = user.timezone;
          this.formData.service_password = user.service_password;
          this.formData.totp_secret = user.totp_secret;

          // Make last_login_at human readable data time in the browser's timezone
          if (user.last_login_at) {
            const date = new Date(user.last_login_at);
            this.last_login_at = date.toLocaleString();
          } else {
            this.last_login_at = 'Never';
          }
        }
      } else {
        this.formData.preferred_shell = 'bash';
        this.formData.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
      }

      this.$dispatch('refresh-autocompleter');

      this.loading = false;
    },
    toggleRole(roleId) {
      if (this.formData.roles.includes(roleId)) {
        const index = this.formData.roles.indexOf(roleId);
        this.formData.roles.splice(index, 1);
      } else {
        this.formData.roles.push(roleId);
      }
    },
    toggleGroup(groupId) {
      if (this.formData.groups.includes(groupId)) {
        const index = this.formData.groups.indexOf(groupId);
        this.formData.groups.splice(index, 1);
      } else {
        this.formData.groups.push(groupId);
      }
    },
    checkUsername() {
      return this.usernameValid = validate.name(this.formData.username);
    },
    checkEmail() {
      return this.emailValid = validate.email(this.formData.email);
    },
    checkPassword() {
      return this.passwordValid = validate.password(this.formData.password);
    },
    checkConfirmPassword() {
      return this.confirmPasswordValid = this.formData.password == this.formData.password_confirm;
    },
    checkShell() {
      return this.shellValid = validate.isOneOf(this.formData.preferred_shell, ['bash', 'zsh', 'fish', 'sh']);
    },
    checkTz() {
      return this.tzValid = validate.isOneOf(this.formData.timezone, window.Timezones);
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
    checkMaxTunnels() {
      return this.maxTunnelsValid = validate.isNumber(this.formData.max_tunnels, 0, 100);
    },
    checkServicePassword() {
      return this.servicePasswordValid = this.formData.service_password.length <= 255;
    },
    checkGithubUsername() {
      return this.githubUsernameValid = this.formData.github_username.length <= 255;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkUsername() || err;
      err = !this.checkEmail() || err;
      if(!isEdit || this.formData.password.length > 0 || this.formData.password_confirm.length > 0) {
        err = !this.checkPassword() || err;
        err = !this.checkConfirmPassword() || err;
      }
      err = !this.checkMaxSpaces() || err;
      err = !this.checkComputeUnits() || err;
      err = !this.checkStorageUnits() || err;
      err = !this.checkMaxTunnels() || err;
      err = !this.checkServicePassword() || err;
      err = !this.checkShell() || err;
      err = !this.checkTz() || err;
      err = !this.checkGithubUsername() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = (isEdit ? 'Updating ' : 'Creating ') + entity + '...';
      }

      data = {
        username: this.formData.username,
        email: this.formData.email,
        password: this.formData.password,
        service_password: this.formData.service_password,
        preferred_shell: this.formData.preferred_shell,
        ssh_public_key: this.formData.ssh_public_key,
        github_username: this.formData.github_username,
        active: this.formData.active,
        max_spaces: parseInt(this.formData.max_spaces),
        storage_units: parseInt(this.formData.storage_units),
        compute_units: parseInt(this.formData.compute_units),
        max_tunnels: parseInt(this.formData.max_tunnels),
        roles: this.formData.roles,
        groups: this.formData.groups,
        timezone: this.formData.timezone,
      };

      fetch(isEdit ? '/api/users/' + userId : '/api/users', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 200) {
            if(this.stayOnPage) {
              self.$dispatch('show-alert', { msg: entity + " updated", type: 'success' });
            } else {
              window.location.href = isProfile ? '/' : '/users';
            }
          } else if (response.status === 201) {
            window.location.href = '/users';
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: (isEdit ? "Failed to update user, " : "Failed to create user, ") + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create ' + entity;
        })
    },
    resetTOTP() {
      this.formData.totp_secret = "";
      this.resetConfirmShow = false;
      this.submitData();
    },
  }
}
