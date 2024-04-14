# knot

Knot serves as an advanced management solution for developer environments within a Nomad cluster, providing a seamless blend of a user-friendly web interface and a command line interface. This dual approach streamlines the deployment process and simplifies access to development environments, making management an effortless endeavor and eliminating the need for each developer to manage their own configurations.

As of release 0.7.0, knot is expected to be feature complete and the APIs stable, further work on the path to 1.0.0 will be around cleaning up the code and improvements.

## Features

- **Web-Based Management Interface:** Provides an easy-to-use, browser-based interface for managing environments.
- **Visual Studio Code Integration:** Allows access to Visual Studio Code right from your browser.
- **Terminal Access:** Offers in-browser terminal access for seamless command-line operations.
- **Command-Line Tools:** Simplifies container access with handy command-line tools.
- **User & Permission Management:** Effectively manage users and their permissions.
- **Groups:** Control which templates are available to users.
- **Environment Templates:** Customizable templates for creating consistent development environments.
- **Integration with Nomad:** Ensures seamless integration with Nomad for efficient cluster management.
- **Quotas:** Limit by disk space usage and by number of spaces per user.
- **Development URL Management:** Automatically generated URLs for development spaces.
- **Support for VNC:** Support for web based VNC servers such as KasmVNC.
- **Remote Servers** Maximize performance by deploying environment close to developers but manage templates and users from one central location.

## Security

knot is designed to be used within trusted environments rather than on the open internet, that is it runs on a private network with developers connecting to the it via a VPN or similar technology.

## Documentation

Documentation and [Getting Started](https://getknot.dev/docs/install/)
