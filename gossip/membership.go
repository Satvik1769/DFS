package gossip

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

type EventDelegate struct {
	onJoin   func(*memberlist.Node)
	onLeave  func(*memberlist.Node)
	onUpdate func(*memberlist.Node)
}

func (e *EventDelegate) NotifyJoin(n *memberlist.Node) {
	if e.onJoin != nil {
		e.onJoin(n)
	}
}
func (e *EventDelegate) NotifyLeave(n *memberlist.Node) {
	if e.onLeave != nil {
		e.onLeave(n)
	}
}
func (e *EventDelegate) NotifyUpdate(n *memberlist.Node) {
	if e.onUpdate != nil {
		e.onUpdate(n)
	}
}

type Membership struct {
	list *memberlist.Memberlist
	mu   sync.Mutex
}

func New(bindAddr string, p2pPort int, onJoin, onLeave func(*memberlist.Node)) *Membership {
	cfg := memberlist.DefaultLocalConfig()

	// Node.Name carries the P2P address for dialing later
	cfg.Name = fmt.Sprintf("%s:%d", bindAddr, p2pPort)

	cfg.BindAddr = bindAddr
	cfg.BindPort = p2pPort + 1200 // gossip separate from P2P

	cfg.Events = &EventDelegate{
		onJoin:  onJoin,
		onLeave: onLeave,
	}

	list, err := memberlist.Create(cfg)
	if err != nil {
		log.Fatalf("failed to create memberlist: %v", err)
	}

	return &Membership{list: list}
}

func (m *Membership) Join(existing []string) error {
	_, err := m.list.Join(existing)
	if err != nil {
		fmt.Printf("failed to join cluster: %v\n", err)
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
