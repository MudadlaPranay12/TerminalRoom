package client

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/terminalroom/terminalroom/internal/clipboard"
	"github.com/terminalroom/terminalroom/internal/invitation"
	"github.com/terminalroom/terminalroom/internal/protocol"
	"github.com/terminalroom/terminalroom/internal/terminal"
	"github.com/terminalroom/terminalroom/internal/tlsutil"
)

// clipboardCopy is the function used to copy invitations to the clipboard.
// It is a variable to allow mocking in tests.
var clipboardCopy = clipboard.Copy

type Client struct {
	addr            string
	certDir         string
	conn            net.Conn
	reader          *bufio.Reader
	writer          *bufio.Writer
	participantID   int
	roomID          string
	endpoint        string
	networkType     string
	expiryTime      time.Time
	bundledInvite   string
	friendJoined    bool
	currentRemaining time.Duration
}

func New(addr string, certDir string) *Client {
	return &Client{addr: addr, certDir: certDir}
}

func (c *Client) Run() error {
	terminal.Println("> Establishing secure connection...")

	tlsConfig, err := tlsutil.ClientConfig(c.certDir)
	if err != nil {
		terminal.Println(terminal.RenderError("SECURE CONNECTION FAILED", "Please verify the certificate configuration."))
		return fmt.Errorf("TLS config: %w", err)
	}
	conn, err := tls.Dial("tcp", c.addr, tlsConfig)
	if err != nil {
		terminal.Println(terminal.RenderError("CONNECTION FAILED", "Unable to connect to the TerminalRoom server. Check the server address and private network connection."))
		return fmt.Errorf("connect: %w", err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	defer c.conn.Close()

	terminal.Println("> Secure connection established.")
	terminal.Println("")

	for {
		terminal.Println(terminal.RenderHome())
		terminal.Println("")
		terminal.Print("> ")

		choice := strings.ToLower(strings.TrimSpace(c.readInput()))
		switch choice {
		case "1", "c", "create":
			return c.createRoom()
		case "2", "j", "join":
			return c.joinRoom()
		case "3", "q", "quit", "exit":
			terminal.Println("")
			terminal.Println("> Goodbye.")
			return nil
		case "h", "help":
			terminal.Println(terminal.RenderHelp())
			terminal.Println("")
		case "d", "diagnose":
			// Diagnose is handled via main.go, but show hint
			terminal.Println(terminal.RenderError("DIAGNOSE", "Run: terminalroom.exe diagnose"))
			terminal.Println("")
		default:
			terminal.Println(terminal.RenderError("INVALID CHOICE", "Enter C, J, H, D, or Q (or 1,2,3)."))
			terminal.Println("")
		}
	}
}

func (c *Client) createRoom() error {
	// Per-room duration selection — simple, native to existing terminal style.
	selected := c.selectDuration()

	terminal.Println("")
	terminal.Println("> Creating private room...")

	err := protocol.Send(c.writer, protocol.Message{
		Type:    protocol.MsgCreate,
		Payload: fmt.Sprintf("%d", int(selected.Seconds())),
	})
	if err != nil {
		terminal.Println("> Failed to communicate with server.")
		return fmt.Errorf("send create: %w", err)
	}

	var roomID, invitationToken, expiryStr, endpoint string

	for roomID == "" || invitationToken == "" || c.participantID == 0 {
		msg, err := protocol.Decode(c.reader)
		if err != nil {
			terminal.Println("> Lost connection to server.")
			return fmt.Errorf("read response: %w", err)
		}
		switch msg.Type {
		case protocol.MsgError:
			terminal.Printf("> Error: %s\n", msg.Payload)
			return nil
		case protocol.MsgRoomID:
			roomID = msg.Payload
		case protocol.MsgInvToken:
			invitationToken = msg.Payload
		case protocol.MsgExpiry:
			expiryStr = msg.Payload
		case protocol.MsgEndpoint:
			endpoint = msg.Payload
		case protocol.MsgParticipantID:
			fmt.Sscanf(msg.Payload, "%d", &c.participantID)
		case protocol.MsgSystem:
			terminal.Printf("> %s\n", msg.Payload)
		}
	}

	c.roomID = roomID
	c.endpoint = endpoint
	c.networkType = terminal.NetworkTypeFromEndpoint(endpoint)

	if expiryStr != "" {
		if t, err := time.Parse(time.RFC3339, expiryStr); err == nil {
			c.expiryTime = t
		}
	}

	terminal.Println("> Secure environment ready.")
	terminal.Println("")
	c.bundledInvite = invitation.BundledInvitation(invitationToken, endpoint)
	if err := clipboardCopy(c.bundledInvite); err == nil {
		terminal.Println("> Copied to clipboard. Share this invitation with the other person.")
	} else {
		terminal.Println("> Could not copy invitation automatically. Copy the invitation above manually.")
	}
	terminal.Println("> The invitation is temporary and can only be used once.")
	terminal.Println("")

	return c.chatLoop()
}

func (c *Client) joinRoom() error {
	terminal.Println("")
	terminal.Print("> Enter invitation: ")
	raw := c.readInput()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		terminal.Println("> No invitation entered.")
		return nil
	}

	token, endpoint, err := invitation.ParseInvite(raw)
	if err != nil {
		terminal.Println(terminal.RenderError("INVALID INVITATION", "This invitation is invalid or unavailable."))
		return nil
	}

	var joinAddr string
	if endpoint != "" {
		joinAddr = endpoint
	} else {
		terminal.Print("> Server address (leave empty for default): ")
		serverAddr := c.readInput()
		serverAddr = strings.TrimSpace(serverAddr)
		if serverAddr != "" {
			joinAddr = serverAddr
		} else {
			joinAddr = c.addr
		}
	}

	if joinAddr != c.addr {
		if endpoint != "" {
			terminal.Println("> Connecting to invitation endpoint...")
		} else {
			terminal.Println("> Reconnecting to specified server...")
		}
		tlsConfig, err := tlsutil.ClientConfig(c.certDir)
		if err != nil {
			terminal.Println("> Secure connection setup failed.")
			terminal.Println("> Please verify the certificate configuration.")
			return fmt.Errorf("TLS config: %w", err)
		}
		conn, err := tls.Dial("tcp", joinAddr, tlsConfig)
		if err != nil {
			terminal.Println("> Unable to connect to the specified server.")
			terminal.Println("> Check the server address and private network connection.")
			return fmt.Errorf("connect: %w", err)
		}
		c.conn.Close()
		c.conn = conn
		c.reader = bufio.NewReader(conn)
		c.writer = bufio.NewWriter(conn)
	}

	terminal.Println("")
	terminal.Println("> Verifying invitation...")

	err = protocol.Send(c.writer, protocol.Message{
		Type:    protocol.MsgJoin,
		Payload: token,
	})
	if err != nil {
		terminal.Println("> Failed to communicate with server.")
		return fmt.Errorf("send join: %w", err)
	}

	for {
		msg, err := protocol.Decode(c.reader)
		if err != nil {
			terminal.Println("> Lost connection to server.")
			return fmt.Errorf("read response: %w", err)
		}

		switch msg.Type {
		case protocol.MsgParticipantID:
			fmt.Sscanf(msg.Payload, "%d", &c.participantID)
		case protocol.MsgExpiry:
			c.expiryTime, _ = time.Parse(time.RFC3339, msg.Payload)
			remaining := time.Until(c.expiryTime)
			terminal.Printf("> Room expires in %s.\n", terminal.FormatDuration(remaining))
		case protocol.MsgEndpoint:
			c.endpoint = msg.Payload
			c.networkType = terminal.NetworkTypeFromEndpoint(msg.Payload)
		case protocol.MsgReady:
			terminal.Println("> Secure two-person session established.")
			terminal.Println("")
			return c.chatLoop()
		case protocol.MsgNotFound:
			terminal.Println(terminal.RenderError("UNABLE TO JOIN", "This invitation is invalid or unavailable."))
			return nil
		case protocol.MsgExpired:
			terminal.Println(terminal.RenderError("INVITATION EXPIRED", "This invitation has expired."))
			return nil
		case protocol.MsgUsed:
			terminal.Println(terminal.RenderError("INVITATION USED", "This invitation has already been used."))
			return nil
		case protocol.MsgFull:
			terminal.Println(terminal.RenderError("ROOM FULL", "This room already has two participants."))
			return nil
		case protocol.MsgError:
			terminal.Println(terminal.RenderError("ERROR", msg.Payload))
			return nil
		default:
			terminal.Println(terminal.RenderError("ERROR", "Unexpected response from server."))
			return nil
		}
	}
}

func (c *Client) chatLoop() error {
	if c.participantID == 2 {
		c.friendJoined = true
	}
	// Single initial render: WAITING 1/2 or ACTIVE 2/2 — one clean box, not duplicate
	{
		remaining := time.Until(c.expiryTime)
		expires := ""
		if remaining > 0 {
			expires = terminal.FormatCountdown(remaining)
		}
		if c.friendJoined {
			terminal.Println(terminal.RenderActiveHeader(c.roomID, expires, "ACTIVE"))
		} else {
			terminal.Println(terminal.RenderWaiting(c.roomID, expires, c.networkType, c.bundledInvite))
		}
		terminal.Println("")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	msgCh := make(chan *protocol.Message, 64)
	errCh := make(chan error, 1)
	leaveCh := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			msg, err := protocol.Decode(c.reader)
			if err != nil {
				errCh <- err
				return
			}
			msgCh <- msg
		}
	}()

	inputCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			inputCh <- scanner.Text()
		}
	}()

	warningsSent := make(map[int]bool)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	terminal.Print("> ")

	for {
		select {
		case <-ticker.C:
			remaining := time.Until(c.expiryTime)
			if remaining <= 0 {
				terminal.Println("")
				terminal.Println(terminal.RenderDestroyed())
				return nil
			}
			// Update header countdown in one stable location (header) without new lines
			expires := terminal.FormatCountdown(remaining)
			status := "WAITING"
			if c.friendJoined {
				status = "ACTIVE"
			}
			terminal.UpdateHeaderInPlace(c.roomID, expires, status)
			// Check warnings via single owner (no concurrent print)
			c.checkExpiryWarnings(warningsSent)
		case <-sigCh:
			protocol.Send(c.writer, protocol.Message{
				Type:    protocol.MsgLeave,
				Payload: "",
			})
			terminal.Println("")
			terminal.Println(terminal.RenderDestroyed())
			return nil

		case err := <-errCh:
			if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "EOF") {
				terminal.Println("")
				terminal.Println(terminal.RenderError("FRIEND LEFT", "The other participant has left. Session ended."))
				return nil
			}
			terminal.Println("")
			terminal.Println(terminal.RenderError("CONNECTION LOST", "Connection to the room was lost. Session ended."))
			return nil

		case <-leaveCh:
			terminal.Println("")
			terminal.Println(terminal.RenderDestroyed())
			return nil

		case msg := <-msgCh:
			switch msg.Type {
			case protocol.MsgChat:
				parts := strings.SplitN(msg.Payload, "|", 2)
				if len(parts) == 2 {
					senderID := 0
					fmt.Sscanf(parts[0], "%d", &senderID)
					content := parts[1]
					if senderID == c.participantID {
						terminal.Printf("%s[%s] You: %s\n", terminal.ClearLine(), terminal.FormatNowTimestamp(), content)
					} else {
						terminal.Printf("%s[%s] Friend: %s\n", terminal.ClearLine(), terminal.FormatNowTimestamp(), content)
					}
				}
			case protocol.MsgSystem:
				if strings.Contains(msg.Payload, "Friend joined") {
					c.friendJoined = true
					remaining := time.Until(c.expiryTime)
					expires := terminal.FormatCountdown(remaining)
					terminal.UpdateHeaderInPlace(c.roomID, expires, "ACTIVE")
					terminal.Printf("%s> %s\n", terminal.ClearLine(), "Friend joined. Secure two-person session established.")
				} else {
					terminal.Printf("%s> %s\n", terminal.ClearLine(), msg.Payload)
				}
			case protocol.MsgLeave:
				terminal.Println("")
				terminal.Println(terminal.RenderError("FRIEND LEFT", "The other participant has left. Session ended."))
				return nil
			}
			terminal.Print("> ")

		case input := <-inputCh:
			input = strings.TrimSpace(input)
			if input == "" {
				terminal.Print("> ")
				continue
			}
			// Waiting-room controls: C/H/Q only when still waiting (1/2)
			if !c.friendJoined {
				switch strings.ToLower(input) {
				case "c":
					if c.bundledInvite != "" {
						_ = clipboardCopy(c.bundledInvite)
						terminal.Println("> Invitation copied to clipboard.")
					} else {
						terminal.Println("> No invitation to copy.")
					}
					terminal.Print("> ")
					continue
				case "h":
					c.showHelp()
					terminal.Print("> ")
					continue
				case "q":
					protocol.Send(c.writer, protocol.Message{Type: protocol.MsgLeave, Payload: ""})
					terminal.Println(terminal.RenderDestroyed())
					return nil
				}
			}
			if strings.HasPrefix(input, "/") {
				if c.handleCommand(input, done) {
					return nil
				}
				terminal.Print("> ")
				continue
			}

			err := protocol.Send(c.writer, protocol.Message{
				Type:    protocol.MsgChat,
				Payload: input,
			})
			if err != nil {
				terminal.Println("> Failed to send message.")
				return nil
			}
			terminal.Print("> ")
		}
	}
}

func (c *Client) handleCommand(input string, done chan struct{}) bool {
	cmd := strings.ToLower(strings.TrimSpace(input))
	switch cmd {
	case "/help":
		c.showHelp()
		return false
	case "/info":
		c.showInfo()
		return false
	case "/quit":
		// Attempt graceful leave; ignore errors if connection already dead
		if c.writer != nil {
			_ = protocol.Send(c.writer, protocol.Message{
				Type:    protocol.MsgLeave,
				Payload: "",
			})
		}
		terminal.Println(terminal.RenderDestroyed())
		return true
	default:
		terminal.Println(terminal.RenderError("UNKNOWN COMMAND", "Type /help for available commands."))
		return false
	}
}

func (c *Client) showHelp() {
	terminal.Println(terminal.RenderHelp())
}

func (c *Client) showInfo() {
	remaining := time.Until(c.expiryTime)
	// WAITING 1/2 before friend joins, ACTIVE 2/2 after
	state := "waiting"
	count := 1
	if c.friendJoined {
		state = "active"
		count = 2
	}
	status := terminal.SessionStatusText(
		c.friendJoined,
		state,
		count,
	)
	if status == "WAITING" && c.friendJoined {
		status = "ACTIVE"
	}
	if status == "ACTIVE" && !c.friendJoined {
		status = "WAITING"
	}

	expiresStr := "expired"
	if remaining > 0 {
		expiresStr = "in " + terminal.FormatDuration(remaining)
	}
	terminal.Println(terminal.RenderInfo(c.roomID, status, "You + Friend", expiresStr, c.networkType))
}

func (c *Client) checkExpiryWarnings(warningsSent map[int]bool) {
	if c.expiryTime.IsZero() {
		return
	}
	remaining := time.Until(c.expiryTime)
	secs := int(remaining.Seconds())

	thresholds := []struct {
		secs int
		msg  string
	}{
		{300, "> Room expires in 5 minutes."},
		{120, "> Room expires in 2 minutes."},
		{60, "> Room expires in 1 minute."},
		{30, "> Room expires in 30 seconds."},
	}

	for _, t := range thresholds {
		if secs <= t.secs && !warningsSent[t.secs] {
			warningsSent[t.secs] = true
			terminal.Println(t.msg)
			terminal.Print("> ")
		}
	}
}

func (c *Client) runCountdown(done chan struct{}, leaveCh chan struct{}, warningsSent map[int]bool) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-leaveCh:
			return
		case <-ticker.C:
			if c.expiryTime.IsZero() {
				continue
			}
			remaining := time.Until(c.expiryTime)
			if remaining <= 0 {
				terminal.Println("")
				terminal.Println(terminal.RenderDestroyed())
				select {
				case leaveCh <- struct{}{}:
				default:
				}
				return
			}
			// Coordinated update: same header in-place logic as chatLoop, no spam line
			expires := terminal.FormatCountdown(remaining)
			status := "WAITING"
			if c.friendJoined {
				status = "ACTIVE"
			}
			terminal.UpdateHeaderInPlace(c.roomID, expires, status)
			c.checkExpiryWarnings(warningsSent)
		}
	}
}

func (c *Client) readInput() string {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func (c *Client) selectDuration() time.Duration {
	selected := 10 * time.Minute
	for {
		terminal.Println(terminal.RenderDurationSelector(selected))
		terminal.Print("> Select duration: ")
		raw := c.readInput()
		if d, ok := parseDurationInput(raw); ok {
			return d
		}
		terminal.Println(terminal.RenderError("INVALID SELECTION", "Enter 1-5, or 10/20/30/45/60 (Enter for 10m)."))
	}
}

// parseDurationInput parses user input for duration selection.
// Returns duration and true if valid, or 0 and false if invalid.
// Empty input defaults to 10 minutes.
func parseDurationInput(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 10 * time.Minute, true
	}
	switch raw {
	case "1", "10", "10m", "10 minutes", "10 minute":
		return 10 * time.Minute, true
	case "2", "20", "20m", "20 minutes", "20 minute":
		return 20 * time.Minute, true
	case "3", "30", "30m", "30 minutes", "30 minute":
		return 30 * time.Minute, true
	case "4", "45", "45m", "45 minutes", "45 minute":
		return 45 * time.Minute, true
	case "5", "60", "60m", "60 minutes", "60 minute":
		return 60 * time.Minute, true
	}
	if n, err := strconv.Atoi(raw); err == nil {
		switch n {
		case 10:
			return 10 * time.Minute, true
		case 20:
			return 20 * time.Minute, true
		case 30:
			return 30 * time.Minute, true
		case 45:
			return 45 * time.Minute, true
		case 60:
			return 60 * time.Minute, true
		}
	}
	trimmed := strings.TrimSuffix(raw, "m")
	trimmed = strings.TrimSpace(trimmed)
	if n, err := strconv.Atoi(trimmed); err == nil {
		switch n {
		case 10:
			return 10 * time.Minute, true
		case 20:
			return 20 * time.Minute, true
		case 30:
			return 30 * time.Minute, true
		case 45:
			return 45 * time.Minute, true
		case 60:
			return 60 * time.Minute, true
		}
	}
	return 0, false
}

func (c *Client) printRoomInfoBox() {
	remaining := time.Until(c.expiryTime)
	remainingStr := terminal.FormatDuration(remaining)
	if remaining <= 0 {
		remainingStr = "expired"
	}

	terminal.Println("+----------------------------------------------+")
	terminal.Println("| PRIVATE ROOM                                 |")
	terminal.Println("|                                              |")
	terminal.Printf("| Room       : %-33s |\n", c.roomID)
	terminal.Printf("| Expires    : %-33s |\n", "in "+remainingStr)
	terminal.Printf("| Network    : %-33s |\n", c.networkType)
	terminal.Println("| Security   : TLS + invitation                |")
	terminal.Println("|                                              |")
	terminal.Println("+----------------------------------------------+")
}

func (c *Client) printInvitationBlock(token string) {
	terminal.Println("INVITATION")
	terminal.Println(terminal.Separator())
	terminal.Println(token)
	terminal.Println(terminal.Separator())
}
