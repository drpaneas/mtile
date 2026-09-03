//go:build !mtile_static_strings_1 && !mtile_static_strings_8 && !mtile_static_strings_32 && !mtile_static_strings_128

package runtime

const staticStringSlots = MaxStaticStrings
