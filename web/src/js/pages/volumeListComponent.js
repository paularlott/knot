import Alpine from 'alpinejs';

window.volumeListComponent = function() {
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      document.getElementById('search').focus();
      }
    }
  );

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      volume: {
        volume_id: '',
        name: '',
      }
    },
    stopConfirm: {
      show: false,
      volume: {
        volume_id: '',
        name: '',
      }
    },
    volumes: [],
    searchTerm: Alpine.$persist('').as('vol-search-term').using(sessionStorage),

    async init() {
      await this.getVolumes();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getVolumes();
      }, 3000);
    },

    async getVolumes() {
      await fetch('/api/volumes', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((volList) => {
            this.volumes = volList.volumes;

            this.volumes.forEach(volume => {
              volume.starting = false;
              volume.stopping = false;
            });

            // Apply search filter
            this.searchChanged();

            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
    editVolume(volumeId) {
      window.location.href = `/volumes/edit/${volumeId}`;
    },
    async deleteVolume(volumeId) {
      const self = this;

      await fetch(`/api/volumes/${volumeId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Volume deleted", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Volume could not be deleted", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
      this.getVolumes();
    },
    async startVolume(volumeId) {
      const self = this;

      await fetch(`/api/volumes/${volumeId}/start`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((v) => {
            const volume = self.volumes.find(vol => vol.volume_id === volumeId);
            volume.active = true;
            volume.starting = false;
            volume.zone = v.zone;
          });

          self.$dispatch('show-alert', { msg: "Volume started", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          response.json().then((d) => {
            self.$dispatch('show-alert', { msg: `Volume could not be started: ${d.error}`, type: 'error' });
          });
        }
      }).catch((error) => {
        if (error.message && error.message.includes('401')) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: `Volume could not be started: ${error}`, type: 'error' });
        }
      });
    },
    async stopVolume(volumeId) {
      const self = this;
      const volume = self.volumes.find(vol => vol.volume_id === volumeId);
      volume.stopping = true;
      volume.zone = "";

      await fetch(`/api/volumes/${volumeId}/stop`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          const stoppedVolume = self.volumes.find(vol => vol.volume_id === volumeId);
          stoppedVolume.active = false;
          stoppedVolume.stopping = false;
          stoppedVolume.zone = "";

          self.$dispatch('show-alert', { msg: "Volume stopped", type: 'success' });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Volume could not be stopped", type: 'error' });
        }
      }).catch((error) => {
        if (error.message && error.message.includes('401')) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: `Volume could not be stopped: ${error}`, type: 'error' });
        }
      });
    },
    searchChanged() {
      const term = this.searchTerm.toLowerCase();

      // For all volumes if name contains the term show; else hide
      this.volumes.forEach(v => {
        if(term.length === 0) {
          v.searchHide = false;
        } else {
          v.searchHide = !v.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
