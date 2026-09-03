//go:build mtile_sheet_readonly

package runtime

// Generated cartridges select this tag only when the closed-world source scan
// proves that sset() is unreachable. Runtime sheet-representation branches can
// then be removed by TinyGo instead of rechecking an immutable cart field in
// every sprite/map hot path.
const sheetStorageReadOnly = true
