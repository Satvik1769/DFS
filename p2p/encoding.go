package p2p

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"
)

type Decoder interface {
	Decode(io.Reader, *RPC) error
}

type Encoder interface {
	Encode(io.Writer, *RPC) error
}

type GOBEncoder struct{}

func (e GOBEncoder) Encode(w io.Writer, v *RPC) error {
	// Encode to buffer first to get length
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}

	// Write length prefix (4 bytes)
	msgLen := uint32(buf.Len())
	if err := binary.Write(w, binary.BigEndian, msgLen); err != nil {
		return err
	}

	// Write message data
	_, err := w.Write(buf.Bytes())
	return err
}

type GOBDecoder struct{}

func (d GOBDecoder) Decode(r io.Reader, v *RPC) error {
	// Read message length first (4 bytes)
	var msgLen uint32
	if err := binary.Read(r, binary.BigEndian, &msgLen); err != nil {
		return err
	}

	// Read exactly msgLen bytes
	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(r, msgData); err != nil {
		return err
	}

	// Decode from the complete message
	return gob.NewDecoder(bytes.NewReader(msgData)).Decode(v)
}

type DefaultDecoder struct{}

func (d DefaultDecoder) Decode(r io.Reader, msg *RPC) error {
	peekBuff := make([]byte, 1)
	if _, err := r.Read(peekBuff); err != nil {
		return err
	}

	stream := peekBuff[0] == IncomingStream

	// we will not decode incoming streams here
	if stream {
		msg.Stream = true
		return nil
	}

	buf := make([]byte, 1028)
	n, err := r.Read(buf)
	if err != nil {
		return err
	}

	// this also writes raw data in terminal, uncomment for debugging
	//fmt.Printf("NOPDecoder read %d bytes: %s\n", n, buf[:n])
	msg.Payload = buf[:n]
	return nil
}
