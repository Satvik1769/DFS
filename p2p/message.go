package p2p

const IncomingMessage = 0x1
const IncomingStream =  0x2

type RPC struct {
	Payload []byte
	From    string
	Stream  bool
}