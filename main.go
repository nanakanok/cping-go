package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const version = "0.1.0"

type config struct {
	dst     string
	count   int
	size    int
	timeout time.Duration
	width   int
	verbose bool
	quiet   bool
	// Reserved / NYI
	ttl      int
	df       bool
	source   string
	interval time.Duration
	tos      int
	pattern  string
	ipv6Only bool
	pace     int
	linger   time.Duration
}

func usage() {
	fmt.Fprintf(os.Stdout, `%s — Cisco風の ping 記号(!/U/.) を連続表示し、最後に統計を出力
Usage:
  %s <dst> [flags]           # フラグは <dst> の前後どちらでも可

Positional:
  <dst>                 宛先（必須。ホスト名またはIP）

Implemented flags:
  -c, --count N         送信回数（0=無限） (default 5)
  -s, --size N          ICMPデータ長 (bytes) (default 56)
  -W, --timeout S       タイムアウト秒（小数可） (default 2)
  -l, --width N         改行幅（1行の記号数） (default 70)
  -v, --verbose         詳細ログをstderrに
  -q, --quiet           記号出力を抑制（統計のみ）

Reserved / NYI (受理して無視):
      --ttl N           TTL指定（NYI）
      --df              Don't Fragment（NYI）
      --source IF/IP    送信元IF/アドレス（NYI）
      --interval S      送信間隔（NYI）
      --tos N           TOS/DSCP（NYI）
      --pattern HEX     ペイロードパターン（NYI）
      --ipv6            IPv6強制（NYI）
      --pace N          自動列幅調整（NYI）
      --linger S        終了前待機（NYI）

Notes:
  * raw socketを使うため sudo 推奨
  * 記号の意味: "!"=Echo Reply, "U"=Unreachable系, "."=Timeout
  * ASCIIアート用: -l 70 -c 1400
Examples:
  sudo %s www.google.com -c 10
  sudo %s 8.8.8.8 -c 0   # 無限（Ctrl-Cで停止）
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

// valueFlags lists flags that consume a separate argument value.
// (Bool flags accept only -name or -name=true forms.)
var valueFlags = map[string]bool{
	"c": true, "count": true,
	"s": true, "size": true,
	"W": true, "timeout": true,
	"l": true, "width": true,
	"ttl": true, "source": true, "interval": true, "tos": true,
	"pattern": true, "pace": true, "linger": true,
}

// reorderArgs moves positional arguments to the tail so flags appearing
// after a positional are still parsed by stdlib's flag package, which
// otherwise stops at the first non-flag token.
func reorderArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if !strings.ContainsRune(name, '=') && valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

func parseFlags() *config {
	cfg := &config{
		count:   5,
		size:    56,
		timeout: 2 * time.Second,
		width:   70,
	}

	fs := flag.CommandLine
	fs.IntVar(&cfg.count, "c", cfg.count, "repeat count (0=inf)")
	fs.IntVar(&cfg.count, "count", cfg.count, "repeat count (0=inf)")
	fs.IntVar(&cfg.size, "s", cfg.size, "payload size (bytes)")
	fs.IntVar(&cfg.size, "size", cfg.size, "payload size (bytes)")
	timeoutSec := 2.0
	fs.Float64Var(&timeoutSec, "W", timeoutSec, "timeout seconds per probe")
	fs.Float64Var(&timeoutSec, "timeout", timeoutSec, "timeout seconds per probe")
	fs.IntVar(&cfg.width, "l", cfg.width, "symbols per line")
	fs.IntVar(&cfg.width, "width", cfg.width, "symbols per line")
	fs.BoolVar(&cfg.verbose, "v", false, "verbose")
	fs.BoolVar(&cfg.verbose, "verbose", false, "verbose")
	fs.BoolVar(&cfg.quiet, "q", false, "quiet")
	fs.BoolVar(&cfg.quiet, "quiet", false, "quiet")

	fs.IntVar(&cfg.ttl, "ttl", 0, "TTL (NYI)")
	fs.BoolVar(&cfg.df, "df", false, "Don't Fragment (NYI)")
	fs.StringVar(&cfg.source, "source", "", "source IF/IP (NYI)")
	intervalSec := 0.0
	fs.Float64Var(&intervalSec, "interval", 0, "interval seconds (NYI)")
	fs.IntVar(&cfg.tos, "tos", 0, "TOS/DSCP (NYI)")
	fs.StringVar(&cfg.pattern, "pattern", "", "payload pattern hex (NYI)")
	fs.BoolVar(&cfg.ipv6Only, "ipv6", false, "force IPv6 (NYI)")
	fs.IntVar(&cfg.pace, "pace", 0, "auto width (NYI)")
	lingerSec := 0.0
	fs.Float64Var(&lingerSec, "linger", 0, "linger seconds on exit (NYI)")

	fs.Usage = usage
	if err := fs.Parse(reorderArgs(os.Args[1:])); err != nil {
		os.Exit(2)
	}

	cfg.timeout = time.Duration(timeoutSec * float64(time.Second))
	cfg.interval = time.Duration(intervalSec * float64(time.Second))
	cfg.linger = time.Duration(lingerSec * float64(time.Second))

	if fs.NArg() < 1 {
		usage()
		os.Exit(2)
	}
	cfg.dst = fs.Arg(0)

	var nyi []string
	if cfg.ttl != 0 {
		nyi = append(nyi, "--ttl")
	}
	if cfg.df {
		nyi = append(nyi, "--df")
	}
	if cfg.source != "" {
		nyi = append(nyi, "--source")
	}
	if cfg.interval > 0 {
		nyi = append(nyi, "--interval")
	}
	if cfg.tos != 0 {
		nyi = append(nyi, "--tos")
	}
	if cfg.pattern != "" {
		nyi = append(nyi, "--pattern")
	}
	if cfg.ipv6Only {
		nyi = append(nyi, "--ipv6")
	}
	if cfg.pace != 0 {
		nyi = append(nyi, "--pace")
	}
	if cfg.linger > 0 {
		nyi = append(nyi, "--linger")
	}
	if len(nyi) > 0 {
		fmt.Fprintf(os.Stderr, "[WARN] Reserved options (NYI) ignored: %s\n", strings.Join(nyi, ", "))
	}

	return cfg
}

func isIPv6(ip net.IP) bool { return ip.To4() == nil }

func buildPayload(size int, pattern []byte) []byte {
	if size <= 0 {
		return nil
	}
	if len(pattern) == 0 {
		return bytes.Repeat([]byte("a"), size)
	}
	buf := make([]byte, size)
	for i := 0; i < size; i++ {
		buf[i] = pattern[i%len(pattern)]
	}
	return buf
}

func main() {
	cfg := parseFlags()

	dstIPAddr, err := net.ResolveIPAddr("ip", cfg.dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve error: %v\n", err)
		os.Exit(2)
	}
	ip := dstIPAddr.IP
	v6 := isIPv6(ip)
	if cfg.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] dst=%s ip=%s ipv6=%v count=%d size=%d timeout=%s width=%d\n",
			cfg.dst, ip.String(), v6, cfg.count, cfg.size, cfg.timeout, cfg.width)
	}

	network := "ip4:icmp"
	if v6 {
		network = "ip6:ipv6-icmp"
	}

	c, err := icmp.ListenPacket(network, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen error (need sudo?): %v\n", err)
		os.Exit(3)
	}
	defer c.Close()

	id := os.Getpid() & 0xffff
	seq := 0

	sent := 0
	rcvd := 0
	minRTT := math.Inf(+1)
	maxRTT := 0.0
	sumRTT := 0.0

	payload := buildPayload(cfg.size, nil)

	abort := make(chan os.Signal, 1)
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)
	var aborted atomic.Bool
	go func() {
		<-abort
		aborted.Store(true)
		_ = c.SetReadDeadline(time.Now())
	}()

	printed := 0
	printSym := func(s string) {
		if cfg.quiet {
			return
		}
		fmt.Print(s)
		printed++
		if cfg.width > 0 && printed%cfg.width == 0 {
			fmt.Print("\n")
		}
	}

loop:
	for {
		if aborted.Load() {
			break loop
		}

		var msg icmp.Message
		if v6 {
			msg = icmp.Message{
				Type: ipv6.ICMPTypeEchoRequest, Code: 0,
				Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
			}
		} else {
			msg = icmp.Message{
				Type: ipv4.ICMPTypeEcho, Code: 0,
				Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
			}
		}
		b, err := msg.Marshal(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal error: %v\n", err)
			break
		}

		start := time.Now()
		if _, err := c.WriteTo(b, &net.IPAddr{IP: ip}); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			break
		}
		sent++

		_ = c.SetReadDeadline(time.Now().Add(cfg.timeout))
		rb := make([]byte, 1500)

		proto := 1
		if v6 {
			proto = 58
		}

	reply:
		for {
			n, peer, err := c.ReadFrom(rb)
			if err != nil {
				if aborted.Load() {
					break loop
				}
				printSym(".")
				break reply
			}
			rtt := float64(time.Since(start)) / float64(time.Millisecond)

			rm, err := icmp.ParseMessage(proto, rb[:n])
			if err != nil {
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "[DEBUG] parse error: %v\n", err)
				}
				continue
			}

			switch body := rm.Body.(type) {
			case *icmp.Echo:
				isReply := (v6 && rm.Type == ipv6.ICMPTypeEchoReply) ||
					(!v6 && rm.Type == ipv4.ICMPTypeEchoReply)
				if !isReply || body.ID != id || body.Seq != seq {
					continue
				}
				printSym("!")
				rcvd++
				sumRTT += rtt
				if rtt < minRTT {
					minRTT = rtt
				}
				if rtt > maxRTT {
					maxRTT = rtt
				}
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "[DEBUG] reply from %v: id=%d seq=%d rtt=%.3fms\n",
						peer, body.ID, body.Seq, rtt)
				}
				break reply
			case *icmp.DstUnreach:
				if !refersToIDSeq(body.Data, id, seq, v6) {
					if cfg.verbose {
						fmt.Fprintf(os.Stderr, "[DEBUG] unreachable (not our echo) ignored\n")
					}
					continue
				}
				printSym("U")
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "[DEBUG] unreachable from %v for seq=%d\n", peer, seq)
				}
				break reply
			default:
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "[DEBUG] other ICMP type=%v (ignored)\n", rm.Type)
				}
				continue
			}
		}

		seq = (seq + 1) & 0xffff
		if cfg.count > 0 && sent >= cfg.count {
			break
		}
	}

	if !cfg.quiet && printed > 0 && (cfg.width <= 0 || printed%cfg.width != 0) {
		fmt.Println()
	}

	min := 0.0
	avg := 0.0
	max := 0.0
	if rcvd > 0 {
		min = minRTT
		avg = sumRTT / float64(rcvd)
		max = maxRTT
	}
	rate := 0.0
	if sent > 0 {
		rate = float64(rcvd) * 100.0 / float64(sent)
	}
	fmt.Printf("Success rate is %.1f percent (%d/%d), round-trip min/avg/max = %.1f/%.1f/%.1f ms\n",
		rate, rcvd, sent, min, avg, max)
}

// refersToIDSeq reports whether the quoted original packet inside an
// ICMP error message corresponds to our last Echo Request (id, seq).
//
// quoted is icmp.DstUnreach.Data: the original IP header followed by at
// least the first 8 bytes of its payload (= the embedded ICMP Echo
// header in our case).
func refersToIDSeq(quoted []byte, id, seq int, v6 bool) bool {
	if v6 {
		const v6HdrLen = 40
		if len(quoted) < v6HdrLen+8 {
			return false
		}
		if quoted[6] != 58 { // Next Header = ICMPv6 (extension headers not handled)
			return false
		}
		echo := quoted[v6HdrLen:]
		if echo[0] != byte(ipv6.ICMPTypeEchoRequest) || echo[1] != 0 {
			return false
		}
		embID := int(binary.BigEndian.Uint16(echo[4:6]))
		embSeq := int(binary.BigEndian.Uint16(echo[6:8]))
		return embID == id && embSeq == seq
	}
	if len(quoted) < 20 {
		return false
	}
	ihl := int(quoted[0]&0x0f) * 4
	if ihl < 20 || len(quoted) < ihl+8 {
		return false
	}
	if quoted[9] != 1 { // Protocol = ICMP
		return false
	}
	echo := quoted[ihl:]
	if echo[0] != 8 || echo[1] != 0 { // Echo Request
		return false
	}
	embID := int(binary.BigEndian.Uint16(echo[4:6]))
	embSeq := int(binary.BigEndian.Uint16(echo[6:8]))
	return embID == id && embSeq == seq
}
