package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// name of the webserver
const http_host = "192.168.0.1"

// which port do we contact? can be changed for debug purposes
const http_port = 80

// what should the uploaded firmware be called? probably doesn't matter
const firmware_filename = "factory-to-ddwrt.bin"

// maximum size of the payload for a packet
const payload_size = 1024

// do we want to debug incoming packets? probably not
const recv_debug = false

// should we care about windowsize when streaming? router says no
const wsize_matters = false

// should we slow down transfers when streaming (may solve some problems we don't want to deal with)? 0 = off
const slow_xfers = 300 * time.Microsecond

func main() {
	var no_iptables bool
	var debug bool

	flag.Usage = func() {
		fmt.Printf("usage: %s [-no-iptables] [-debug] <src> <dst> <firmware file>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.BoolVar(&no_iptables, "no-iptables", false, "set to disable automatic iptables rules")
	flag.BoolVar(&debug, "debug", false, "set to enable debug logging")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	args := flag.Args()
	if len(args) != 3 {
		flag.Usage()
		os.Exit(2)
	}
	src_host := args[0]
	dst_host := args[1]
	firmware_filename := args[2]

	ctrlc := make(chan os.Signal, 1)
	signal.Notify(ctrlc, os.Interrupt)
	go func() {
		<-ctrlc
		logrus.Warnf("got ctrl+c, aborting")
		teardown_iptables()
		os.Exit(1)
	}()

	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.999",
		FullTimestamp:   true,
	})

	logrus.Infof("src_host=%v, dst_host=%v, firmware_filename=%v", src_host, dst_host, firmware_filename)
	disclaimer()

	if !no_iptables {
		setup_iptables()
		defer teardown_iptables()
	}

	do_firmware(src_host, dst_host, http_port, http_host, firmware_filename)

	teardown_iptables()
	dlink_progressbar()
	logrus.Infof("i'm done here!")
}

func do_firmware(src_host, dst_host string, http_port int, http_host string, firmware_filename string) {
	do_get(src_host, dst_host, http_port, http_host)
	r := ask("check response above and press enter to continue, ctrl+c to abort")
	if r != "" {
		logrus.Errorf("i don't want a reply")
		os.Exit(1)
	}

	do_post(src_host, dst_host, http_port, http_host, firmware_filename)
}

func do_get(src_host, dst_host string, port int, http_host string) {
	logrus.Infof("http: GET /")

	c := Connect(src_host, dst_host, port)
	c.Send(build_get_request(http_host), true)
	bdata := c.ReadUntilClose(true)
	c.Close()

	handle_http_reply(bdata)
}

func do_post(src_host, dst_host string, port int, http_host string, firmware_filename string) {
	logrus.Infof("http: POST /")
	post_header, post_body := build_post_request(http_host, firmware_filename)

	c := Connect(src_host, dst_host, port)
	t0 := time.Now()
	c.Send(post_header, true)
	c.Stream(bytes.NewBuffer(post_body), len(post_body), wsize_matters, slow_xfers)
	bdata := c.ReadUntilClose(true)
	c.Close()

	logrus.Infof("upload completed in %v", time.Since(t0))
	handle_http_reply(bdata)
}

func handle_http_reply(bdata []byte) {
	if len(bdata) == 0 {
		logrus.Panicf("got empty reply from server, bailing out")
	}

	data := string(bdata)
	i := strings.Index(string(data), "\r\n")
	if i < 4 {
		logrus.Panicf("got unexpected reply from server: [%s]", strings.TrimSpace(data))
	}

	logrus.Infof("response: %s", data[:i])
}
