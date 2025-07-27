package p2p

type Peer interface {};

type  transport interface{
	ListenAndTransport() error;
};