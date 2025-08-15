package p2p

import (
	"encoding/gob"
	"fmt"
	"io"
)

type Decoder interface {
	Decode(io.Reader, *RPC) error
}

type GOBDecoder struct{};



func (d GOBDecoder) Decode(r io.Reader, v *RPC) error {
	return gob.NewDecoder(r).Decode(v)
	
}


type DefaultDecoder struct{};



func (d DefaultDecoder) Decode(r io.Reader, msg *RPC) error {
	buf := make([]byte, 1028)  // This creates a []byte
	n, err := r.Read(buf)   // This works - buf is []byte
	if err != nil {
		return err
	}
	fmt.Printf("NOPDecoder read %d bytes: %s\n", n, buf[:n])
	msg.Payload = buf[:n]  // Assign the read bytes to the message payload
	return nil
}
	
