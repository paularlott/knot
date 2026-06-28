export const sanitize = {
  name(value) {
    return value.replace(/[^a-zA-Z0-9-]/g, "");
  },
};

export const validate = {
  email(email) {
    const re = /^([a-z0-9](?:[a-z0-9&'+=_.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$/i;
    return re.test(email);
  },

  name(name) {
    const re = /^[a-zA-Z][a-zA-Z0-9-]{1,63}$/;
    return re.test(name) && !/--/.test(name);
  },

  username(name) {
    const re = /^[a-zA-Z][a-zA-Z0-9.-]{1,63}$/;
    return re.test(name) && !/--/.test(name) && !/\.\./.test(name);
  },

  templateName(name) {
    return name.length <= 64 && name.length >= 2;
  },

  varName(name) {
    const re = /^[a-zA-Z][a-zA-Z0-9_]{1,63}$/;
    return re.test(name);
  },

  password(password) {
    return password.length >= 8;
  },

  uri(uri) {
    const re = /^(srv\+)?https?:\/\/([\w-]+:[\w-]+@)?[\w-]+(?:\.[\w-]+)*(?::\d+)?(?:\/(?:[\w~()=.,+-]|%[0-9a-f]{2})*)*(?:\?(?:[\w~()=.,+-]|%[0-9a-f]{2})+=(?:[\w~()=.,+-]|%[0-9a-f]{2})*(?:&(?:[\w~()=.,+-]|%[0-9a-f]{2})+=(?:[\w~()=.,+-]|%[0-9a-f]{2})*)*)?(#[\w~()=.,/+-]*)?$/i
    return re.test(uri);
  },

  required(string) {
    return string.length > 0;
  },

  maxLength(string, length) {
    return string.length <= length;
  },

  sshPrivateKey(key) {
    const trimmed = key.trim();
    if (trimmed === "") {
      return true;
    }

    const match = trimmed.match(/^-----BEGIN ([A-Z0-9 ]+PRIVATE KEY)-----\s+([\s\S]+?)\s+-----END \1-----$/);
    if (match === null) {
      return false;
    }

    const body = match[2]
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line.length > 0 && !line.includes(":"))
      .join("");

    return body.length >= 64 && /^[A-Za-z0-9+/=]+$/.test(body);
  },

  isOneOf(value, options) {
    return options.includes(value);
  },

  isNumber(value, min, max) {
    const numValue = Number(value);
    return Number.isInteger(numValue) && numValue >= min && numValue <= max;
  }
};
