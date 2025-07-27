package p2p

type HandshakeFunc func (Peer) error ;

func NOPHandTransport(Peer) error {
	return nil;
}