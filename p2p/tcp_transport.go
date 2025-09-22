package p2p

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
)

type TCPTransportOps struct {
	ListenAddr    string
	HandshakeFunc HandshakeFunc
	Decoder       Decoder
	OnPeer        func(Peer) error
}

type TCPTransport struct {
	TCPTransportOps
	listener net.Listener
	rpcch    chan RPC
}

type TCPPeer struct {
	net.Conn
	// if we accept and retreive a function false
	// if we send and retrieve a function true
	outbound bool
	wg       *sync.WaitGroup
	id       string

	requestReady map[string]chan struct{}
	pending      map[string]bool
	mu           sync.Mutex
}

func NewTcpTransport(opts TCPTransportOps) *TCPTransport {
	return &TCPTransport{
		TCPTransportOps: opts,
		rpcch:           make(chan RPC, 1024),
	}
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		Conn:         conn,
		outbound:     outbound,
		wg:           &sync.WaitGroup{},
		id:           conn.RemoteAddr().String(),
		requestReady: make(map[string]chan struct{}),
		pending:      make(map[string]bool),
	}
}

func (p *TCPPeer) AddRequest(key string) chan struct{} {
	p.mu.Lock()
	if ch, ok := p.requestReady[key]; ok {
		p.mu.Unlock()
		fmt.Printf("[%s] AddRequest(existing) %s\n", p.id, key)
		return ch
	}

	ch := make(chan struct{}, 1)
	p.requestReady[key] = ch

	// if there was an earlier SignalReady for this key, deliver it now
	if p.pending[key] {
		// attempt to deliver (buffered chan so this won't block)
		select {
		case ch <- struct{}{}:
		default:
		}
		delete(p.pending, key)
	}
	p.mu.Unlock()

	fmt.Printf("[%s] AddRequest(new) %s\n", p.id, key)
	return ch
}

func (p *TCPPeer) SignalReady(key string) {
	p.mu.Lock()
	ch, ok := p.requestReady[key]
	if ok {
		// we have a channel; perform non-blocking send so we don't block the writer
		// release lock before sending to avoid holding lock during send
		p.mu.Unlock()
		fmt.Printf("[%s] SignalReady -> channel %s\n", p.id, key)
		select {
		case ch <- struct{}{}:
		default:
		}
		return
	}

	// no channel currently registered — mark pending and return
	p.pending[key] = true
	p.mu.Unlock()
	fmt.Printf("[%s] SignalReady -> pending %s\n", p.id, key)
}

func (p *TCPPeer) RemoveRequest(key string) {
	p.mu.Lock()
	ch, ok := p.requestReady[key]
	if ok {
		delete(p.requestReady, key)
	}
	// clear any pending mark too (optional)
	if _, ok2 := p.pending[key]; ok2 {
		delete(p.pending, key)
	}
	p.mu.Unlock()

	if ok {
		// close outside lock to avoid races
		close(ch)
	}
	fmt.Printf("[%s] RemoveRequest %s (ok=%v)\n", p.id, key, ok)
}

func (t *TCPTransport) Addr() string {
	return t.ListenAddr
}

func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error
	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}
	go t.startAcceptLoop()
	log.Printf("TCP Transport listening on %s", t.ListenAddr)
	return nil
}

func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch
}

func (t *TCPTransport) startAcceptLoop() {
	for {
		conn, err := t.listener.Accept()

		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			fmt.Printf("TCP Accept error: %s \n", err)
		}

		fmt.Printf("TCP connection accepted from %+v \n", conn)
		go t.handleConn(conn, false)
	}
}

// Dial esta lishes a TCP connection to the specified address.
func (t *TCPTransport) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", addr, err)
	}
	fmt.Printf("TCP connection established to %s \n", addr)
	go t.handleConn(conn, true)
	return nil
}

func (p *TCPPeer) CloseStream() {
	p.wg.Done()
}

func (p *TCPPeer) Send(b []byte) error {
	_, err := p.Conn.Write(b)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func (p *TCPPeer) ID() string {
	return p.id
}

func (t *TCPTransport) handleConn(conn net.Conn, outbound bool) {
	var err error

	defer func() {

		if err != nil {
			fmt.Printf("Closing connection from %s because %v \n", conn.RemoteAddr(), err)
		} else {
			fmt.Printf("Closing connection from %s gracefully or client disconnected.\n", conn.RemoteAddr())
		}
		conn.Close()
	}()

	peer := NewTCPPeer(conn, outbound)

	if err = t.HandshakeFunc(peer); err != nil {
		return
	}

	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return
		}
	}

	for {
		msg := RPC{}
		err = t.Decoder.Decode(conn, &msg)
		if err != nil {
			return
		}
		msg.From = conn.RemoteAddr().String()
		if msg.Stream {
			peer.wg.Add(1)
			fmt.Printf("waiting to stream be done\n")
			peer.wg.Wait()
			fmt.Printf("streaming done\n")
			continue
		}

		t.rpcch <- msg
		fmt.Printf("Received message: %v \n", msg)
	}

}
