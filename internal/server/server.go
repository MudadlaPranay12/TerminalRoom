package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/terminalroom/terminalroom/internal/invitation"
	"github.com/terminalroom/terminalroom/internal/protocol"
	"github.com/terminalroom/terminalroom/internal/room"
	"github.com/terminalroom/terminalroom/internal/tlsutil"
)

type Server struct {
	listener     net.Listener
	rooms        *room.Manager
	invitations  *invitation.Manager
	certDir      string
	wg           sync.WaitGroup
	quit         chan struct{}
	shutdownOnce sync.Once
	roomTTL      time.Duration
	endpoint     string
	ready        chan struct{}
}

func New(addr string, certDir string) (*Server, error) {
	return NewWithTTL(addr, certDir, room.DefaultRoomTTL)
}

func NewWithTTL(addr string, certDir string, ttl time.Duration) (*Server, error) {
	return NewWithTTLAndEndpoint(addr, certDir, ttl, "")
}

func NewWithTTLAndEndpoint(addr string, certDir string, ttl time.Duration, endpoint string) (*Server, error) {
	// F-02: Validate explicit endpoint before listening/advertising.
	// Endpoint is routing information but must obey private-network model.
	if endpoint != "" {
		if err := invitation.ValidateEndpoint(endpoint); err != nil {
			return nil, fmt.Errorf("invalid ENDPOINT_ADDR %q: %w", endpoint, err)
		}
	}
	tlsConfig, err := tlsutil.ServerConfig(certDir)
	if err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	if endpoint == "" {
		_, port, _ := net.SplitHostPort(addr)
		if port == "" {
			port = "9090"
		}
		host, _, _ := net.SplitHostPort(ln.Addr().String())
		endpoint = host + ":" + port
	} else {
		// Double-check explicit endpoint even if already validated above (defense in depth)
		if err := invitation.ValidateEndpoint(endpoint); err != nil {
			ln.Close()
			return nil, fmt.Errorf("invalid ENDPOINT_ADDR %q: %w", endpoint, err)
		}
	}

	ready := make(chan struct{})
	close(ready)
	return &Server{
		listener:    ln,
		rooms:       room.NewManager(),
		invitations: invitation.NewManager(),
		certDir:     certDir,
		quit:        make(chan struct{}),
		roomTTL:     ttl,
		endpoint:    endpoint,
		ready:       ready,
	}, nil
}

func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *Server) Endpoint() string {
	return s.endpoint
}

func (s *Server) WaitReady() <-chan struct{} {
	return s.ready
}

func (s *Server) IsReady() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

func (s *Server) Start() {
	log.Printf("Server listening on %s (TLS)", s.listener.Addr())
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.quit)
		s.listener.Close()
		log.Printf("Server shut down. Active rooms: %d", s.rooms.ActiveRoomCount())
	})
	s.wg.Wait()
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered in connection handler: %v", r)
		}
		conn.Close()
	}()

	go func() {
		<-s.quit
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	msg, err := protocol.Decode(reader)
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "Server identity verification failed.",
		})
		return
	}

	switch msg.Type {
	case protocol.MsgCreate:
		s.handleCreate(conn, reader, writer, msg.Payload)
	case protocol.MsgJoin:
		s.handleJoin(conn, reader, writer, msg.Payload)
	default:
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "Expected CREATE or JOIN as first message.",
		})
	}
}

func (s *Server) handleCreate(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, ttlPayload string) {
	ttl := s.roomTTL
	if ttlPayload != "" {
		secs, err := strconv.Atoi(strings.TrimSpace(ttlPayload))
		if err != nil || secs < 600 || secs > 3600 {
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgError,
				Payload: "invalid duration: must be 600-3600 seconds (10-60 minutes)",
			})
			return
		}
		if !room.IsValidRoomTTL(time.Duration(secs) * time.Second) {
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgError,
				Payload: "invalid duration",
			})
			return
		}
		ttl = time.Duration(secs) * time.Second
	}
	r, err := s.rooms.CreateRoomWithTTL(ttl)
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "failed to create room",
		})
		return
	}

	r.SetOnExpire(func() {
		s.expireRoom(r)
	})
	r.StartExpirationTimer()

	inv, err := s.invitations.Create(r.ID, invitation.DefaultExpiry)
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "failed to generate invitation",
		})
		return
	}

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgRoomID,
		Payload: r.ID,
	})

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgInvToken,
		Payload: inv.Token,
	})

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgExpiry,
		Payload: r.ExpiresAt.Format(time.RFC3339),
	})

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgEndpoint,
		Payload: s.endpoint,
	})

	log.Printf("Room created: %s (TTL: %v)", r.ID, ttl)

	p, err := r.Join("user1")
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "failed to join room",
		})
		return
	}

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgParticipantID,
		Payload: fmt.Sprintf("%d", p.ID),
	})

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgSystem,
		Payload: "Waiting for friend...",
	})

	s.handleParticipant(conn, reader, writer, r, p)
}

func (s *Server) handleJoin(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, token string) {
	if token == "" {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgError,
			Payload: "missing invitation token",
		})
		return
	}

	inv, err := s.invitations.ValidateByToken(token)
	if err != nil {
		switch err.Error() {
		case "invalid invitation":
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgNotFound,
				Payload: "Invalid invitation.",
			})
		case "invitation expired":
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgExpired,
				Payload: "Invitation expired.",
			})
		case "invitation already used":
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgUsed,
				Payload: "Invitation already used.",
			})
		default:
			protocol.Send(writer, protocol.Message{
				Type:    protocol.MsgError,
				Payload: "Invitation verification failed.",
			})
		}
		return
	}

	r, err := s.rooms.GetRoom(inv.RoomID)
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgNotFound,
			Payload: "Room not found.",
		})
		return
	}

	p, err := r.Join("user2")
	if err != nil {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgFull,
			Payload: "Room is full.",
		})
		return
	}

	log.Printf("Participant authenticated")
	log.Printf("Participant joined room %s", r.ID)

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgParticipantID,
		Payload: fmt.Sprintf("%d", p.ID),
	})

	remaining := r.TimeRemaining()
	if remaining > 0 {
		protocol.Send(writer, protocol.Message{
			Type:    protocol.MsgExpiry,
			Payload: r.ExpiresAt.Format(time.RFC3339),
		})
	}

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgEndpoint,
		Payload: s.endpoint,
	})

	protocol.Send(writer, protocol.Message{
		Type:    protocol.MsgReady,
		Payload: "Secure two-person session established.",
	})

	other := r.OtherParticipant(p.ID)
	if other != nil {
		log.Printf("Room %s: two participants connected", r.ID)
	}

	s.handleParticipant(conn, reader, writer, r, p)
}

func (s *Server) handleParticipant(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, r *room.Room, p *room.Participant) {
	sendCh := r.RegisterWriter(p.ID)
	defer func() {
		other := r.OtherParticipant(p.ID)
		if other != nil {
			r.BroadcastExcept(p.ID, protocol.Message{
				Type:    protocol.MsgLeave,
				Payload: "",
			})
		}

		r.UnregisterWriter(p.ID)
		// Owner closes its channel after delete; no concurrent broadcast can find it now.
		// Broadcast holds RLock during send, Unregister holds Lock, so in-flight sends complete before delete.
		close(sendCh)
		r.Leave(p.ID)
		log.Printf("Room %s: participant %d left", r.ID, p.ID)

		if r.IsDestroyed() {
			r.StopExpirationTimer()
			s.rooms.RemoveRoom(r.ID)
			s.invitations.RemoveByRoom(r.ID)
			log.Printf("Room %s destroyed and removed", r.ID)
		}
	}()

	var writeMu sync.Mutex
	go func() {
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					return
				}
				writeMu.Lock()
				protocol.Send(writer, msg)
				writeMu.Unlock()
			case <-r.Done():
				return
			case <-s.quit:
				return
			}
		}
	}()

	other := r.OtherParticipant(p.ID)
	if other != nil {
		r.BroadcastExcept(p.ID, protocol.Message{
			Type:    protocol.MsgSystem,
			Payload: "Friend joined.",
		})
	}

	for {
		msg, err := protocol.Decode(reader)
		if err != nil {
			if err.Error() != "EOF" && !strings.Contains(err.Error(), "closed") {
				log.Printf("Read error from participant %d in room %s: %v", p.ID, r.ID, err)
			}
			return
		}

		switch msg.Type {
		case protocol.MsgChat:
			if !r.IsActive() {
				continue
			}
			r.BroadcastExcept(p.ID, protocol.Message{
				Type:    protocol.MsgChat,
				Payload: fmt.Sprintf("%d|%s", p.ID, msg.Payload),
			})
		case protocol.MsgLeave:
			return
		default:
			log.Printf("Unexpected message type from participant %d: %s", p.ID, msg.Type)
		}
	}
}

func (s *Server) expireRoom(r *room.Room) {
	if !r.BeginExpiring() {
		return
	}

	log.Printf("Room %s: TTL expired, beginning expiration", r.ID)

	r.BroadcastAll(protocol.Message{
		Type:    protocol.MsgSystem,
		Payload: "Room is expiring. Connection will close shortly.",
	})

	time.Sleep(1 * time.Second)

	r.BroadcastAll(protocol.Message{
		Type:    protocol.MsgSystem,
		Payload: "Room is closing.",
	})

	if !r.BeginClosing() {
		return
	}

	log.Printf("Room %s: closing", r.ID)

	time.Sleep(500 * time.Millisecond)

	r.FinishDestroying()
	s.rooms.RemoveRoom(r.ID)
	s.invitations.RemoveByRoom(r.ID)

	log.Printf("Room %s destroyed and removed (TTL expired)", r.ID)
}

func (s *Server) CleanupExpiredInvitations() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.invitations.Cleanup()
		case <-s.quit:
			return
		}
	}
}
