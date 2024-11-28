job "knot-server" {
  datacenters = ["dc1"]

  update {
    max_parallel = 1
    min_healthy_time = "30s"
    healthy_deadline = "1m"
    auto_revert = true
  }

  group "knot-server" {
    count = 1

    network {
      port "knot_port" {
        to = 3000
      }
      port "knot_agent_port" {
        to = 3010
      }
    }

    task "knot-server" {
      driver = "docker"
      config {
        image = "paularlott/knot:latest"
        ports = ["knot_port", "knot_agent_port"]
      }

      env {
        KNOT_CONFIG = "/local/knot.yml"
      }

      template {
        data = <<EOF
log:
  level: info
server:
  listen: 0.0.0.0:3000
  listen-agent: 0.0.0.0:3010
  download_path: /srv
  url: "https://knot.example.com"
  wildcard_domain: "*.knot.example.com"
  agent_endpoint: "srv+knot-server-agent.service.consul"
  encrypt: "Gnat9SAejFszCla9n1FjCIXQb3py5i0w" # Replace this using knot genkey

  mysql:
      database: knot
      enabled: true
      host: ""
      password: ""
      user: ""

  nomad:
      addr: "http://nomad.service.consul:4646"
      token: ""
EOF
        destination = "local/knot.yml"
      }

      resources {
        cpu = 256
        memory = 512
      }

      # Knot Server Port
      service {
        name = "${NOMAD_JOB_NAME}"
        port = "knot_port"
        address = "${attr.unique.network.ip-address}"

        # Expose the port on a domain name
        # tags = [
        #  "urlprefix-knot.example.com proto=https tlsskipverify=true",
        #  "urlprefix-*.knot.example.com proto=https tlsskipverify=true"
        # ]

        check {
          name            = "alive"
          type            = "http"
          protocol        = "https"
          tls_skip_verify = true
          path            = "/health"
          interval        = "10s"
          timeout         = "2s"
        }
      }

      service {
        name = "${NOMAD_JOB_NAME}-agent"
        port = "knot_agent_port"
        address = "${attr.unique.network.ip-address}"

        check {
          name            = "alive"
          port            = "knot_port"
          type            = "http"
          protocol        = "https"
          tls_skip_verify = true
          path            = "/health"
          interval        = "10s"
          timeout         = "2s"
        }
      }
    }
  }
}
