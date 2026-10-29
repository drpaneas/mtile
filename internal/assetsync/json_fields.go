package assetsync

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

// UnknownFields retains uninterpreted JSON object members on an authored
// record. Its immutable storage makes ordinary value copies safe, including
// editor snapshots and records that are moved or resized. It is not a second
// source of known fields and is never emitted as a named JSON property.
type UnknownFields struct {
	raw string
}

// IsZero reports whether a record contains no unknown members.
func (f UnknownFields) IsZero() bool { return f.raw == "" }

// The wrappers below use method-free aliases so known fields keep their
// existing matching, default, null, and omitempty semantics. A v2 embedded
// fallback captures only unmatched members without converting JSON numbers
// through float64. The v1 methods also cover the editor's JSON undo snapshots.
type jsonRecord[T any] struct {
	Known *T             `json:",embed"`
	Other jsontext.Value `json:",embed"`
}

func marshalJSONRecord[T any](value T, fields UnknownFields) ([]byte, error) {
	return jsonv2.Marshal(jsonRecord[T]{Known: &value, Other: jsontext.Value(fields.raw)},
		json.DefaultOptionsV1(),
		jsontext.AllowDuplicateNames(false),
		jsontext.AllowInvalidUTF8(false),
	)
}

func unmarshalJSONRecord[T any](raw []byte, target *T, fields *UnknownFields) error {
	// Like a normal struct, a JSON null leaves the existing record intact.
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	record := jsonRecord[T]{Known: target}
	if err := decodeAuthoredJSON(raw, &record, false); err != nil {
		return err
	}
	fields.raw = string(record.Other)
	return nil
}

func (v Project) MarshalJSON() ([]byte, error) {
	type plain Project
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *Project) UnmarshalJSON(raw []byte) error {
	type plain Project
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v RGB15) MarshalJSON() ([]byte, error) {
	type plain RGB15
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *RGB15) UnmarshalJSON(raw []byte) error {
	type plain RGB15
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v Palette) MarshalJSON() ([]byte, error) {
	type plain Palette
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *Palette) UnmarshalJSON(raw []byte) error {
	type plain Palette
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v SpriteRecord) MarshalJSON() ([]byte, error) {
	type plain SpriteRecord
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *SpriteRecord) UnmarshalJSON(raw []byte) error {
	type plain SpriteRecord
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v Spritesheet) MarshalJSON() ([]byte, error) {
	type plain Spritesheet
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *Spritesheet) UnmarshalJSON(raw []byte) error {
	type plain Spritesheet
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v PosePartRecord) MarshalJSON() ([]byte, error) {
	type plain PosePartRecord
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *PosePartRecord) UnmarshalJSON(raw []byte) error {
	type plain PosePartRecord
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v PoseRecord) MarshalJSON() ([]byte, error) {
	type plain PoseRecord
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *PoseRecord) UnmarshalJSON(raw []byte) error {
	type plain PoseRecord
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v PoseColorVariant) MarshalJSON() ([]byte, error) {
	type plain PoseColorVariant
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *PoseColorVariant) UnmarshalJSON(raw []byte) error {
	type plain PoseColorVariant
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v PoseColorSwap) MarshalJSON() ([]byte, error) {
	type plain PoseColorSwap
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *PoseColorSwap) UnmarshalJSON(raw []byte) error {
	type plain PoseColorSwap
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v PoseFile) MarshalJSON() ([]byte, error) {
	type plain PoseFile
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *PoseFile) UnmarshalJSON(raw []byte) error {
	type plain PoseFile
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v MapLayer) MarshalJSON() ([]byte, error) {
	type plain MapLayer
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *MapLayer) UnmarshalJSON(raw []byte) error {
	type plain MapLayer
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v Map) MarshalJSON() ([]byte, error) {
	type plain Map
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *Map) UnmarshalJSON(raw []byte) error {
	type plain Map
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v AnimationWorkspace) MarshalJSON() ([]byte, error) {
	type plain AnimationWorkspace
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *AnimationWorkspace) UnmarshalJSON(raw []byte) error {
	type plain AnimationWorkspace
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v AnimationPage) MarshalJSON() ([]byte, error) {
	type plain AnimationPage
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *AnimationPage) UnmarshalJSON(raw []byte) error {
	type plain AnimationPage
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v AnimationClip) MarshalJSON() ([]byte, error) {
	type plain AnimationClip
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *AnimationClip) UnmarshalJSON(raw []byte) error {
	type plain AnimationClip
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v TileLink) MarshalJSON() ([]byte, error) {
	type plain TileLink
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *TileLink) UnmarshalJSON(raw []byte) error {
	type plain TileLink
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v TileLinkFile) MarshalJSON() ([]byte, error) {
	type plain TileLinkFile
	return marshalJSONRecord(plain(v), v.UnknownFields)
}

func (v *TileLinkFile) UnmarshalJSON(raw []byte) error {
	type plain TileLinkFile
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v Font) MarshalJSON() ([]byte, error) {
	type plain Font
	return marshalJSONRecord(plain(v), v.UnknownFields)
}
func (v *Font) UnmarshalJSON(raw []byte) error {
	type plain Font
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}

func (v FontGlyph) MarshalJSON() ([]byte, error) {
	type plain FontGlyph
	return marshalJSONRecord(plain(v), v.UnknownFields)
}
func (v *FontGlyph) UnmarshalJSON(raw []byte) error {
	type plain FontGlyph
	return unmarshalJSONRecord(raw, (*plain)(v), &v.UnknownFields)
}
