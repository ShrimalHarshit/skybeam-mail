// Package imap provides a thin wrapper around go-imap/v2 for internal
// Dovecot access. End-users never touch IMAP directly — this is
// only for the watcher and reconciler services.
package imap

import (
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Client wraps an IMAP connection to Dovecot.
type Client struct {
	c *imapclient.Client
}

// Message is a parsed IMAP message summary (headers only, no body).
type Message struct {
	UID        imap.UID
	SeqNum     uint32
	Flags      []imap.Flag
	Envelope   *imap.Envelope
	SizeBytes  int64
}

// FolderInfo holds the result of a LIST command.
type FolderInfo struct {
	Name string
}

// DialNoTLS connects to the given host:port without TLS.
// For internal Docker network use only.
func DialNoTLS(addr string) (*Client, error) {
	c, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{c: c}, nil
}

// Login authenticates with the given credentials.
func (c *Client) Login(username, password string) error {
	if err := c.c.Login(username, password).Wait(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

// Logout gracefully closes the IMAP connection.
func (c *Client) Logout() {
	_ = c.c.Logout().Wait()
}

// Close forcibly closes the connection.
func (c *Client) Close() {
	_ = c.c.Close()
}

// ListFolders returns all mailbox names for the authenticated account.
func (c *Client) ListFolders() ([]FolderInfo, error) {
	data, err := c.c.List("", "*", nil).Collect()
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	folders := make([]FolderInfo, 0, len(data))
	for _, m := range data {
		folders = append(folders, FolderInfo{Name: m.Mailbox})
	}
	return folders, nil
}

// Select selects the given mailbox and returns SelectData.
func (c *Client) Select(mailbox string) (*imap.SelectData, error) {
	data, err := c.c.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("select %s: %w", mailbox, err)
	}
	return data, nil
}

// FetchHeaders fetches envelope + flags + size for all messages using
// a sequence set (e.g. "1:*"). Returns parsed Message summaries.
func (c *Client) FetchHeaders(seqSet imap.SeqSet) ([]Message, error) {
	fetchOpts := &imap.FetchOptions{
		UID:        true,
		Flags:      true,
		Envelope:   true,
		RFC822Size: true,
	}

	cmd := c.c.Fetch(seqSet, fetchOpts)
	msgs, err := cmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch headers: %w", err)
	}

	results := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		results = append(results, Message{
			UID:       m.UID,
			SeqNum:    m.SeqNum,
			Flags:     m.Flags,
			Envelope:  m.Envelope,
			SizeBytes: m.RFC822Size,
		})
	}
	return results, nil
}

// FetchBody fetches the full RFC 2822 message body for a single UID.
func (c *Client) FetchBody(uid imap.UID) ([]byte, error) {
	seqSet := imap.UIDSet{imap.UIDRange{Start: uid, Stop: uid}}
	bodySection := &imap.FetchItemBodySection{}
	fetchOpts := &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	cmd := c.c.Fetch(seqSet, fetchOpts)
	msgs, err := cmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch body uid %d: %w", uid, err)
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("message not found: uid %d", uid)
	}

	for k, v := range msgs[0].BodySection {
		_ = k
		return v, nil
	}
	return nil, fmt.Errorf("no body section returned")
}

// AddFlags adds IMAP flags to a message by sequence number.
func (c *Client) AddFlags(seqNum uint32, flags ...imap.Flag) error {
	seqSet := imap.SeqSetNum(seqNum)
	cmd := c.c.Store(seqSet, &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Flags:  flags,
		Silent: true,
	}, nil)
	_, err := cmd.Collect()
	if err != nil {
		return fmt.Errorf("add flags seq %d: %w", seqNum, err)
	}
	return nil
}

// RemoveFlags removes IMAP flags from a message by sequence number.
func (c *Client) RemoveFlags(seqNum uint32, flags ...imap.Flag) error {
	seqSet := imap.SeqSetNum(seqNum)
	cmd := c.c.Store(seqSet, &imap.StoreFlags{
		Op:     imap.StoreFlagsDel,
		Flags:  flags,
		Silent: true,
	}, nil)
	_, err := cmd.Collect()
	if err != nil {
		return fmt.Errorf("remove flags seq %d: %w", seqNum, err)
	}
	return nil
}

// MoveMessage moves a message (by UID) to dest mailbox.
func (c *Client) MoveMessage(uid imap.UID, dest string) error {
	uidSet := imap.UIDSet{imap.UIDRange{Start: uid, Stop: uid}}
	_, err := c.c.Move(uidSet, dest).Wait()
	if err != nil {
		return fmt.Errorf("move uid %d to %s: %w", uid, dest, err)
	}
	return nil
}

// Expunge permanently removes messages with the \Deleted flag.
func (c *Client) Expunge() error {
	_, err := c.c.Expunge().Collect()
	return err
}

// Idle starts IMAP IDLE and blocks until timeout or a server push.
// The caller should re-enter Idle in a loop.
func (c *Client) Idle(timeout time.Duration) error {
	idleCmd, err := c.c.Idle()
	if err != nil {
		return fmt.Errorf("idle: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		time.Sleep(timeout)
		done <- idleCmd.Close()
	}()

	// Wait() returns when IDLE ends (either we closed it or server pushed).
	if err := idleCmd.Wait(); err != nil {
		return fmt.Errorf("idle wait: %w", err)
	}
	<-done
	return nil
}
