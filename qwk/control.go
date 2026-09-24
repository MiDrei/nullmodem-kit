package qwk

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ConferenceInfo is one CONTROL.DAT conference entry -- conference 0
// is personal/netmail by QWK convention; every other number is a
// message area (see internal/bbs/qwk.go for how this codebase maps
// area IDs to conference numbers).
type ConferenceInfo struct {
	Number int
	// Name is truncated to 255 characters, CONTROL.DAT's own limit
	// under QWKE (extended from the classic format's 13). A reader
	// with no QWKE support simply displays/stores whatever fits its
	// own UI -- CONTROL.DAT is a plain line-based text file, so a
	// longer line here doesn't break its parsing, only (at worst)
	// its own display width.
	Name string
}

// ControlInfo is everything CONTROL.DAT needs. Conferences must
// include conference 0 itself (its own entry, typically named
// something like "Personal") -- WriteControlDAT's "number of
// conferences" line is exactly len(Conferences)-1, per the format's
// own convention of counting every conference beyond the mandatory 0.
type ControlInfo struct {
	BBSName   string
	City      string
	Phone     string
	SysopName string
	// BBSID identifies this system in the packet -- also the expected
	// base filename of an uploaded .REP's single message file (see
	// ParseReplyPacket). Written uppercased; keep it short (spec
	// convention is 8 characters or fewer, matching old DOS filename
	// limits) since it becomes part of a filename.
	BBSID      string
	PacketTime time.Time
	// CallerName is the display name written into CONTROL.DAT itself.
	CallerName string
	// PersonalNames are every other name a message could be addressed
	// to and still mean "this caller" for PERSONAL.NDX purposes (e.g.
	// their login username, when CallerName is instead their real
	// name) -- BuildQWKPacket matches a message's To field against
	// CallerName and every one of these, case-insensitively. CallerName
	// itself never needs repeating here.
	PersonalNames []string
	Conferences   []ConferenceInfo
	// WelcomeFile, NewsFile and GoodbyeFile name the display files a
	// packet may carry alongside its messages -- classically HELLO/NEWS/
	// GOODBYE, and in practice often .ANS art a reader is expected to
	// show on opening the packet. CONTROL.DAT's last three lines carry
	// them; an empty field falls back to the conventional name, so a
	// caller that doesn't care can leave all three alone.
	WelcomeFile string
	NewsFile    string
	GoodbyeFile string
	// Username, if set, is written into TOREADER.EXT's ALIAS line
	// (see WriteToReaderEXT) -- the caller's actual login handle,
	// kept distinct from CallerName since that may be their real name
	// instead.
	Username string
}

// WriteControlDAT writes CONTROL.DAT: a CRLF text file with lines in
// the QWK format's own fixed, exact order (see this package's doc
// comment for where that order was verified).
func WriteControlDAT(w io.Writer, c ControlInfo) error {
	lines := []string{
		c.BBSName,
		c.City,
		c.Phone,
		c.SysopName + ",Sysop",
		"00000," + strings.ToUpper(c.BBSID),
		c.PacketTime.Format("01-02-2006,15:04:05"),
		strings.ToUpper(c.CallerName),
		"",
		"0",
		"0",
		strconv.Itoa(len(c.Conferences) - 1),
	}
	for _, conf := range c.Conferences {
		lines = append(lines, strconv.Itoa(conf.Number), truncate(conf.Name, 255))
	}
	lines = append(lines,
		orDefault(c.WelcomeFile, "WELCOME"),
		orDefault(c.NewsFile, "NEWS"),
		orDefault(c.GoodbyeFile, "GOODBYE"))

	for _, line := range lines {
		if _, err := io.WriteString(w, line+"\r\n"); err != nil {
			return fmt.Errorf("qwk: writing CONTROL.DAT: %w", err)
		}
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// controlDATTimeLayouts are the packet-time spellings seen in the
// wild. The format documents a four-digit year, and that is what
// WriteControlDAT emits, but plenty of DOS-era doors wrote two
// digits -- and a reader that rejects those rejects most of the
// packets actually in circulation.
var controlDATTimeLayouts = []string{
	"01-02-2006,15:04:05",
	"01-02-06,15:04:05",
	"01-02-2006,15:04",
	"01-02-06,15:04",
}

// ReadControlDAT is WriteControlDAT's inverse. It is deliberately
// forgiving, because CONTROL.DAT is the one file in a packet that
// every door writes slightly differently: a short file simply yields
// zero values for the lines it lacks, an unparseable packet time
// leaves PacketTime zero rather than failing the whole read, and the
// sysop line's conventional ",Sysop" suffix is stripped when present.
//
// The conference block is read using the format's own "number of
// conferences minus one" convention, but bounded by how many lines
// are actually there: a truncated packet costs the caller the
// conferences past the truncation, not the entire CONTROL.DAT.
func ReadControlDAT(r io.Reader) (ControlInfo, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return ControlInfo{}, fmt.Errorf("qwk: reading CONTROL.DAT: %w", err)
	}
	lines := splitControlLines(string(data))

	at := func(i int) string {
		if i < 0 || i >= len(lines) {
			return ""
		}
		return lines[i]
	}

	c := ControlInfo{
		BBSName:    at(0),
		City:       at(1),
		Phone:      at(2),
		SysopName:  strings.TrimSuffix(at(3), ",Sysop"),
		CallerName: at(6),
	}
	if _, id, found := strings.Cut(at(4), ","); found {
		c.BBSID = strings.TrimSpace(id)
	} else {
		c.BBSID = strings.TrimSpace(at(4))
	}
	for _, layout := range controlDATTimeLayouts {
		if t, err := time.Parse(layout, at(5)); err == nil {
			c.PacketTime = t
			break
		}
	}

	// Line 11 holds the conference count minus one; the pairs follow.
	count, err := strconv.Atoi(strings.TrimSpace(at(10)))
	if err != nil {
		return c, nil
	}
	for i := 0; i <= count; i++ {
		numLine, nameLine := 11+i*2, 12+i*2
		if nameLine >= len(lines) {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(at(numLine)))
		if err != nil {
			continue
		}
		c.Conferences = append(c.Conferences, ConferenceInfo{Number: n, Name: at(nameLine)})
	}

	// The three display-file names trail the conference block.
	if tail := 11 + (count+1)*2; tail+2 < len(lines) {
		c.WelcomeFile, c.NewsFile, c.GoodbyeFile = at(tail), at(tail+1), at(tail+2)
	}
	return c, nil
}

// splitControlLines splits on CRLF or bare LF and drops a single
// trailing empty element, so a file ending in CRLF doesn't present as
// having one blank line more than it has.
func splitControlLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
