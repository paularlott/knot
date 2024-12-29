# knot

<div align="center">

[![release](https://img.shields.io/github/v/release/paularlott/knot)](https://github.com/paularlott/knot/releases/latest)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)

</div>

Knot is a powerful tool to manage Cloud Development Environment within a Nomad cluster. It provides a seamless blend of a user-friendly web interface and a command line interface. This dual approach streamlines the deployment process and simplifies access to development environments, making management an effortless endeavor and eliminating the need for each developer to manage their own configurations.

## Features

- **Web-Based Management Interface:** Provides an easy-to-use, browser-based interface for managing environments.
- **Visual Studio Code Integration:** Allows access to Visual Studio Code right from your browser.
- **Code Server Integration:** Offers access to Code Server right from your browser.
- **Terminal Access:** Offers in-browser terminal access for seamless command-line operations.
- **Command-Line Tools:** Simplifies container access with handy command-line tools.
- **User & Permission Management:** Effectively manage users and their permissions.
- **Groups:** Control which templates are available to users.
- **Environment Templates:** Customizable templates for creating consistent development environments.
- **Integration with Nomad:** Ensures seamless integration with Nomad for efficient cluster management.
- **Local Containers:** Run containers on the local machine using Docker or Podman.
- **Quotas:** Limit by resource usage and by number of spaces per user.
- **Development URL Management:** Automatically generated URLs for development spaces.
- **Support for VNC:** Support for web based VNC servers such as KasmVNC.
- **Remote Servers** Maximize performance by deploying environment close to developers but manage templates and users from one central location.
- **Custom Roles:** Create custom roles to manage permissions.
- **API:** Provides an API for integration with other systems.

## Security

knot is designed to be used within trusted environments rather than on the open internet, that is it is expected to be run on a private network with developers connecting to the it via a VPN or similar technology.

For complex deployments over many different locations a mesh network may be more appropriate.

## Documentation

Documentation and [Getting Started](https://getknot.dev/docs/install/)
