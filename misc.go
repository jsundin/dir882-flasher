package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

func dlink_progressbar() {
	logrus.Infof("the upload is complete, and it is safe to ctrl+c if you like, however...")
	logrus.Warnf("DO NOT TURN OFF YOUR ROUTER!")
	n := 220
	p := progressbar.NewOptions(n, progressbar.OptionFullWidth())
	p.Describe("Chill time")
	for {
		time.Sleep(1 * time.Second)
		p.Add(1)
		n--
		if n == 0 {
			break
		}
	}
	p.Close()
	fmt.Println()
}

func ask(question string) string {
	fmt.Printf(">> %s: ", question)

	b := make([]byte, 64)
	n, _ := os.Stdin.Read(b)
	return strings.TrimSpace(string(b[:n]))
}

func disclaimer() {
	if os.Getenv("IUNDERSTAND") == "yes" {
		return
	}
	logrus.Warnf("THIS MAY BREAK YOUR ROUTER - LOOK IN README FOR FULL DISCLAIMER!")
	a := ask("do you understand? (type 'i understand')")
	if a != "i understand" {
		logrus.Infof("responsible user found, quitting")
		os.Exit(0)
	}
}

func build_get_request(http_host string) []byte {
	body := "GET / HTTP/1.1\r\n"
	body += "Accept: text/html, application/xhtml+xml, image/jxr, */*\r\n"
	body += "Accept-Language: en-SE\r\n"
	body += "User-Agent: Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko\r\n"
	body += "Accept-Encoding: gzip, deflate\r\n"
	body += "Host: " + http_host + "\r\n"
	body += "Connection: Keep-Alive\r\n"
	body += "\r\n"
	return []byte(body)
}

func build_post_request(http_host, firmware_file string) ([]byte, []byte) {
	fw, err := ioutil.ReadFile(firmware_file)
	if err != nil {
		panic(err)
	}

	post_body := []byte("-----------------------------7e62221510336\r\n")
	post_body = append(post_body, []byte("Content-Disposition: form-data; name=\"firmware\"; filename=\""+firmware_filename+"\"\r\n")...)
	post_body = append(post_body, []byte("Content-Type: application/octet-stream\r\n")...)
	post_body = append(post_body, []byte("\r\n")...)
	post_body = append(post_body, fw...)
	post_body = append(post_body, []byte("\r\n")...)
	post_body = append(post_body, []byte("-----------------------------7e62221510336\r\n")...)
	post_body = append(post_body, []byte("Content-Disposition: form-data; name=\"btn\"\r\n")...)
	post_body = append(post_body, []byte("\r\n")...)
	post_body = append(post_body, []byte("Upload\r\n")...)
	post_body = append(post_body, []byte("-----------------------------7e62221510336--\r\n")...)

	post_header := "POST / HTTP/1.1\r\n"
	post_header += "Accept: text/html, application/xhtml+xml, image/jxr, */*\r\n"
	post_header += "Referer: http://" + http_host + "/\r\n"
	post_header += "Accept-Language: en-SE\r\n"
	post_header += "User-Agent: Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko\r\n"
	post_header += "Content-Type: multipart/form-data; boundary=---------------------------7e62221510336\r\n"
	post_header += "Accept-Encoding: gzip, deflate\r\n"
	post_header += "Host: " + http_host + "\r\n"
	post_header += "Content-Length: " + strconv.Itoa(len(post_body)) + "\r\n"
	post_header += "Connection: Keep-Alive\r\n"
	post_header += "Cache-Control: no-cache\r\n"
	post_header += "\r\n"

	return []byte(post_header), post_body
}

func ig(v ...interface{}) {}
