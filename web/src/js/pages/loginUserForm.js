window.loginUserForm = function(redirect) {
  sessionStorage.clear();

  return {
    formData: {
      email: "",
      password: "",
      totp_code: "",
    },
    loading: false,
    buttonLabel: 'Login to Your Account',
    emailValid: true,
    passwordValid: true,
    showTOTP: false,
    totpSecret: "",
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
        totp_code: this.formData.totp_code,
      }

      fetch('/api/auth/web', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 200) {

            return response.json().then((data) => {
              // If need to show the TOTP code then show it otherwise redirect
              if(data.totp_secret.length > 0) {
                self.showTOTP = true;
                self.totpSecret = data.totp_secret;
              }
              else {
                window.location.href = self.redirect;
              }
            });
          } else if (response.status == 429) {
            self.$dispatch('show-alert', { msg: "Too many login attempts, please try again later", type: 'error' });
          } else {
            self.$dispatch('show-alert', { msg: "Invalid email, password or TOTP code", type: 'error' });
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
