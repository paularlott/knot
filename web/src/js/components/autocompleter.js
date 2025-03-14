window.autocompleter = function() {
  return {
    options: [],
    search: '',
    showList: false,
    parentVariable: '',
    parentVarGroup: '',
    dataSource: [],
    setData(data) {
      this.dataSource = data;
      this.loadOptions();
    },
    loadOptions() {
      this.parentVarGroup = this.$el.getAttribute('data-parent-var-group');
      if(!this.parentVarGroup) {
        this.parentVarGroup = 'formData';
      }
      this.parentVariable = this.$el.getAttribute('data-parent-variable');

      this.options = this.dataSource;
      this.search = this[this.parentVarGroup] ? this[this.parentVarGroup][this.parentVariable] : '';
    },
    selectOption(option) {
      this.search = this[this.parentVarGroup][this.parentVariable] = option;
    },
    get filteredOptions() {
      return this.options.filter(option => option.toLowerCase().includes(this.search.toLowerCase()));
    },
    refresh() {
      this.loadOptions();
    }
  }
}

window.autocompleterUser = function() {
  return {
    search: '',
    showList: false,
    parentVariable: '',
    parentVariableUsername: '',
    parentVarGroup: '',
    dataSource: 'users',
    element: null,

    setDataSource(dataSource) {
      this.dataSource = dataSource;
      this.loadOptions();
    },
    loadOptions() {
      this.element = this.$el;

      this.parentVarGroup = this.$el.getAttribute('data-parent-var-group');
      if(!this.parentVarGroup) {
        this.parentVarGroup = '';
      }
      this.parentVariable = this.$el.getAttribute('data-parent-variable');
      this.parentVariableUsername = this.$el.getAttribute('data-parent-variable-username');

      if(this.parentVarGroup === '') {
        let selectedUser = this[this.dataSource].find(user => user.user_id === this[this.parentVariable]);
        this.search = selectedUser ? selectedUser.username : '';
      }
      else if(this[this.parentVarGroup]) {
        let selectedUser = this[this.dataSource].find(user => user.user_id === this[this.parentVarGroup][this.parentVariable]);
        this.search = selectedUser ? selectedUser.username : '';
      } else {
        this.search = '';
      }
    },
    selectOption(option) {
      this.search = option.username;
      if(this.parentVarGroup === '') {
        this[this.parentVariable] = option.user_id;
        if(this.parentVariableUsername) {
          this[this.parentVariableUsername] = option.username;
        }
      }
      else {
        this[this.parentVarGroup][this.parentVariable] = option.user_id;
        if(this.parentVariableUsername) {
          this[this.parentVarGroup][this.parentVariableUsername] = option.username;
        }
      }

      this.element.dispatchEvent(new Event('user-selected'));
    },
    get filteredOptions() {
      if(this[this.dataSource] === undefined) {
        return [];
      }
      return this[this.dataSource].filter(option => option.username.toLowerCase().includes(this.search.toLowerCase()));
    },
    refresh() {
      this.loadOptions();
    }
  }
}
