# NullModem Kit

**English** · [Deutsch](README.de.md)

The shared foundation of the NullModem family: [NullModem
BBS](https://github.com/midrei/nullmodem-bbs) and [NullModem
Reader](https://github.com/midrei/nullmodem-reader).

| Package | Contents |
|---|---|
| `ansi` | The grid: CP437 ↔ Unicode, ANSI/SGR parser, layout engine with `{FILL}`/`{PLACEHOLDER}`, HTML converter |
| `qwk` | QWK/QWKE format layer: `CONTROL.DAT`, `MESSAGES.DAT`, `.NDX`, `TOREADER.EXT`, reading and writing packets, `.REP` both ways |
| `zmodem` | ZMODEM send and receive |

```sh
go get github.com/midrei/nullmodem-kit@latest
```

## Why a repository of its own

Server and reader have to agree on the same file format and the same
screen rendering. Two copies of the same code drift apart from the first
bug fix on -- and with QWK you only notice when someone's mail goes
missing.

Go doesn't allow importing another module's `internal/` packages, so
these live here rather than in either project.

## The grid as the one representation

`ansi.Grid` is a grid of cells: each holds the raw CP437 byte plus its
foreground and background colour. To show something, parse it into a
grid and write a blitter for it -- for Telnet (`Grid.Encode`), HTML
(`ToHTML`), a UTF-8 terminal, a window with a bitmap font. The character
byte is kept because mapping Unicode back to CP437 would lose
information.

## Status

All packages use the standard library only. `go test ./...`

## Releases

The BBS and the reader pin the kit to a fixed version in their `go.mod`.
Their builds (the Docker image, reader releases) run with `GOWORK=off`
and take exactly that version, so a kit change only reaches them once
it is tagged and both have moved to it:

```
scripts/release.sh v0.3.0
```

The script checks that `main` is clean and in sync with `origin`, runs
`go vet` and the tests, sets and pushes the tag, and then moves the
checkouts next to it, `../bbs` and `../reader`, onto the new version with
`go get` -- each built and tested. Committing and pushing there is up to
you, after looking at the diff.

For local development across all three repositories, use a `go.work` in
the parent directory; it applies to your machine only, never to a
release.

## License

MIT -- see [LICENSE](LICENSE). The kit is meant to serve other BBS and
reader projects too: QWK/QWKE, CP437 and ANSI are craft nobody needs to
write twice.
