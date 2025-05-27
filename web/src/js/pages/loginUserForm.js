import { validate } from '../validators.js';
import { focus } from '../focus.js';

window.loginUserForm = function(redirect) {
  sessionStorage.clear();

  return {
    formData: {
      email: "",
      password: "",
      totp_code: "",
    },
    loading: false,
    buttonLabel: 'Sign In',
    emailValid: true,
    passwordValid: true,
    showTOTP: false,
    totpSecret: "",
    redirect,
    init() {
      focus.Element('input[name="email"]');
    },
    checkEmail() {
      this.emailValid = validate.email(this.formData.email);
      return this.emailValid;
    },
    checkPassword() {
      this.passwordValid = this.formData.password.length > 0;
      return this.passwordValid;
    },
    submitData() {
      let err = false;
      const self = this;
      err = !this.checkEmail() || err;
      err = !this.checkPassword() || err;
      if(err) {
        return;
      }

      this.buttonLabel = 'Signing In...'
      this.loading = true;

      const data = {
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

            return response.json().then((d) => {
              // If need to show the TOTP code then show it otherwise redirect
              if(d.totp_secret.length > 0) {
                self.showTOTP = true;
                self.totpSecret = d.totp_secret;
              }
              else {
                window.location.href = self.redirect;
              }
            });
          } else if (response.status === 429) {
            self.$dispatch('show-alert', { msg: "Too many login attempts, please try again later", type: 'error' });
          } else {
            self.$dispatch('show-alert', { msg: "Invalid email, password or TOTP code", type: 'error' });
          }

          return null;
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = 'Sign In';
          this.loading = false;
        })
    },
  }
}
