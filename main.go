package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ANSI colour helpers — disabled when NO_COLOR is set or stdout is not a TTY.
var useColor = os.Getenv("NO_COLOR") == "" && isTerminal()

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func ansi(code string) string {
	if !useColor {
		return ""
	}
	return "\x1b[" + code + "m"
}

var (
	reset  = func() string { return ansi("0") }
	bold   = func() string { return ansi("1") }
	dim    = func() string { return ansi("2") }
	purple = func() string { return ansi("35") }
	cyan   = func() string { return ansi("36") }
	green  = func() string { return ansi("32") }
	yellow = func() string { return ansi("33") }
	blue   = func() string { return ansi("34") }
	white  = func() string { return ansi("37") }
)

func main() {
	// ── Flags ──────────────────────────────────────────────────────────────
	port        := flag.Int("port", 8060, "HTTP port to listen on")
	name        := flag.String("name", "Living Room NOW TV", "User-visible device name")
	model       := flag.String("model", "NOW TV Box", "Model name")
	modelNum    := flag.String("model-number", "NOWTVBOX4K", "Model number")
	serial      := flag.String("serial", "NTV20240001", "Serial number")
	udn         := flag.String("udn", "015e5108-9000-1046-8035-b0a737964dfb", "UDN (UUID)")
	swVersion   := flag.String("sw-version", "9.2.0", "Software version")
	lang        := flag.String("lang", "en", "Language code")
	country     := flag.String("country", "GB", "Country code")
	advertiseIP := flag.String("advertise-ip", "", "IP to advertise in SSDP and banner (useful when running in Docker — set to host LAN IP)")
	flag.Parse()

	// ── State ──────────────────────────────────────────────────────────────
	state := newState(*name, *model, *modelNum, *serial, *udn, *swVersion, *lang, *country)

	// Channel for events produced by HTTP handlers and SSDP.
	events := make(chan Event, 64)

	// ── HTTP server ────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      newECPServer(state, events),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// ── SSDP responder ─────────────────────────────────────────────────────
	ip := *advertiseIP
	if ip == "" {
		ip = localIP()
	}
	if ip == "127.0.0.1" {
		fmt.Fprintf(os.Stderr, "[warn] could not determine local IP — SSDP will advertise 127.0.0.1\n")
		fmt.Fprintf(os.Stderr, "[hint] if running in Docker, pass --advertise-ip <your-host-LAN-ip>\n")
	}
	advertiseAddr := fmt.Sprintf("%s:%d", ip, *port)
	go startSSDPResponder(advertiseAddr, *udn, events)

	// ── Banner ─────────────────────────────────────────────────────────────
	printBanner(state, advertiseAddr, *port)

	// ── Event loop ─────────────────────────────────────────────────────────
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case ev := <-events:
			printEvent(ev, state)

		case <-sig:
			fmt.Printf("\n%s%sStopping simulator…%s\n", bold(), yellow(), reset())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
			return
		}
	}
}

// ── Display helpers ───────────────────────────────────────────────────────────

func printBanner(s *State, advertiseAddr string, port int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hr := purple() + strings.Repeat("─", 58) + reset()
	fmt.Println(hr)
	fmt.Printf("%s%s  NOW TV Simulator%s\n", bold(), purple(), reset())
	fmt.Printf("  Device   %s%s%s  (%s)\n", bold(), s.DeviceName, reset(), s.ModelName)
	fmt.Printf("  Serial   %s\n", s.SerialNumber)
	fmt.Printf("  Address  %shttp://%s%s\n", cyan(), advertiseAddr, reset())
	fmt.Printf("  Port     %d   (ECP / Roku External Control Protocol)\n", port)
	fmt.Printf("  SSDP     listening on %s for auto-discovery\n", ssdpAddr)
	fmt.Println(hr)
	fmt.Printf("  Power    %s%s%s\n", green(), s.Power, reset())
	fmt.Printf("  App      %s%s%s\n", bold(), activeAppLabel(s.ActiveApp), reset())
	fmt.Println(hr)
	fmt.Printf("\n%sInstalled channels (%d):%s\n", dim(), len(s.Apps), reset())
	for _, a := range s.Apps {
		fmt.Printf("  %s%-18s%s  id=%-14s  %s\n",
			white(), a.Name, reset(),
			a.ID,
			dim()+a.Version+reset(),
		)
	}
	fmt.Println("\n" + hr)
	fmt.Printf("%sEvents:%s\n", bold(), reset())
}

func printEvent(ev Event, s *State) {
	ts := ev.Time.Format("15:04:05")
	var kindColor string
	switch ev.Kind {
	case EventKeypress:
		kindColor = yellow()
	case EventKeydown, EventKeyup:
		kindColor = dim() + yellow()
	case EventLaunch:
		kindColor = green()
	case EventQuery:
		kindColor = blue()
	default:
		kindColor = white()
	}

	fmt.Printf("  %s%s%s  %s%s%-9s%s  %s%-30s%s  %s%s%s\n",
		dim(), ts, reset(),
		kindColor, bold(), string(ev.Kind), reset(),
		white(), ev.Detail, reset(),
		dim(), ev.Remote, reset(),
	)

	// Show updated state after launches and power-affecting keypresses.
	if ev.Kind == EventLaunch || ev.Kind == EventKeypress {
		snap := s.Snapshot()
		fmt.Printf("  %s[app: %s | power: %s]%s\n",
			dim(), activeAppLabel(snap.ActiveApp), snap.Power, reset())
	}
}

func activeAppLabel(a App) string {
	if a.ID == "" {
		return "Home (Roku)"
	}
	return fmt.Sprintf("%s (id=%s)", a.Name, a.ID)
}

// ── Network helpers ───────────────────────────────────────────────────────────

// localIP returns the preferred outbound local IP, falling back to "127.0.0.1".
// It prefers private LAN addresses (192.168.x.x, 10.x.x.x) over Docker bridge
// addresses (172.17-31.x.x) so that the advertised address is reachable from
// other devices on the same WiFi network.
func localIP() string {
	// Walk all interfaces and collect candidate IPs, scoring them by preference.
	ifaces, err := net.Interfaces()
	if err == nil {
		best := ""
		bestScore := 0
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				var ip net.IP
				switch v := a.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() || ip.To4() == nil {
					continue
				}
				score := ipScore(ip)
				if score > bestScore {
					bestScore = score
					best = ip.String()
				}
			}
		}
		if best != "" {
			return best
		}
	}

	// Fallback: UDP trick to find the default outbound interface IP.
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// ipScore returns a preference score for an IP address.
// Higher score = more likely to be the LAN IP we want to advertise.
func ipScore(ip net.IP) int {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	switch {
	case ip4[0] == 192 && ip4[1] == 168:
		return 30 // home/office LAN — highest preference
	case ip4[0] == 10:
		return 20 // corporate / VPN LAN
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return 5 // Docker bridge / link-local — low preference
	default:
		return 1
	}
}
