export var validate = {
  email: function(email) {
    var re = /^([a-z0-9](?:[a-z0-9&'+=_\.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$/i;
    return re.test(email);
  },

  name: function(name) {
    var re = /^[a-zA-Z][a-zA-Z0-9\-]{1,63}$/;
    return re.test(name) && !/--/.test(name);
  },

  varName: function(name) {
    var re = /^[a-zA-Z][a-zA-Z0-9_]{1,63}$/;
    return re.test(name);
  },

  password: function(password) {
    return password.length >= 8;
  },

  uri: function(uri) {
    var re = /^(srv\+)?https?:\/\/([\w-]+:[\w-]+@)?[\w-]+(?:\.[\w-]+)*(?::\d+)?(?:\/[\w ~\\\(\)%\=\.,\+-]*)*(?:\?[\w ~\\\(\)%\=\.,\+-]+=[\w ~\\\(\)%\=\.,\+-]*(?:&[\w ~\\\(\)%\=\.,\+-]+=[\w ~\\\(\)%\=\.,\+-]*)*)?(#[\w ~\\\(\)%\=\.,\/\+-]*)?$/i
    return re.test(uri);
  },

  required: function(string) {
    return string.length > 0;
  },

  maxLength: function(string, length) {
    return string.length <= length;
  },

  isOneOf: function(value, options) {
    return options.includes(value);
  }
};
