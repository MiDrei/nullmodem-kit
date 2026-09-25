package qwk

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteToReaderEXTWritesAliasLine(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteToReaderEXT(&buf, "alice"); err != nil {
		t.Fatalf("WriteToReaderEXT: %v", err)
	}
	if buf.String() != "ALIAS alice\r\n" {
		t.Fatalf("TOREADER.EXT contents = %q, want %q", buf.String(), "ALIAS alice\r\n")
	}
}

func TestWriteToReaderEXTWritesNothingForBlankUsername(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteToReaderEXT(&buf, ""); err != nil {
		t.Fatalf("WriteToReaderEXT: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("TOREADER.EXT contents = %q, want empty for a blank username", buf.String())
	}
}

func TestAddKludgesAddsOnlyFieldsOverTheClassicLimit(t *testing.T) {
	shortTo := "All"
	shortFrom := "alice"
	longSubject := "This subject is deliberately longer than twenty-five characters"

	got := AddKludges(shortTo, shortFrom, longSubject, "the body")

	if strings.Contains(got, kludgeTo) || strings.Contains(got, kludgeFrom) {
		t.Fatalf("kludge text = %q, should not carry To:/From: kludges for fields under the 25-char limit", got)
	}
	want := "Subject: " + longSubject + "\n\nthe body"
	if got != want {
		t.Fatalf("kludge text = %q, want %q", got, want)
	}
}

func TestAddKludgesAddsAllThreeWhenAllExceedTheLimit(t *testing.T) {
	longTo := "A Very Long Recipient Name Indeed"
	longFrom := "A Very Long Sender Name As Well"
	longSubject := "A Very Long Subject Line That Exceeds Limits"

	got := AddKludges(longTo, longFrom, longSubject, "body text")

	want := "To: " + longTo + "\nFrom: " + longFrom + "\nSubject: " + longSubject + "\n\nbody text"
	if got != want {
		t.Fatalf("kludge text = %q, want %q", got, want)
	}
}

func TestAddKludgesLeavesTextUnchangedWhenNothingExceedsTheLimit(t *testing.T) {
	got := AddKludges("All", "alice", "Hello", "the body")
	if got != "the body" {
		t.Fatalf("kludge text = %q, want the original body unchanged", got)
	}
}

// The prefixes are the spec's own ("To:"/"From:"/"Subject:"), not the
// "TO:"/"FROM:"/"SUBJ:" labels its worked example uses for the
// truncated header fields -- getting these two confused is what makes
// a packet's kludges invisible to every other QWKE reader.
func TestAddKludgesUsesSpecPrefixesNotHeaderLabels(t *testing.T) {
	long := strings.Repeat("x", qwkeKludgeLimit+1)
	got := AddKludges(long, long, long, "body")

	for _, wrong := range []string{"TO:", "FROM:", "SUBJ:"} {
		if strings.Contains(got, wrong) {
			t.Fatalf("kludge text = %q, must not use the header label %q as a kludge prefix", got, wrong)
		}
	}
}

func TestAddKludgesRoundTripsThroughParseQWKEKludges(t *testing.T) {
	longTo := "A Very Long Recipient Name Indeed"
	longFrom := "A Very Long Sender Name As Well"
	longSubject := "A Very Long Subject Line That Exceeds Limits"

	packed := AddKludges(longTo, longFrom, longSubject, "body text")
	k, body := ParseQWKEKludges(packed)

	if k.To != longTo || k.From != longFrom || k.Subject != longSubject {
		t.Fatalf("parsed kludges = %+v, want To/From/Subject %q/%q/%q", k, longTo, longFrom, longSubject)
	}
	if body != "body text" {
		t.Fatalf("parsed body = %q, want %q", body, "body text")
	}
}

func TestParseQWKEKludgesLeavesOrdinaryBodyUntouched(t *testing.T) {
	body := "Hi there\n\nSee you: tomorrow\n"
	k, got := ParseQWKEKludges(body)
	if k != (Kludges{}) {
		t.Fatalf("parsed kludges = %+v, want none for an ordinary body", k)
	}
	if got != body {
		t.Fatalf("parsed body = %q, want it unchanged", got)
	}
}

// Packets built by an older NullModem BBS carry the header-label
// spelling; a reader still has to understand them.
func TestParseQWKEKludgesAcceptsLegacyUppercasePrefixes(t *testing.T) {
	k, body := ParseQWKEKludges("TO:Peter Rocca\nSUBJ:A long subject line\n\nthe body")
	if k.To != "Peter Rocca" {
		t.Fatalf("To = %q, want %q", k.To, "Peter Rocca")
	}
	if k.Subject != "A long subject line" {
		t.Fatalf("Subject = %q, want %q", k.Subject, "A long subject line")
	}
	if body != "the body" {
		t.Fatalf("body = %q, want %q", body, "the body")
	}
}

// "however you should program to handle if one is not present" --
// QWKE 1.02 on the blank line after the last kludge.
func TestParseQWKEKludgesCopesWithMissingBlankSeparator(t *testing.T) {
	k, body := ParseQWKEKludges("To: Peter Rocca\nthe body starts immediately")
	if k.To != "Peter Rocca" {
		t.Fatalf("To = %q, want %q", k.To, "Peter Rocca")
	}
	if body != "the body starts immediately" {
		t.Fatalf("body = %q, want %q", body, "the body starts immediately")
	}
}

func TestParseQWKEKludgesStopsAtFirstNonKludgeLine(t *testing.T) {
	k, body := ParseQWKEKludges("To: Peter Rocca\nNotes: not a kludge\nSubject: too late to count")
	if k.To != "Peter Rocca" {
		t.Fatalf("To = %q, want %q", k.To, "Peter Rocca")
	}
	if k.Subject != "" {
		t.Fatalf("Subject = %q, want empty -- it sits below a non-kludge line", k.Subject)
	}
	if body != "Notes: not a kludge\nSubject: too late to count" {
		t.Fatalf("body = %q, want everything from the first non-kludge line on", body)
	}
}

func TestReadToReaderEXTParsesEveryIdentifier(t *testing.T) {
	const raw = "ALIAS alice\r\n" +
		"AREA 3 apN\r\n" +
		"AREA 7 ag\r\n" +
		"BULL BULLET1.TXT New user information\r\n" +
		"ATTACH PHOTO.JPG 3 1042\r\n" +
		"FILE README.TXT some description\r\n" +
		"KEYWORD golang\r\n" +
		"FILTER spam\r\n" +
		"TWIT loudmouth\r\n" +
		"WHATEVER something we do not know\r\n"

	got, err := ReadToReaderEXT(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadToReaderEXT: %v", err)
	}
	if got.Alias != "alice" {
		t.Fatalf("Alias = %q, want %q", got.Alias, "alice")
	}
	if len(got.Areas) != 2 {
		t.Fatalf("Areas = %+v, want 2 entries", got.Areas)
	}
	area, ok := got.Area(3)
	if !ok || area.Flags != "apN" {
		t.Fatalf("Area(3) = %+v (ok=%v), want flags %q", area, ok, "apN")
	}
	if !area.IsNetmail() {
		t.Fatal("Area(3) should be netmail: its flags carry 'N'")
	}
	if other, _ := got.Area(7); other.IsNetmail() {
		t.Fatal("Area(7) should not be netmail: its flags carry no 'N'")
	}
	if len(got.Bulletins) != 1 || got.Bulletins[0].Filename != "BULLET1.TXT" ||
		got.Bulletins[0].Description != "New user information" {
		t.Fatalf("Bulletins = %+v", got.Bulletins)
	}
	want := Attachment{Filename: "PHOTO.JPG", Conference: 3, Message: 1042}
	if len(got.Attachments) != 1 || got.Attachments[0] != want {
		t.Fatalf("Attachments = %+v, want %+v", got.Attachments, want)
	}
	if len(got.Files) != 1 || got.Files[0] != "README.TXT" {
		t.Fatalf("Files = %+v", got.Files)
	}
	if len(got.Keywords) != 1 || len(got.Filters) != 1 || len(got.Twits) != 1 {
		t.Fatalf("Keywords/Filters/Twits = %+v/%+v/%+v", got.Keywords, got.Filters, got.Twits)
	}
}

func TestReadToReaderEXTSkipsMalformedLinesInsteadOfFailing(t *testing.T) {
	got, err := ReadToReaderEXT(strings.NewReader("AREA notanumber ap\r\nATTACH F.ZIP 3\r\nALIAS bob\r\n"))
	if err != nil {
		t.Fatalf("ReadToReaderEXT: %v", err)
	}
	if len(got.Areas) != 0 || len(got.Attachments) != 0 {
		t.Fatalf("malformed lines should be skipped, got areas=%+v attachments=%+v", got.Areas, got.Attachments)
	}
	if got.Alias != "bob" {
		t.Fatalf("Alias = %q -- parsing must continue past a malformed line", got.Alias)
	}
}

func TestAreaLinesRoundTripThroughAPacket(t *testing.T) {
	path := t.TempDir() + "/T.QWK"
	control := ControlInfo{BBSName: "Test", BBSID: "TEST", Username: "alice",
		Conferences: []ConferenceInfo{{Number: 0, Name: "Personal"}, {Number: 3, Name: "General"}},
		Areas:       []AreaEntry{{Number: 0, Flags: "N"}}}
	msgs := []PackedMessage{{Header: MessageHeader{Number: 7, Conference: 3, To: "All", From: "Bob", Subject: "hi"}, Text: "x"}}
	if err := BuildQWKPacket(path, control, msgs); err != nil {
		t.Fatalf("BuildQWKPacket: %v", err)
	}
	p, err := OpenPacket(path)
	if err != nil {
		t.Fatalf("OpenPacket: %v", err)
	}
	defer p.Close()
	if a, ok := p.Ext.Area(0); !ok || !a.IsNetmail() {
		t.Fatalf("conference 0 = %+v, %v; want flagged netmail", a, ok)
	}
	if a, ok := p.Ext.Area(3); ok && a.IsNetmail() {
		t.Fatal("conference 3 must not be netmail")
	}
	if p.Ext.Alias != "alice" {
		t.Fatalf("alias = %q", p.Ext.Alias)
	}
}

func TestWriteToReaderEXTWritesAreasWithoutAUsername(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteToReaderEXT(&buf, "", AreaEntry{Number: 0, Flags: "N"}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "AREA 0 N\r\n" {
		t.Fatalf("contents = %q", buf.String())
	}
}
