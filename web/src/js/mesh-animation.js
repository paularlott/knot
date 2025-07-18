class MeshAnimation {
  constructor(canvasId) {
    this.canvas = document.getElementById(canvasId);
    if (!this.canvas) return;

    this.ctx = this.canvas.getContext('2d');
    this.nodes = [];
    this.connections = [];
    this.pulses = [];
    this.animationId = null;
    this.pulseInterval = null; // Add this to track the interval

    this.init();
    this.createMesh();
    this.animate();

    // Handle resize
    window.addEventListener('resize', () => this.handleResize());
  }

  init() {
    this.resizeCanvas();
    this.nodeCount = Math.floor((this.canvas.width * this.canvas.height) / 15000); // Sparse mesh
    this.maxConnections = 3; // Limit connections per node for sparsity
  }

  resizeCanvas() {
    const parent = this.canvas.parentElement;
    if (!parent) return;

    // Reset the transformation matrix before applying a new scale
    this.ctx.setTransform(1, 0, 0, 1, 0, 0);

    const rect = parent.getBoundingClientRect();
    this.canvas.width = rect.width * window.devicePixelRatio;
    this.canvas.height = rect.height * window.devicePixelRatio;
    this.ctx.scale(window.devicePixelRatio, window.devicePixelRatio);
    this.canvas.style.width = rect.width + 'px';
    this.canvas.style.height = rect.height + 'px';
  }

  handleResize() {
    console.log('Canvas resized');
    this.resizeCanvas();
    this.createMesh();
  }

  createMesh() {
    // Clear existing pulse interval before creating new mesh
    if (this.pulseInterval) {
      clearInterval(this.pulseInterval);
      this.pulseInterval = null;
    }

    this.nodes = [];
    this.connections = [];
    this.pulses = []; // Clear existing pulses

    // Create nodes with 3D positions
    for (let i = 0; i < this.nodeCount; i++) {
      this.nodes.push({
        x: Math.random() * this.canvas.width / window.devicePixelRatio,
        y: Math.random() * this.canvas.height / window.devicePixelRatio,
        z: Math.random() * 100 - 50, // 3D depth
        baseZ: Math.random() * 100 - 50,
        vx: (Math.random() - 0.5) * 0.5,
        vy: (Math.random() - 0.5) * 0.5,
        vz: (Math.random() - 0.5) * 0.3,
        pulseTime: 0,
        pulsing: false,
        connections: []
      });
    }

    // Create sparse connections
    this.nodes.forEach((node, i) => {
      const nearbyNodes = this.nodes
        .map((otherNode, j) => ({ node: otherNode, index: j, distance: this.distance3D(node, otherNode) }))
        .filter(item => item.index !== i && item.distance < 150)
        .sort((a, b) => a.distance - b.distance)
        .slice(0, this.maxConnections);

      nearbyNodes.forEach(item => {
        if (!this.connectionExists(i, item.index)) {
          this.connections.push({ from: i, to: item.index });
          node.connections.push(item.index);
        }
      });
    });

    // Start random pulsing (only once)
    this.startRandomPulsing();
  }

  distance3D(a, b) {
    return Math.sqrt(Math.pow(a.x - b.x, 2) + Math.pow(a.y - b.y, 2) + Math.pow(a.z - b.z, 2) * 0.1);
  }

  connectionExists(from, to) {
    return this.connections.some(conn =>
      (conn.from === from && conn.to === to) || (conn.from === to && conn.to === from)
    );
  }

  startRandomPulsing() {
    // Clear any existing interval first
    if (this.pulseInterval) {
      clearInterval(this.pulseInterval);
    }

    this.pulseInterval = setInterval(() => {
      if (this.nodes.length > 0 && Math.random() < 0.3) { // 30% chance every interval
        const randomNode = this.nodes[Math.floor(Math.random() * this.nodes.length)];
        if (!randomNode.pulsing && randomNode.connections.length > 0) {
          this.triggerPulse(this.nodes.indexOf(randomNode));
        }
      }
    }, 1000 + Math.random() * 2000); // Random interval between 1 - 2 seconds
  }

  triggerPulse(nodeIndex) {
    const node = this.nodes[nodeIndex];
    node.pulsing = true;
    node.pulseTime = 0;

    // Create traveling pulses to connected nodes
    this.connections
      .filter(conn => conn.from === nodeIndex || conn.to === nodeIndex)
      .forEach(conn => {
        if (Math.random() < 0.7) { // 70% chance to send pulse
          const targetIndex = conn.from === nodeIndex ? conn.to : conn.from;
          this.pulses.push({
            from: nodeIndex,
            to: targetIndex,
            progress: 0,
            life: 1.0
          });
        }
      });

    // Reset pulsing after animation
    setTimeout(() => {
      node.pulsing = false;
    }, 1000);
  }

  updateNodes() {
    this.nodes.forEach(node => {
      // Gentle floating movement
      node.x += node.vx;
      node.y += node.vy;
      node.z += node.vz;

      // 3D floating effect
      node.z = node.baseZ + Math.sin(Date.now() * 0.001 + node.x * 0.01) * 20;

      // Boundary checking with wrapping
      if (node.x < -50) node.x = this.canvas.width / window.devicePixelRatio + 50;
      if (node.x > this.canvas.width / window.devicePixelRatio + 50) node.x = -50;
      if (node.y < -50) node.y = this.canvas.height / window.devicePixelRatio + 50;
      if (node.y > this.canvas.height / window.devicePixelRatio + 50) node.y = -50;

      // Update pulse animation
      if (node.pulsing) {
        node.pulseTime += 0.05;
      }
    });

    // Update traveling pulses
    this.pulses = this.pulses.filter(pulse => {
      pulse.progress += 0.02;
      pulse.life -= 0.01;

      // Trigger pulse at destination
      if (pulse.progress >= 1.0 && pulse.life > 0) {
        const targetNode = this.nodes[pulse.to];
        if (!targetNode.pulsing && Math.random() < 0.5) {
          this.triggerPulse(pulse.to);
        }
      }

      return pulse.life > 0 && pulse.progress <= 1.2;
    });
  }

  draw() {
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);

    // Draw connections
    this.connections.forEach(conn => {
      const fromNode = this.nodes[conn.from];
      const toNode = this.nodes[conn.to];

      // 3D perspective effect
      const fromScale = 1 + fromNode.z * 0.002;
      const toScale = 1 + toNode.z * 0.002;
      const opacity = Math.max(0.1, 0.3 - Math.abs(fromNode.z + toNode.z) * 0.002);

      this.ctx.strokeStyle = `rgba(255, 255, 255, ${opacity})`;
      this.ctx.lineWidth = 0.5;
      this.ctx.beginPath();
      this.ctx.moveTo(fromNode.x * fromScale, fromNode.y * fromScale);
      this.ctx.lineTo(toNode.x * toScale, toNode.y * toScale);
      this.ctx.stroke();
    });

    // Draw traveling pulses
    this.pulses.forEach(pulse => {
      const fromNode = this.nodes[pulse.from];
      const toNode = this.nodes[pulse.to];

      const x = fromNode.x + (toNode.x - fromNode.x) * pulse.progress;
      const y = fromNode.y + (toNode.y - fromNode.y) * pulse.progress;
      const z = fromNode.z + (toNode.z - fromNode.z) * pulse.progress;
      const scale = 1 + z * 0.002;

      const opacity = pulse.life * 0.8;
      const size = 2 + Math.sin(pulse.progress * Math.PI) * 2;

      this.ctx.fillStyle = `rgba(255, 255, 255, ${opacity})`;
      this.ctx.beginPath();
      this.ctx.arc(x * scale, y * scale, size, 0, Math.PI * 2);
      this.ctx.fill();
    });

    // Draw nodes
    this.nodes.forEach(node => {
      const scale = 1 + node.z * 0.002;
      const baseOpacity = Math.max(0.2, 0.6 - Math.abs(node.z) * 0.003);

      let opacity = baseOpacity;
      let size = 1.5;

      if (node.pulsing) {
        const pulseIntensity = Math.sin(node.pulseTime * 10) * 0.5 + 0.5;
        opacity = baseOpacity + pulseIntensity * 0.4;
        size = 1.5 + pulseIntensity * 2;
      }

      this.ctx.fillStyle = `rgba(255, 255, 255, ${opacity})`;
      this.ctx.beginPath();
      this.ctx.arc(node.x * scale, node.y * scale, size, 0, Math.PI * 2);
      this.ctx.fill();
    });
  }

  animate() {
    this.updateNodes();
    this.draw();
    this.animationId = requestAnimationFrame(() => this.animate());
  }

  destroy() {
    if (this.animationId) {
      cancelAnimationFrame(this.animationId);
    }
    if (this.pulseInterval) {
      clearInterval(this.pulseInterval);
    }
  }
}

// Mesh animation initialization
let heroMeshAnimation = null;

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
  if (heroMeshAnimation) {
    heroMeshAnimation.destroy();
  }
});

// Initialize everything when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
  const heroCanvas = document.getElementById('heroMesh');
  if (heroCanvas && MeshAnimation) {
    heroMeshAnimation = new MeshAnimation('heroMesh');
  }
});
