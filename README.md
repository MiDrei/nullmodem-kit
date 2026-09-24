# NullModem Kit

Der gemeinsame Unterbau der NullModem-Familie: [NullModem
BBS](https://git.maik.ch/nullmodem/bbs) und [NullModem
Reader](https://git.maik.ch/nullmodem/reader).

| Paket | Inhalt |
|---|---|
| `ansi` | Die Grid-Matrix: CP437 ↔ Unicode, ANSI/SGR-Parser, Layout-Engine mit `{FILL}`/`{PLACEHOLDER}`, HTML-Konverter |
| `qwk` | QWK/QWKE-Formatschicht: `CONTROL.DAT`, `MESSAGES.DAT`, `.NDX`, `TOREADER.EXT`, Pakete lesen und schreiben, `.REP` in beide Richtungen |
| `zmodem` | ZMODEM senden und empfangen |

## Warum ein eigenes Repo

Server und Reader müssen sich über dasselbe Dateiformat und dieselbe
Bildschirmdarstellung einig sein. Zwei Kopien desselben Codes driften ab dem
ersten Bugfix auseinander — und beim QWK-Format merkt man das erst, wenn
jemandem Post verlorengeht.

Go erlaubt keinen modulübergreifenden Import aus `internal/`, deshalb liegen
diese Pakete hier statt in einem der beiden Projekte.

## Die Grid als einzige Darstellung

`ansi.Grid` ist ein Zellenraster: jede Zelle hält das rohe CP437-Byte plus
Vorder- und Hintergrundfarbe. Wer etwas anzeigen will, parst es in eine Grid
und schreibt einen Blitter dafür — für Telnet (`Grid.Encode`), für HTML
(`ToHTML`), für ein UTF-8-Terminal, für ein Fenster mit Bitmapfont. Das
Zeichenbyte bleibt erhalten, weil eine Rückabbildung Unicode → CP437
verlustbehaftet wäre.

## Stand

Alle Pakete sind stdlib-only. `go test ./...`
