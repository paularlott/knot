window.autocompleter = function() {
  return {
    options: [],
    search: '',
    filteredOptions: [],
    showList: false,
    parentVariable: '',
    loadOptions() {
      this.parentVariable = this.$el.getAttribute('data-parent-variable');

      this.options = window.Timezones;
      this.filteredOptions = this.options;
      this.search = this.formData[this.parentVariable];
    },
    selectOption(option) {
      this.search = this.formData[this.parentVariable] = option;
      this.filteredOptions = [];
    },
    get filteredOptions() {
        return this.options.filter(option => option.toLowerCase().includes(this.search.toLowerCase()));
    },
    refresh() {
      this.loadOptions();
    }
  }
}