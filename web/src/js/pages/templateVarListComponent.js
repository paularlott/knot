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
        window.sseClient.subscribe('templatevars:changed', (payload) => {
          if (payload?.id) this.getTemplateVars(payload.id);
        });

        window.sseClient.subscribe('templatevars:deleted', (payload) => {
          this.variables = this.variables.filter(x => x.templatevar_id !== payload?.id);
          this.searchChanged();
        });
      }
    },

    async getTemplateVars(templateVarId) {
      const url = templateVarId ? `/api/templatevars/${templateVarId}` : '/api/templatevars';
      await fetch(url, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((data) => {
            const variableList = templateVarId ? [data] : data.variables;
            variableList.forEach(variable => {
              variable.showIdPopup = false;
              const index = this.variables.findIndex(v => v.templatevar_id === variable.templatevar_id);
              if (index >= 0) {
                this.variables[index] = variable;
              } else {
                this.variables.push(variable);
              }
            });

            this.variables.sort((a, b) => {
              return a.name.localeCompare(b.name);
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
