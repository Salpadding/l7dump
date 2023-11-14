package main

import (
	"context"
	"testing"

	"github.com/Salpadding/l7dump/mysql"
	"github.com/Salpadding/l7dump/session"
)

// --ssl-mode=DISABLED
func TestMysql(t *testing.T) {
	mgr := session.NewMgr(context.Background())
	tracker := &mysql.Tracker{}
	mgr.AddTracker(3307, tracker)
	mgr.Listen("tcp and port 3307", "en7")
}
