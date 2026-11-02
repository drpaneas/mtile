package hostfiles

import (
	"reflect"
	"testing"
)

func TestPickerOutputKeepsSpacesUnicodeAndDelimitedPaths(t *testing.T) {
	want := []string{"/Music/jump sound.wav", "/Music/ήχος.wav", "/Music/trailing space.wav "}
	for _, separator := range []string{"\x00", "\x1f", "\n"} {
		out := want[0] + separator + want[1] + separator + want[2]
		if separator == "\x00" {
			out += separator
		}
		out += "\n"
		if got := parsePickerPaths([]byte(out), separator); !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: %#v", separator, got)
		}
	}
	if got := parsePickerPaths(nil, "\x00"); len(got) != 0 {
		t.Fatal("cancel created an empty file")
	}
	if got := parsePickerPaths([]byte("/Music/line\nbreak.wav\x00\n"), "\x00"); len(got) != 1 || got[0] != "/Music/line\nbreak.wav" {
		t.Fatal("native NUL-delimited name was split")
	}
}
