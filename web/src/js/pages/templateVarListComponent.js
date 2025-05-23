window.templateVarListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      variable: {
        templatevar_id: '',
        name: '',
      }
    },
    variables: [],
    searchTerm: Alpine.$persist('').as('var-search-term').using(sessionStorage),

    async init() {
      this.getTemplateVars();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getTemplateVars();
      }, 3000);
    },

    async getTemplateVars() {
      const response = await fetch('/api/templatevars', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      variableList = await response.json();
      this.variables = variableList.variables;

      this.variables.forEach(variable => {
        variable.showIdPopup = false;
      });

      // Apply search filter
      this.searchChanged();

      this.loading = false;
    },
    editTemplateVar(templateVarId) {
      window.location.href = `/variables/edit/${templateVarId}`;
    },
    async deleteTemplateVar(templateVarId) {
      var self = this;
      await fetch(`/api/templatevars/${templateVarId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Variable deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Variable could not be deleted", type: 'error' });
        }
      });
      this.getTemplateVars();
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all variabkes if name contains the term show; else hide
      this.variables.forEach(v => {
        if(term.length == 0) {
          v.searchHide = false;
        } else {
          v.searchHide = !v.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
