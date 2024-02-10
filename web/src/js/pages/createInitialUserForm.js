window.createInitialUserForm = function() {
  return {
    formData: {
      username: "",
      email: "",
      password: "",
      password_confirm: "",
    },
    loading: false,
    buttonLabel: 'Create User',
    usernameValid: true,
    emailValid: true,
    passwordValid: true,
    confirmPasswordValid: true,
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
    submitData() {
      var err = false,
          self = this;
      err = !this.checkUsername() || err;
      err = !this.checkEmail() || err;
      err = !this.checkPassword() || err;
      err = !this.checkConfirmPassword() || err;
      if(err) {
        return;
      }

      this.buttonLabel = 'Initializing system...'
      this.loading = true;

      var data = {
        username: this.formData.username,
        email: this.formData.email,
        password: this.formData.password,
        roles: ['00000000-0000-0000-0000-000000000000'],
        active: true,
        ssh_public_key: "",
        preferred_shell: "bash",
        timezone: "UTC",
      }

      fetch('/api/v1/users', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 201) {
            window.location.href = '/';
          } else {
            response.json().then(function(data) {
              self.$disaptch('show-alert', { msg: data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$disaptch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = 'Create User';
          this.loading = false;
        })
    },
  }
}