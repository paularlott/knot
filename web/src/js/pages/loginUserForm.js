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
    totpEnabled: !!document.getElementById('totp'),
    confirmCode: "",
    confirmCodeValid: true,
    confirmCodeError: "",
    verifying: false,
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
    // Check a code from the authenticator app against the newly saved secret
    // before leaving the page, so a mistyped or unsaved secret is caught now
    // rather than at the next sign-in. The secret doesn't change on retry.
    async verifyTOTP() {
      const code = this.confirmCode.replace(/\s+/g, '');
      if (!/^\d{6}$/.test(code)) {
        this.confirmCodeError = 'Enter the 6-digit code shown in your authenticator app.';
        this.confirmCodeValid = false;
        focus.firstInvalid(this.$root);
        return;
      }

      this.verifying = true;
      try {
        const response = await fetch('/api/auth/totp/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ code }),
        });
        if (response.status === 200) {
          window.location.href = this.redirect;
          return;
        }
        if (response.status === 400) {
          this.confirmCodeError = "That code didn't match. Check the secret in your app is the one shown here, wait for a new code, then try again.";
        } else if (response.status === 429) {
          this.confirmCodeError = 'Too many wrong codes. Wait a few minutes, then try again.';
        } else {
          this.confirmCodeError = await window.knotErrorMessage('check the code', response);
        }
      } catch (error) {
        this.confirmCodeError = await window.knotErrorMessage('check the code', error);
      } finally {
        this.verifying = false;
      }
      this.confirmCodeValid = false;
      focus.firstInvalid(this.$root);
    },
    submitData() {
      let err = false;
      const self = this;
      err = !this.checkEmail() || err;
      err = !this.checkPassword() || err;
      if(err) {
        focus.firstInvalid(this.$root);
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
              // A new authenticator secret was generated: show it and ask
              // for a code before continuing, otherwise go straight in.
              if (d.totp_secret && d.totp_secret.length > 0) {
                self.showTOTP = true;
                self.totpSecret = d.totp_secret;
                focus.Element('#totp-confirm');
              }
              else {
                window.location.href = self.redirect;
              }
            });
          } else if (response.status === 429) {
            window.knotToast("Too many sign-in attempts. Wait a few minutes, then try again.", 'error');
          } else if (response.status === 400 || response.status === 401) {
            // One message for any mismatch, so it doesn't reveal which part was wrong.
            window.knotToast(self.totpEnabled
              ? "That email, password or authenticator code didn't match. Check them and try again."
              : "That email or password didn't match. Check them and try again.", 'error');
          } else {
            window.knotError('sign in', response);
          }

          return null;
        })
        .catch((error) => {
          window.knotError('sign in', error);
        })
        .finally(() => {
          this.buttonLabel = 'Sign In';
          this.loading = false;
        })
    },
  }
}
