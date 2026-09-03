//go:build mtile_mode0 && mtile_mode0_sheet_immutable

package runtime

// Generated builds select this only after the closed-world source scan proves
// sset unreachable. Keep it separate from mtile_sheet_readonly: Mode 0 retains
// the faster generic sheet representation but can remove its impossible
// per-frame dirty-tile upload scan.
const mode0SheetImmutable = true
