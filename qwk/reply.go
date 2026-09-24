package qwk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ParseReplyPacket opens an uploaded .REP file at path, confirms it's
// actually a zip by its magic bytes before trusting the extension
// (mirrors internal/tosser's own extractPacketBundle, which does the
// same check for FTS-5005 packet bundles), and reads its single
// <bbsID>.MSG entry (matched case-insensitively) via ReadMessagesDAT.
//
// Per the REP format's own convention, each returned message's
// Header.Number holds the destination conference number rather than
// a real message number -- see MessageHeader's own doc comment; the
// caller (internal/bbs's uploadQWKReply) is the one that knows to
// read it that way.
func ParseReplyPacket(path, bbsID string) ([]PackedMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("qwk: opening %s: %w", path, err)
	}
	magic := make([]byte, 4)
	_, readErr := io.ReadFull(f, magic)
	f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("qwk: reading %s: %w", path, readErr)
	}
	if !bytes.HasPrefix(magic, []byte("PK\x03\x04")) && !bytes.HasPrefix(magic, []byte("PK\x05\x06")) {
		return nil, fmt.Errorf("qwk: %s is not a zip file", path)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("qwk: opening %s as zip: %w", path, err)
	}
	defer zr.Close()

	wantName := strings.ToUpper(bbsID) + ".MSG"
	for _, zf := range zr.File {
		if !strings.EqualFold(zf.Name, wantName) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("qwk: opening %s in %s: %w", zf.Name, path, err)
		}
		defer rc.Close()
		return ReadMessagesDAT(rc)
	}
	return nil, fmt.Errorf("qwk: %s not found in %s", wantName, path)
}

// Reply is one outgoing message bound for a .REP packet. It is a
// deliberately narrower type than PackedMessage: a reader composes
// replies, it does not fabricate the bookkeeping fields (block
// counts, logical numbers, kill flags) that only make sense coming
// back off a disk. BuildReplyPacket fills those in.
type Reply struct {
	// Conference is the destination conference number. The .REP
	// format carries it in the header's message-number field rather
	// than its conference field -- see MessageHeader.Number -- which
	// BuildReplyPacket takes care of, so callers set this one field
	// and needn't know the quirk.
	Conference        int
	To, From, Subject string
	Written           time.Time
	Text              string
	// RefNumber is the message being replied to, 0 for a new thread.
	RefNumber int
	// Private asks the door to post this as private mail where the
	// conference allows it.
	Private bool
}

// BuildReplyPacket writes a .REP packet to path: a zip whose single
// <BBSID>.MSG entry holds the replies in MESSAGES.DAT's layout, with
// the BBS ID in record 0 so the receiving door can confirm the packet
// is meant for it.
//
// Long To/From/Subject values get QWKE kludge lines prepended to their
// body automatically (see AddKludges), which is the only way past
// the header's fixed 25-byte fields. That is safe to do
// unconditionally: a door without QWKE support ignores the lines and
// still has the truncated header fields to work from, exactly as it
// would have had otherwise.
func BuildReplyPacket(path, bbsID string, replies []Reply) error {
	if bbsID == "" {
		return fmt.Errorf("qwk: building %s: no BBS ID", path)
	}

	messages := make([]PackedMessage, len(replies))
	for i, r := range replies {
		status := byte(' ')
		if r.Private {
			status = '*'
		}
		written := r.Written
		if written.IsZero() {
			written = time.Now()
		}
		messages[i] = PackedMessage{
			Header: MessageHeader{
				Status:    status,
				Number:    r.Conference,
				Written:   written,
				To:        r.To,
				From:      r.From,
				Subject:   r.Subject,
				RefNumber: r.RefNumber,
				// Conference stays 0: in a .REP the destination lives
				// in Number, and writing it in both places makes a
				// door that reads the other field post duplicates.
			},
			Text: AddKludges(r.To, r.From, r.Subject, r.Text),
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("qwk: creating %s: %w", path, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	id := strings.ToUpper(bbsID)
	mw, err := zw.Create(id + ".MSG")
	if err != nil {
		return fmt.Errorf("qwk: creating %s.MSG entry: %w", id, err)
	}
	if err := writeMessagesDAT(mw, id, messages); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("qwk: finishing %s: %w", path, err)
	}
	return nil
}
