window.createTokenForm = function() {
  return {
    formData: {
      name: "",
    },
    loading: false,
    buttonLabel: 'Create Token',
    nameValid: true,
    init() {
      focusElement('input[name="name"]');
    },
    checkName() {
      return this.nameValid = this.formData.name.length > 0 && this.formData.name.length < 255;
    },
    submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      if(err) {
        return;
      }

      this.buttonLabel = 'Creating token...'
      this.loading = true;

      var data = {
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
            window.location.href = '/api-tokens';
          } else {
            self.$dispatch('show-alert', { msg: "Failed to create API token", type: 'error' });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = 'Create Token';
          this.loading = false;
        })
    },
  }
}