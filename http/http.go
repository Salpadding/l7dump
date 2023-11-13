package http

import (
	"bufio"
	"net/http"
	"sync"

	"github.com/Salpadding/l7dump/tracker"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var (
	_ tracker.ProtocolTracker     = (*HttpTracker)(nil)
	_ tracker.ProtocolConnTracker = (*HttpConnTracker)(nil)
)

type HttpTracker struct {
	conns sync.Map

	// PreReq 决定这个http请求是否会被追踪
	PreReq func(*http.Request) bool
	// 被追踪的请求会被调用 PostReq
	PostReq func(req *http.Request, resp *http.Response)
}

func (h *HttpTracker) GetConnect(meta tracker.ConnMeta) tracker.ProtocolConnTracker {
	actual, _ := h.conns.LoadOrStore(meta.String(), &HttpConnTracker{
		ConnMeta: meta,
		Tracker:  h,
	})
	return actual.(tracker.ProtocolConnTracker)
}

func (h *HttpTracker) OnClose(conn tracker.ProtocolConnTracker) {
	h.conns.Delete((conn.(*HttpConnTracker)).ConnMeta.String())
}

func (h *HttpTracker) RequestDecoder(stream *tcpreader.ReaderStream, conn tracker.ProtocolConnTracker) func() (interface{}, error) {
	buf := bufio.NewReader(stream)
	return func() (interface{}, error) {
		req, err := http.ReadRequest(buf)
		(conn.(*HttpConnTracker)).LastReq = req
		return req, err
	}
}

func (h *HttpTracker) ResponseDecoder(stream *tcpreader.ReaderStream, conn tracker.ProtocolConnTracker) func() (interface{}, error) {
	buf := bufio.NewReader(stream)
	return func() (val interface{}, err error) {
		defer func() {
			if err != nil {
			}
		}()
		req := (conn.(*HttpConnTracker)).LastReq
		conn.(*HttpConnTracker).LastResp, err = http.ReadResponse(buf, req)
		return conn.(*HttpConnTracker).LastResp, err
	}
}

type HttpConnTracker struct {
	Tracker  *HttpTracker
	ConnMeta tracker.ConnMeta
	LastReq  *http.Request
	LastResp *http.Response

	Record bool
}

func (h *HttpConnTracker) OnRequest(req interface{}) error {
	if req == nil {
		return nil
	}
	r := req.(*http.Request)

	h.Record = h.Tracker.PreReq(r)
	if !h.Record {
		tcpreader.DiscardBytesToEOF(r.Body)
		r.Body.Close()
		return nil
	}

	return nil
}

func (h *HttpConnTracker) OnResponse(resp interface{}) error {
	if resp == nil {
		return nil
	}
	r := resp.(*http.Response)

	if !h.Record {
		tcpreader.DiscardBytesToEOF(r.Body)
		r.Body.Close()
		return nil
	}
	h.Tracker.PostReq(h.LastReq, h.LastResp)
	return nil
}

func (h *HttpConnTracker) OnError(err error) {
}
