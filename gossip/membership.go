package gossip

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

// EventDelegate handles join/leave/update events with stable peerID
type EventDelegate struct {
	onJoin   func(peerID string)
	onLeave  func(peerID string)
	onUpdate func(peerID string)
}

func (e *EventDelegate) NotifyJoin(n *memberlist.Node) {
	if e.onJoin != nil {
		storePort, _ := strconv.Atoi(string(n.Meta))
		peerID := fmt.Sprintf("%s:%d", n.Addr.String(), storePort)
		log.Printf("Peer joined: %s", peerID)
		e.onJoin(peerID)
	}
}

func (e *EventDelegate) NotifyLeave(n *memberlist.Node) {
	if e.onLeave != nil {
		storePort, _ := strconv.Atoi(string(n.Meta))
		peerID := fmt.Sprintf("%s:%d", n.Addr.String(), storePort)
		log.Printf("Peer left: %s", peerID)
		e.onLeave(peerID)
	}
}

func (e *EventDelegate) NotifyUpdate(n *memberlist.Node) {
	if e.onUpdate != nil {
		storePort, _ := strconv.Atoi(string(n.Meta))
		peerID := fmt.Sprintf("%s:%d", n.Addr.String(), storePort)
		log.Printf("Peer updated: %s", peerID)
		e.onUpdate(peerID)
	}
}

// StoreDelegate advertises the store/P2P port
type StoreDelegate struct {
	StorePort int
}

func (d *StoreDelegate) NodeMeta(limit int) []byte {
	return []byte(fmt.Sprintf("%d", d.StorePort))
}

func (d *StoreDelegate) NotifyMsg([]byte)                           {}
func (d *StoreDelegate) GetBroadcasts(overhead, limit int) [][]byte { return nil }
func (d *StoreDelegate) LocalState(join bool) []byte                { return nil }
func (d *StoreDelegate) MergeRemoteState(buf []byte, join bool)     {}

// Membership wraps memberlist
type Membership struct {
	list *memberlist.Memberlist
	mu   sync.Mutex
}

func New(bindAddr string, p2pPort int, onJoin, onLeave func(peerID string)) *Membership {
	cfg := memberlist.DefaultLocalConfig()

	// Bind gossip port (separate from P2P)
	cfg.BindAddr = bindAddr
	cfg.BindPort = p2pPort + 1200
	cfg.Name = fmt.Sprintf("%s:%d", bindAddr, p2pPort) // optional, just for logging

	// Event delegate with stable peerID
	cfg.Events = &EventDelegate{
		onJoin:  onJoin,
		onLeave: onLeave,
	}

	// Delegate advertises store port
	cfg.Delegate = &StoreDelegate{StorePort: p2pPort}

	list, err := memberlist.Create(cfg)
	if err != nil {
		log.Fatalf("failed to create memberlist: %v", err)
	}

	return &Membership{list: list}
}

func (m *Membership) Join(existing []string) error {
	_, err := m.list.Join(existing)
	if err != nil {
		log.Printf("failed to join cluster: %v", err)
		return err
	}
	return nil
}

func (m *Membership) Members() []*memberlist.Node {
	return m.list.Members()
}

func (m *Membership) Leave(timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.list != nil {
		return m.list.Leave(timeout)
	}
	return nil
}

func (m *Membership) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.list != nil {
		m.list.Shutdown()
	}
}
