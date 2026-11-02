import { focus } from '../focus.js';
import { defaultPasskeyName, passkeyOriginProblem, passkeysSupported, registerPasskey } from '../webauthn.js';

// The Passkeys fieldset of the user form (partials/user-passkeys.tmpl):
// lists a user's passkeys and removes them; on the user's own profile it
// also adds one, from the Add Passkey dialog. origin is the one address
// passkeys work at.
window.userPasskeys = function (userId, isProfile, origin) {
  return {
    userId,
    isProfile,
    passkeys: [],
    loaded: false,
    // Why passkeys can't be added from this page, or '' when they can.
    unavailable: !passkeysSupported() ? "This browser doesn't support passkeys." : passkeyOriginProblem(origin),
    addModal: false,
    addError: '',
    newName: '',
    busy: false,
    _opener: null,

    init() {
      if (this.userId) this.load();
    },

    async load() {
      const resp = await fetch(`/api/users/${this.userId}/passkeys`, { headers: { Accept: 'application/json' } });
      if (!resp.ok) {
        window.knotError('load the passkeys', resp);
        return;
      }
      const data = await resp.json();
      this.passkeys = data.passkeys || [];
      this.loaded = true;
    },

    formatDate(value) {
      const d = new Date(value);
      return isNaN(d) ? '' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
    },

    openAdd(event) {
      this._opener = event && event.currentTarget;
      this.newName = defaultPasskeyName();
      this.addError = '';
      this.addModal = true;
      focus.Element('#passkey-name');
    },

    closeAdd() {
      if (this.busy || !this.addModal) return;
      this.addModal = false;
      if (this._opener) this._opener.focus();
    },

    async add() {
      if (this.busy) return;
      this.busy = true;
      this.addError = '';
      try {
        await registerPasskey(this.newName.trim());
        this.busy = false;
        this.closeAdd();
        window.knotToast('Passkey added', 'success');
        await this.load();
      } catch (e) {
        // Cancelling the browser's prompt needs no message: the dialog
        // stays open to try again.
        if (!e.cancelled) this.addError = e.message;
      } finally {
        this.busy = false;
      }
    },

    confirmRemove(pk) {
      window.knotConfirm({
        danger: true,
        title: 'Remove Passkey?',
        message: this.isProfile
          ? "You won't be able to sign in with {name} any more. Remove it from your device or password manager as well."
          : 'The user will no longer be able to sign in with {name}.',
        name: pk.name,
        confirmLabel: 'Remove',
        cancelLabel: 'Keep',
      }).then((ok) => ok && this.remove(pk));
    },

    async remove(pk) {
      const resp = await fetch(`/api/users/${this.userId}/passkeys/${pk.passkey_id}`, { method: 'DELETE' });
      if (!resp.ok) {
        window.knotError('remove the passkey', resp);
        return;
      }
      this.passkeys = this.passkeys.filter((p) => p.passkey_id !== pk.passkey_id);
      window.knotToast('Passkey removed', 'success');
    },
  };
};
