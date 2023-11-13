package http

import (
	"bufio"
	"net/http"

	"github.com/Salpadding/l7dump/core"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var (
	_ core.ProtocolTracker     = (*Tracker)(nil)
	_ core.ProtocolConnTracker = (*ConnTracker)(nil)
)

type Tracker struct {

	// PreReq 决定这个http请求是否会被追踪
	PreReq func(*http.Request) bool
	// 被追踪的请求会被调用 PostReq
	PostReq func(req *http.Request, resp *http.Response)
}

func (h *Tracker) NewConnect(meta core.ConnMeta) core.ProtocolConnTracker {
	return &ConnTracker{
		ConnMeta: meta,
		Tracker:  h,
	}
}

func (h *Tracker) OnClose(conn core.ProtocolConnTracker) {
}

func (h *Tracker) RequestDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	buf := bufio.NewReader(stream)
	return func() (interface{}, error) {
		req, err := http.ReadRequest(buf)
		(conn.(*ConnTracker)).LastReq = req
		return req, err
	}
}

func (h *Tracker) ResponseDecoder(stream *tcpreader.ReaderStream, conn core.ProtocolConnTracker) func() (interface{}, error) {
	buf := bufio.NewReader(stream)
	return func() (val interface{}, err error) {
		defer func() {
			if err != nil {
			}
		}()
		req := (conn.(*ConnTracker)).LastReq
		conn.(*ConnTracker).LastResp, err = http.ReadResponse(buf, req)
		return conn.(*ConnTracker).LastResp, err
	}
}

type ConnTracker struct {
	Tracker  *Tracker
	ConnMeta core.ConnMeta
	LastReq  *http.Request
	LastResp *http.Response

	Record bool
}

func (h *ConnTracker) OnRequest(req interface{}) error {
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

func (h *ConnTracker) OnResponse(resp interface{}) error {
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

func (h *ConnTracker) OnError(err error) {
}
