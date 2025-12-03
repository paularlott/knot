import Alpine from 'alpinejs';

window.apiTokensComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    tokens: [],
    tokenFormModal: {
      show: false
    },
    formData: {
      name: "",
    },
    buttonLabel: 'Create Token',
    nameValid: true,
    deleteConfirm: {
      show: false,
      token: {
        token_id: '',
        name: ''
      }
    },
    searchTerm: Alpine.$persist('').as('apitoken-search-term').using(sessionStorage),

    async init() {
      await this.getTokens();

      this.$watch('tokenFormModal.show', (value) => {
        if (!value) {
          this.formData.name = '';
          this.nameValid = true;
          this.buttonLabel = 'Create Token';
        }
      });

      window.addEventListener('close-token-form', () => {
        this.tokenFormModal.show = false;
        this.getTokens();
      });

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('tokens:changed', () => {
          this.getTokens();
        });
      }
    },

    async getTokens() {
      await fetch('/api/tokens', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((tokens) => {
            this.tokens = tokens;
            // Apply search filter
            this.searchChanged();
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    async deleteToken(tokenId) {
      const self = this;
      await fetch(`/api/tokens/${tokenId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Token deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Token could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
      this.getTokens();
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all tokens if name contains the term show; else hide
      this.tokens.forEach(t => {
        if(term.length === 0) {
          t.searchHide = false;
        } else {
          t.searchHide = !t.name.toLowerCase().includes(term);
        }
      });
    },
    async copyToClipboard(text) {
      await navigator.clipboard.writeText(text);
      this.$dispatch('show-alert', { msg: "Copied to clipboard", type: 'success' });
    },
    createToken() {
      this.tokenFormModal.show = true;
    },
    checkName() {
      this.nameValid = this.formData.name.length > 0 && this.formData.name.length < 255;
      return this.nameValid;
    },
    async submitTokenForm() {
      let err = !this.checkName();
      if(err) {
        return;
      }

      this.buttonLabel = 'Creating token...';
      this.loading = true;

      const data = {
        name: this.formData.name,
      };

      await fetch('/api/tokens', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
      })
      .then((response) => {
        if (response.status === 201) {
          this.$dispatch('show-alert', { msg: "Token created", type: 'success' });
          this.tokenFormModal.show = false;
          this.getTokens();
        } else {
          this.$dispatch('show-alert', { msg: "Failed to create API token", type: 'error' });
        }
      })
      .catch((error) => {
        this.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
      })
      .finally(() => {
        this.buttonLabel = 'Create Token';
        this.loading = false;
      });
    },
  };
}
