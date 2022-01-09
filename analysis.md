# DISCLAIMER (again)
The provided [pcap](router.pcap) contains firmware for the DIR-882 from DD-WRT, `v3.0 [Beta] Build 44715` to be specific. **The pcap is only provided for documentation**, and you should download it from <https://dd-wrt.com/> if you wish to use it.

# The analysis
So my initial thoughts was that this is some HTTP stuff, and the router might be very susceptible to some quirks only found in IE.

The interesting thing is that when trying to upload using anything else will fail at the TCP handshake. The GET-request will work, but the POST-request will never receive the SYN,ACK.

So I performed an upload using IE and captured the entire exchange using wireshark to see what was happening in an attempt to recreate it. And there's some weird things happening.

Also worth pointing out at this stage is that it's impossible to make two subsequent GET-requests even using IE. I think the router changes "mode" into some kind of "firmware-upload-mode" after the first GET-request.

After a successful upload we get a webpage with a progressbar. This is entirely client side and has nothing to do with the flashing process, it is just to keep people from messing with the router while it's doing it's thing. The progressbar will work for 220s (3m40s).

## So, to the quirks.

I'm not sure which IP and TCP flags are actually required, so I have tried to recreate them as closely as possible, most of them can be found below. The real kicker, and what I struggled for a bit with, was the disconnect handshake in the GET request. I implemented this correctly, and this broke my head for a while, and is probably the reason why all browsers fail.

A normal TCP disconnect handshake will look like this,
- Server: FIN,ACK
- Client: FIN,ACK
- Server: ACK

The D-Link/IE handshake looks like this,
- Server: FIN,ACK
- Client: ACK
- Server: ACK
- Client: FIN,ACK
- Server: ACK

There's an extra ACK/ACK pair in there for good sport. This extra ACK packet is what allows us to actually upload firmware. I think the router is singlethreaded, and without the "broken" handshake at the end of the GET-request I think the router is stuck waiting for the disconnect to end.

Another interesting finding is that the router sends the text "HTTP/1" as a packet padding sometimes. It is not part of the TCP payload, it's just.. there.. I have no idea why.

The upload takes about 5.3s using IE, so I have tried to recreate the same packetrate.

(I love the fact that the HTML pages actually have a `DOCTYPE` header, as if anyone involved with this router cared about standards. Kudos for not writing overly complicated JavaScript though.)

## Packets
### GET /
| # | Sender | Flags | Notes |
|-|-|-|-|
1 | Client | SYN | See "Client SYN" below
2 | S | SYN, ACK | See "Server SYN,ACK" below
3 | Client | ACK |
4 | Client | PSH, ACK | See `HTTP GET /` below
5 | S | ACK | This packet contains the `index.html`
6 | Client | ACK | 
7 | S | FIN, ACK | Disconnect handshake, padded with "HTTP/1", weird?
8 | Client | ACK | Extra ACK from client
9 | S | ACK | Extra ACK from server
10 | Client | FIN, ACK |
11 | S | ACK  | 
| | | | |

### POST /
| | | | |
|-|-|-|-|
12 | Client | SYN | See "Client SYN" below
13 | S | SYN, ACK | See "Server SYN,ACK" below
14 | Client | ACK | 
15 | Client | PSH, ACK | First 426 bytes of the POST request, see `HTTP POST /` below
16 | S | ACK | 
17 | Client | ACK | Payload is next 1024 bytes of the POST request - wireshark notes that the server window is full (1024 bytes)
18 | S | ACK | 
.. | .. | .. | #17 and #18 is pretty much what's happening for a while
31169 | Client | PSH, ACK | Last 814 bytes of POST request
31170 | S | ACK | This packet contains the `success.html`
31171 | Client | ACK | 
31172 | S | FIN, ACK | Disconnect handshake, again, padded with "HTTP/1", why?
31173 | Client | ACK | Extra ACK from client
31174 | S | ACK | Extra ACK from server
31175 | Client | FIN, ACK | 
31176 | S | ACK | Again with the "HTTP/1" padding, srsly?
| | | | |

### Client SYN
| | |
|-|-|
| ip.ttl | 127 |
| tcp.window_size | 65535 |
| tcp.options[] | kind=mss, len=4, value=0x05b4 (1460)
| tcp.options[] | kind=nop
| tcp.options[] | kind=window scale, len=3, shift count=8
| tcp.options[] | kind=nop
| tcp.options[] | kind=nop
| tcp.options[] | kind=sack permitted, len=2
| | |

### Server SYN,ACK
| | |
|-|-|
| ip.ttl | 250 |
| tcp.window_size | 1024 |
| tcp.options[] | kind=mss, len=4, value=0x0578 (1400) |
| | |

## HTTP
### GET /
```
GET / HTTP/1.1
Accept: text/html, application/xhtml+xml, image/jxr, */*
Accept-Language: en-SE
User-Agent: Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko
Accept-Encoding: gzip, deflate
Host: 192.168.0.1
Connection: Keep-Alive
```

### POST /
Obviously `Content-Length` and the firmware data will change, depending on firmware. The rest is can be static. If the `filename` needs to change for some reason, there's a constant at the top of `main.go` that can be changed.
```
POST / HTTP/1.1
Accept: text/html, application/xhtml+xml, image/jxr, */*
Referer: http://192.168.0.1/
Accept-Language: en-SE
User-Agent: Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko
Content-Type: multipart/form-data; boundary=---------------------------7e62221510336
Accept-Encoding: gzip, deflate
Host: 192.168.0.1
Content-Length: 15950638
Connection: Keep-Alive
Cache-Control: no-cache

-----------------------------7e62221510336
Content-Disposition: form-data; name="firmware"; filename="factory-to-ddwrt.bin"
Content-Type: application/octet-stream

(15950324 bytes of firmware data here)
-----------------------------7e62221510336
Content-Disposition: form-data; name="btn"

Upload
-----------------------------7e62221510336--
```
