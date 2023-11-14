package mysql

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

type Parser struct {
	io.Reader
}

func (p *Parser) ReadNullStr() (string, error) {
	var (
		buf  [1]byte
		err  error
		data bytes.Buffer
	)

	for {
		_, err = p.Read(buf[:])

		if err != nil {
			if err == io.EOF {
				return data.String(), nil
			}

			return "", err
		}

		if buf[0] == 0 {
			return data.String(), nil
		}

		data.Write(buf[:])
	}
}

func (p *Parser) Byte() (val int, err error) {
	var buf [1]byte
	_, err = p.Reader.Read(buf[:])
	if err != nil {
		return
	}
	val = int(buf[0])
	return
}

func (p *Parser) DropN(n int) (err error) {
	_, err = io.CopyN(io.Discard, p.Reader, int64(n))
	return
}

func (p *Parser) Drop() (err error) {
	_, err = io.Copy(io.Discard, p.Reader)
	return
}

func (p *Parser) Uint32() (val uint32, err error) {
	var buf [4]byte
	_, err = p.Read(buf[:])
	if err != nil {
		return
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

type ServerHandShake struct {
	Parser       Parser `json:"-"`
	ProtoVersion int
	Version      string
	Stauts       uint16
	Capabilities uint32
}

func (s *ServerHandShake) Type() string {
	return "ServerHandShake"
}

// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_v10.html
func (s *ServerHandShake) parse() (err error) {
	var (
		flagBuf   [4]byte
		statusBuf [2]byte
	)

	if s.ProtoVersion, err = s.Parser.Byte(); err != nil {
		return
	}

	if s.Version, err = s.Parser.ReadNullStr(); err != nil {
		return
	}

	// 连接 id auth-plugin-data-part-1
	s.Parser.DropN(13)

	// flag 低2byte
	if _, err = s.Parser.Read(flagBuf[:2]); err != nil {
		return
	}

	// charset
	if err = s.Parser.DropN(1); err != nil {
		return
	}

	// status
	if _, err = s.Parser.Read(statusBuf[:]); err != nil {
		return
	}
	s.Stauts = binary.LittleEndian.Uint16(statusBuf[:])

	if _, err = s.Parser.Read(flagBuf[2:]); err != nil {
		return
	}

	s.Capabilities = binary.LittleEndian.Uint32(flagBuf[:])

	s.Parser.Drop()
	return nil
}

// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_response.html
type ClientHandShake struct {
	Capabilities  uint32
	MaxPacketSize int
	Parser        Parser
	User          string
	Database      string
}

func (c *ClientHandShake) parse() (err error) {
	if c.Capabilities, err = c.Parser.Uint32(); err != nil {
		return
	}

	if c.Capabilities&uint32(clientProtocol41) == 0 {
		return errors.New("unsupported client protocol")
	}

	var tmp uint32
	if tmp, err = c.Parser.Uint32(); err != nil {
		return
	}

	c.MaxPacketSize = int(tmp)

	if err = c.Parser.DropN(24); err != nil {
		return
	}

	if c.User, err = c.Parser.ReadNullStr(); err != nil {
		return
	}

	c.Parser.Drop()
	return nil
}

// Ok packet 以 0 开头
// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_ok_packet.html
type Ok []byte

func (ok Ok) Type() string {
	return "ok"
}
