# knot

A project to allow for the easy management of developer environments within a Nomad cluster via a web interface.

Templates for environments can be created and developers can launch and destroy environments as required without needing to worry about configuration.

This project is under active development and is not feature complete, as such there's no guarantee of compatibility between builds.

This project is designed to be used within trusted environments rather than on the open internet, therefore

## Features

- Web base management interface
- Visual Studio Code in the browser
- Terminal in the browser
- Command line tools to simply container access
- Users
- Permissions
- Environment Templates
- Integration with Nomad

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
