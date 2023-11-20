package util

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func GetTargetFromSRV(service string, nameserver string) (string, string, error) {
  var host string = ""
  var port string = ""
  var err error = nil
  var srvAddrs []*net.SRV
  var resolver *net.Resolver

  if nameserver == "" {
    resolver = net.DefaultResolver
  } else {
    resolver = &net.Resolver{
      PreferGo: true,
      Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
        dialer := &net.Dialer{
          Timeout: time.Second,
        }
        return dialer.DialContext(ctx, "udp", nameserver)
      },
    }
  }

  _, srvAddrs, err = resolver.LookupSRV(context.Background(), "", "", service)

  if err == nil && len(srvAddrs) > 0 {
    ips, err := resolver.LookupIP(context.Background(), "ip4", srvAddrs[0].Target)
    if err != nil || len(ips) == 0 {
      host = srvAddrs[0].Target
    } else {
      host = ips[0].String()
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
  var resolver *net.Resolver

  if nameserver == "" {
    resolver = net.DefaultResolver
  } else {
    resolver = &net.Resolver{
      PreferGo: true,
      Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
        dialer := &net.Dialer{
          Timeout: time.Second,
        }
        return dialer.DialContext(ctx, "udp", nameserver)
      },
    }
  }

  ips, err = resolver.LookupIP(context.Background(), "ip4", service)

  if err == nil && len(ips) > 0 {
    ip = ips[0].String()
  } else {
    err = errors.New("Can't find service")
  }

  return ip, err
}

func ResolveSRVHttp(uri string, nameserver string) string {
  // If url starts with srv+ then remove it and resolve the actual url
  if len(uri) > 4 && uri[0:4] == "srv+" {

    // Parse the url excluding the srv+ prefix
    u, err := url.Parse(uri[4:])
    if err != nil {
      return uri[4:];
    }

    host, port, err := GetTargetFromSRV(u.Host, nameserver)
    if err != nil {
      return uri[4:];
    }

    u.Host = host + ":" + port
    uri = u.String()
  } else if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
    uri = "https://" + uri
  }

  return uri
}