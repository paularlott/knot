export var validate = {
  email: function(email) {
    var re = /^([a-z0-9](?:[a-z0-9&'+=_\.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$/i;
    return re.test(email);
  },

  username: function(username) {
    var re = /^[a-z0-9]{1,32}$/;
    return re.test(username);
  },

  password: function(password) {
    return password.length >= 8;
  }
};
