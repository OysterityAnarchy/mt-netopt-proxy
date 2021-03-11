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
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

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
	invLists := make(map[string][][]byte)

	for {
		pkt, err := src.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			continue
		}

		if src.IsSrv() && len(pkt.Data) >= 2 && pkt.Data[0] == 0 && pkt.Data[1] == 39 {
			var b bytes.Buffer
			b.Write(pkt.Data[:2])
			keep(&b, pkt.Data[2:], invLists)
			pkt.Data = b.Bytes()
		}

		dest.Send(pkt)
	}

	dest.SendDisco(0, true)
	dest.Close()
}

func keep(b *bytes.Buffer, inv []byte, lists map[string][][]byte) {
	for {
		ln := getln(&inv)
		if len(ln) == 0 {
			break
		}
		b.Write(ln)

		if bytes.HasPrefix(ln, []byte("List ")) {
			var (
				nm string
				sz int
			)
			fmt.Sscanf(string(ln), "List %s %d", &nm, &sz)
			b.Write(getln(&inv)) // Width
			stks := make([][]byte, sz)
			for i := range stks {
				stks[i] = getln(&inv)
				if i < len(lists[nm]) && bytes.Equal(stks[i], lists[nm][i]) {
					b.WriteString("Keep\n")
				} else {
					b.Write(stks[i])
				}
			}
			lists[nm] = stks
		}
	}
}

func getln(p *[]byte) []byte {
	if i := bytes.IndexByte(*p, '\n'); i != -1 {
		defer func() {
			*p = (*p)[i+1:]
		}()
		return (*p)[:i+1]
	}

	defer func() {
		*p = nil
	}()
	return *p
}
