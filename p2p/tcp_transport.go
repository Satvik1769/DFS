package p2p

import (
	"fmt"
	"net"
)

type TCPTransportOps struct {
	ListenAddr string;
	HandshakeFunc HandshakeFunc;
	Decoder Decoder;
	OnPeer func(Peer) error;

}

type TCPTransport struct {
	TCPTransportOps ;
	listener net.Listener;
	rpcch chan RPC;
}


type TCPPeer struct {
	conn net.Conn;
	// if we accept and retreive a function false
	// if we send and retrieve a function true
	outbound bool;
}

func (p *TCPPeer) Close() error {
	return p.conn.Close();
}

func NewTcpTransport(opts TCPTransportOps) *TCPTransport{
	return &TCPTransport{
		TCPTransportOps: opts,
		rpcch: make(chan RPC), 
	}
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer{
	return &TCPPeer{conn: conn, outbound: outbound}
}


func (t *TCPTransport) ListenAndAccept() error {
	var err error;
	t.listener, err = net.Listen("tcp", t.ListenAddr);
	if(err != nil){
		return  err;
	}
	go t.startAcceptLoop();
	return  nil;
}

func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch;
}

func (t *TCPTransport) startAcceptLoop(){
	for {
		conn, err := t.listener.Accept();
		if(err != nil){
			fmt.Printf("TCP Accept error: %s \n", err)
		}

		fmt.Printf("TCP connection accepted from %+v \n", conn);
		go t.handleConn(conn);
	}
}


func (t *TCPTransport) handleConn(conn net.Conn){
	var err error;

	   defer func() {
     
        if err != nil {
            fmt.Printf("Closing connection from %s because %v \n", conn.RemoteAddr(), err)
        } else {
            fmt.Printf("Closing connection from %s gracefully or client disconnected.\n", conn.RemoteAddr())
        }
        conn.Close()
    }()

	peer := NewTCPPeer(conn, true);

	if err = t.HandshakeFunc(peer); err != nil {
		return;
	}

	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return;
		}
	}

	msg := RPC{};
	
	for{
		err = t.Decoder.Decode(conn, &msg);
		if  err != nil{
			return;
		}
		msg.From = conn.RemoteAddr();
		t.rpcch <- msg;
		fmt.Printf("Received message: %v \n", msg);
	}

}