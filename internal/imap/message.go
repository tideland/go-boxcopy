package imap

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Standard IMAP flags.
const (
	FlagSeen     = "\\Seen"
	FlagAnswered = "\\Answered"
	FlagFlagged  = "\\Flagged"
	FlagDeleted  = "\\Deleted"
	FlagDraft    = "\\Draft"
)

// Message represents an email message with metadata.
type Message struct {
	UID          uint32
	SeqNum       uint32
	Flags        []string
	InternalDate time.Time
	Size         uint32
	Envelope     *Envelope
	Body         []byte
}

// Envelope contains message envelope information.
type Envelope struct {
	Date      time.Time
	Subject   string
	From      []Address
	To        []Address
	Cc        []Address
	Bcc       []Address
	ReplyTo   []Address
	MessageID string
	InReplyTo string
}

// Address represents an email address.
type Address struct {
	Name    string
	Mailbox string
	Host    string
}

// String returns the formatted email address.
func (a Address) String() string {
	if a.Name != "" {
		return fmt.Sprintf("%s <%s@%s>", a.Name, a.Mailbox, a.Host)
	}
	return fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
}

// FetchOptions configures what data to fetch for messages.
type FetchOptions struct {
	UID          bool
	Flags        bool
	InternalDate bool
	Size         bool
	Envelope     bool
	Body         bool
	BodyPeek     bool // If true, don't mark as seen
}

// DefaultFetchOptions returns options for fetching message metadata.
func DefaultFetchOptions() *FetchOptions {
	return &FetchOptions{
		UID:          true,
		Flags:        true,
		InternalDate: true,
		Size:         true,
		Envelope:     true,
		Body:         false,
	}
}

// FullFetchOptions returns options for fetching complete messages.
func FullFetchOptions() *FetchOptions {
	return &FetchOptions{
		UID:          true,
		Flags:        true,
		InternalDate: true,
		Size:         true,
		Envelope:     true,
		Body:         true,
		BodyPeek:     true,
	}
}

// buildFetchOpts converts FetchOptions into the imap library's FetchOptions.
func buildFetchOpts(opts *FetchOptions) *imap.FetchOptions {
	fo := &imap.FetchOptions{
		UID:          opts.UID,
		Flags:        opts.Flags,
		InternalDate: opts.InternalDate,
		RFC822Size:   opts.Size,
		Envelope:     opts.Envelope,
	}
	if opts.Body {
		fo.BodySection = []*imap.FetchItemBodySection{{Peek: opts.BodyPeek}}
	}
	return fo
}

// parseFetchMessage maps the items from a single IMAP fetch response into a
// Message. It returns an error if the server returns an invalid UID (0) or
// omits the UID when opts.UID is true.
func parseFetchMessage(msg *imapclient.FetchMessageData, opts *FetchOptions) (Message, error) {
	message := Message{SeqNum: msg.SeqNum}

	for {
		item := msg.Next()
		if item == nil {
			break
		}

		switch data := item.(type) {
		case imapclient.FetchItemDataUID:
			if data.UID == 0 {
				return Message{}, fmt.Errorf("server returned invalid UID 0 for message seqnum %d", msg.SeqNum)
			}
			message.UID = uint32(data.UID)
		case imapclient.FetchItemDataFlags:
			message.Flags = convertFlags(data.Flags)
		case imapclient.FetchItemDataInternalDate:
			message.InternalDate = data.Time
		case imapclient.FetchItemDataRFC822Size:
			message.Size = uint32(data.Size)
		case imapclient.FetchItemDataEnvelope:
			message.Envelope = convertEnvelope(data.Envelope)
		case imapclient.FetchItemDataBodySection:
			body, err := io.ReadAll(data.Literal)
			if err != nil {
				return Message{}, fmt.Errorf("failed to read message body: %w", err)
			}
			message.Body = body
		}
	}

	if opts.UID && message.UID == 0 {
		return Message{}, fmt.Errorf("server did not return UID for message seqnum %d", msg.SeqNum)
	}

	return message, nil
}

// drainFetchCmd collects all messages from a fetch command into a slice.
func drainFetchCmd(fetchCmd *imapclient.FetchCommand, opts *FetchOptions) ([]Message, error) {
	var messages []Message
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		m, err := parseFetchMessage(msg, opts)
		if err != nil {
			fetchCmd.Close() //nolint:errcheck
			return nil, err
		}
		messages = append(messages, m)
	}
	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	return messages, nil
}

// FetchByUID fetches messages by their UIDs.
func (c *Client) FetchByUID(uids []uint32, opts *FetchOptions) ([]Message, error) {
	if len(uids) == 0 {
		return nil, nil
	}

	if opts == nil {
		opts = DefaultFetchOptions()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("fetching messages by UID", slog.Int("count", len(uids)))

	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	messages, err := drainFetchCmd(c.client.Fetch(uidSet, buildFetchOpts(opts)), opts)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("messages fetched", slog.Int("count", len(messages)))
	return messages, nil
}

// FetchAll fetches all messages in the currently selected mailbox using a
// UID range (1:*) to avoid sequence-number instability from concurrent expunges.
func (c *Client) FetchAll(opts *FetchOptions) ([]Message, error) {
	if opts == nil {
		opts = DefaultFetchOptions()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("fetching all messages")

	// Use UID range 1:* — stable against concurrent expunges.
	uidSet := imap.UIDSet{}
	uidSet.AddRange(imap.UID(1), 0) // 0 means * (last message)

	messages, err := drainFetchCmd(c.client.Fetch(uidSet, buildFetchOpts(opts)), opts)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("all messages fetched", slog.Int("count", len(messages)))
	return messages, nil
}

// FetchAllSizes fetches RFC822.SIZE for every message in the currently
// selected mailbox and returns the total in bytes. Uses a UID range (1:*)
// to avoid sequence-number instability from concurrent expunges.
func (c *Client) FetchAllSizes() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("fetching all message sizes")

	// Use UID range 1:* — stable against concurrent expunges.
	uidSet := imap.UIDSet{}
	uidSet.AddRange(imap.UID(1), 0) // 0 means * (last message)

	fetchCmd := c.client.Fetch(uidSet, &imap.FetchOptions{RFC822Size: true})

	var total uint64
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			if data, ok := item.(imapclient.FetchItemDataRFC822Size); ok {
				total += uint64(data.Size)
			}
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return 0, fmt.Errorf("fetch sizes failed: %w", err)
	}

	c.logger.Debug("message sizes fetched", slog.Uint64("total_bytes", total))
	return total, nil
}

// serverSetFlags are flags that only the server may set; clients must not
// include them in APPEND commands or the server will reject the request.
var serverSetFlags = map[string]bool{
	"\\Recent": true,
}

// clientSettableFlags returns only the flags a client is allowed to set.
func clientSettableFlags(flags []string) []string {
	result := flags[:0:0]
	for _, f := range flags {
		if !serverSetFlags[f] {
			result = append(result, f)
		}
	}
	return result
}

// Append appends a message to a mailbox.
func (c *Client) Append(mailbox string, msg *Message) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("appending message",
		slog.String("mailbox", mailbox),
		slog.Int("size", len(msg.Body)))

	appendOpts := &imap.AppendOptions{
		Flags: convertToFlags(clientSettableFlags(msg.Flags)),
	}

	if !msg.InternalDate.IsZero() {
		appendOpts.Time = msg.InternalDate
	}

	size := int64(len(msg.Body))
	appendCmd := c.client.Append(mailbox, size, appendOpts)

	if _, err := io.Copy(appendCmd, bytes.NewReader(msg.Body)); err != nil {
		return 0, fmt.Errorf("failed to write message body: %w", err)
	}

	if err := appendCmd.Close(); err != nil {
		return 0, fmt.Errorf("failed to close append: %w", err)
	}

	data, err := appendCmd.Wait()
	if err != nil {
		return 0, fmt.Errorf("append failed: %w", err)
	}

	if data == nil || data.UID == 0 {
		return 0, fmt.Errorf("append succeeded but server returned no valid UID")
	}

	uid := uint32(data.UID)
	c.logger.Debug("message appended", slog.Uint64("uid", uint64(uid)))
	return uid, nil
}

// SetFlags sets the flags on messages by UID.
func (c *Client) SetFlags(uids []uint32, flags []string) error {
	return c.storeFlags(uids, flags, imap.StoreFlagsSet)
}

// AddFlags adds flags to messages by UID.
func (c *Client) AddFlags(uids []uint32, flags []string) error {
	return c.storeFlags(uids, flags, imap.StoreFlagsAdd)
}

// RemoveFlags removes flags from messages by UID.
func (c *Client) RemoveFlags(uids []uint32, flags []string) error {
	return c.storeFlags(uids, flags, imap.StoreFlagsDel)
}

// storeFlags performs a STORE operation on message flags.
func (c *Client) storeFlags(uids []uint32, flags []string, op imap.StoreFlagsOp) error {
	if len(uids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("storing flags",
		slog.Int("uids", len(uids)),
		slog.Any("flags", flags),
		slog.Int("op", int(op)))

	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	storeFlags := &imap.StoreFlags{
		Op:     op,
		Flags:  convertToFlags(flags),
		Silent: true,
	}

	storeCmd := c.client.Store(uidSet, storeFlags, nil)
	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("store flags failed: %w", err)
	}

	return nil
}

// MarkDeleted marks messages as deleted by UID.
func (c *Client) MarkDeleted(uids []uint32) error {
	return c.AddFlags(uids, []string{FlagDeleted})
}

// Expunge permanently removes messages marked as deleted.
func (c *Client) Expunge() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("expunging deleted messages")

	expungeCmd := c.client.Expunge()
	if err := expungeCmd.Close(); err != nil {
		return fmt.Errorf("expunge failed: %w", err)
	}

	c.logger.Debug("expunge completed")
	return nil
}

// Copy copies messages by UID to another mailbox.
func (c *Client) Copy(uids []uint32, destMailbox string) error {
	if len(uids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("copying messages",
		slog.Int("count", len(uids)),
		slog.String("dest", destMailbox))

	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	if _, err := c.client.Copy(uidSet, destMailbox).Wait(); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	c.logger.Debug("messages copied")
	return nil
}

// Move moves messages by UID to another mailbox.
func (c *Client) Move(uids []uint32, destMailbox string) error {
	if len(uids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("moving messages",
		slog.Int("count", len(uids)),
		slog.String("dest", destMailbox))

	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	if _, err := c.client.Move(uidSet, destMailbox).Wait(); err != nil {
		return fmt.Errorf("move failed: %w", err)
	}

	c.logger.Debug("messages moved")
	return nil
}

// SearchSince searches for messages since a given date.
func (c *Client) SearchSince(since time.Time) ([]uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("searching messages since", slog.Time("since", since))

	criteria := &imap.SearchCriteria{
		Since: since,
	}

	data, err := c.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	uids := make([]uint32, 0, len(data.AllUIDs()))
	for _, uid := range data.AllUIDs() {
		if uid == 0 {
			continue
		}
		uids = append(uids, uint32(uid))
	}

	c.logger.Debug("search completed", slog.Int("count", len(uids)))
	return uids, nil
}

// convertEnvelope converts an imap.Envelope to our Envelope type.
func convertEnvelope(env *imap.Envelope) *Envelope {
	if env == nil {
		return nil
	}

	return &Envelope{
		Date:      env.Date,
		Subject:   env.Subject,
		From:      convertAddresses(env.From),
		To:        convertAddresses(env.To),
		Cc:        convertAddresses(env.Cc),
		Bcc:       convertAddresses(env.Bcc),
		ReplyTo:   convertAddresses(env.ReplyTo),
		MessageID: env.MessageID,
		InReplyTo: firstString(env.InReplyTo),
	}
}

// convertAddresses converts imap.Address slice to our Address slice.
func convertAddresses(addrs []imap.Address) []Address {
	result := make([]Address, len(addrs))
	for i, a := range addrs {
		result[i] = Address{
			Name:    a.Name,
			Mailbox: a.Mailbox,
			Host:    a.Host,
		}
	}
	return result
}

// firstString returns the first element of a slice or empty string.
func firstString(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// HasFlag checks if a message has a specific flag.
func (m *Message) HasFlag(flag string) bool {
	for _, f := range m.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// IsSeen returns true if the message has been read.
func (m *Message) IsSeen() bool {
	return m.HasFlag(FlagSeen)
}

// IsDeleted returns true if the message is marked for deletion.
func (m *Message) IsDeleted() bool {
	return m.HasFlag(FlagDeleted)
}

// IsFlagged returns true if the message is flagged/starred.
func (m *Message) IsFlagged() bool {
	return m.HasFlag(FlagFlagged)
}
