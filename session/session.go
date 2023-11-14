package session

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"sync"

	"context"

	"github.com/Salpadding/l7dump/core"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

const (
	MinClientPort = 49152
)

type rwmap struct {
	data map[string]core.ProtocolConnTracker
	mtx  sync.RWMutex
}

func (r *rwmap) GetOrLoad(key string, compute func() core.ProtocolConnTracker) core.ProtocolConnTracker {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	ret, ok := r.data[key]
	if ok {
		return ret
	}

	r.data[key] = compute()
	return r.data[key]
}

func (r *rwmap) Delete(key string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	delete(r.data, key)
}

type ProtocolSessionMgr struct {
	Trackers map[int]core.ProtocolTracker
	connPool map[int]*rwmap
	ctx      context.Context
}

func NewMgr(ctx context.Context) ProtocolSessionMgr {
	return ProtocolSessionMgr{
		Trackers: make(map[int]core.ProtocolTracker),
		connPool: make(map[int]*rwmap),
		ctx:      ctx,
	}
}

func (p *ProtocolSessionMgr) AddTracker(port int, tracker core.ProtocolTracker) {
	fmt.Printf("add tracker at port %d\n", port)
	p.Trackers[port] = tracker
	p.connPool[port] = &rwmap{
		data: make(map[string]core.ProtocolConnTracker),
	}
}

type protocolConnTrackerWrapper struct {
	conn      core.ProtocolConnTracker
	tracker   core.ProtocolTracker
	decoder   func() (interface{}, error)
	handler   func(interface{}) error
	errHandle func(error)
	stream    tcpreader.ReaderStream
	meta      *core.ConnMeta
	connPool  *rwmap
}

func (s *protocolConnTrackerWrapper) run() {
	for {
		payload, err := s.decoder()
		if err == io.EOF {
			s.tracker.OnClose(s.conn)
			s.connPool.Delete(s.meta.String())
			return
		}
		if err != nil {
			s.errHandle(err)
			continue
		}
		s.handler(payload)
	}
}

type noop struct {
}

var nop noop

func (s *noop) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	return s
}
func (s *noop) Reassembled([]tcpassembly.Reassembly) {
}
func (s *noop) ReassemblyComplete() {
}

func (s *ProtocolSessionMgr) getConnect(meta *core.ConnMeta, tk core.ProtocolTracker) core.ProtocolConnTracker {
	m, ok := s.connPool[meta.ServerPort]
	if !ok {
		panic(fmt.Sprintf("port %d doesn't related to tracker", meta.ServerPort))
	}
	return m.GetOrLoad(meta.String(), func() core.ProtocolConnTracker {
		return tk.NewConnect(meta)
	})
}

// newWrapper
// connTracker 和 wrapper 是 1:2 的关系
func (s *ProtocolSessionMgr) newWrapper(meta *core.ConnMeta, isReq bool, net, transport gopacket.Flow) protocolConnTrackerWrapper {
	tracker := s.Trackers[meta.ServerPort]
	conn := s.getConnect(meta, tracker)
	wrapper := protocolConnTrackerWrapper{
		tracker:   tracker,
		conn:      conn,
		errHandle: conn.OnError,
		stream:    tcpreader.NewReaderStream(),
		meta:      meta,
		connPool:  s.connPool[meta.ServerPort],
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

// New 只是实现接口
func (s *ProtocolSessionMgr) New(net, transport gopacket.Flow) tcpassembly.Stream {
	meta, isReq := s.connKey(net, transport)
	if meta.ClientPort < MinClientPort {
		fmt.Printf("drop connection %s\n", meta.String())
		return &nop
	}
	wrapper := s.newWrapper(&meta, isReq, net, transport)
	go wrapper.run()
	return &wrapper.stream
}

// connKey 客户端的端口一定大于服务端的端口
// pcap 里面客户端到服务端 服务端到客户端 会触发两次 ServerSessionMgr.New
// 我们通过这个 key 判断是否是新的连接
func (s *ProtocolSessionMgr) connKey(netFlow, transportFlow gopacket.Flow) (core.ConnMeta, bool) {
	ip1 := (net.IP)(netFlow.Src().Raw())
	ip2 := (net.IP)(netFlow.Dst().Raw())
	port1 := int(binary.BigEndian.Uint16(transportFlow.Src().Raw()))
	port2 := int(binary.BigEndian.Uint16(transportFlow.Dst().Raw()))
	if port1 > port2 {
		return core.ConnMeta{
			ClientIP:   ip1,
			ClientPort: port1,
			ServerIP:   ip2,
			ServerPort: port2,
		}, true
	}
	return core.ConnMeta{
		ClientIP:   ip2,
		ClientPort: port2,
		ServerIP:   ip1,
		ServerPort: port1,
	}, false

}

// Listen
// TODO: 复用 bpf 表达式
func (s *ProtocolSessionMgr) Listen(bpf, iface string) error {
	// 以太网 MTU 通常小于 1600
	handle, err := pcap.OpenLive(iface, 1600, true, pcap.BlockForever)

	if err != nil {
		return err
	}

	// TODO: 多端口复用同一个 bpffilter
	// 只保留 ip.protocol = tcp 的 而且 tcp.port = serverPort 的 包
	fmt.Printf("create listener at interface %s with filter %s\n", iface, bpf)
	if err = handle.SetBPFFilter(bpf); err != nil {
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
		case <-s.ctx.Done():
			return nil
		case <-ticker:
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -2))
		}
	}
}
