/*
MIT License

Copyright (c) 2021 anon5

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"

	"github.com/anon55555/mt/rudp"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy dial:port listen:port")
		os.Exit(1)
	}

	srvaddr, err := net.ResolveUDPAddr("udp", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	lc, err := net.ListenPacket("udp", os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer lc.Close()

	l := rudp.Listen(lc)
	for {
		clt, err := l.Accept()
		if err != nil {
			continue
		}

		conn, err := net.DialUDP("udp", nil, srvaddr)
		if err != nil {
			clt.Close()
			continue
		}
		srv := rudp.Connect(conn, conn.RemoteAddr())

		go proxy(clt, srv)
		go proxy(srv, clt)
	}
}

func proxy(src, dest *rudp.Peer) {
	invLists := make(map[string][]string)

	for {
		pkt, err := src.Recv()
		if err != nil {
			if err == rudp.ErrClosed {
				break
			}
			continue
		}

		if src.IsSrv() && len(pkt.Data) >= 2 && pkt.Data[0] == 0 && pkt.Data[1] == 39 {
			pkt.Data = append(pkt.Data[:2], keep(string(pkt.Data[2:]), invLists)...)
		}

		dest.Send(pkt)
	}

	dest.SendDisco(0, true)
	dest.Close()
}

var InvListRe = regexp.MustCompile("(?m)^(List ([^ ]*).*\nWidth .*\n)((Empty\n|Item .*\n)*)EndInventoryList\n")

func getln(p *string) string {
	var ln []byte
	for c := byte(0); c != '\n' && len(*p) > 0; *p = (*p)[1:] {
		c = (*p)[0]
		ln = append(ln, c)
	}
	return string(ln)
}

func keep(inv string, lists map[string][]string) string {
	return InvListRe.ReplaceAllStringFunc(inv, func(list string) string {
		match := InvListRe.FindStringSubmatch(list)

		newinv := match[1]

		var items []string
		for {
			ln := getln(&match[3])
			if len(ln) == 0 {
				break
			}
			items = append(items, ln)
		}

		olditems := lists[match[2]]
		for i := range items {
			if i < len(olditems) && items[i] == olditems[i] {
				newinv += "Keep\n"
				continue
			}
			newinv += items[i]
		}
		lists[match[2]] = items

		newinv += "EndInventoryList\n"
		return newinv
	})
}