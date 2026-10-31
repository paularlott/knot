// WebAuthn helpers for passkeys: run the browser side of the ceremonies the
// server begins (/api/auth/passkey/*, /api/users/whoami/passkeys/*), with
// JSON in the shapes go-webauthn produces and expects.

const b64uToBuf = (s) => {
  const p = s.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(s.length / 4) * 4, '=');
  return Uint8Array.from(atob(p), (c) => c.charCodeAt(0)).buffer;
};
const bufToB64u = (b) =>
  btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');

export const passkeysSupported = () =>
  typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials;

// passkeyOriginProblem returns why passkeys can't be used on this page, or
// '' when they can. The server accepts passkeys only at its configured URL
// (origin, as the page receives it), so on any other address the browser
// would refuse or the server would reject them.
export function passkeyOriginProblem(origin) {
  if (!origin || window.location.origin === origin) return '';
  return `Passkeys only work at ${origin}. Open knot there to use them.`;
}

function creationOptions(json) {
  const pk = json.publicKey || json;
  if (PublicKeyCredential.parseCreationOptionsFromJSON) return PublicKeyCredential.parseCreationOptionsFromJSON(pk);
  return {
    ...pk, challenge: b64uToBuf(pk.challenge), user: { ...pk.user, id: b64uToBuf(pk.user.id) },
    excludeCredentials: (pk.excludeCredentials || []).map((c) => ({ ...c, id: b64uToBuf(c.id) })),
  };
}

function requestOptions(json) {
  const pk = json.publicKey || json;
  if (PublicKeyCredential.parseRequestOptionsFromJSON) return PublicKeyCredential.parseRequestOptionsFromJSON(pk);
  return { ...pk, challenge: b64uToBuf(pk.challenge), allowCredentials: (pk.allowCredentials || []).map((c) => ({ ...c, id: b64uToBuf(c.id) })) };
}

function credentialJSON(c) {
  if (c.toJSON) return c.toJSON();
  const r = c.response;
  const out = {
    id: c.id, rawId: bufToB64u(c.rawId), type: c.type, authenticatorAttachment: c.authenticatorAttachment,
    clientExtensionResults: c.getClientExtensionResults ? c.getClientExtensionResults() : {},
    response: { clientDataJSON: bufToB64u(r.clientDataJSON) },
  };
  if (r.attestationObject) {
    out.response.attestationObject = bufToB64u(r.attestationObject);
    out.response.transports = r.getTransports ? r.getTransports() : [];
  } else {
    out.response.authenticatorData = bufToB64u(r.authenticatorData);
    out.response.signature = bufToB64u(r.signature);
    if (r.userHandle) out.response.userHandle = bufToB64u(r.userHandle);
  }
  return out;
}

// PasskeyError carries a message fit to show the user. cancelled is set
// when the user dismissed the browser's prompt, which needs no message.
export class PasskeyError extends Error {
  constructor(message, cancelled = false) {
    super(message);
    this.cancelled = cancelled;
  }
}

function describe(e, what) {
  if (e instanceof PasskeyError) return e;
  if (e && e.name === 'NotAllowedError') return new PasskeyError(`${what} was cancelled or timed out.`, true);
  if (e && e.name === 'AbortError') return new PasskeyError(`${what} was cancelled.`, true);
  if (e && e.name === 'InvalidStateError') return new PasskeyError('This authenticator already has a passkey for this account.');
  // On the right address this means the browser doesn't trust the site's
  // certificate: browsers turn passkeys off on a page with a certificate error.
  if (e && e.name === 'SecurityError') return new PasskeyError(`Your browser blocked passkeys on this page. This happens when it doesn't trust the site's certificate: use a trusted certificate for ${window.location.host}.`);
  return new PasskeyError(`${what} didn't work on this device.`);
}

// post sends JSON and returns the parsed reply, throwing a PasskeyError
// with the server's message on failure.
async function post(url, body) {
  let res;
  try {
    res = await fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new PasskeyError("The server couldn't be reached. Check your connection and try again.");
  }
  let data = null;
  try { data = await res.json(); } catch { data = null; }
  if (!res.ok) {
    if (res.status === 429) throw new PasskeyError('Too many sign-in attempts. Wait a few minutes, then try again.');
    throw new PasskeyError((data && data.error) ? data.error.charAt(0).toUpperCase() + data.error.slice(1) + '.' : 'The request failed.');
  }
  return data;
}

// signInWithPasskey runs a discoverable passkey sign-in; on success the
// server has set the session cookie.
export async function signInWithPasskey() {
  const options = await post('/api/auth/passkey/begin');
  let credential;
  try {
    const cred = await navigator.credentials.get({ publicKey: requestOptions(options) });
    if (!cred) throw new PasskeyError('No passkey was provided.');
    credential = credentialJSON(cred);
  } catch (e) {
    throw describe(e, 'Passkey sign-in');
  }
  return post('/api/auth/passkey/finish', credential);
}

// registerPasskey adds a passkey named name to the signed-in user.
export async function registerPasskey(name) {
  const options = await post('/api/users/whoami/passkeys/begin');
  let credential;
  try {
    const cred = await navigator.credentials.create({ publicKey: creationOptions(options) });
    if (!cred) throw new PasskeyError('No passkey was created.');
    credential = credentialJSON(cred);
  } catch (e) {
    throw describe(e, 'Creating the passkey');
  }
  return post('/api/users/whoami/passkeys/finish', { name, credential });
}

// defaultPasskeyName suggests a name for a new passkey from the browser's
// platform, so a list of them can be told apart.
export function defaultPasskeyName() {
  const p = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || '';
  if (/mac/i.test(p)) return 'Mac';
  if (/win/i.test(p)) return 'Windows';
  if (/iphone|ipad|ios/i.test(p) || /iPhone|iPad/.test(navigator.userAgent)) return 'iPhone or iPad';
  if (/android/i.test(p) || /Android/.test(navigator.userAgent)) return 'Android';
  if (/linux/i.test(p)) return 'Linux';
  return 'Passkey';
}
