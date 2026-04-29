package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

const colsPerLine = 70

type result struct {
	mark byte
	rtt  time.Duration
}

func main() {
	count := flag.Int("c", 1400, "number of echo requests to send")
	size := flag.Int("s", 56, "payload size in bytes")
	timeout := flag.Duration("W", 2*time.Second, "per-packet timeout")
	interval := flag.Duration("i", 0, "interval between sends (0 = back-to-back)")
	privileged := flag.Bool("privileged", true, "use raw ICMP socket (requires root/CAP_NET_RAW)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] <destination>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}
	dst := flag.Arg(0)

	if err := run(dst, *count, *size, *timeout, *interval, *privileged); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dst string, count, size int, timeout, interval time.Duration, privileged bool) error {
	if _, err := net.LookupHost(dst); err != nil {
		return fmt.Errorf("resolve %q: %w", dst, err)
	}

	payload := strings.Repeat("a", size)
	results := make([]result, count)

	for i := 0; i < count; i++ {
		results[i] = pingOnce(dst, payload, timeout, privileged)
		fmt.Printf("%c", results[i].mark)
		if (i+1)%colsPerLine == 0 {
			fmt.Println()
		}
		if interval > 0 && i < count-1 {
			time.Sleep(interval)
		}
	}
	if count%colsPerLine != 0 {
		fmt.Println()
	}

	printSummary(results)
	return nil
}

// pingOnce sends one ICMP echo and classifies the outcome Cisco-style:
// '!' echo reply, 'U' destination unreachable, '.' timeout / other.
func pingOnce(dst, payload string, timeout time.Duration, privileged bool) result {
	r := result{mark: '.'}

	p, err := probing.NewPinger(dst)
	if err != nil {
		return r
	}
	p.Count = 1
	p.Timeout = timeout
	p.Size = len(payload)
	p.SetPrivileged(privileged)

	p.OnRecv = func(pkt *probing.Packet) {
		r.mark = '!'
		r.rtt = pkt.Rtt
	}

	if err := p.Run(); err != nil {
		return r
	}

	stats := p.Statistics()
	if stats.PacketsRecv == 0 {
		// pro-bing does not expose ICMP unreachable directly; surface as
		// timeout. Real Cisco IOS distinguishes 'U' via the upstream router
		// reply, which would require a raw listener — left for a follow-up.
		return r
	}
	return r
}

func printSummary(results []result) {
	var (
		ok                     int
		minRTT, maxRTT, sumRTT time.Duration
		gotAny                 bool
	)
	count := len(results)
	for _, r := range results {
		if r.mark != '!' {
			continue
		}
		ok++
		sumRTT += r.rtt
		if !gotAny || r.rtt < minRTT {
			minRTT = r.rtt
		}
		if !gotAny || r.rtt > maxRTT {
			maxRTT = r.rtt
		}
		gotAny = true
	}

	rate := 0
	if count > 0 {
		rate = ok * 100 / count
	}

	if !gotAny {
		fmt.Printf("Success rate is %d percent (%d/%d)\n", rate, ok, count)
		return
	}
	avg := sumRTT / time.Duration(ok)
	fmt.Printf("Success rate is %d percent (%d/%d), round-trip min/avg/max = %d/%d/%d ms\n",
		rate, ok, count, minRTT.Milliseconds(), avg.Milliseconds(), maxRTT.Milliseconds())
}
