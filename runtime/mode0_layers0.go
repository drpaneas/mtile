//go:build mtile_mode0 && mtile_mode0_layers0

package runtime

const (
	mode0LayerCount        = 0
	mode0LayerStorageCount = 1 // keeps shared map code compilable; no BG is enabled or presented
)
