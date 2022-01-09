package main

import (
	"bytes"
	"fmt"
	"os"
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
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "-debug" {
		logrus.SetLevel(logrus.DebugLevel)
		args = args[1:]
	}
	if len(args) != 3 || (len(args) > 0 && args[0][0] == '-') {
		fmt.Printf("usage: %s [-debug] <src> <dst> <firmware file> (e.g; %s 192.168.0.2 192.168.0.1 fw.bin)\n", os.Args[0], os.Args[0])
		os.Exit(1)
	}
	src_host := args[0]
	dst_host := args[1]
	firmware_filename := args[2]

	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.999",
		FullTimestamp:   true,
	})

	disclaimer()

	do_get(src_host, dst_host, http_port, http_host)
	r := ask("check response above and press enter to continue, ctrl+c to abort")
	if r != "" {
		logrus.Errorf("i don't want a reply")
		os.Exit(1)
	}

	do_post(src_host, dst_host, http_port, http_host, firmware_filename)
	ig(firmware_filename)

	dlink_progressbar()
	logrus.Infof("i'm done here!")
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
