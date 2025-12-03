/**
 * SSE Client for real-time updates
 * Replaces polling with Server-Sent Events for efficient data synchronization
 */

class SSEClient {
  constructor() {
    this.eventSource = null;
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.listeners = new Map();
    this.connected = false;
    this.reconnecting = false;
  }

  /**
   * Connect to the SSE endpoint
   */
  connect() {
    if (this.eventSource) {
      return;
    }

    this.eventSource = new EventSource('/api/events');

    this.eventSource.onopen = () => {
      console.log('SSE connected');
      this.connected = true;
      this.reconnecting = false;
      this.reconnectDelay = 1000; // Reset reconnect delay on successful connection
    };

    this.eventSource.addEventListener('connected', (event) => {
      console.log('SSE connection confirmed:', event.data);
    });

    this.eventSource.addEventListener('message', (event) => {
      try {
        const data = JSON.parse(event.data);
        this.handleMessage(data);
      } catch (e) {
        console.error('SSE parse error:', e);
      }
    });

    this.eventSource.onerror = (event) => {
      console.error('SSE error:', event);
      this.handleError();
    };
  }

  /**
   * Handle incoming SSE message
   * @param {Object} event - The parsed event data
   */
  handleMessage(event) {
    // Handle auth errors (redirect to login)
    if (event.type === 'auth:required') {
      console.log('SSE auth required, redirecting to logout');
      window.location.href = '/logout';
      return;
    }

    // Dispatch to registered listeners
    // First check for exact match
    const exactListeners = this.listeners.get(event.type) || [];
    exactListeners.forEach(callback => {
      try {
        callback(event.payload);
      } catch (e) {
        console.error('SSE listener error:', e);
      }
    });

    // Then check for wildcard listeners (e.g., 'space:*' matches 'space:created', 'space:updated', etc.)
    this.listeners.forEach((callbacks, pattern) => {
      if (pattern.endsWith(':*')) {
        const prefix = pattern.slice(0, -1); // Remove the '*'
        if (event.type.startsWith(prefix)) {
          callbacks.forEach(callback => {
            try {
              callback(event.payload, event.type);
            } catch (e) {
              console.error('SSE wildcard listener error:', e);
            }
          });
        }
      }
    });
  }

  /**
   * Handle SSE errors with exponential backoff reconnection
   */
  handleError() {
    this.connected = false;

    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }

    if (this.reconnecting) {
      return;
    }

    this.reconnecting = true;
    console.log(`SSE reconnecting in ${this.reconnectDelay}ms...`);

    setTimeout(() => {
      this.reconnecting = false;
      this.connect();
      // Exponential backoff
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
    }, this.reconnectDelay);
  }

  /**
   * Subscribe to a specific event type
   * @param {string} eventType - The event type to subscribe to (e.g., 'templates:changed', 'space:*')
   * @param {Function} callback - The callback function to call when the event is received
   * @returns {Function} Unsubscribe function
   */
  subscribe(eventType, callback) {
    if (!this.listeners.has(eventType)) {
      this.listeners.set(eventType, []);
    }
    this.listeners.get(eventType).push(callback);

    // Return unsubscribe function
    return () => this.unsubscribe(eventType, callback);
  }

  /**
   * Unsubscribe from a specific event type
   * @param {string} eventType - The event type to unsubscribe from
   * @param {Function} callback - The callback function to remove
   */
  unsubscribe(eventType, callback) {
    const listeners = this.listeners.get(eventType);
    if (listeners) {
      const index = listeners.indexOf(callback);
      if (index > -1) {
        listeners.splice(index, 1);
      }
    }
  }

  /**
   * Disconnect from SSE
   */
  disconnect() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.connected = false;
    this.reconnecting = false;
  }

  /**
   * Check if connected
   * @returns {boolean}
   */
  isConnected() {
    return this.connected;
  }
}

// Create global singleton instance
window.sseClient = new SSEClient();

// Auto-connect when the page loads
document.addEventListener('DOMContentLoaded', () => {
  window.sseClient.connect();
});

// Export for module usage if needed
export { SSEClient };
