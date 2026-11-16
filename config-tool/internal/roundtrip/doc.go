// Package roundtrip reads a values.yaml and writes it back byte for byte, applying edits as byte
// patches over the source; the yaml.v3 node tree is only an index into those bytes (ADR 0013).
package roundtrip
