package p2p

import (
	"encoding/gob"
	"fmt"
	"io"
)

type Decoder interface {
	Decode(io.Reader, *RPC) error
}

type GOBDecoder struct{}

func (d GOBDecoder) Decode(r io.Reader, v *RPC) error {
	return gob.NewDecoder(r).Decode(v)

}

type DefaultDecoder struct{}

func (d DefaultDecoder) Decode(r io.Reader, msg *RPC) error {
	peekBuff := make([]byte, 1)
	if _, err := r.Read(peekBuff); err != nil {
		return err
	}

	stream := peekBuff[0] == IncomingStream;

	// we will not decode incoming streams here 
	if(stream){
		msg.Stream = true
		return nil
	}

	buf := make([]byte, 1028) 
	n, err := r.Read(buf)     
	if err != nil {
		return err
	}
	fmt.Printf("NOPDecoder read %d bytes: %s\n", n, buf[:n])
	msg.Payload = buf[:n]  
	return nil
}
