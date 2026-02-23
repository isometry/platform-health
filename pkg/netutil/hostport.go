// Package netutil provides network-related utility functions.
package netutil

import (
	"net"
	"strconv"
)

// ParseHostPort parses a host:port string into separate host and port values.
func ParseHostPort(arg string) (host string, port int, err error) {
	var portStr string
	host, portStr, err = net.SplitHostPort(arg)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
