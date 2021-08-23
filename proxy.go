package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/anon55555/mt"
	"github.com/anon55555/mt/rudp"
)

var be = binary.BigEndian

type Conn struct {
	mu    sync.Mutex
	lists map[string][][]byte
}

func (c *Conn) proxy(src, dest *rudp.Conn) {
	for {
		pkt, err := src.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			continue
		}

		c.processPkt(src, dest, pkt)
	}

	if src.IsSrv() {
		if err := src.WhyClosed(); err != nil {
			mt.Peer{dest}.SendCmd(&mt.ToCltDisco{
				Reason: mt.Custom,
				Custom: err.Error(),
			})
		}
	}

	dest.Close()
}

func (c *Conn) processPkt(src, dest *rudp.Conn, pkt rudp.Pkt) {
	c.mu.Lock()
	defer c.mu.Unlock()

	buf := make([]byte, 2)
	_, err := io.ReadFull(pkt, buf)
	if err != nil {
		return
	}
	cmd := be.Uint16(buf)

	if src.IsSrv() {
		switch cmd {
		case 39:
			pkt.Reader = popen(c.keep, pkt)
		}
	} else {
		switch cmd {
		case 49:
			act, err := io.ReadAll(pkt)
			if err != nil {
				return
			}
			c.invAct(string(act))
			pkt.Reader = bytes.NewReader(act)
		}
	}

	pkt.Reader = io.MultiReader(bytes.NewReader(buf), pkt)

	dest.Send(pkt)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "dial:port listen:port")
		os.Exit(1)
	}

	pc, err := net.ListenPacket("udp", os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	l := rudp.Listen(pc)
	for {
		clt, err := l.Accept()
		if err != nil {
			continue
		}

		conn, err := net.Dial("udp", os.Args[1])
		if err != nil {
			mt.Peer{clt}.SendCmd(&mt.ToCltDisco{
				Reason: mt.Custom,
				Custom: err.Error(),
			})
			clt.Close()
			continue
		}
		srv := rudp.Connect(conn)

		c := &Conn{lists: make(map[string][][]byte)}
		go c.proxy(clt, srv)
		go c.proxy(srv, clt)
	}
}

func popen(f func(io.Writer, io.Reader), r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		f(pw, r)
		pw.Close()
	}()
	return pr
}

var keepLine = []byte("Keep\n")

func (c *Conn) keep(w io.Writer, r io.Reader) {
	inv, err := io.ReadAll(r)
	if err != nil {
		return
	}

	for {
		ln := getln(&inv)
		if len(ln) == 0 {
			break
		}
		w.Write(ln)

		if bytes.HasPrefix(ln, []byte("List ")) {
			var (
				nm string
				sz int
			)
			fmt.Sscanf(string(ln), "List %s %d", &nm, &sz)
			w.Write(getln(&inv)) // Width
			stks := make([][]byte, sz)
			for i := range stks {
				stks[i] = getln(&inv)
				if i < len(c.lists[nm]) && bytes.Equal(stks[i], c.lists[nm][i]) {
					w.Write(keepLine)
				} else {
					w.Write(stks[i])
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
