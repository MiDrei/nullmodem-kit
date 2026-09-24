package qwk

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestRepZip builds a minimal .REP file at path: a zip containing
// one BBSID.MSG entry with the given messages (each Header.Number
// already set to whatever conference number the test wants, per the
// REP format's own repurposing of that field).
func buildTestRepZip(t *testing.T, path, bbsID string, messages []PackedMessage) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(bbsID + ".MSG")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if err := WriteMessagesDAT(w, messages); err != nil {
		t.Fatalf("WriteMessagesDAT: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
}

func TestParseReplyPacketReadsBBSIDMsg(t *testing.T) {
	messages := []PackedMessage{
		{Header: MessageHeader{Number: 5, To: "All", From: "Alice", Subject: "reply"}, Text: "here's my reply"},
	}
	path := filepath.Join(t.TempDir(), "ALICE.REP")
	buildTestRepZip(t, path, "TESTBBS", messages)

	got, err := ParseReplyPacket(path, "testbbs")
	if err != nil {
		t.Fatalf("ParseReplyPacket: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	// Number field is repurposed to hold the conference number in a
	// REP -- ParseReplyPacket must hand it back unchanged, not
	// reinterpret it.
	if got[0].Header.Number != 5 {
		t.Fatalf("Header.Number = %d, want 5 (the conference number)", got[0].Header.Number)
	}
	if got[0].Text != "here's my reply" {
		t.Fatalf("Text = %q", got[0].Text)
	}
}

func TestParseReplyPacketRejectsNonZipFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.rep")
	if err := os.WriteFile(path, []byte("not a zip file at all"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := ParseReplyPacket(path, "testbbs"); err == nil {
		t.Fatal("ParseReplyPacket succeeded on a non-zip file, want an error")
	}
}

func TestParseReplyPacketErrorsWhenBBSIDFileMissing(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("WRONGID.MSG")
	_ = WriteMessagesDAT(w, nil)
	zw.Close()

	path := filepath.Join(t.TempDir(), "x.rep")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := ParseReplyPacket(path, "testbbs"); err == nil {
		t.Fatal("ParseReplyPacket succeeded despite no matching BBSID.MSG entry, want an error")
	}
}

func TestBuildReplyPacketRoundTripsThroughParseReplyPacket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NULLMDM.REP")
	written := time.Date(2026, 9, 24, 18, 30, 0, 0, time.UTC)

	replies := []Reply{
		{Conference: 3, To: "BOB", From: "ALICE", Subject: "Re: Hello",
			Written: written, Text: "a public reply", RefNumber: 42},
		{Conference: 0, To: "SYSOP", From: "ALICE", Subject: "Thanks",
			Written: written, Text: "a private note", Private: true},
	}
	if err := BuildReplyPacket(path, "nullmdm", replies); err != nil {
		t.Fatalf("BuildReplyPacket: %v", err)
	}

	got, err := ParseReplyPacket(path, "NULLMDM")
	if err != nil {
		t.Fatalf("ParseReplyPacket: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}

	// The .REP convention: destination conference rides in the header's
	// message-number field, not its conference field.
	if got[0].Header.Number != 3 {
		t.Fatalf("Header.Number = %d, want the destination conference 3", got[0].Header.Number)
	}
	if got[0].Header.Conference != 0 {
		t.Fatalf("Header.Conference = %d, want 0 -- writing it in both fields makes doors post twice", got[0].Header.Conference)
	}
	if got[0].Header.RefNumber != 42 {
		t.Fatalf("RefNumber = %d, want 42", got[0].Header.RefNumber)
	}
	if got[0].Text != "a public reply" {
		t.Fatalf("Text = %q, want %q", got[0].Text, "a public reply")
	}
	if got[0].Header.Status != ' ' {
		t.Fatalf("Status = %q, want a space for a public reply", got[0].Header.Status)
	}
	if got[1].Header.Status != '*' {
		t.Fatalf("Status = %q, want '*' for a private reply", got[1].Header.Status)
	}
}

func TestBuildReplyPacketAddsQWKEKludgesForOverlongFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NULLMDM.REP")
	longSubject := "A subject line well beyond the classic twenty-five byte field"

	err := BuildReplyPacket(path, "NULLMDM", []Reply{{
		Conference: 3, To: "BOB", From: "ALICE", Subject: longSubject, Text: "body",
	}})
	if err != nil {
		t.Fatalf("BuildReplyPacket: %v", err)
	}

	got, err := ParseReplyPacket(path, "NULLMDM")
	if err != nil {
		t.Fatalf("ParseReplyPacket: %v", err)
	}
	if got[0].Header.Subject != longSubject[:25] {
		t.Fatalf("header Subject = %q, want it truncated to the 25-byte field", got[0].Header.Subject)
	}

	k, body := ParseQWKEKludges(got[0].Text)
	if k.Subject != longSubject {
		t.Fatalf("kludge Subject = %q, want the untruncated %q", k.Subject, longSubject)
	}
	if body != "body" {
		t.Fatalf("body = %q, want %q", body, "body")
	}
}

func TestBuildReplyPacketWritesBBSIDInRecordZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NULLMDM.REP")
	if err := BuildReplyPacket(path, "nullmdm", []Reply{{Conference: 1, Text: "hi"}}); err != nil {
		t.Fatalf("BuildReplyPacket: %v", err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening reply packet: %v", err)
	}
	defer zr.Close()

	if len(zr.File) != 1 || zr.File[0].Name != "NULLMDM.MSG" {
		t.Fatalf("packet entries = %v, want exactly NULLMDM.MSG (uppercased)", zr.File)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("opening NULLMDM.MSG: %v", err)
	}
	defer rc.Close()
	var rec0 [128]byte
	if _, err := io.ReadFull(rc, rec0[:]); err != nil {
		t.Fatalf("reading record 0: %v", err)
	}
	if got := strings.TrimRight(string(rec0[:]), " "); got != "NULLMDM" {
		t.Fatalf("record 0 = %q, want the BBS ID so the door can confirm the packet is its own", got)
	}
}

func TestBuildReplyPacketRejectsBlankBBSID(t *testing.T) {
	if err := BuildReplyPacket(filepath.Join(t.TempDir(), "X.REP"), "", nil); err == nil {
		t.Fatal("BuildReplyPacket should refuse a blank BBS ID -- it names the packet's only entry")
	}
}
