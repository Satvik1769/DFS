package p2p

type Peer interface {
	Close() error;
};

type  transport interface{
	ListenAndTransport() error;
	Consume() <- chan RPC;
};