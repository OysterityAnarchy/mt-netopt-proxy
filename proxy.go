package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/anon55555/mt/rudp"
)

type Conn struct {
	mu    sync.Mutex
	lists map[string][][]byte
}

func (c *Conn) proxy(src, dest *rudp.Peer) {
	for {
		pkt, err := src.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			continue
		}

		if len(pkt.Data) < 2 {
			continue
		}
		cmd := binary.BigEndian.Uint16(pkt.Data)

		c.mu.Lock()
		switch {
		case !src.IsSrv() && cmd == 49:
			c.invAct(string(pkt.Data[2:]))
		case src.IsSrv() && cmd == 39:
			var b bytes.Buffer
			b.Write(pkt.Data[:2])
			c.keep(&b, pkt.Data[2:])
			pkt.Data = b.Bytes()
		}
		c.mu.Unlock()

		dest.Send(pkt)
	}

	dest.SendDisco(0, true)
	dest.Close()
}

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

		c := &Conn{lists: make(map[string][][]byte)}
		go c.proxy(clt, srv)
		go c.proxy(srv, clt)
	}
}

func (c *Conn) keep(b *bytes.Buffer, inv []byte) {
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
				if i < len(c.lists[nm]) && bytes.Equal(stks[i], c.lists[nm][i]) {
					b.WriteString("Keep\n")
				} else {
					b.Write(stks[i])
				}
			}
			c.lists[nm] = stks
		}
	}
}

func (c *Conn) invAct(act string) {
	defer func() {
		recover() // Don't crash if a slice is indexed out of bounds.
	}()

	dirty := func(loc []string) {
		if loc[0] == "current_player" {
			i, err := strconv.Atoi(loc[2])
			if err != nil {
				return
			}
			c.lists[loc[1]][i] = nil
		}
	}

	switch f := strings.Split(act, " "); f[0] {
	case "Craft":
		delete(c.lists, "craft")
		delete(c.lists, "craftresult")
	case "Drop":
		dirty(f[2:])
	case "Move":
		dirty(f[2:])
		dirty(f[5:])
	case "MoveSomewhere":
		dirty(f[2:])
		if f[5] == "current_player" {
			delete(c.lists, f[6])
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
