package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type MsgType string

const (
	MsgCreate        MsgType = "CREATE"
	MsgJoin          MsgType = "JOIN"
	MsgRoomID        MsgType = "ROOM_ID"
	MsgInvToken      MsgType = "INV_TOKEN"
	MsgParticipantID MsgType = "PARTICIPANT_ID"
	MsgReady         MsgType = "READY"
	MsgChat          MsgType = "CHAT"
	MsgLeave         MsgType = "LEAVE"
	MsgError         MsgType = "ERROR"
	MsgSystem        MsgType = "SYSTEM"
	MsgFull          MsgType = "FULL"
	MsgNotFound      MsgType = "NOT_FOUND"
	MsgExpired       MsgType = "EXPIRED"
	MsgUsed          MsgType = "USED"
	MsgExpiry        MsgType = "EXPIRY"
	MsgEndpoint      MsgType = "ENDPOINT"
)

type Message struct {
	Type    MsgType
	Payload string
}

// MaxPayloadSize is the maximum allowed payload length for a protocol message.
// 4 KiB covers all legitimate control messages (<100 bytes) and chat messages
// (user input, invitation token 54, endpoint ~21, expiry ~20) with generous margin
// while preventing peer-controlled unbounded allocation.
const MaxPayloadSize = 4096

// MaxLineSize is the maximum total frame length (Type + "|" + Payload + "\n").
// 8 KiB = 2 * MaxPayload plus overhead, ensures a single chat line cannot force
// multi-megabyte allocation on the receiver.
const MaxLineSize = 8192

func (m Message) Encode() string {
	return fmt.Sprintf("%s|%s\n", m.Type, m.Payload)
}

// readLineLimited reads a newline-terminated line with an explicit byte limit.
// It returns the line including the trailing "\n" on success.
// If the line exceeds limit without a newline, it returns "message too large".
func readLineLimited(r *bufio.Reader, limit int) (string, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		// Copy chunk because ReadSlice's slice is invalid after next Read
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		if len(buf)+len(copied) > limit {
			return "", fmt.Errorf("message too large: exceeds %d bytes", limit)
		}
		buf = append(buf, copied...)
		if err == nil {
			return string(buf), nil
		}
		if err == bufio.ErrBufferFull {
			// No newline yet, continue reading
			continue
		}
		if err == io.EOF {
			return "", fmt.Errorf("incomplete message: missing newline")
		}
		return "", err
	}
}

func Decode(reader *bufio.Reader) (*Message, error) {
	line, err := readLineLimited(reader, MaxLineSize)
	if err != nil {
		return nil, err
	}

	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, fmt.Errorf("empty message")
	}

	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed message: %s", line)
	}

	payload := parts[1]
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes exceeds %d", len(payload), MaxPayloadSize)
	}

	return &Message{
		Type:    MsgType(parts[0]),
		Payload: payload,
	}, nil
}

func Send(w io.Writer, msg Message) error {
	if len(msg.Payload) > MaxPayloadSize {
		return fmt.Errorf("payload too large: %d bytes exceeds %d", len(msg.Payload), MaxPayloadSize)
	}
	enc := msg.Encode()
	if len(enc) > MaxLineSize {
		return fmt.Errorf("message too large: %d bytes exceeds %d", len(enc), MaxLineSize)
	}
	_, err := fmt.Fprint(w, enc)
	if err != nil {
		return err
	}
	if f, ok := w.(*bufio.Writer); ok {
		f.Flush()
	}
	return nil
}
