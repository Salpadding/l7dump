package mysql

type ServerHandShake []byte

func readNullStr(data []byte, pos int) string {
	start := pos
	for data[pos] != 0 && pos < len(data) {
		pos++
	}
	return string(data[start:pos])
}

func (s ServerHandShake) Type() string {
	return "ServerHandShake"
}

func (c ServerHandShake) parse(ver *string) {
	*ver = readNullStr(c, 1)
}

type ClientHandShake struct {
	buf []byte
	User string
}

func (c ClientHandShake) parse(user, db *string) {

}

// Ok packet 以 0 开头
// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_ok_packet.html
type Ok []byte

func(ok Ok) Type() string {
    return "ok"
}


