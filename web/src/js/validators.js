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
  },

  // 5-field cron expression (minute hour day month weekday). Mirrors the
  // server's parser: "*", numbers, ranges (a-b), lists (a,b,c) and steps
  // (*/n, a-b/n, n/m); weekday 0-7 with 7 = Sunday.
  cron(value) {
    const fields = value.trim().split(/\s+/);
    if (fields.length !== 5) {
      return false;
    }
    const specs = [
      [0, 59], // minute
      [0, 23], // hour
      [1, 31], // day of month
      [1, 12], // month
      [0, 7], // day of week (7 wraps to 0 = Sunday)
    ];
    return fields.every((field, i) => validCronField(field, specs[i][0], specs[i][1]));
  }
};

function validCronField(field, min, max) {
  return field.split(",").every((part) => {
    if (part === "") {
      return false;
    }

    let value = part;
    let step = 1;
    const slash = part.indexOf("/");
    if (slash >= 0) {
      step = Number(part.slice(slash + 1));
      if (!Number.isInteger(step) || step <= 0) {
        return false;
      }
      value = part.slice(0, slash);
    }

    let lo = min;
    let hi = max;
    if (value === "*") {
      // full range
    } else if (value.includes("-")) {
      const bounds = value.split("-");
      if (bounds.length !== 2) {
        return false;
      }
      lo = Number(bounds[0]);
      hi = Number(bounds[1]);
    } else {
      lo = Number(value);
      // "n/step" means from n to the field maximum, otherwise a single value.
      hi = step > 1 ? max : lo;
    }

    if (!Number.isInteger(lo) || !Number.isInteger(hi) || lo < min || hi > max || lo > hi) {
      return false;
    }
    return true;
  });
}
