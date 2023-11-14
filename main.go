package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Salpadding/l7dump/http"
	"github.com/Salpadding/l7dump/mysql"
	"github.com/Salpadding/l7dump/session"
)

// 示例程序
// 打印所有 url 中包含 /kapis/resources.kubesphere.io/v1alpha2/componenthealth 的请求
const matchUrl = "/kapis/resources.kubesphere.io/v1alpha2/components"

type IfaceCfg struct {
	Bpf      string          `json:"bpf"`
	Trackers []TrackerConfig `json:"trackers"`
}

type TrackerConfig struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // 协议 mysql, http
	Program  string `json:"program"`  // lua 脚本 尚未支持
}

// l7dump en0 80 /order
func main() {
	if len(os.Args) != 2 {
		panic("usage: l7dump [config.json]")
	}

	bg := context.Background()
	var config map[string]IfaceCfg
	jsonData, err := os.ReadFile(os.Args[1])

	if err != nil {
		panic(err)
	}

	if err = json.Unmarshal(jsonData, &config); err != nil {
		panic(err)
	}

	// mgr 和 iface 1:1
	for iface := range config {
		mgr := session.NewMgr(bg)

		cfg := config[iface]

		for i := range cfg.Trackers {
			switch cfg.Trackers[i].Protocol {
			case "mysql":
				mgr.AddTracker(cfg.Trackers[i].Port, &mysql.Tracker{})
			case "http":
				// TODO: 完善 http tracker
				mgr.AddTracker(cfg.Trackers[i].Port, &http.Tracker{})
			default:
				panic(fmt.Sprintf("unknown protocol %s", cfg.Trackers[i].Protocol))
			}
		}
		go mgr.Listen(cfg.Bpf, iface)
	}

	<-bg.Done()
}
