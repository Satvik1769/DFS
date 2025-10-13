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

	requestReady chan struct{}
	pending      bool
	epepheral    bool
	mu           sync.Mutex
}

func NewTcpTransport(opts TCPTransportOps) *TCPTransport {
	return &TCPTransport{
		TCPTransportOps: opts,
		rpcch:           make(chan RPC, 1024),
	}
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {

	var id string
	if outbound {
		// We dialed -> remote is the server with real store port
		id = conn.RemoteAddr().String()
	} else {
		// We accepted -> local is our listening port
		id = conn.LocalAddr().String()
	}

	return &TCPPeer{
		Conn:         conn,
		outbound:     outbound,
		wg:           &sync.WaitGroup{},
		id:           id,
		requestReady: nil,
		pending:      false,
	}
}

func (p *TCPPeer) AddRequest() chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.requestReady == nil {
		p.requestReady = make(chan struct{}, 1)
	}

	// If there was a pending signal, deliver it now
	if p.pending {
		select {
		case p.requestReady <- struct{}{}:
		default:
		}
		p.pending = false
	}

	fmt.Printf("[%s] AddRequest\n", p.id)
	return p.requestReady
}

func (p *TCPPeer) SignalReady() {
	p.mu.Lock()
	// Lazily create the channel if nil
	if p.requestReady == nil {
		p.requestReady = make(chan struct{}, 1)
	}

	// Mark as pending and send non-blocking
	p.pending = true
	ch := p.requestReady
	p.mu.Unlock()

	select {
	case ch <- struct{}{}:
	default:
	}
}

func (p *TCPPeer) RemoveRequest() {
	p.mu.Lock()
	ch := p.requestReady
	p.requestReady = nil
	p.pending = false
	p.mu.Unlock()

	if ch != nil {
		close(ch)
	}
	fmt.Printf("[%s] RemoveRequest\n", p.id)
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

// Get/Set ephemeral flag
func (p *TCPPeer) GetEpepheral() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.epepheral
}

func (p *TCPPeer) SetEpepheral(epepheral bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epepheral = epepheral
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
		fmt.Printf("Received message 2: %v \n", msg)
	}

}
