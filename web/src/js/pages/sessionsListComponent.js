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
      this.getSessions();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getSessions();
      }, 3000);
    },

    async getSessions() {
      const response = await fetch('/api/sessions', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.sessions = await response.json();
      this.loading = false;
      this.sessions.forEach(session => {
        session.showIdPopup = false;
      });
    },
    async deleteSession(sessionId) {
      var self = this;
      await fetch(`/api/sessions/${sessionId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Session deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Session could not be deleted", type: 'error' });
        }
      });
      this.getSessions();
    },
  };
}
