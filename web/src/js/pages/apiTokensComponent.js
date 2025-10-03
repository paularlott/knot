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

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getTokens();
      }, 3000);
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
  };
}
