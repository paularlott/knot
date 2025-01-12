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
      this.getVolumes();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getVolumes();
      }, 3000);
    },

    async getVolumes() {
      const response = await fetch('/api/v1/volumes', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      volList = await response.json();
      this.volumes = volList.volumes;

      this.volumes.forEach(volume => {
        volume.starting = false;
        volume.stopping = false;
      });

      // Apply search filter
      this.searchChanged();

      this.loading = false;
    },
    editVolume(volumeId) {
      window.location.href = `/volumes/edit/${volumeId}`;
    },
    async deleteVolume(volumeId) {
    var self = this;
      await fetch(`/api/v1/volumes/${volumeId}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          self.$dispatch('show-alert', { msg: "Volume deleted", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Volume could not be deleted", type: 'error' });
        }
      });
      this.getVolumes();
    },
    async startVolume(volumeId) {
      var self = this;

      await fetch(`/api/v1/volumes/${volumeId}/start`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((v) => {
            const volume = self.volumes.find(volume => volume.volume_id === volumeId);
            volume.active = true;
            volume.starting = false;
            volume.location = v.location;
          });

          self.$dispatch('show-alert', { msg: "Volume started", type: 'success' });
        } else {
          response.json().then((data) => {
            self.$dispatch('show-alert', { msg: "Volume could not be started: " + data.error, type: 'error' });
          });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Volume could not be started: " + error, type: 'error' });
      });
    },
    async stopVolume(volumeId) {
      var self = this;

      const volume = self.volumes.find(volume => volume.volume_id === volumeId);
      volume.stopping = true;
      volume.location = "";

      await fetch(`/api/v1/volumes/${volumeId}/stop`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          const volume = self.volumes.find(volume => volume.volume_id === volumeId);
          volume.active = false;
          volume.stopping = false;
          volume.location = "";

          self.$dispatch('show-alert', { msg: "Volume stopped", type: 'success' });
        } else {
          self.$dispatch('show-alert', { msg: "Volume could not be stopped", type: 'error' });
        }
      }).catch((error) => {
        self.$dispatch('show-alert', { msg: "Volume could not be stopped: " + error, type: 'error' });
      });
    },
    async searchChanged() {
      let term = this.searchTerm.toLowerCase();

      // For all volumes if name contains the term show; else hide
      this.volumes.forEach(v => {
        if(term.length == 0) {
          v.searchHide = false;
        } else {
          v.searchHide = !v.name.toLowerCase().includes(term);
        }
      });
    },
  };
}
