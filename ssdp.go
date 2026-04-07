package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	ssdpAddr    = "239.255.255.250:1900"
	ssdpGroupIP = "239.255.255.250"
)

// startSSDPResponder listens on the SSDP multicast group and responds to
// Roku ECP M-SEARCH requests so the Now Remote app can auto-discover the
// simulator on the local network.
//
// advertiseAddr is the HTTP address the responder advertises in its LOCATION
// header, e.g. "192.168.1.42:8060".
func startSSDPResponder(advertiseAddr, udn string, events chan<- Event) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		fmt.Printf("[ssdp] resolve error: %v\n", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		// Non-fatal: SSDP may be blocked or the port already in use.
		// Log a warning and continue — manual IP entry still works.
		fmt.Printf("[ssdp] listen error (discovery disabled): %v\n", err)
		return
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Time{}) // no deadline

	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		msg := string(buf[:n])
		if !isMSearchForRoku(msg) {
			continue
		}

		// Send unicast response back to the searcher
		resp := buildSSDPResponse(advertiseAddr, udn)
		respConn, err := net.DialUDP("udp4", nil, src)
		if err == nil {
			_, _ = respConn.Write([]byte(resp))
			respConn.Close()
		}

		events <- Event{
			Time:   time.Now(),
			Kind:   EventQuery,
			Detail: "SSDP M-SEARCH (discovery)",
			Remote: src.IP.String(),
		}
	}
}

// isMSearchForRoku returns true if the raw SSDP message is an M-SEARCH
// targeting "roku:ecp" or "ssdp:all".
func isMSearchForRoku(msg string) bool {
	if !strings.Contains(msg, "M-SEARCH") {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "roku:ecp") ||
		strings.Contains(lower, "ssdp:all") ||
		strings.Contains(lower, "upnp:rootdevice")
}

func buildSSDPResponse(location, udn string) string {
	now := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	return strings.Join([]string{
		"HTTP/1.1 200 OK",
		"CACHE-CONTROL: max-age=3600",
		"DATE: " + now,
		"EXT:",
		"LOCATION: http://" + location + "/",
		"SERVER: Roku/9.2 UPnP/1.0 Roku/9.2",
		"ST: roku:ecp",
		"USN: uuid:" + udn + "::roku:ecp",
		"",
		"",
	}, "\r\n")
}
