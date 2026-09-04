package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	maxUsers  = 2
	roomTitle = "TERMINAL ROOM v0.8"
)

type Client struct {
	channel ssh.Channel
	name    string
}

var (
	clients   = make(map[ssh.Channel]*Client)
	clientsMu sync.Mutex
)

// ============================================================
// HOST KEY
// ============================================================

func loadOrCreateHostKey(path string) (ssh.Signer, error) {

	// --------------------------------------------------------
	// ENV VAR: SSH_HOST_KEY
	// --------------------------------------------------------

	if envKey := os.Getenv("SSH_HOST_KEY"); envKey != "" {

		block, _ := pem.Decode([]byte(envKey))

		if block == nil {
			return nil, fmt.Errorf("invalid PEM in SSH_HOST_KEY")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)

		if err != nil {
			return nil, fmt.Errorf("invalid RSA key in SSH_HOST_KEY: %w", err)
		}

		fmt.Println("[SYSTEM] Host key loaded from SSH_HOST_KEY env var.")

		return ssh.NewSignerFromKey(key)
	}

	// --------------------------------------------------------
	// FILE: host_key.pem (local development)
	// --------------------------------------------------------

	data, err := os.ReadFile(path)

	if err == nil {

		block, _ := pem.Decode(data)

		if block == nil {
			return nil, fmt.Errorf("invalid PEM host key")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)

		if err != nil {
			return nil, fmt.Errorf("invalid RSA host key: %w", err)
		}

		fmt.Println("[SYSTEM] Existing host key loaded.")

		return ssh.NewSignerFromKey(key)
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read host key: %w", err)
	}

	fmt.Println("[SYSTEM] Generating SSH host key...")

	key, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return nil, fmt.Errorf("RSA generation failed: %w", err)
	}

	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	err = os.WriteFile(
		path,
		pem.EncodeToMemory(pemBlock),
		0600,
	)

	if err != nil {
		return nil, fmt.Errorf("cannot save host key: %w", err)
	}

	fmt.Println("[SYSTEM] Host key created.")

	return ssh.NewSignerFromKey(key)
}

// ============================================================
// BROADCAST
// ============================================================

func broadcast(message string, sender ssh.Channel) {

	clientsMu.Lock()
	defer clientsMu.Unlock()

	for ch := range clients {

		if ch == sender {
			continue
		}

		_, err := io.WriteString(ch, message)

		if err != nil {
			fmt.Println("[BROADCAST ERROR]", err)
		}
	}
}

// ============================================================
// MAIN
// ============================================================

func main() {

	// --------------------------------------------------------
	// ROOM PASSWORD
	// --------------------------------------------------------

	roomPassword := os.Getenv("ROOM_PASSWORD")

	if roomPassword == "" {
		fmt.Println("[ERROR] ROOM_PASSWORD environment variable is not set.")
		fmt.Println("[ERROR] Set ROOM_PASSWORD before starting the server.")
		return
	}

	// --------------------------------------------------------
	// PORT ENVIRONMENT VARIABLE
	// --------------------------------------------------------

	listenPort := os.Getenv("PORT")

	if listenPort == "" {
		listenPort = "2222"
	}

	if !strings.HasPrefix(listenPort, ":") {
		listenPort = ":" + listenPort
	}

	// --------------------------------------------------------
	// STARTUP
	// --------------------------------------------------------

	fmt.Println("==========================================")
	fmt.Println("        TERMINAL ROOM SSH SERVER")
	fmt.Println("==========================================")
	fmt.Println()

	signer, err := loadOrCreateHostKey("host_key.pem")

	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	// --------------------------------------------------------
	// SSH SERVER CONFIGURATION
	// --------------------------------------------------------

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if subtle.ConstantTimeCompare(password, []byte(roomPassword)) != 1 {
				return nil, fmt.Errorf("incorrect password")
			}
			return &ssh.Permissions{}, nil
		},

		AuthLogCallback: func(conn ssh.ConnMetadata, method string, err error) {
			fmt.Printf(
				"[AUTH] User=%s Method=%s Error=%v\n",
				conn.User(),
				method,
				err,
			)
		},
	}

	config.AddHostKey(signer)

	// --------------------------------------------------------
	// TCP LISTENER
	// --------------------------------------------------------

	listener, err := net.Listen("tcp", listenPort)

	if err != nil {
		fmt.Println("[ERROR] Could not start server:", err)
		return
	}

	defer listener.Close()

	fmt.Println("Version       :", roomTitle)
	fmt.Println("Protocol      : SSH")
	fmt.Println("Port          :", strings.TrimPrefix(listenPort, ":"))
	fmt.Println("Maximum users :", maxUsers)
	fmt.Println()
	fmt.Println("Server is listening on 0.0.0.0" + listenPort)
	fmt.Println()
	fmt.Println("Waiting for connections...")
	fmt.Println()

	// --------------------------------------------------------
	// GRACEFUL SHUTDOWN
	// --------------------------------------------------------

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {

		sig := <-sigChan

		fmt.Println()
		fmt.Println("[SYSTEM] Received signal:", sig)
		fmt.Println("[SYSTEM] Shutting down gracefully...")

		listener.Close()

		fmt.Println("[SYSTEM] Listener closed. Server stopped.")
	}()

	// --------------------------------------------------------
	// ACCEPT CONNECTIONS
	// --------------------------------------------------------

	for {

		conn, err := listener.Accept()

		if err != nil {

			// If the listener was closed by shutdown, exit the loop.
			if errors.Is(err, net.ErrClosed) {
				break
			}

			fmt.Println("[ERROR] Accept:", err)

			continue
		}

		fmt.Println("------------------------------------------")
		fmt.Println(
			"[CONNECTION] Incoming connection from:",
			conn.RemoteAddr(),
		)
		fmt.Println("------------------------------------------")

		go handleConnection(conn, config)
	}
}

// ============================================================
// SSH CONNECTION
// ============================================================

func handleConnection(
	conn net.Conn,
	config *ssh.ServerConfig,
) {

	remoteAddr := conn.RemoteAddr().String()

	// If SSH handshake fails, close TCP connection.
	sshConn, chans, reqs, err :=
		ssh.NewServerConn(conn, config)

	if err != nil {

		fmt.Println("[SSH ERROR] Handshake failed from:", remoteAddr)
		fmt.Println("[SSH ERROR]", err)

		conn.Close()

		return
	}

	fmt.Println("[SSH] Handshake successful!")
	fmt.Println("[SSH] User:", sshConn.User())
	fmt.Println("[SSH] Remote:", sshConn.RemoteAddr())
	fmt.Println("[SSH] Client version:", string(sshConn.ClientVersion()))

	// Handle global SSH requests.
	go ssh.DiscardRequests(reqs)

	// --------------------------------------------------------
	// SESSION CHANNELS
	// --------------------------------------------------------

	for newChannel := range chans {

		fmt.Println(
			"[SSH] New channel:",
			newChannel.ChannelType(),
		)

		if newChannel.ChannelType() != "session" {

			fmt.Println(
				"[SSH] Rejecting unsupported channel:",
				newChannel.ChannelType(),
			)

			newChannel.Reject(
				ssh.UnknownChannelType,
				"session required",
			)

			continue
		}

		channel, requests, err :=
			newChannel.Accept()

		if err != nil {

			fmt.Println(
				"[SSH ERROR] Channel accept:",
				err,
			)

			continue
		}

		fmt.Println("[SSH] Session channel accepted.")

		go handleSession(channel, requests)
	}

	fmt.Println("[SSH] SSH channel stream closed:", remoteAddr)

	sshConn.Close()

	fmt.Println("[SSH] Connection closed:", remoteAddr)
}

// ============================================================
// SESSION
// ============================================================

func handleSession(
	channel ssh.Channel,
	requests <-chan *ssh.Request,
) {

	defer func() {

		fmt.Println("[SESSION] Closing session.")

		channel.Close()

	}()

	// --------------------------------------------------------
	// SSH REQUEST HANDLING
	// --------------------------------------------------------

	shellReady := make(chan struct{}, 1)

	go func() {

		for req := range requests {

			fmt.Println("[SSH REQUEST]", req.Type)

			switch req.Type {

			case "pty-req":

				fmt.Println("[SSH] PTY requested.")

				if err := req.Reply(true, nil); err != nil {
					fmt.Println("[SSH ERROR] PTY reply:", err)
				}

			case "shell":

				fmt.Println("[SSH] Shell requested.")

				if err := req.Reply(true, nil); err != nil {
					fmt.Println("[SSH ERROR] Shell reply:", err)
				}

				select {

				case shellReady <- struct{}{}:

				default:
				}

			case "window-change":

				fmt.Println("[SSH] Window size changed.")

				req.Reply(true, nil)

			case "env":

				// Accept environment variables without using them.
				req.Reply(true, nil)

			case "exec":

				// We don't execute operating-system commands.
				fmt.Println("[SSH] EXEC request rejected.")

				req.Reply(
					false,
					nil,
				)

			default:

				fmt.Println(
					"[SSH] Unsupported request:",
					req.Type,
				)

				req.Reply(false, nil)
			}
		}

		fmt.Println("[SSH] Request channel closed.")
	}()

	// --------------------------------------------------------
	// WAIT FOR SHELL
	// --------------------------------------------------------

	<-shellReady

	fmt.Println("[SESSION] Shell is ready.")

	writer := bufio.NewWriter(channel)

	// --------------------------------------------------------
	// WELCOME SCREEN
	// --------------------------------------------------------

	clearScreen(writer)

	printBox(
		writer,
		roomTitle,
		"A private room for two people",
	)

	fmt.Fprintln(writer)

	fmt.Fprintln(
		writer,
		"Welcome to Terminal Room.",
	)

	fmt.Fprintln(writer)

	fmt.Fprint(
		writer,
		"Enter your name: ",
	)

	writer.Flush()

	// --------------------------------------------------------
	// READ NAME
	// --------------------------------------------------------

	name, connected := readLine(channel, writer)

	if !connected {

		fmt.Println("[SESSION] Client disconnected while entering name.")

		return
	}

	name = strings.TrimSpace(name)

	if name == "" {
		name = "Anonymous"
	}

	// Prevent excessively long names.
	if len(name) > 30 {
		name = name[:30]
	}

	fmt.Println("[USER] Name entered:", name)

	// --------------------------------------------------------
	// ROOM LIMIT
	// --------------------------------------------------------

	clientsMu.Lock()

	if len(clients) >= maxUsers {

		clientsMu.Unlock()

		fmt.Fprintln(writer)
		fmt.Fprintln(writer)
		fmt.Fprintln(
			writer,
			"╔══════════════════════════════════════════╗",
		)
		fmt.Fprintln(
			writer,
			"║              ROOM IS FULL               ║",
		)
		fmt.Fprintln(
			writer,
			"╚══════════════════════════════════════════╝",
		)
		fmt.Fprintln(writer)
		fmt.Fprintln(
			writer,
			"Maximum users:",
			maxUsers,
		)
		fmt.Fprintln(writer)

		writer.Flush()

		fmt.Println(
			"[ROOM] Rejected user:",
			name,
			"(room full)",
		)

		return
	}

	clients[channel] = &Client{
		channel: channel,
		name:    name,
	}

	count := len(clients)

	clientsMu.Unlock()

	fmt.Println("[ROOM] User joined:", name)
	fmt.Printf(
		"[ROOM] Users: %d/%d\n",
		count,
		maxUsers,
	)

	// --------------------------------------------------------
	// ROOM SCREEN
	// --------------------------------------------------------

	clearScreen(writer)

	printBox(
		writer,
		"YOU ARE INSIDE THE ROOM",
		"",
	)

	fmt.Fprintln(writer)

	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  /quit    Leave the room")
	fmt.Fprintln(writer)

	fmt.Fprintf(
		writer,
		"People in room: %d/%d\n",
		count,
		maxUsers,
	)

	fmt.Fprintln(writer)

	fmt.Fprintln(
		writer,
		"------------------------------------------",
	)

	fmt.Fprintln(writer)

	fmt.Fprint(writer, "> ")

	writer.Flush()

	// --------------------------------------------------------
	// JOIN NOTIFICATION
	// --------------------------------------------------------

	broadcast(
		fmt.Sprintf(
			"\r\n[ROOM] %s joined the room.\r\n> ",
			name,
		),
		channel,
	)

	// --------------------------------------------------------
	// CHAT LOOP
	// --------------------------------------------------------

	for {

		message, connected := readLine(
			channel,
			writer,
		)

		if !connected {

			fmt.Println(
				"[ROOM] Connection lost:",
				name,
			)

			break
		}

		message = strings.TrimSpace(message)

		// Empty message.
		if message == "" {

			fmt.Fprint(
				writer,
				"> ",
			)

			writer.Flush()

			continue
		}

		// ----------------------------------------------------
		// QUIT
		// ----------------------------------------------------

		if message == "/quit" {

			fmt.Println(
				"[ROOM] User requested exit:",
				name,
			)

			break
		}

		// ----------------------------------------------------
		// MESSAGE
		// ----------------------------------------------------

		formatted := fmt.Sprintf(
			"\r\n[%s] %s: %s\r\n> ",
			time.Now().Format("15:04"),
			name,
			message,
		)

		fmt.Println(
			"[MESSAGE]",
			name+":",
			message,
		)

		// Send to other users.
		broadcast(
			formatted,
			channel,
		)

		// Show to sender.
		fmt.Fprint(
			writer,
			formatted,
		)

		writer.Flush()
	}

	// --------------------------------------------------------
	// REMOVE USER
	// --------------------------------------------------------

	clientsMu.Lock()

	delete(
		clients,
		channel,
	)

	remaining := len(clients)

	clientsMu.Unlock()

	fmt.Printf(
		"[ROOM] %s left. Users remaining: %d/%d\n",
		name,
		remaining,
		maxUsers,
	)

	// --------------------------------------------------------
	// LEAVE NOTIFICATION
	// --------------------------------------------------------

	broadcast(
		fmt.Sprintf(
			"\r\n[ROOM] %s left the room.\r\n> ",
			name,
		),
		channel,
	)
}

// ============================================================
// CLEAR SCREEN
// ============================================================

func clearScreen(
	writer *bufio.Writer,
) {

	fmt.Fprint(
		writer,
		"\033[2J\033[H",
	)

	writer.Flush()
}

// ============================================================
// BOX
// ============================================================

func printBox(
	writer *bufio.Writer,
	title string,
	subtitle string,
) {

	fmt.Fprintln(
		writer,
		"╔══════════════════════════════════════════╗",
	)

	fmt.Fprintf(
		writer,
		"║ %-40s ║\n",
		title,
	)

	if subtitle != "" {

		fmt.Fprintf(
			writer,
			"║ %-40s ║\n",
			subtitle,
		)
	}

	fmt.Fprintln(
		writer,
		"╚══════════════════════════════════════════╝",
	)
}

// ============================================================
// INPUT
// ============================================================

func readLine(
	channel ssh.Channel,
	writer *bufio.Writer,
) (string, bool) {

	var line []byte

	buf := make([]byte, 1)

	for {

		n, err := channel.Read(buf)

		if err != nil {

			if err == io.EOF {
				return string(line), false
			}

			return string(line), false
		}

		if n == 0 {
			continue
		}

		b := buf[0]

		switch b {

		case '\r':

			fmt.Fprint(
				writer,
				"\r\n",
			)

			writer.Flush()

			return string(line), true

		case '\n':

			// Ignore LF if CRLF was already handled.
			// This prevents an extra empty message.
			continue

		case 127, 8:

			if len(line) > 0 {

				line = line[:len(line)-1]

				fmt.Fprint(
					writer,
					"\b \b",
				)

				writer.Flush()
			}

		case 3:

			// Ctrl+C
			fmt.Fprint(
				writer,
				"^C\r\n",
			)

			writer.Flush()

			return "/quit", true

		default:

			line = append(
				line,
				b,
			)

			fmt.Fprint(
				writer,
				string(b),
			)

			writer.Flush()
		}
	}
}
