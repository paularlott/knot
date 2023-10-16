package util

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

func GetTargetFromSRV(service string, nameserver string) (string, string, error) {
  var host string = ""
  var port string = ""
  var err error = nil
  var srvAddrs []*net.SRV

  if nameserver == "" {
    _, srvAddrs, err = net.LookupSRV("", "", service)
  } else {
    // Look up against a specific consul host
    resolver := &net.Resolver{
      PreferGo: true,
      Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
        dialer := &net.Dialer{
          Timeout: time.Second,
        }
        return dialer.DialContext(ctx, "udp", nameserver)
      },
    }

    _, srvAddrs, err = resolver.LookupSRV(context.Background(), "", "", service)
  }

  if err == nil && len(srvAddrs) > 0 {
    ip, err := GetIP(srvAddrs[0].Target, nameserver)
    if err != nil {
      host = srvAddrs[0].Target
    } else {
      host = ip
    }
    port = strconv.Itoa(int(srvAddrs[0].Port))
  } else {
    err = errors.New("Can't find service")
  }

  return host, port, err
}

func GetIP(service string, nameserver string) (string, error) {
  var ip string = ""
  var err error = nil
  var ips []net.IP

  if nameserver == "" {
    ips, err = net.LookupIP(service)
  } else {
    // Look up against a specific consul host
    resolver := &net.Resolver{
      PreferGo: true,
      Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
        dialer := &net.Dialer{
          Timeout: time.Second,
        }
        return dialer.DialContext(ctx, "udp", nameserver)
      },
    }

    ips, err = resolver.LookupIP(context.Background(), "ip4", service)
  }

  if err == nil && len(ips) > 0 {
    ip = ips[0].String()
  } else {
    err = errors.New("Can't find service")
  }

  return ip, err
}
