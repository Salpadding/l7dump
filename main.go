package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

const (
	expr = "tcp and dst port 80"
)

var (
	// Endpoint 在 layers 中定义
	ipv4                    = layers.EndpointIPv4
	_    tcpassembly.Stream = (*httpDecoder)(nil)
)

// httpDecoder 针对 http 协议进行解包
// assembler 作了封装处理
// 我们只需要负责解包 不需要关心 ip 层的分片 tcp 连接的状态等
type httpDecoder struct {
	// net 中记录了网络层的元数据 (源ip 目标ip 等)
	// transport 记录了传输层的元数据 (源端口 目标端口等)
	net, transport gopacket.Flow

	// tcp 数据流会被丢到这里面 我们从这里面读取就行了
	stream tcpreader.ReaderStream
}

// Ip 获取源ip 目标 ip
func (h *httpDecoder) Ip() (net.IP, net.IP) {
	if h.net.EndpointType() != ipv4 {
		return net.IPv4zero, net.IPv4zero
	}
	return (net.IP)(h.net.Src().Raw()), (net.IP)(h.net.Dst().Raw())

}

// Port 获取源端口 目标端口
func (h *httpDecoder) Port() (int, int) {
	return int(binary.BigEndian.Uint16(h.transport.Src().Raw())), int(binary.BigEndian.Uint16(h.transport.Dst().Raw()))
}

// Reassembled 代理模式
func (h *httpDecoder) Reassembled(data []tcpassembly.Reassembly) {
	h.stream.Reassembled(data)
}

// ReassemblyComplete 代理模式
func (h *httpDecoder) ReassemblyComplete() {
	h.stream.ReassemblyComplete()
}

func (h *httpDecoder) run() {
	// 加缓冲 减少阻塞
	buf := bufio.NewReader(&h.stream)

	for {
		// 把 tcp 数据流转成 http 报文
		req, err := http.ReadRequest(buf)
		// 连接断开了
		if err == io.EOF {
			return
		}

		if err != nil {
			continue
		}

		fmt.Printf("%s\n", req.Host)

		// 很多时候不需要解析 body
		// 例如 body 不是文本
		// 丢弃 不然会阻塞
		tcpreader.DiscardBytesToEOF(req.Body)
		req.Body.Close()
	}
}

type httpDecoderFactory struct{}

func (h *httpDecoderFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	hstream := &httpDecoder{
		net:       net,
		transport: transport,
		stream:    tcpreader.NewReaderStream(),
	}

	srcIp, dstIp := hstream.Ip()
	srcPort, dstPort := hstream.Port()
	log.Printf("new tcp conn %s:%d -> %s:%d\n", srcIp, srcPort, dstIp, dstPort)
	go hstream.run() // Important... we must guarantee that data from the reader stream is read.

	// ReaderStream implements tcpassembly.Stream, so we can return a pointer to it.
	return &hstream.stream
}

func main() {
	// 以太网 MTU 通常小于 1600
	handle, err := pcap.OpenLive("en0", 1600, true, pcap.BlockForever)
	if err != nil {
		panic(err)
	}

	// http 协议通常是 80 端口
	if err = handle.SetBPFFilter(expr); err != nil {
		panic(err)
	}

	var factory httpDecoderFactory
	// 源数据队列
	ps := gopacket.NewPacketSource(handle, handle.LinkType())

	// assembler 具有处理 ip 分片, tcp 分包的能力
	pool := tcpassembly.NewStreamPool(&factory)
	assembler := tcpassembly.NewAssembler(pool)
	ticker := time.Tick(time.Minute)
	packets := ps.Packets()

	for {
		select {
		case packet := <-packets:
			if packet == nil {
				return
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
