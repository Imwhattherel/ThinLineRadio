package main

import (
	"net/http"
	"regexp"
	"strings"
)

func GetRemoteAddr(r *http.Request) string {
	re := regexp.MustCompile(`(.+):.*$`)

	for _, addr := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := re.ReplaceAllString(addr, "$1"); len(ip) > 0 {
			return ip
		}
	}

	if ip := re.ReplaceAllString(r.RemoteAddr, "$1"); len(ip) > 0 {
		return ip
	}

	return r.RemoteAddr
}
