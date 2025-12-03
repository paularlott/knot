import Alpine from 'alpinejs';

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
    variableFormModal: {
      show: false,
      isEdit: false,
      templateVarId: '',
    },
    variables: [],
    searchTerm: Alpine.$persist('').as('var-search-term').using(sessionStorage),

    async init() {
      await this.getTemplateVars();

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('templatevars:changed', () => {
          this.getTemplateVars();
        });
      }
    },

    async getTemplateVars() {
      await fetch('/api/templatevars', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((variableList) => {
            this.variables = variableList.variables;

            this.variables.forEach(variable => {
              variable.showIdPopup = false;
            });

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
    createVariable() {
      this.variableFormModal.isEdit = false;
      this.variableFormModal.templateVarId = '';
      this.variableFormModal.show = true;
    },
    editVariable(templateVarId) {
      this.variableFormModal.isEdit = true;
      this.variableFormModal.templateVarId = templateVarId;
      this.variableFormModal.show = true;
    },
    loadVariables() {
      this.getTemplateVars();
    },
    async deleteTemplateVar(templateVarId) {
      const self = this;
      await fetch(`/api/templatevars/${templateVarId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Variable deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Variable could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
      this.getTemplateVars();
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all variables if name contains the term show; else hide
      this.variables.forEach(v => {
        if(term.length === 0) {
          v.searchHide = false;
        } else {
          v.searchHide = !v.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
