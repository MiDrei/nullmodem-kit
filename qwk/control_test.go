package qwk

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteControlDATLineOrder(t *testing.T) {
	c := ControlInfo{
		BBSName:    "Maiks Place BBS",
		City:       "Zurich, CH",
		Phone:      "000-000-0000",
		SysopName:  "SwissMaik",
		BBSID:      "nullmodm",
		PacketTime: time.Date(2026, time.September, 23, 14, 5, 30, 0, time.UTC),
		CallerName: "alice",
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "Personal"},
			{Number: 3, Name: "General Chat"},
		},
	}
	var buf bytes.Buffer
	if err := WriteControlDAT(&buf, c); err != nil {
		t.Fatalf("WriteControlDAT: %v", err)
	}
	lines := strings.Split(buf.String(), "\r\n")

	want := []string{
		"Maiks Place BBS",
		"Zurich, CH",
		"000-000-0000",
		"SwissMaik,Sysop",
		"00000,NULLMODM",
		"09-23-2026,14:05:30",
		"ALICE",
		"",
		"0",
		"0",
		"1", // len(Conferences)-1 = 2-1
		"0",
		"Personal",
		"3",
		"General Chat",
		"WELCOME",
		"NEWS",
		"GOODBYE",
	}
	if len(lines) < len(want) {
		t.Fatalf("got %d lines, want at least %d: %q", len(lines), len(want), lines)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestWriteControlDATAllowsLongConferenceNamesButCapsAt255(t *testing.T) {
	longName := strings.Repeat("x", 300)
	c := ControlInfo{
		BBSID:      "id",
		PacketTime: time.Now(),
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "This Name Is Way Too Long For classic QWK, but QWKE allows up to 255 chars"},
			{Number: 1, Name: longName},
		},
	}
	var buf bytes.Buffer
	if err := WriteControlDAT(&buf, c); err != nil {
		t.Fatalf("WriteControlDAT: %v", err)
	}
	lines := strings.Split(buf.String(), "\r\n")
	// line index 12 is the conference-0 name (11 header lines, index 0-10, then
	// conf number at 11, name at 12); conf 1's number/name follow at 13/14.
	if lines[12] != "This Name Is Way Too Long For classic QWK, but QWKE allows up to 255 chars" {
		t.Fatalf("conference 0 name line = %q, want the full untruncated name (QWKE allows up to 255 chars)", lines[12])
	}
	if len(lines[14]) != 255 {
		t.Fatalf("conference 1 name line len = %d, want capped at 255 (QWKE's own limit)", len(lines[14]))
	}
}

func TestControlDATRoundTrip(t *testing.T) {
	want := ControlInfo{
		BBSName:     "NullModem BBS",
		City:        "Zurich, CH",
		Phone:       "000-000-0000",
		SysopName:   "Maik",
		BBSID:       "NULLMDM",
		PacketTime:  time.Date(2026, 9, 24, 17, 5, 30, 0, time.UTC),
		CallerName:  "ALICE",
		WelcomeFile: "WELCOME.ANS",
		NewsFile:    "NEWS.ANS",
		GoodbyeFile: "GOODBYE.ANS",
		Conferences: []ConferenceInfo{
			{Number: 0, Name: "Personal"},
			{Number: 3, Name: "Go Programming"},
			{Number: 7, Name: "Retro Computing"},
		},
	}

	var buf bytes.Buffer
	if err := WriteControlDAT(&buf, want); err != nil {
		t.Fatalf("WriteControlDAT: %v", err)
	}
	got, err := ReadControlDAT(&buf)
	if err != nil {
		t.Fatalf("ReadControlDAT: %v", err)
	}

	if got.BBSName != want.BBSName || got.City != want.City || got.Phone != want.Phone {
		t.Fatalf("system fields = %q/%q/%q", got.BBSName, got.City, got.Phone)
	}
	if got.SysopName != want.SysopName {
		t.Fatalf("SysopName = %q, want %q -- the \",Sysop\" suffix should be stripped", got.SysopName, want.SysopName)
	}
	if got.BBSID != want.BBSID {
		t.Fatalf("BBSID = %q, want %q", got.BBSID, want.BBSID)
	}
	if !got.PacketTime.Equal(want.PacketTime) {
		t.Fatalf("PacketTime = %v, want %v", got.PacketTime, want.PacketTime)
	}
	if got.CallerName != want.CallerName {
		t.Fatalf("CallerName = %q, want %q", got.CallerName, want.CallerName)
	}
	if len(got.Conferences) != len(want.Conferences) {
		t.Fatalf("Conferences = %+v, want %d entries", got.Conferences, len(want.Conferences))
	}
	for i, c := range want.Conferences {
		if got.Conferences[i] != c {
			t.Fatalf("Conferences[%d] = %+v, want %+v", i, got.Conferences[i], c)
		}
	}
	if got.WelcomeFile != want.WelcomeFile || got.NewsFile != want.NewsFile || got.GoodbyeFile != want.GoodbyeFile {
		t.Fatalf("screen files = %q/%q/%q, want %q/%q/%q",
			got.WelcomeFile, got.NewsFile, got.GoodbyeFile,
			want.WelcomeFile, want.NewsFile, want.GoodbyeFile)
	}
}

func TestReadControlDATAcceptsTwoDigitYear(t *testing.T) {
	const raw = "Some BBS\r\nCity\r\nPhone\r\nSysop,Sysop\r\n00000,SOMEBBS\r\n" +
		"09-24-26,17:05:30\r\nALICE\r\n\r\n0\r\n0\r\n0\r\n0\r\nPersonal\r\n" +
		"HELLO\r\nNEWS\r\nGOODBYE\r\n"

	got, err := ReadControlDAT(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadControlDAT: %v", err)
	}
	want := time.Date(2026, 9, 24, 17, 5, 30, 0, time.UTC)
	if !got.PacketTime.Equal(want) {
		t.Fatalf("PacketTime = %v, want %v -- two-digit years are common in DOS-era packets", got.PacketTime, want)
	}
}

func TestReadControlDATOnTruncatedFileKeepsWhatItHas(t *testing.T) {
	got, err := ReadControlDAT(strings.NewReader("Some BBS\r\nCity\r\n"))
	if err != nil {
		t.Fatalf("ReadControlDAT should not fail on a short file: %v", err)
	}
	if got.BBSName != "Some BBS" || got.City != "City" {
		t.Fatalf("got = %+v, want the two lines that were present", got)
	}
	if len(got.Conferences) != 0 {
		t.Fatalf("Conferences = %+v, want none", got.Conferences)
	}
}

func TestReadControlDATStopsAtTruncatedConferenceBlock(t *testing.T) {
	// Claims 4 conferences (3 + 1) but the file ends after two.
	const raw = "BBS\r\nCity\r\nPhone\r\nSysop,Sysop\r\n00000,ID\r\n" +
		"09-24-2026,17:05:30\r\nALICE\r\n\r\n0\r\n0\r\n3\r\n0\r\nPersonal\r\n1\r\nChat\r\n"

	got, err := ReadControlDAT(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadControlDAT: %v", err)
	}
	if len(got.Conferences) != 2 {
		t.Fatalf("Conferences = %+v, want the 2 that are actually present", got.Conferences)
	}
}
