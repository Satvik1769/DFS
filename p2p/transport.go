package p2p

import "net"

type Peer interface {
	Send([]byte) error
	net.Conn
}

type Transport interface {
	Dial(string) error
	ListenAndAccept() error
	Consume() <-chan RPC
	Close() error
}
