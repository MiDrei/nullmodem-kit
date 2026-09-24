package qwk

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestBuildQWKPacketProducesExpectedFiles(t *testing.T) {
	control := ControlInfo{
		BBSName:    "Test BBS",
		BBSID:      "testbbs",
		PacketTime: time.Now(),
		CallerName: "Alice",
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "Personal"},
			{Number: 5, Name: "General"},
		},
	}
	messages := []PackedMessage{
		{Header: MessageHeader{To: "Alice", From: "SYSTEM", Subject: "hi", Conference: 0}, Text: "a netmail"},
		{Header: MessageHeader{To: "All", From: "Bob", Subject: "post", Conference: 5}, Text: "an echomail post"},
	}

	path := filepath.Join(t.TempDir(), "TESTBBS.QWK")
	if err := BuildQWKPacket(path, control, messages); err != nil {
		t.Fatalf("BuildQWKPacket: %v", err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening built packet as zip: %v", err)
	}
	defer zr.Close()

	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"000.NDX", "005.NDX", "CONTROL.DAT", "MESSAGES.DAT", "PERSONAL.NDX"}
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("packet contains %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("packet contains %v, want %v", names, want)
		}
	}
}

func TestBuildQWKPacketPersonalNDXMatchesCallerNameOrAliases(t *testing.T) {
	control := ControlInfo{
		BBSID:         "testbbs",
		PacketTime:    time.Now(),
		CallerName:    "Alice Example",
		PersonalNames: []string{"alice"},
		Conferences:   []ConferenceInfo{{Number: 0, Name: "Personal"}},
	}
	messages := []PackedMessage{
		{Header: MessageHeader{To: "Alice Example", Conference: 0}, Text: "matches CallerName"},
		{Header: MessageHeader{To: "alice", Conference: 0}, Text: "matches a PersonalNames alias"},
		{Header: MessageHeader{To: "Someone Else", Conference: 0}, Text: "matches nothing"},
	}
	path := filepath.Join(t.TempDir(), "x.qwk")
	if err := BuildQWKPacket(path, control, messages); err != nil {
		t.Fatalf("BuildQWKPacket: %v", err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening built packet: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "PERSONAL.NDX" {
			continue
		}
		rc, _ := f.Open()
		defer rc.Close()
		var buf [10]byte
		n, _ := rc.Read(buf[:])
		if n != 10 { // two 5-byte records
			t.Fatalf("PERSONAL.NDX contains %d bytes, want 10 (2 records)", n)
		}
		return
	}
	t.Fatal("PERSONAL.NDX entry not found")
}

func TestIndexMessagesGroupsByConferenceAndFindsPersonal(t *testing.T) {
	messages := []PackedMessage{
		{Header: MessageHeader{To: "All", Conference: 5}, Text: "short"},
		{Header: MessageHeader{To: "Alice", Conference: 0}, Text: "hi alice"},
		{Header: MessageHeader{To: "  alice  ", Conference: 5}, Text: "another for alice, different conference"},
	}
	perConf, personal := indexMessages([]string{"Alice"}, messages)

	if len(perConf[5]) != 2 {
		t.Fatalf("conference 5 = %+v, want 2 entries", perConf[5])
	}
	if len(perConf[0]) != 1 {
		t.Fatalf("conference 0 = %+v, want 1 entry", perConf[0])
	}
	if len(personal) != 2 {
		t.Fatalf("personal = %+v, want 2 entries (case/whitespace-insensitive match on To)", personal)
	}

	// Record numbers must be strictly increasing and start at 2 (record
	// 1 is the copyright notice).
	if perConf[5][0].MessageRecordNumber != 2 {
		t.Fatalf("first message record number = %d, want 2", perConf[5][0].MessageRecordNumber)
	}
}

// TestBuildQWKPacketIncludesToReaderEXTWhenUsernameIsSet locks in that
// TOREADER.EXT (QWKE) is only added when ControlInfo.Username is set
// -- its presence is what a QWKE-aware reader uses to recognize the
// packet's extended CONTROL.DAT conference names/kludge lines, so a
// classic-only packet (no Username) should stay exactly as before.
func TestBuildQWKPacketIncludesToReaderEXTWhenUsernameIsSet(t *testing.T) {
	control := ControlInfo{
		BBSName:    "Test BBS",
		BBSID:      "testbbs",
		PacketTime: time.Now(),
		CallerName: "Alice",
		Username:   "alice",
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "Personal"},
		},
	}
	path := filepath.Join(t.TempDir(), "TESTBBS.QWK")
	if err := BuildQWKPacket(path, control, nil); err != nil {
		t.Fatalf("BuildQWKPacket: %v", err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening built packet as zip: %v", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != "TOREADER.EXT" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening TOREADER.EXT: %v", err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("reading TOREADER.EXT: %v", err)
		}
		if buf.String() != "ALIAS alice\r\n" {
			t.Fatalf("TOREADER.EXT contents = %q, want %q", buf.String(), "ALIAS alice\r\n")
		}
		return
	}
	t.Fatal("packet has no TOREADER.EXT entry, want one since Username was set")
}

// buildTestPacket writes a small but complete .QWK to a temp dir and
// returns its path, so the open-side tests exercise the real writer
// rather than a hand-rolled fixture that could drift from it.
func buildTestPacket(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "TEST.QWK")

	control := ControlInfo{
		BBSName:     "NullModem BBS",
		City:        "Zurich, CH",
		SysopName:   "Maik",
		BBSID:       "NULLMDM",
		PacketTime:  time.Date(2026, 9, 24, 17, 5, 30, 0, time.UTC),
		CallerName:  "ALICE",
		Username:    "alice",
		WelcomeFile: "WELCOME.ANS",
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "Personal"},
			{Number: 3, Name: "Go Programming"},
		},
	}
	messages := []PackedMessage{
		{Header: MessageHeader{Number: 1, Conference: 3, To: "ALICE", From: "BOB",
			Subject: "Hello", Written: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)},
			Text: "first line\nsecond line"},
		{Header: MessageHeader{Number: 2, Conference: 0, To: "ALICE", From: "SYSOP",
			Subject: "Welcome", Written: time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC)},
			Text: "personal note"},
	}
	if err := BuildQWKPacket(path, control, messages); err != nil {
		t.Fatalf("BuildQWKPacket: %v", err)
	}
	return path
}

func TestOpenPacketReadsControlMessagesAndQWKEFlag(t *testing.T) {
	p, err := OpenPacket(buildTestPacket(t))
	if err != nil {
		t.Fatalf("OpenPacket: %v", err)
	}
	defer p.Close()

	if p.Control.BBSName != "NullModem BBS" || p.Control.BBSID != "NULLMDM" {
		t.Fatalf("Control = %+v", p.Control)
	}
	if len(p.Messages) != 2 {
		t.Fatalf("Messages = %d, want 2", len(p.Messages))
	}
	if p.Messages[0].Text != "first line\nsecond line" {
		t.Fatalf("Messages[0].Text = %q", p.Messages[0].Text)
	}
	if !p.QWKE {
		t.Fatal("QWKE = false, want true -- the packet carries a TOREADER.EXT")
	}
	if p.Ext.Alias != "alice" {
		t.Fatalf("Ext.Alias = %q, want %q", p.Ext.Alias, "alice")
	}
	conf, ok := p.Conference(3)
	if !ok || conf.Name != "Go Programming" {
		t.Fatalf("Conference(3) = %+v (ok=%v)", conf, ok)
	}
}

func TestOpenPacketListsOtherFilesAndReadsThemRaw(t *testing.T) {
	p, err := OpenPacket(buildTestPacket(t))
	if err != nil {
		t.Fatalf("OpenPacket: %v", err)
	}
	defer p.Close()

	// The .NDX files the builder emits are what's left over once
	// CONTROL/MESSAGES/TOREADER are accounted for.
	if len(p.Names) == 0 {
		t.Fatal("Names is empty, want the packet's .NDX entries")
	}
	var found bool
	for _, n := range p.Names {
		if strings.EqualFold(n, "PERSONAL.NDX") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Names = %v, want PERSONAL.NDX among them", p.Names)
	}
	if _, err := p.ReadFile("personal.ndx"); err != nil {
		t.Fatalf("ReadFile should match case-insensitively: %v", err)
	}
	if _, err := p.ReadFile("NOSUCH.TXT"); err == nil {
		t.Fatal("ReadFile on a missing entry should fail")
	}
}

func TestOpenPacketRejectsArchiveWithoutMessagesDAT(t *testing.T) {
	path := filepath.Join(t.TempDir(), "EMPTY.QWK")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	zw := zip.NewWriter(f)
	if _, err := zw.Create("README.TXT"); err != nil {
		t.Fatalf("creating entry: %v", err)
	}
	zw.Close()
	f.Close()

	if _, err := OpenPacket(path); err == nil {
		t.Fatal("OpenPacket should reject an archive with no MESSAGES.DAT")
	}
}
