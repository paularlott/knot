const autocompleterBase = () => ({
  search: '',
  showList: false,
  selectedIndex: -1,
  handleKeydown(e) {
    if (!this.showList) return;
    const filtered = this.filteredOptions;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.selectedIndex = this.selectedIndex < filtered.length - 1 ? this.selectedIndex + 1 : 0;
      this.$nextTick(() => this.scrollToSelected());
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.selectedIndex = this.selectedIndex > 0 ? this.selectedIndex - 1 : filtered.length - 1;
      this.$nextTick(() => this.scrollToSelected());
    } else if (e.key === 'Enter' && this.selectedIndex >= 0) {
      e.preventDefault();
      this.selectOption(filtered[this.selectedIndex]);
    } else if (e.key === 'Escape') {
      this.showList = false;
      this.selectedIndex = -1;
    }
  },
  scrollToSelected() {
    const container = this.$root.querySelector('.overflow-auto');
    if (!container) return;
    const items = container.querySelectorAll('li');
    if (items[this.selectedIndex]) {
      items[this.selectedIndex].scrollIntoView({ block: 'nearest' });
    }
  },
  handleFocus() {
    this.showList = true;
    this.selectedIndex = -1;
  }
});

window.autocompleter = function() {
  return {
    ...autocompleterBase(),
    options: [],
    parentVariable: '',
    parentVarGroup: '',
    dataSource: [],
    init() {
      this.$watch('search', () => { this.selectedIndex = -1; });
    },
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
      this.showList = false;
      this.selectedIndex = -1;
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
    ...autocompleterBase(),
    parentVariable: '',
    parentVariableUsername: '',
    parentVarGroup: '',
    dataSource: 'users',
    element: null,
    init() {
      this.$watch('search', () => { this.selectedIndex = -1; });
    },
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
        const selectedUser = this[this.dataSource].find(user => user.user_id === this[this.parentVariable]);
        this.search = selectedUser ? selectedUser.username : '';
      }
      else if(this[this.parentVarGroup]) {
        const selectedUser = this[this.dataSource].find(user => user.user_id === this[this.parentVarGroup][this.parentVariable]);
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
      this.showList = false;
      this.selectedIndex = -1;
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

window.autocompleterIcon = function(dataSource) {
  return {
    ...autocompleterBase(),
    parentVariable: '',
    parentVarGroup: '',
    dataSource,
    init() {
      this.parentVarGroup = this.$el.getAttribute('data-parent-var-group');
      this.parentVariable = this.$el.getAttribute('data-parent-variable');
      this.loadOptions();
      this.$watch('search', () => { this.selectedIndex = -1; });
    },
    loadOptions() {
      const selectedIcon = this.dataSource.find(itm => itm.url === this[this.parentVarGroup][this.parentVariable]);
      this.search = selectedIcon ? selectedIcon.description : '';
    },
    selectOption(option) {
      this.search = option.description;
      this[this.parentVarGroup][this.parentVariable] = option.url;
      this.showList = false;
      this.selectedIndex = -1;
    },
    get filteredOptions() {
      return this.dataSource
      .filter(option =>
        option.description.toLowerCase().includes(this.search.toLowerCase())
      )
      .slice(0, 50);
    },
    clear() {
      this.search = '';
      this[this.parentVarGroup][this.parentVariable] = '';
      this.showList = false;
      this.selectedIndex = -1;
    },
    refresh() {
      this.loadOptions();
    }
  }
}
