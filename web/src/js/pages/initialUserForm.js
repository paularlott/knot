import { validate } from '../validators.js';

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
      this.usernameValid = validate.name(this.formData.username);
      return this.usernameValid;
    },
    checkEmail() {
      this.emailValid = validate.email(this.formData.email);
      return this.emailValid;
    },
    checkPassword() {
       this.passwordValid = validate.password(this.formData.password);
       return this.passwordValid;
    },
    checkConfirmPassword() {
       this.confirmPasswordValid = this.formData.password === this.formData.password_confirm;
       return this.confirmPasswordValid;
    },
    submitData() {
      let err = false;
      const self = this;
      err = !this.checkUsername() || err;
      err = !this.checkEmail() || err;
      err = !this.checkPassword() || err;
      err = !this.checkConfirmPassword() || err;
      if(err) {
        return;
      }

      this.buttonLabel = 'Initializing system...'
      this.loading = true;

      const data = {
        username: this.formData.username,
        email: this.formData.email,
        password: this.formData.password,
        service_password: "",
        roles: ['00000000-0000-0000-0000-000000000000'],
        groups: [],
        active: true,
        ssh_public_key: "",
        github_username: "",
        preferred_shell: "bash",
        timezone: "UTC",
      }

      fetch('/api/users', {
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
            response.json().then((responseData) => {
              self.$dispatch('show-alert', { msg: responseData.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = 'Create User';
          this.loading = false;
        })
    },
  }
}
