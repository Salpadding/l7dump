package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	httpTracker "github.com/Salpadding/l7dump/http"
	"github.com/Salpadding/l7dump/session"
	"github.com/Salpadding/l7dump/tracker"
)

// 示例程序
// 打印所有 url 中包含 /kapis/resources.kubesphere.io/v1alpha2/componenthealth 的请求
const matchUrl = "/kapis/resources.kubesphere.io/v1alpha2/components"

// l7dump en0 80 /order
func main() {
	if len(os.Args) < 4 {
		panic("usage: l7dump [iface] [port] [path]")
	}
	mgr := session.ProtocolSessionMgr{
		Trackers: make(map[int]tracker.ProtocolTracker),
	}

	httpTracker := httpTracker.HttpTracker{
		PreReq: func(r *http.Request) bool {
			return strings.Contains(r.URL.String(), os.Args[3])
		},
		PostReq: func(req *http.Request, resp *http.Response) {
			fmt.Printf("%s %s\n", req.Method, req.URL.String())
			io.Copy(os.Stdout, req.Body)
			fmt.Println()
			io.Copy(os.Stdout, resp.Body)
			fmt.Println()
		},
	}

	port, _ := strconv.Atoi(os.Args[2])

	fmt.Printf("add http tracker at port %d", port)
	mgr.Trackers[port] = &httpTracker
	if err := mgr.Listen(os.Args[1]); err != nil {
		panic(err)
	}
}
