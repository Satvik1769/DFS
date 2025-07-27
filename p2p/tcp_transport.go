package p2p

import (
	"fmt"
	"net"
	"sync"
)

type TCPTransport struct {
	listenAddress string;
	listener net.Listener;
	handshakeFunc HandshakeFunc;
	decoder Decoder;

	mu sync.RWMutex;
	peers map[net.Addr]Peer ;
}

type TCPPeer struct {
	conn net.Conn;
	// if we accept and retreive a function false
	// if we send and retrieve a function true
	outbound bool;
}

func NewTcpTransport(listenAddress string) *TCPTransport{
	return &TCPTransport{
		handshakeFunc: NOPHandTransport,
		listenAddress: listenAddress,
	}
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer{
	return &TCPPeer{conn: conn, outbound: outbound}
}


func (t *TCPTransport) ListenAndAccept() error {
	var err error;
	t.listener, err = net.Listen("tcp", t.listenAddress);
	if(err != nil){
		return  err;
	}
	go t.startAcceptLoop();
	return  nil;
}

func (t *TCPTransport) startAcceptLoop(){
	for {
		conn, err := t.listener.Accept();
		if(err != nil){
			fmt.Printf("TCP Accept error: %s \n", err)
		}
		go t.handleConn(conn);
	}
}

type Temp struct {

}
func (t *TCPTransport) handleConn(conn net.Conn){
	peer := NewTCPPeer(conn, true);

	if err := t.handshakeFunc(peer); err != nil {
	}

	msg := &Temp{};
	for{
		if err := t.decoder.Decode(conn, msg); err != nil{
			fmt.Printf("TCP error: %s \n", err);
			continue;
		}
	}
}