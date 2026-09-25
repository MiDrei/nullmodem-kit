package qwk

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// WriteToReaderEXT writes TOREADER.EXT, the file whose mere presence
// in a .QWK packet is what a reader uses to recognize QWKE support
// (the QWKE 1.02 spec, wmcbrine.com/mmail/specs/qwke.html, defines no
// separate marker line for this in CONTROL.DAT). This package writes
// the ALIAS line -- the caller's real login handle -- and one AREA line
// per entry in areas, e.g. the netmail conference flagged 'N' so a
// reader asks for a recipient there instead of addressing "All"; the
// format also defines BULL/ATTACH/FILE/KEYWORD/FILTER/TWIT lines for
// other purposes this codebase has no equivalent state for yet.
// ReadToReaderEXT understands all of them, since a reader has to cope
// with whatever a foreign door sends. Nothing is written when there is
// neither a username nor an area.
func WriteToReaderEXT(w io.Writer, username string, areas ...AreaEntry) error {
	var b strings.Builder
	if username != "" {
		b.WriteString("ALIAS " + username + "\r\n")
	}
	for _, a := range areas {
		fmt.Fprintf(&b, "AREA %d %s\r\n", a.Number, a.Flags)
	}
	if b.Len() == 0 {
		return nil
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("qwk: writing TOREADER.EXT: %w", err)
	}
	return nil
}

// qwkeKludgeLimit is the classic MESSAGES.DAT header's fixed field
// width for To/From/Subject (25 bytes) -- unlike a conference name,
// this can never simply be made longer, since it's a real fixed-width
// binary field every reader (QWKE-aware or not) decodes at a fixed
// offset. QWKE's kludge lines are the documented workaround: a reader
// that understands them shows/imports the untruncated value instead
// of what's in the header.
const qwkeKludgeLimit = 25

// The kludge line prefixes QWKE 1.02 defines, quoted from the spec's
// own list ("These kludges are... To: / From: / Subject:").
//
// Note these are NOT the "TO:"/"FROM:"/"SUBJ:" strings that appear in
// the spec's worked example: those label the truncated *header*
// fields in that illustration ("<--- HEADER"), while the lines
// actually written into the message text are the ones below
// ("<--- MESSAGE TEXT"). An earlier version of this function emitted
// the header labels instead, which no spec-conforming reader
// (MultiMail, NoCarrierMail, ...) recognizes -- such a reader would
// silently fall back to the 25-byte truncated header fields and
// additionally show the stray label lines as body text.
const (
	kludgeTo      = "To: "
	kludgeFrom    = "From: "
	kludgeSubject = "Subject: "
)

// AddKludges prepends QWKE kludge lines ahead of text for any of
// to/from/subject that exceeds the classic 25-byte header field
// width, followed by a blank line -- the spec states kludges are only
// added when a field actually exceeds the limit. Fields at or under
// the limit need no kludge: the header alone already carries them in
// full.
//
// It is exported because a caller that assembles message text itself,
// rather than handing a Reply to BuildReplyPacket, still has to get
// the kludges right -- and getting them wrong is invisible until
// someone's name is long.
func AddKludges(to, from, subject, text string) string {
	var kludges []string
	if len(to) > qwkeKludgeLimit {
		kludges = append(kludges, kludgeTo+to)
	}
	if len(from) > qwkeKludgeLimit {
		kludges = append(kludges, kludgeFrom+from)
	}
	if len(subject) > qwkeKludgeLimit {
		kludges = append(kludges, kludgeSubject+subject)
	}
	if len(kludges) == 0 {
		return text
	}
	return strings.Join(kludges, "\n") + "\n\n" + text
}

// Kludges are the QWKE values lifted out of a message body by
// ParseQWKEKludges. An empty field means that kludge wasn't present,
// in which case the header's own (possibly truncated) field stands.
type Kludges struct {
	To, From, Subject string
}

// ParseQWKEKludges is AddKludges' inverse: it strips any leading
// QWKE kludge lines off text and returns them alongside the remaining
// body. It is deliberately more liberal than the writer, because a
// reader has to cope with whatever the sending door emitted:
// recognition is case-insensitive, the space after the colon is
// optional, and "Subj:" is accepted next to the spec's "Subject:"
// (both are in circulation, and this package itself wrote the former
// until the prefixes were corrected -- packets built by an older
// NullModem BBS are still out there and must keep working).
//
// Per the spec the kludge block ends at a blank line, "however you
// should program to handle if one is not present", so scanning simply
// stops at the first line that isn't a known kludge, and a single
// blank separator line is consumed when it is there.
//
// The To: kludge is stripped like the others. That is right for
// display, but a caller relaying a message onward to UUCP/internet
// netmail should be aware the spec wants a "To: <address>" line left
// standing as the body's first line for that transport -- it has the
// parsed value and the body and can reinstate it.
func ParseQWKEKludges(text string) (Kludges, string) {
	var k Kludges
	rest := text
	for {
		line, tail, found := strings.Cut(rest, "\n")
		if !found && line == "" {
			break
		}
		name, value, ok := splitKludge(line)
		if !ok {
			break
		}
		switch name {
		case "to":
			k.To = value
		case "from":
			k.From = value
		case "subject", "subj":
			k.Subject = value
		}
		rest = tail
	}
	if k == (Kludges{}) {
		return k, text
	}
	// Consume the blank separator line the spec asks for, if present.
	if line, tail, found := strings.Cut(rest, "\n"); found && strings.TrimSpace(line) == "" {
		rest = tail
	}
	return k, rest
}

// splitKludge recognizes one "Name: value" kludge line, returning the
// lowercased name and the trimmed value. It reports false for
// anything that isn't one of the kludges this package knows, which is
// what stops ParseQWKEKludges from eating ordinary body text that
// merely happens to contain a colon.
func splitKludge(line string) (name, value string, ok bool) {
	head, tail, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	switch strings.ToLower(strings.TrimSpace(head)) {
	case "to", "from", "subject", "subj":
		return strings.ToLower(strings.TrimSpace(head)), strings.TrimSpace(tail), true
	}
	return "", "", false
}

// AreaEntry is one TOREADER.EXT "AREA <conference#> <flags>" line.
// Flags carry three separate vocabularies sharing one field -- reader
// state (a/p/g), door capabilities (w/k/f/F/B) and BBS attributes
// (P/O/X/R/Z/L/N/E/I/U/H/A/&) -- so they are kept verbatim rather
// than decoded into booleans this package would have to guess at.
type AreaEntry struct {
	Number int
	Flags  string
}

// IsNetmail reports whether this conference is flagged 'N' (netmail).
// A reader needs this to know a conference takes an explicit
// recipient address rather than being a broadcast echo -- without it
// netmail conferences are indistinguishable from ordinary echoes and
// replies go to the wrong place.
func (a AreaEntry) IsNetmail() bool { return strings.ContainsRune(a.Flags, 'N') }

// Attachment is one "ATTACH <filename> <conference#> <message#>" line:
// a file the door shipped alongside the packet, belonging to a
// specific message.
type Attachment struct {
	Filename   string
	Conference int
	Message    int
}

// Bulletin is one "BULL <filename> <description>" line.
type Bulletin struct {
	Filename    string
	Description string
}

// ToReaderEXT is a parsed TOREADER.EXT. Every field is optional: the
// file's presence alone already signals QWKE support, and doors emit
// wildly different subsets of it.
type ToReaderEXT struct {
	Alias       string
	Areas       []AreaEntry
	Bulletins   []Bulletin
	Attachments []Attachment
	Files       []string
	Keywords    []string
	Filters     []string
	Twits       []string
}

// Area returns the entry for conference number n.
func (t ToReaderEXT) Area(n int) (AreaEntry, bool) {
	for _, a := range t.Areas {
		if a.Number == n {
			return a, true
		}
	}
	return AreaEntry{}, false
}

// ReadToReaderEXT parses TOREADER.EXT. Unknown identifiers and
// malformed lines are skipped rather than failing the read: this file
// is an open-ended, extensible list ("Additional kludge names can be
// created"), and one unparseable line from a foreign door must not
// cost the user the entire packet.
func ReadToReaderEXT(r io.Reader) (ToReaderEXT, error) {
	var t ToReaderEXT
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		ident, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch strings.ToUpper(ident) {
		case "ALIAS":
			t.Alias = rest
		case "AREA":
			num, flags, _ := strings.Cut(rest, " ")
			if n, err := strconv.Atoi(strings.TrimSpace(num)); err == nil {
				t.Areas = append(t.Areas, AreaEntry{Number: n, Flags: strings.TrimSpace(flags)})
			}
		case "BULL":
			name, desc, _ := strings.Cut(rest, " ")
			if name != "" {
				t.Bulletins = append(t.Bulletins, Bulletin{Filename: name, Description: strings.TrimSpace(desc)})
			}
		case "ATTACH":
			fields := strings.Fields(rest)
			if len(fields) >= 3 {
				conf, errC := strconv.Atoi(fields[1])
				msg, errM := strconv.Atoi(fields[2])
				if errC == nil && errM == nil {
					t.Attachments = append(t.Attachments, Attachment{Filename: fields[0], Conference: conf, Message: msg})
				}
			}
		case "FILE":
			if name, _, _ := strings.Cut(rest, " "); name != "" {
				t.Files = append(t.Files, name)
			}
		case "KEYWORD":
			t.Keywords = append(t.Keywords, rest)
		case "FILTER":
			t.Filters = append(t.Filters, rest)
		case "TWIT":
			t.Twits = append(t.Twits, rest)
		}
	}
	if err := sc.Err(); err != nil {
		return ToReaderEXT{}, fmt.Errorf("qwk: reading TOREADER.EXT: %w", err)
	}
	return t, nil
}
