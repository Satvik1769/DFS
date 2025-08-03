package p2p

import (
"testing"
"github.com/stretchr/testify/assert"
)

func TestTcpTransport(t *testing.T){
	opts := TCPTransportOps{
		ListenAddr:    ":3000",
		HandshakeFunc: NOPHandTransport,
		Decoder:       DefaultDecoder{},
	}
	tr := NewTcpTransport(opts);
	assert.Equal(t, tr.ListenAddr, ":3000");
	assert.Nil(t, tr.ListenAndAccept() )
}