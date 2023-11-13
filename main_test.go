package main

import (
	"fmt"
	"testing"

	"github.com/Salpadding/l7dump/mysql"
	"github.com/Salpadding/l7dump/session"
)

func TestMysql(t *testing.T) {
	// mysql
	fmt.Println("mysql")
	mgr := session.NewMgr()
	tracker := &mysql.Tracker{}
	mgr.AddTracker(3307, tracker)
	mgr.Listen("en7")
}
