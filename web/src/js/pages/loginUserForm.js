window.loginUserForm = function(redirect) {
  return {
    formData: {
      email: "",
      password: "",
    },
    loading: false,
    buttonLabel: 'Login to Your Account',
    emailValid: true,
    passwordValid: true,
    redirect: redirect,
    init() {
      focusElement('input[name="email"]');
    },
    checkEmail() {
      return this.emailValid = validate.email(this.formData.email);
    },
    checkPassword() {
      return this.passwordValid = this.formData.password.length > 0;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkEmail() || err;
      err = !this.checkPassword() || err;
      if(err) {
        return;
      }

      this.buttonLabel = 'Logging in...'
      this.loading = true;

      var data = {
        email: this.formData.email,
        password: this.formData.password,
      }

      fetch('/api/v1/auth/web', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 200) {
            window.location.href = self.redirect;
          } else {
            self.$dispatch('show-alert', { msg: "Invalid email or password", type: 'error' });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = 'Login to Your Account';
          this.loading = false;
        })
    },
  }
}
