package tracker

import (
	"fmt"
	"net"

	"github.com/google/gopacket/tcpassembly/tcpreader"
)

type ConnMeta struct {
	ClientIP   net.IP
	ServerIP   net.IP
	ClientPort int
	ServerPort int
}

type ProtocolConnTracker interface {
	OnRequest(req interface{}) error
	OnResponse(resp interface{}) error
	OnError(error)
}

// ProtocolTracker 追踪 C/S 架构的 TCP 连接
// 通常在服务端运行 以便于追踪所有客户端请求
// 主要作用是管理连接池 解码
type ProtocolTracker interface {
	RequestDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error)
	ResponseDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error)

	GetConnect(ConnMeta) ProtocolConnTracker

	OnClose(ProtocolConnTracker)
}

func (c *ConnMeta) String() string {
	return fmt.Sprintf("%s:%d %s:%d", c.ClientIP, c.ClientPort, c.ServerIP, c.ServerPort)
}
