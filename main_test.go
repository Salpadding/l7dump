package main

import (
	"testing"

	"github.com/Salpadding/l7dump/mysql"
	"github.com/Salpadding/l7dump/session"
)

// --ssl-mode=DISABLED
func TestMysql(t *testing.T) {
	mgr := session.NewMgr()
	tracker := &mysql.Tracker{}
	mgr.AddTracker(3307, tracker)
	mgr.Listen("en7")
}
