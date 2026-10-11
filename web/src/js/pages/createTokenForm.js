import { focus } from '../focus.js';

window.createTokenForm = function() {
  return {
    formData: {
      name: "",
    },
    loading: false,
    buttonLabel: 'Create Token',
    nameValid: true,
    init() {
      focus.Element('input[name="name"]');
    },
    checkName() {
      this.nameValid = this.formData.name.length > 0 && this.formData.name.length < 255;
      return this.nameValid;
    },
    submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      if(err) {
        focus.firstInvalid(this.$root);
        return;
      }

      this.buttonLabel = 'Creating token...'
      this.loading = true;

      const data = {
        name: this.formData.name,
      }

      fetch('/api/tokens', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 201) {
            self.$dispatch('close-token-form');
          } else {
            window.knotError('create the API token', response);
          }
        })
        .catch((error) => {
          window.knotError('create the API token', error);
        })
        .finally(() => {
          this.buttonLabel = 'Create Token';
          this.loading = false;
        })
    },
  }
}