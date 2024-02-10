window.templateVarListComponent = function() {
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
    async getTemplateVars() {
      this.loading = true;

      const response = await fetch('/api/v1/templatevars', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.variables = await response.json();

      this.variables.forEach(variable => {
        variable.showIdPopup = false;
      });

      this.loading = false;
    },
    editTemplateVar(templateVarId) {
      window.location.href = `/variables/edit/${templateVarId}`;
    },
    async deleteTemplateVar(templateVarId) {
      var self = this;
      await fetch(`/api/v1/templatevars/${templateVarId}`, {
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
  };
}
