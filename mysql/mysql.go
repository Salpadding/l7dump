package mysql

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"io"

	"github.com/Salpadding/l7dump/core"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var (
	_ core.ProtocolTracker     = (*Tracker)(nil)
	_ core.ProtocolConnTracker = (*ConnTracker)(nil)

	_ ProtoPacket = (*ServerHandShake)(nil)
)

// ProtoPacket mysql 协议规定的包类型
type ProtoPacket interface {
	Type() string
	parse() error
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

	ServerHandshake *ServerHandShake

	ClientHandShake *ClientHandShake
}

// DecodeReq
func (c *ConnTracker) DecodeReq() (val interface{}, err error) {
	defer c.reqPacket.Close()
	// 跳过握手阶段
	if c.ClientHandShake == nil {
		c.ClientHandShake = &ClientHandShake{
			Parser: Parser{
				Reader: &c.reqPacket,
			},
		}

		err = c.ClientHandShake.parse()

		if err != nil {
			fmt.Printf("parse client handshake failed %v\n", err)
		} else {
			fmt.Printf("client user = %s\n", c.ClientHandShake.User)
		}
		return
	}

	var cmd [1]byte
	c.reqPacket.Read(cmd[:])

	// query 纯文本
	if cmd[0] == comQuery {
		// 打印 sql 语句
		io.Copy(os.Stdout, &c.reqPacket)
		fmt.Println()
		return nil, nil
	}

	// stmt
	io.Copy(io.Discard, &c.reqPacket)
	return nil, nil
}

func (c *ConnTracker) DecodeResp() (val interface{}, err error) {
	defer c.respPacket.Close()
	if c.ServerHandshake == nil {
		c.ServerHandshake = &ServerHandShake{
			Parser: Parser{
				Reader: &c.respPacket,
			},
		}
		err = c.ServerHandshake.parse()
		if err != nil {
			fmt.Printf("parse server handshake failed %v\n", err)
		} else {
			js, _ := json.Marshal(c.ServerHandshake)
			fmt.Printf("server = %s\n", string(js))
		}
		return
	}

	var cmd [1]byte
	c.respPacket.Read(cmd[:])

	if cmd[0] == iOK {

	}

	io.Copy(io.Discard, &c.reqPacket)
	return nil, nil
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

func (m *Tracker) NewConnect(meta *core.ConnMeta) core.ProtocolConnTracker {
	fmt.Printf("new mysql connect to %s\n", meta.String())
	return &ConnTracker{
		meta: meta,
		reqPacket: RawPacket{
			reading: true,
		},
		respPacket: RawPacket{
			reading: true,
		},
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
