window.sessionsListComponent = function() {
  return {
    loading: true,
    sessions: [],
    deleteConfirm: {
      show: false,
      session: {
        session_id: ''
      }
    },

    async init() {
      await this.getSessions();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getSessions();
      }, 3000);
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
        window.location.href = '/logout';
      });
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
          self.$dispatch('show-alert', { msg: "Session could not be deleted", type: 'error' });
        }
      }).catch(() => {
        window.location.href = '/logout';
      });
      this.getSessions();
    },
  };
}
