package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var (
	_ ProtocolTracker     = (*HttpTracker)(nil)
	_ ProtocolConnTracker = (*HttpConnTracker)(nil)
)

type HttpTracker struct {
	conns sync.Map

	PreReq  func(*http.Request) bool
	PostReq func(req *http.Request, resp *http.Response, reqBody, respBody []byte)
}

func (h *HttpTracker) GetConnect(meta ConnMeta) ProtocolConnTracker {
	actual, _ := h.conns.LoadOrStore(meta.String(), &HttpConnTracker{
		ConnMeta: meta,
		Tracker:  h,
	})
	return actual.(ProtocolConnTracker)
}

func (h *HttpTracker) OnClose(conn ProtocolConnTracker) {
	h.conns.Delete((conn.(*HttpConnTracker)).ConnMeta.String())
}

func (h *HttpTracker) RequestDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error) {
	buf := bufio.NewReader(stream)
	return func() (interface{}, error) {
		req, err := http.ReadRequest(buf)
		(conn.(*HttpConnTracker)).LastReq = req
		return req, err
	}
}

func (h *HttpTracker) ResponseDecoder(stream *tcpreader.ReaderStream, conn ProtocolConnTracker) func() (interface{}, error) {
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
	ConnMeta ConnMeta
	LastReq  *http.Request
	LastResp *http.Response

	Record   bool
	ReqBody  []byte
	RespBody []byte
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

	h.ReqBody, _ = ioutil.ReadAll(r.Body)
	r.Body.Close()
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
	h.RespBody, _ = ioutil.ReadAll(r.Body)
	h.Tracker.PostReq(h.LastReq, h.LastResp, h.ReqBody, h.RespBody)
	return nil
}

func (h *HttpConnTracker) OnError(err error) {
}

// 示例程序
// 打印所有 url 中包含 /kapis/resources.kubesphere.io/v1alpha2/componenthealth 的请求
const matchUrl = "/kapis/resources.kubesphere.io/v1alpha2/components"

// l7dump en0 80 /order
func main() {
	if len(os.Args) < 4 {
		panic("usage: l7dump [iface] [port] [path]")
	}
	mgr := ProtocolSessionMgr{
		trackers: make(map[int]ProtocolTracker),
	}

	httpTracker := HttpTracker{
		PreReq: func(r *http.Request) bool {
			return strings.Contains(r.URL.String(), os.Args[3])
		},
		PostReq: func(req *http.Request, resp *http.Response, reqBody, respBody []byte) {
			fmt.Printf("%s %s\n", req.Method, req.URL.String())
			fmt.Print(string(reqBody))
			fmt.Println()
			fmt.Print(string(respBody))
			fmt.Println()
		},
	}

	port, _ := strconv.Atoi(os.Args[2])

	fmt.Printf("add http tracker at port %d", port)
	mgr.trackers[port] = &httpTracker
	if err := mgr.Listen(os.Args[1]); err != nil {
		panic(err)
	}
}
