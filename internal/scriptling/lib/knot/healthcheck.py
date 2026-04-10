# knot.healthcheck - Health check library for space monitoring
#
# Available in agent-side health check scripts only.
# Each function terminates the script immediately with a health result.
#
# Usage:
#   import knot.healthcheck as hc
#   hc.http_head("http://localhost:8080/health")

import _knot_healthcheck as _hc


def http_head(url, skip_ssl_verify=False, timeout=10):
    """HTTP HEAD check. Status 200 = healthy, anything else = unhealthy."""
    _hc.http_head(url, skip_ssl_verify, timeout)


def tcp_port(port, timeout=10):
    """TCP port check. Open = healthy, closed = unhealthy."""
    _hc.tcp_port(port, timeout)


def program(command, timeout=10):
    """Run command. Exit code 0 = healthy, non-zero = unhealthy."""
    _hc.program(command, timeout)


def pass_check():
    """Report healthy (for custom checks)."""
    _hc.pass_check()


# Allow `hc.pass()` — Python keyword workaround via __getattr__ not needed;
# users call hc.pass_check() or use the built-in directly.
# For convenience, also expose as 'hc_pass' alias.
hc_pass = pass_check


def fail(reason=""):
    """Report unhealthy with optional reason (for custom checks)."""
    _hc.fail(reason)
