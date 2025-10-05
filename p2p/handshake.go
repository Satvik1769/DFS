package p2p

import (
	"fmt"
	"strings"
)

type HandshakeFunc func(Peer) error

func NOPHandTransport(peer Peer) error {
	myID := peer.LocalAddr().String() // or better: storePort based ID
	if _, err := peer.Write([]byte(myID + "\n")); err != nil {
		return err
	}

	fmt.Println("NOPHandTransport ", myID)

	// read their ID
	buf := make([]byte, 128)
	n, err := peer.Read(buf)
	if err != nil {
		return err
	}
	peerID := strings.TrimSpace(string(buf[:n]))

	// overwrite peer’s internal ID with stable one
	if tcpPeer, ok := peer.(*TCPPeer); ok {
		tcpPeer.id = peerID
		if peer.GetEpepheral() {
			return nil
		}
		peer.SignalReady()
	}
	return nil
}
