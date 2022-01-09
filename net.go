package main

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

var cnx_sport = 56312

type Cnx_t struct {
	conn     net.PacketConn
	src_ip   net.IP
	dst_ip   net.IP
	src_port int
	dst_port int

	read_lock           *sync.Mutex
	read_queue          []*layers.TCP
	read_running_atomic uint32
	read_wg             *sync.WaitGroup

	connected       bool
	next_seq        uint32
	next_ack_atomic uint32
}

func Connect(src_host, dst_host string, port int) *Cnx_t {
	conn, e := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if e != nil {
		logrus.Panicf("connect failed: %v", e)
	}

	c := &Cnx_t{
		conn:       conn,
		src_ip:     host_to_ip(src_host),
		dst_ip:     host_to_ip(dst_host),
		src_port:   cnx_sport,
		dst_port:   port,
		next_seq:   3702297675,
		read_lock:  &sync.Mutex{},
		read_queue: make([]*layers.TCP, 0),
		read_wg:    &sync.WaitGroup{},
	}
	cnx_sport += 3

	c.listen()
	c.do_handshake()

	return c
}

func (c *Cnx_t) Close() {
	if c.connected {
		c.disconnect()
	}
	c.close_internal()
}

func (c *Cnx_t) Send(data []byte, psh bool) {
	tcp := c.tcp_packet()
	tcp.ACK = true
	tcp.Ack = atomic.LoadUint32(&c.next_ack_atomic)
	//tcp.ACK, tcp.Ack = c.get_ack()
	if psh {
		tcp.PSH = true
	}
	c.send_tcp(tcp, data)
	c.next_seq += uint32(len(data))
}

func (c *Cnx_t) Stream(r io.Reader, data_len int, care_about_wsize bool, xfer_sleep time.Duration) {
	full_buf := make([]byte, payload_size)
	wsize := len(full_buf)
	sent_bytes := 0
	p := progressbar.DefaultBytes(int64(data_len), "Upload")
	for {
		buf := full_buf[:wsize]

		n, e := r.Read(buf)
		if e == io.EOF || n == 0 {
			break
		} else if e != nil {
			c.Close()
			logrus.Panicf("stream failed: %v", e)
		}

		c.Send(buf[:n], false)
		sent_bytes += n
		p.Add(n)

		tcps := c.PollAll()
		for _, tcp := range tcps {
			if tcp.FIN {
				c.peer_disconnect()
				c.Close()
				logrus.Panicf("stream failed: peer disconnected")
			}

			if tcp.RST {
				c.close_internal()
				logrus.Panicf("stream failed: connection reset")
			}

			if care_about_wsize && int(tcp.Window) < len(full_buf) { // the router doesn't care about scaling, don't hate the player
				wsize = int(tcp.Window)
			}
		}
		if xfer_sleep > 0 {
			time.Sleep(xfer_sleep)
		}
	}
	p.Close()
}

func (c *Cnx_t) Poll() *layers.TCP {
	c.read_lock.Lock()
	defer c.read_lock.Unlock()
	if len(c.read_queue) == 0 {
		return nil
	}

	p := c.read_queue[0]
	c.read_queue = c.read_queue[1:]
	return p
}

func (c *Cnx_t) PollAll() []*layers.TCP {
	c.read_lock.Lock()
	ret := c.read_queue
	c.read_queue = []*layers.TCP{}
	c.read_lock.Unlock()
	return ret
}

func (c *Cnx_t) ReadUntilClose(ack bool) []byte {
	data := []byte{}
	for {
		tcp := c.Poll()
		if tcp == nil {
			continue
		}

		if tcp.RST {
			c.close_internal()
			logrus.Panicf("readuntilclose failed: connection reset")
		}

		if len(tcp.Payload) > 0 {
			data = append(data, tcp.Payload...)
		}

		if tcp.FIN {
			c.peer_disconnect_dlink_style()
			break
		}

		if ack {
			tcp_ack := c.tcp_packet()
			tcp_ack.ACK = true
			tcp_ack.Ack = tcp.Seq + uint32(len(tcp.Payload))
			c.send_tcp(tcp_ack, nil)
		}
	}
	return data
}

func (c *Cnx_t) close_internal() {
	c.connected = false

	if atomic.LoadUint32(&c.read_running_atomic) > 0 {
		atomic.StoreUint32(&c.read_running_atomic, 0)
		c.read_wg.Wait()
	}

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *Cnx_t) do_handshake() {
	{ // send syn
		tcp := c.tcp_packet()
		tcp.SYN = true
		tcp.Window = 65535
		tcp.Options = []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 4,
				OptionData:   []byte{0x05, 0xb4}, // 1460
			},
			{
				OptionType: layers.TCPOptionKindNop,
			},
			{
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionLength: 3,
				OptionData:   []byte{8},
			},
			{
				OptionType: layers.TCPOptionKindNop,
			},
			{
				OptionType: layers.TCPOptionKindNop,
			},
			{
				OptionType:   layers.TCPOptionKindSACKPermitted,
				OptionLength: 2,
			},
		}
		c.send_tcp(tcp, nil)
		c.next_seq++
	}

	var ack uint32
	{ // wait for syn,ack
		for {
			tcp := c.Poll()
			if tcp == nil {
				continue
			}
			if tcp.RST {
				c.close_internal()
				logrus.Panicf("do_handshake: connection reset")
			}
			if tcp.SYN && tcp.ACK {
				ack = tcp.Seq + 1
				break
			}
		}
	}

	{ // send ack
		tcp := c.tcp_packet()
		tcp.ACK = true
		tcp.Ack = ack
		c.send_tcp(tcp, nil)
	}

	c.connected = true
	logrus.Infof("connected to '%v:%d'", c.dst_ip, c.dst_port)
}

func (c *Cnx_t) disconnect() {
	{ // send fin,ack
		tcp := c.tcp_packet()
		tcp.FIN = true
		tcp.ACK, tcp.Ack = c.get_ack()
		c.send_tcp(tcp, nil)
		c.next_seq++
	}

	var ack uint32
	{ // wait for fin,ack (or rst)
		for {
			tcp := c.Poll()
			if tcp == nil {
				continue
			}

			if tcp.RST {
				logrus.Warnf("received rst while disconnecting")
				break
			}

			if tcp.FIN && tcp.ACK {
				ack = tcp.Seq + 1
				break
			}
		}
	}

	if ack > 0 { // send ack
		tcp := c.tcp_packet()
		tcp.ACK = true
		tcp.Ack = ack
		c.send_tcp(tcp, nil)
	}

	c.connected = false
	logrus.Infof("disconnected from '%v:%d", c.dst_ip, c.dst_port)
}

func (c *Cnx_t) peer_disconnect_dlink_style() {
	{ // ack
		tcp := c.tcp_packet()
		tcp.ACK, tcp.Ack = c.get_ack()
		tcp.Ack++
		c.send_tcp(tcp, nil)
	}

	{ // wait for ack resend
		for {
			tcp := c.Poll()
			if tcp != nil && tcp.ACK {
				break
			}
		}
	}

	{ // send fin,ack
		tcp := c.tcp_packet()
		tcp.FIN = true
		tcp.ACK, tcp.Ack = c.get_ack()
		c.send_tcp(tcp, nil)
	}

	c.connected = false
	logrus.Infof("peer disconnect from '%v:%d'", c.dst_ip, c.dst_port)
}

func (c *Cnx_t) peer_disconnect() {
	tcp := c.tcp_packet()
	tcp.FIN = true
	tcp.ACK, tcp.Ack = c.get_ack()
	tcp.Ack++
	c.send_tcp(tcp, nil)
	c.connected = false
	logrus.Infof("peer disconnect from '%v:%d'", c.dst_ip, c.dst_port)
}

func (c *Cnx_t) send_tcp(tcp *layers.TCP, payload []byte) {
	ip := c.ip_packet()
	packet := c.serialize_packet(ip, tcp, payload)
	_, e := c.conn.WriteTo(packet.Bytes(), &net.IPAddr{IP: c.dst_ip})
	if e != nil {
		c.Close()
		logrus.Panicf("send_tcp failed: %v", e)
	}
}

func (c *Cnx_t) get_ack() (bool, uint32) {
	/*
		c.read_lock.Lock()
		ack := atomic.LoadUint32(&c.next_ack_atomic)
		c.read_lock.Unlock()
	*/
	ack := atomic.LoadUint32(&c.next_ack_atomic)
	if ack > 0 {
		return true, ack
	}
	return false, 0
}

func (c *Cnx_t) listen() {
	atomic.StoreUint32(&c.read_running_atomic, 1)
	c.read_wg.Add(1)

	go func() {
		logrus.Debugf("packet listener started")
		for atomic.LoadUint32(&c.read_running_atomic) > 0 {
			buf := make([]byte, 8192)
			n, addr, e := c.conn.ReadFrom(buf)
			if e != nil {
				c.Close()
				logrus.Panicf("listener failed: %v", e)
			}

			if addr.String() != c.dst_ip.String() {
				continue // not in conversation
			}

			i := 0
			add := []*layers.TCP{}
			var set_ack uint32
			for {
				if i >= n {
					break
				}

				packet := gopacket.NewPacket(buf[i:n], layers.LayerTypeTCP, gopacket.Default)
				i += len(packet.Data())
				tcpLayer := packet.Layer(layers.LayerTypeTCP)
				if tcpLayer == nil {
					continue // we only do tcp
				}

				tcp, _ := tcpLayer.(*layers.TCP)
				if tcp.DstPort != layers.TCPPort(c.src_port) || tcp.SrcPort != layers.TCPPort(c.dst_port) {
					continue // not in conversation
				}
				ack := tcp.Seq + uint32(len(tcp.Payload))
				if ack > set_ack {
					set_ack = ack
				}
				add = append(add, tcp)

				if tcp.RST {
					logrus.Warnf("received a rst packet - did you read the instructions?")
				}

				if recv_debug && logrus.GetLevel() <= logrus.DebugLevel {
					flags := []string{}
					if tcp.RST {
						flags = append(flags, "rst")
					}
					if tcp.SYN {
						flags = append(flags, "syn")
					}
					if tcp.ACK {
						flags = append(flags, "ack")
					}
					if tcp.FIN {
						flags = append(flags, "fin")
					}
					if tcp.PSH {
						flags = append(flags, "psh")
					}
					logrus.Debugf("recv packet: flags=%v, seq=%v, ack=%v, payload=%d bytes, wsize=%d", flags, tcp.Seq, tcp.Ack, len(tcp.Payload), tcp.Window)
				}
			}

			if len(add) > 0 {
				c.read_lock.Lock()
				if set_ack > atomic.LoadUint32(&c.next_ack_atomic) {
					atomic.StoreUint32(&c.next_ack_atomic, set_ack)
				}
				c.read_queue = append(c.read_queue, add...)
				c.read_lock.Unlock()
			}
		}
		logrus.Debugf("packet listener stopped")
		c.read_wg.Done()
	}()
}

func (c *Cnx_t) serialize_packet(ip *layers.IPv4, tcp *layers.TCP, payload []byte) gopacket.SerializeBuffer {
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	layers := []gopacket.SerializableLayer{tcp}
	if payload != nil {
		layers = append(layers, gopacket.Payload(payload))
	}
	if err := gopacket.SerializeLayers(buf, opts, layers...); err != nil {
		c.Close()
		logrus.Panicf("serialize_packet failed: %v", err)
	}
	return buf
}

func (c *Cnx_t) tcp_packet() *layers.TCP {
	window_size := uint16(65535)

	return &layers.TCP{
		SrcPort: layers.TCPPort(c.src_port),
		DstPort: layers.TCPPort(c.dst_port),
		Seq:     c.next_seq,
		Window:  window_size,
	}
}

func (c *Cnx_t) ip_packet() *layers.IPv4 {
	return &layers.IPv4{
		SrcIP:    c.src_ip,
		DstIP:    c.dst_ip,
		Protocol: layers.IPProtocolTCP,
		TTL:      127,
	}
}

func host_to_ip(host string) net.IP {
	addrs, e := net.LookupIP(host)
	if e != nil {
		logrus.Panicf("host_to_ip failed: %v", e)
	}
	return addrs[0].To4()
}
