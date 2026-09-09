package runtime

// runtimePanic preserves panic behavior and makes mTile's own runtime failures
// visible in the preview's Runtime log before the cartridge stops advancing.
func runtimePanic(message string) {
	DebugLog("mTile runtime error:")
	DebugLog(message)
	panic(message)
}
