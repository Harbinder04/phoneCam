package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"phonecam/internal/server"
)

func main() {
	bind := flag.String("bind", "0.0.0.0", "host/interface to bind to")
	port := flag.Int("port", 8090, "port to listen on")
	token := flag.String("token", "", "shared token required for frame uploads")
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		fmt.Fprintln(os.Stderr, "error: --token is required")
		os.Exit(2)
	}

	addr := fmt.Sprintf("%s:%d", *bind, *port)
	h, err := server.New(*token)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	log.Printf("phonecam listening on http://%s", addr)
	if ips, err := localIPs(); err == nil {
		for _, ip := range ips {
			log.Printf("open on phone: http://%s:%d/?token=%s", ip, *port, *token)
		}
	}
	log.Printf("use mjpeg URL in desktop apps: http://127.0.0.1:%d/mjpeg", *port)

	if err := h.ListenAndServe(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func localIPs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out, nil
}
