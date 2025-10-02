package p2p

import "net"

type Peer interface {
	Send([]byte) error
	net.Conn
	CloseStream()
	ID() string
	SignalReady()
	AddRequest() chan struct{}
	RemoveRequest()
	GetEpepheral() bool
	SetEpepheral(bool)
}

type Transport interface {
	Addr() string
	Dial(string) error
	ListenAndAccept() error
	Consume() <-chan RPC
	Close() error
}
