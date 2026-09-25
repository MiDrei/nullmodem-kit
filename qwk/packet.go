package qwk

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"strings"
)

// BuildQWKPacket writes a complete .QWK packet to disk at path: a zip
// containing CONTROL.DAT, MESSAGES.DAT, one <conf>.NDX per conference
// listed in control.Conferences (zero-padded to 3 digits), PERSONAL.NDX
// (pointers to every message whose To field matches control.CallerName
// or any of control.PersonalNames, case-insensitively and trimmed),
// and TOREADER.EXT (QWKE, see WriteToReaderEXT) when control.Username
// is set.
//
// messages must already be in the order they should appear in
// MESSAGES.DAT -- conference-grouped is conventional but not
// required; each message's own Header.Conference is what actually
// determines which .NDX file it's indexed into, independent of
// ordering.
func BuildQWKPacket(path string, control ControlInfo, messages []PackedMessage) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("qwk: creating %s: %w", path, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	cw, err := zw.Create("CONTROL.DAT")
	if err != nil {
		return fmt.Errorf("qwk: creating CONTROL.DAT entry: %w", err)
	}
	if err := WriteControlDAT(cw, control); err != nil {
		return err
	}

	mw, err := zw.Create("MESSAGES.DAT")
	if err != nil {
		return fmt.Errorf("qwk: creating MESSAGES.DAT entry: %w", err)
	}
	if err := WriteMessagesDAT(mw, messages); err != nil {
		return err
	}

	if control.Username != "" || len(control.Areas) > 0 {
		tw, err := zw.Create("TOREADER.EXT")
		if err != nil {
			return fmt.Errorf("qwk: creating TOREADER.EXT entry: %w", err)
		}
		if err := WriteToReaderEXT(tw, control.Username, control.Areas...); err != nil {
			return err
		}
	}

	personalNames := append([]string{control.CallerName}, control.PersonalNames...)
	perConference, personal := indexMessages(personalNames, messages)

	for _, conf := range control.Conferences {
		nw, err := zw.Create(fmt.Sprintf("%03d.NDX", conf.Number))
		if err != nil {
			return fmt.Errorf("qwk: creating %03d.NDX entry: %w", conf.Number, err)
		}
		if err := WriteNDX(nw, perConference[conf.Number]); err != nil {
			return err
		}
	}

	pw, err := zw.Create("PERSONAL.NDX")
	if err != nil {
		return fmt.Errorf("qwk: creating PERSONAL.NDX entry: %w", err)
	}
	if err := WriteNDX(pw, personal); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("qwk: finishing %s: %w", path, err)
	}
	return nil
}

// indexMessages computes each message's own MESSAGES.DAT record
// number (1-indexed; record 1 is always the copyright notice, so the
// first message's header starts at record 2) and groups the
// resulting .NDX entries by conference, separately collecting the
// subset addressed to any of names for PERSONAL.NDX.
func indexMessages(names []string, messages []PackedMessage) (perConference map[int][]NDXRecord, personal []NDXRecord) {
	perConference = map[int][]NDXRecord{}
	var trimmed []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			trimmed = append(trimmed, n)
		}
	}

	record := 2
	for _, m := range messages {
		rec := NDXRecord{MessageRecordNumber: record, Conference: m.Header.Conference}
		perConference[m.Header.Conference] = append(perConference[m.Header.Conference], rec)
		to := strings.TrimSpace(m.Header.To)
		for _, n := range trimmed {
			if strings.EqualFold(to, n) {
				personal = append(personal, rec)
				break
			}
		}
		record += 1 + len(encodeText(m.Text))
	}
	return perConference, personal
}

// Packet is an opened .QWK packet: its CONTROL.DAT metadata, every
// message in MESSAGES.DAT, and, when the packet carries one, its
// parsed TOREADER.EXT. The underlying zip stays open so ReadFile can
// still fetch the packet's other contents -- welcome/news/goodbye
// screens (often .ANS art), bulletins, attachments -- which a reader
// shows but which have no place in a fixed struct. Close it when done.
type Packet struct {
	Path     string
	Control  ControlInfo
	Messages []PackedMessage
	// Ext is the parsed TOREADER.EXT; QWKE reports whether the packet
	// actually contained one. Its mere presence is QWKE's only support
	// marker, so that flag is what a caller should branch on rather
	// than inspecting Ext's fields for emptiness.
	Ext  ToReaderEXT
	QWKE bool
	// Names lists every other file in the packet, in archive order and
	// with its original spelling, for ReadFile.
	Names []string

	zr *zip.ReadCloser
}

// OpenPacket opens the .QWK packet at path and reads its CONTROL.DAT,
// MESSAGES.DAT and (if present) TOREADER.EXT. Entry names are matched
// case-insensitively: DOS-era doors wrote them uppercase, newer ones
// don't always, and the format never promised either.
//
// A packet with no MESSAGES.DAT is an error -- that file is the one
// part the format requires -- but a missing or unreadable CONTROL.DAT
// only costs the caller the metadata, since the messages themselves
// carry their own conference numbers and are still worth reading.
func OpenPacket(path string) (*Packet, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("qwk: opening %s: %w", path, err)
	}

	p := &Packet{Path: path, zr: zr}
	var messagesFound bool
	for _, zf := range zr.File {
		switch {
		case strings.EqualFold(zf.Name, "MESSAGES.DAT"):
			rc, err := zf.Open()
			if err != nil {
				zr.Close()
				return nil, fmt.Errorf("qwk: opening MESSAGES.DAT in %s: %w", path, err)
			}
			p.Messages, err = ReadMessagesDAT(rc)
			rc.Close()
			if err != nil {
				zr.Close()
				return nil, err
			}
			messagesFound = true
		case strings.EqualFold(zf.Name, "CONTROL.DAT"):
			if rc, err := zf.Open(); err == nil {
				p.Control, _ = ReadControlDAT(rc)
				rc.Close()
			}
		case strings.EqualFold(zf.Name, "TOREADER.EXT"):
			p.QWKE = true
			if rc, err := zf.Open(); err == nil {
				p.Ext, _ = ReadToReaderEXT(rc)
				rc.Close()
			}
		default:
			p.Names = append(p.Names, zf.Name)
		}
	}
	if !messagesFound {
		zr.Close()
		return nil, fmt.Errorf("qwk: %s contains no MESSAGES.DAT", path)
	}
	return p, nil
}

// ReadFile returns the contents of one file inside the packet, found
// case-insensitively. Screens and bulletins are CP437 art, so the
// bytes come back raw and unconverted -- see ansi.LoadScreen's doc
// comment for why decoding them here would corrupt the escape
// sequences they rely on.
func (p *Packet) ReadFile(name string) ([]byte, error) {
	for _, zf := range p.zr.File {
		if !strings.EqualFold(zf.Name, name) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("qwk: opening %s in %s: %w", name, p.Path, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("qwk: reading %s in %s: %w", name, p.Path, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("qwk: %s not found in %s", name, p.Path)
}

// Conference returns the CONTROL.DAT name for conference number n.
func (p *Packet) Conference(n int) (ConferenceInfo, bool) {
	for _, c := range p.Control.Conferences {
		if c.Number == n {
			return c, true
		}
	}
	return ConferenceInfo{}, false
}

// Close releases the packet's underlying archive.
func (p *Packet) Close() error {
	if p.zr == nil {
		return nil
	}
	return p.zr.Close()
}
