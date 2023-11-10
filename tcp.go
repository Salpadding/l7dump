package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

type ProtocolConnTracker interface {
	OnRequest(req interface{}) error
	OnResponse(resp interface{}) error
	OnError(error)
}

// ProtocolTracker 追踪 C/S 架构的 TCP 连接
// 通常在服务端运行 以便于追踪所有客户端请求
type ProtocolTracker interface {
	ServerPort() int
	RequestDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error)
	ResponseDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error)

	GetConnect(ConnMeta) ProtocolConnTracker

	OnClose(ProtocolConnTracker)
}

type ProtocolSessionMgr struct {
	trackers map[int]ProtocolTracker
}

type protocolConnTrackerWrapper struct {
	conn      ProtocolConnTracker
	tracker   ProtocolTracker
	decoder   func() (interface{}, error)
	handler   func(interface{}) error
	errHandle func(error)
	stream    tcpreader.ReaderStream
}

func (s *protocolConnTrackerWrapper) run() {
	for {
		payload, err := s.decoder()
		if err == io.EOF {
			s.tracker.OnClose(s.conn)
			return
		}
		if err != nil {
			s.errHandle(err)
			continue
		}
		s.handler(payload)
	}
}

func (s *ProtocolSessionMgr) newWrapper(net, transport gopacket.Flow) protocolConnTrackerWrapper {
	meta, isReq := s.connKey(net, transport)
	tracker := s.trackers[meta.ServerPort]
	conn := tracker.GetConnect(meta)
	wrapper := protocolConnTrackerWrapper{
		tracker:   tracker,
		conn:      conn,
		errHandle: conn.OnError,
		stream:    tcpreader.NewReaderStream(),
	}

	if isReq {
		wrapper.decoder = tracker.RequestDecoder(&wrapper.stream, conn)
		wrapper.handler = conn.OnRequest
	} else {
		wrapper.decoder = tracker.ResponseDecoder(&wrapper.stream, conn)
		wrapper.handler = conn.OnResponse
	}
	return wrapper
}

// New
func (s *ProtocolSessionMgr) New(net, transport gopacket.Flow) tcpassembly.Stream {
	wrapper := s.newWrapper(net, transport)
	go wrapper.run()
	return &wrapper.stream
}

type ConnMeta struct {
	ClientIP   net.IP
	ServerIP   net.IP
	ClientPort int
	ServerPort int
}

func (c *ConnMeta) String() string {
	return fmt.Sprintf("%s:%d %s:%d", c.ClientIP.String(), c.ClientPort, c.ServerIP.String(), c.ServerPort)
}

// connKey 客户端的端口一定大于服务端的端口
// pcap 里面客户端到服务端 服务端到客户端 会触发两次 ServerSessionMgr.New
// 我们通过这个 key 判断是否是新的连接
func (s *ProtocolSessionMgr) connKey(netFlow, transportFlow gopacket.Flow) (ConnMeta, bool) {
	ip1 := (net.IP)(netFlow.Src().Raw())
	ip2 := (net.IP)(netFlow.Dst().Raw())
	port1 := int(binary.BigEndian.Uint16(transportFlow.Src().Raw()))
	port2 := int(binary.BigEndian.Uint16(transportFlow.Dst().Raw()))
	if port1 > port2 {
		return ConnMeta{
			ClientIP:   ip1,
			ClientPort: port1,
			ServerIP:   ip2,
			ServerPort: port2,
		}, true
	}
	return ConnMeta{
		ClientIP:   ip2,
		ClientPort: port2,
		ServerIP:   ip1,
		ServerPort: port1,
	}, false

}

func (s *ProtocolSessionMgr) Listen(iface string) error {
	// 以太网 MTU 通常小于 1600
	handle, err := pcap.OpenLive("en0", 1600, true, pcap.BlockForever)

	if err != nil {
		return err
	}

	// TODO: 多端口复用同一个 bpffilter
	// 只保留 ip.protocol = tcp 的 而且 tcp.port = serverPort 的 包
	if err = handle.SetBPFFilter(fmt.Sprintf("tcp and port %d", 80)); err != nil {
		panic(err)
	}

	pool := tcpassembly.NewStreamPool(s)
	assembler := tcpassembly.NewAssembler(pool)

	source := gopacket.NewPacketSource(handle, handle.LinkType())
	ticker := time.Tick(time.Minute)
	packets := source.Packets()

	for {
		select {
		case packet := <-packets:
			if packet == nil {
				return nil
			}
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil ||
				packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				continue
			}

			tcp := packet.TransportLayer().(*layers.TCP)
			assembler.AssembleWithTimestamp(packet.NetworkLayer().NetworkFlow(), tcp, packet.Metadata().Timestamp)
		case <-ticker:
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -2))
		}
	}
}