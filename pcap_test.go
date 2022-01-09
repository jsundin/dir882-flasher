package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// go test -v
func TestAnalyzePCAP(t *testing.T) {
	pcap_analyze(t, "virbr0.pcap", 8000)
	pcap_analyze(t, "router.pcap", 80)
}

func pcap_analyze(t *testing.T, src_file string, port int) {
	http_port := layers.TCPPort(port)
	pcap_src, e := pcap.OpenOffline(src_file)
	if e != nil {
		panic(e)
	}
	defer pcap_src.Close()

	if e = pcap_src.SetBPFFilter(fmt.Sprintf("tcp and port %d", http_port)); e != nil {
		panic(e)
	}

	pkt_src := gopacket.NewPacketSource(pcap_src, pcap_src.LinkType())

	n := uint64(0)
	in_stream := false
	var last_t *time.Time
	var t0, t1 time.Time

	var min, max time.Duration
	var min_n, max_n uint64
	var avg, avg_n uint64
	var datalen uint64

	md := md5.New()

	for p := range pkt_src.Packets() {
		n++

		tcpLayer := p.Layer(layers.LayerTypeTCP)
		if tcpLayer == nil {
			t.Errorf("non-tcp packet at %d", n)
			return
		}
		tcp, _ := tcpLayer.(*layers.TCP)
		ts := p.Metadata().Timestamp

		if len(tcp.Payload) > 0 && strings.HasPrefix(string(tcp.Payload), "POST /") {
			t0 = ts
			// stream starts after POST /
			in_stream = true
			continue
		}

		if !in_stream {
			// don't care about non-stream stuff
			continue
		}

		if tcp.SrcPort == http_port && len(tcp.Payload) > 0 {
			// stream has stopped when server replies with data
			break
		}

		if tcp.DstPort != http_port {
			// don't care about response
			continue
		}

		datalen += uint64(len(tcp.Payload))
		md.Write(tcp.Payload)

		if last_t != nil {
			t := ts.Sub(*last_t)
			if min == 0 || t < min {
				min = t
				min_n = n
			}
			if max == 0 || t > max {
				max = t
				max_n = n
			}
			avg += uint64(t)
			avg_n++
		}
		last_t = &ts
		t1 = ts
	}
	avg /= avg_n
	t.Logf("%s:\n", src_file)
	t.Logf("  pkt: %d", avg_n)
	t.Logf("  len: %d", datalen)
	t.Logf("  md5: %v (NOT firmware md5!)", hex.EncodeToString(md.Sum(nil)))
	t.Logf("  min: %v (#%d)", min, min_n)
	t.Logf("  max: %v (#%d)", max, max_n)
	t.Logf("  avg: %v", time.Duration(avg))
	t.Logf("  dur: %v", t1.Sub(t0))
}
