import { sortable } from '../components/sortable.js';
window.sessionsListComponent = function() {
  return {
    ...sortable('sessions', {
      current: (s) => (s.current ? 0 : 1),
      ip: (s) => s.ip,
      agent: (s) => s.user_agent,
    }),
    loading: true,
    sessions: [],
    async init() {
      await this.getSessions();

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('sessions:changed', () => {
          this.getSessions();
        });

        window.sseClient.subscribe('sessions:deleted', (payload) => {
          this.sessions = this.sessions.filter(x => x.session_id !== payload?.id);
        });
      }
    },

    async getSessions() {
      await fetch('/api/sessions', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((sessions) => {
            this.sessions = sessions;
            this.loading = false;
            this.sessions.forEach(session => {
              session.showIdPopup = false;
            });
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    confirmDelete(session) {
      window.knotConfirm({
        danger: true,
        message: 'Are you sure you want to delete the session?',
        detail: 'The browser using this session will be signed out.',
        confirmLabel: 'Delete Session',
        cancelLabel: 'Cancel',
      }).then((ok) => ok && this.deleteSession(session.session_id));
    },
    async deleteSession(sessionId) {
      const self = this;
      await fetch(`/api/sessions/${sessionId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Session deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          window.knotError('delete the session', response);
        }
      }).catch((err) => {
        // Don't logout on network errors - Safari closes connections aggressively
        window.knotError('delete the session', err);
      });
      this.getSessions();
    },
  };
}
