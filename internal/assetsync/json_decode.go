package assetsync

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

// decodeAuthoredJSON keeps existing asset field matching and null/default
// semantics while rejecting ambiguous keys and damaged text, even in ignored
// fields. Unmarshal also requires exactly one complete JSON value.
func decodeAuthoredJSON(raw []byte, target any, rejectUnknown bool) error {
	return jsonv2.Unmarshal(raw, target,
		json.DefaultOptionsV1(),
		jsontext.AllowDuplicateNames(false),
		jsontext.AllowInvalidUTF8(false),
		jsonv2.RejectUnknownMembers(rejectUnknown),
	)
}
