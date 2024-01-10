# knot

Knot is a powerful tool that simplifies the deployment and management of developer environments within a Nomad cluster through a web interface. It enables the creation of environment templates and allows developers to launch or terminate environments as needed, eliminating the need for each developer to manage their own configurations.

This project is under active development and is not feature complete, as such there's no guarantee of compatibility between builds.

This project is designed to be used within trusted environments rather than on the open internet.

## Features

- **Web-Based Management Interface:** Provides an easy-to-use, browser-based interface for managing environments.
- **Visual Studio Code Integration:** Allows access to Visual Studio Code right from your browser.
- **Terminal Access:** Offers in-browser terminal access for seamless command-line operations.
- **Command-Line Tools:** Simplifies container access with handy command-line tools.
- **User & Permission Management:** Effectively manage users and their permissions.
- **Environment Templates:** Customizable templates for creating consistent development environments.
- **Integration with Nomad:** Ensures seamless integration with Nomad for efficient cluster management.

## CLI

knot allows forwarding of ports and SSH connections over WebSockets. As well as helping in forming direct connections to services through the use of SRV records.

### Proxy

Start a server with:

```shell
knot server -l 127.0.0.1:3000
```

#### SSH

Create a SSH connection from the local machine to a remote server via the knot proxy server.

```shell
ssh -o ProxyCommand='knot proxy ssh --server http://127.0.0.1:3000 server.example.com 22' user@server.example.com
```

`.ssh/config`

```
Host server.example.com
  User user
  HostName server.example.com
  Port 22
  ProxyCommand knot proxy ssh --server http://127.0.0.1:3000 %h %p
```

### Port

Create a connection from a local port to a remote server and port via the knot proxy server.

```shell
knot proxy port :8080 example.service.consul --server http://127.0.0.1:3000
```

### Forward

Where the client is part of the same network as the services being connected to the `forward` command can be used to create a direct connection.

#### SSH

```shell
ssh -o ProxyCommand='knot forward ssh server.example.com 22' user@server.example.com
```

`.ssh/config`

```
Host server.example.com
  User user
  HostName server.example.com
  Port 22
  ProxyCommand knot forward ssh %h %p
```

### Port

```shell
knot forward port :8080 example.service.consul
```
