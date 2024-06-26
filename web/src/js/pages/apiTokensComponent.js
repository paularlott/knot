window.apiTokensComponent = function() {
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
    searchTerm: '',

    async getTokens() {
      this.loading = true;
      const response = await fetch('/api/v1/tokens', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.tokens = await response.json();
      this.loading = false;
    },
    async deleteToken(tokenId) {
      var self = this;
      await fetch(`/api/v1/tokens/${tokenId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Token deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Token could not be deleted", type: 'error' });
        }
      });
      this.getTokens();
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all tokens if name contains the term show; else hide
      this.tokens.forEach(t => {
        if(term.length == 0) {
          t.searchHide = false;
        } else {
          t.searchHide = !t.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
