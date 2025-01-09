window.autocompleter = function() {
  return {
    options: [],
    search: '',
    showList: false,
    parentVariable: '',
    loadOptions() {
      this.parentVariable = this.$el.getAttribute('data-parent-variable');

      this.options = window.Timezones;
      this.search = this.formData[this.parentVariable];
    },
    selectOption(option) {
      this.search = this.formData[this.parentVariable] = option;
    },
    get filteredOptions() {
        return this.options.filter(option => option.toLowerCase().includes(this.search.toLowerCase()));
    },
    refresh() {
      this.loadOptions();
    }
  }
}