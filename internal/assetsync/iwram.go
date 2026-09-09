package assetsync

import "fmt"

// ValidateIWRAMFunctions validates the project setting independently of source
// loading. The transpiler additionally resolves every name to a declaration.
func ValidateIWRAMFunctions(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !plainLuaFunctionName(name) {
			return fmt.Errorf("%q must be a plain top-level Lua function name", name)
		}
		switch name {
		case "_init", "_update", "_draw":
			return fmt.Errorf("%q is a reserved lifecycle callback; select a helper instead", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate function %q", name)
		}
		seen[name] = true
	}
	return nil
}

func plainLuaFunctionName(name string) bool {
	if name == "" {
		return false
	}
	for i, ch := range name {
		valid := ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 0 && ch >= '0' && ch <= '9'
		if !valid {
			return false
		}
	}
	switch name {
	case "and", "break", "do", "else", "elseif", "end", "false", "for", "function", "goto", "if", "in", "local", "nil", "not", "or", "repeat", "return", "then", "true", "until", "while":
		return false
	}
	return true
}
