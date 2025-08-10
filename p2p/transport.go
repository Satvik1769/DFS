package p2p

import "net"

type Peer interface {
	Send([] byte) error;
	Close() error;
	RemoteAddr() net.Addr;
};

type  Transport interface{
	Dial(string) error;
	ListenAndAccept() error;
	Consume() <- chan RPC;
	Close() error;
};