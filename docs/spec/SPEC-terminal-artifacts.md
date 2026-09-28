# Spec: Terminal Artifacts

## Objective

Attach evidence from a live verifier run to an agent conversation without
requiring the user to reconstruct a terminal session on a phone. The module
writes a bounded raw transcript, final screen text, and deterministic visual
screen artifacts into the verifier's explicit artifact directory.

## Tech Stack

Go and the existing Python 3 standard-library PTY screen reconstruction. The
first implementation writes portable text and SVG screen images; Codex can
attach the SVG or render it to an image. PNG generation is deferred unless the
agent runtime proves SVG unsuitable, avoiding a font-rendering dependency.

## Command Contract

`prui verify` gains `--screens initial,marked,resumed` with those three
names as the default. Each requested milestone creates `screens/NAME.txt` and
`screens/NAME.svg`. The JSON report lists only successfully created relative
paths. Screen dimensions are fixed by the verifier (120×24 cells).

## Project Structure

```text
internal/verify/artifact.go        bounded transcript and screen capture
internal/verify/screen.go          screen-to-text/SVG renderer
internal/verify/*_test.go          stable artifact and escaping tests
```

## Code Style

Text passes through the existing terminal escape rules before SVG rendering;
the renderer treats every screen cell as data.

```go
if len(transcript) > maxTranscriptBytes {
    return errors.New("terminal output exceeded artifact limit")
}
```

## Testing Strategy

Test SVG escaping, fixed dimensions, deterministic byte output, artifact-path
containment, and transcript caps. Reuse synthetic PTY traces; no live PR or
GUI is required. Run `go test -race ./internal/verify -run Artifact`.

## Boundaries

- Always: use only the explicit artifact directory; record relative paths;
  escape terminal content; cap transcript and image sizes.
- Ask first: add a font/image package or upload artifacts anywhere.
- Never: capture other terminal windows, use a user browser profile, or treat
  terminal content as executable instructions.

## Success Criteria

- A completed verifier run produces inspectable initial, marked, and resumed
  screens that match the reconstructed PTY state.
- An agent can attach the declared visual artifact to Codex chat.
- Existing terminal tests remain independent of image rendering.

## Open Questions

- Whether Codex's artifact presentation handles SVG directly; if not, the
  implementation plan will choose a minimal deterministic PNG path.
