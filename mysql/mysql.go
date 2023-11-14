package mysql

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
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

type RawPacket struct {
	conn    io.Reader
	size    int
	hdr     [4]byte
	seqId   int
	reading bool // reading = true 表示还没有读取结束
}

// Close 重置状态
func (packet *RawPacket) Close() {
	packet.size = 0
	packet.reading = true
}

// Packets documentation:
// http://dev.mysql.com/doc/internals/en/client-server-protocol.html
// 3 + 1 + n
func (packet *RawPacket) Read(p []byte) (n int, err error) {
	if packet.size == 0 {
		if !packet.reading {
			return 0, io.EOF
		}

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
			packet.reading = true
		} else {
			packet.reading = false
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
	meta       *core.ConnMeta
	respPacket RawPacket

	reqPacket RawPacket

	handshake ServerHandShake
	conns     sync.Map

	ServerMeta struct {
		Protocol int
		Version  string
	}

	ClientMeta struct {
		HandshakeData []byte
	}
}

// DecodeReq
func (c *ConnTracker) DecodeReq() (val interface{}, err error) {
	defer c.reqPacket.Close()
	// 跳过握手阶段
	if c.ClientMeta.HandshakeData == nil {
		c.ClientMeta.HandshakeData, err = io.ReadAll(&c.reqPacket)
		return c.ClientMeta.HandshakeData, err
	}

	var cmd [1]byte
	c.reqPacket.Read(cmd[:])

	// 跳过非 query 包
	if cmd[0] != comQuery {
		tcpreader.DiscardBytesToEOF(&c.reqPacket)
		return nil, nil
	}

	// 打印 sql 语句
	io.Copy(os.Stdout, &c.reqPacket)
	fmt.Println()
	return nil, nil
}

func (c *ConnTracker) DecodeResp() (val interface{}, err error) {
	if c.handshake == nil {
		c.handshake, err = io.ReadAll(&c.respPacket)
		if err != nil {
			return
		}
		val = c.handshake
		c.handshake.parse(&c.ServerMeta.Version)
		fmt.Println(c.ServerMeta.Version)
		c.respPacket.Close()
		return
	}
	tcpreader.DiscardBytesToEOF(&c.respPacket)
	c.respPacket.Close()
	return nil, nil
}

// Tracker
// mysql client 连接时候设置 --ssl-mode=DISABLED 关闭ssl
type Tracker struct {
}

func (m *Tracker) RequestDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	c := conn.(*ConnTracker)
	c.reqPacket.conn = bufio.NewReader(stream)
	c.reqPacket.reading = true
	return c.DecodeReq
}

func (m *Tracker) ResponseDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	c := conn.(*ConnTracker)
	c.respPacket.conn = bufio.NewReader(stream)
	c.respPacket.reading = true
	return c.DecodeResp
}

func (m *Tracker) NewConnect(meta *core.ConnMeta) core.ProtocolConnTracker {
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
