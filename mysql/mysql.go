package mysql

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"sync"

	"io"

	"github.com/Salpadding/l7dump/core"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var (
	_ core.ProtocolTracker     = (*Tracker)(nil)
	_ core.ProtocolConnTracker = (*ConnTracker)(nil)

	_ ProtoPacket = (ServerHandShake)(nil)
)

// ProtoPacket mysql 协议规定的包类型
type ProtoPacket interface {
	Type() string
}

type ServerHandShake []byte

func (s ServerHandShake) Type() string {
	return "ServerHandShake"
}

type RawPacket struct {
	conn  io.Reader
	size  int
	hdr   [4]byte
	seqId int
	cont  bool
}

// Reset 重置状态
func (packet *RawPacket) Reset() {
	packet.size = 0
	packet.cont = true
}

// Packets documentation:
// http://dev.mysql.com/doc/internals/en/client-server-protocol.html
// 3 + 1 + n
func (packet *RawPacket) Read(p []byte) (n int, err error) {
	if packet.size == 0 {
		if !packet.cont {
			return 0, io.EOF
		}

		packet.cont = false
		// 跳过头部
		n, err = io.ReadAtLeast(packet.conn, packet.hdr[:], 4)
		if err != nil {
			return
		}

		packet.seqId = int(packet.hdr[3])
		packet.hdr[3] = 0
		packet.size = int(binary.LittleEndian.Uint32(packet.hdr[:]))

		if packet.size == 0 {
			return 0, io.EOF
		}

		// 表示还有剩余
		if packet.size == 0xffffff {
			packet.cont = true
		}
	}

	minLen := func() int {
		if packet.size < len(p) {
			return packet.size
		}
		return len(p)
	}()

	n, err = packet.conn.Read(p[:minLen])
	packet.size -= n

	return
}

type ConnTracker struct {
	meta       core.ConnMeta
	respPacket RawPacket

	reqPacket RawPacket

	handshake ServerHandShake
	conns     sync.Map

	ServerMeta struct {
		Protocol int
		Version  string
	}
}

func (c *ConnTracker) DecodeReq() (interface{}, error) {
	tcpreader.DiscardBytesToEOF(c.reqPacket.conn)
	return nil, nil
}

func (c *ConnTracker) DecodeResp() (val interface{}, err error) {
	c.respPacket.Reset()
	if c.handshake == nil {
		c.handshake, err = io.ReadAll(&c.respPacket)
		if err != nil {
			return
		}
		val = c.handshake
		c.parseHandShake()
		fmt.Println(c.ServerMeta.Version)
		return
	}
	tcpreader.DiscardBytesToEOF(&c.respPacket)
	return nil, nil
}

// parseHandShake 解析服务端握手消息
func (c *ConnTracker) parseHandShake() {
	c.ServerMeta.Protocol = int(c.handshake[0])

	pos := 1
	for c.handshake[pos] != 0 {
		pos++
	}

	c.ServerMeta.Version = string(c.handshake[1:pos])
}

// Tracker
// mysql client 连接时候设置 --ssl-mode=DISABLED 关闭ssl
type Tracker struct {
}

func (m *Tracker) RequestDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	c := conn.(*ConnTracker)
	c.reqPacket.conn = bufio.NewReader(stream)
	return c.DecodeReq
}

func (m *Tracker) ResponseDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	c := conn.(*ConnTracker)
	c.respPacket.conn = bufio.NewReader(stream)
	return c.DecodeResp
}

func (m *Tracker) NewConnect(meta core.ConnMeta) core.ProtocolConnTracker {
	return &ConnTracker{
		meta: meta,
	}
}

func (m *Tracker) OnClose(core.ProtocolConnTracker) {
	return
}

func (m *ConnTracker) OnRequest(req interface{}) error {
	return nil
}

func (m *ConnTracker) OnResponse(resp interface{}) error {
	return nil
}

func (m *ConnTracker) OnError(err error) {
	fmt.Printf("%v\n", err)
}
