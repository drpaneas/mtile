// Package website exposes the same API reference used to build the learning
// website, so installed editor tools do not need a source checkout or network.
package website

import _ "embed"

//go:embed api.json
var APIJSON string
