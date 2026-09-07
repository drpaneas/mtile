package audio

import (
	"testing"
	"unsafe"
)

func indexedSoundLibrary(count int) Library {
	base := nativeTestLibrary().Sounds[1]
	sounds := make([]Sound, count)
	for i := range sounds {
		sounds[i] = base
		sounds[i].ID = uint8(i*73 + 19)
	}
	return Library{Sounds: sounds}
}

func TestNativeSoundIndexMatchesLinearLookupAcrossEntireIDDomain(t *testing.T) {
	for _, count := range []int{0, 1, 2, 17, 128, 255, 256} {
		library := indexedSoundLibrary(count)
		var controller Controller
		if !controller.Bind(library) {
			t.Fatalf("valid%d sound library rejected", count)
		}
		for id := 0; id < 256; id++ {
			if got, want := controller.soundIndex.find(library.Sounds, uint8(id)), findSoundIn(library.Sounds, uint8(id)); got != want {
				t.Fatalf("count%d ID%d indexed lookup%p want%p", count, id, got, want)
			}
		}
		if count == 256 && controller.soundIndex.slots[library.Sounds[255].ID] != 255 {
			t.Fatal("last slice position cannot represent a valid sound")
		}
	}
	if unsafe.Sizeof(nativeSoundIndex{}) != 288 {
		t.Fatal("sound index exceeds bounded ID cache storage")
	}
}

func TestNativeSoundIndexPreservesValidationAndFailedBinding(t *testing.T) {
	var controller Controller
	initial := indexedSoundLibrary(256)
	if !controller.Bind(initial) {
		t.Fatal("initial bind failed")
	}
	before := controller.soundIndex
	for _, invalid := range []string{"duplicate", "over-capacity", "missing-event-sound", "wrong-event-kind", "bad-sound"} {
		library := indexedSoundLibrary(256)
		switch invalid {
		case "duplicate":
			library.Sounds[255].ID = library.Sounds[0].ID
		case "over-capacity":
			library.Sounds = append(library.Sounds, library.Sounds[0])
		case "missing-event-sound":
			library.Sounds = library.Sounds[:255]
			library.Clips = []Clip{{Kind: SoundCH2, LengthTicks: 1, NoteEvents: []NoteEvent{{DurationTicks: 1, Note: 69, SoundID: initial.Sounds[255].ID}}}}
		case "wrong-event-kind":
			library.Clips = []Clip{{Kind: SoundCH1, LengthTicks: 1, NoteEvents: []NoteEvent{{DurationTicks: 1, Note: 69, SoundID: library.Sounds[255].ID}}}}
		case "bad-sound":
			library.Sounds[255].Kind = SoundKind(255)
		}
		if ValidateLibrary(library) || controller.Bind(library) {
			t.Fatalf("%s accepted", invalid)
		}
		if controller.soundIndex != before || &controller.library.Sounds[0] != &initial.Sounds[0] || !controller.bound {
			t.Fatalf("%s failure changed the previous binding", invalid)
		}
	}
	if controller.invalidAssets != 5 {
		t.Fatal("invalid binding accounting changed")
	}
	replacement := indexedSoundLibrary(1)
	if !controller.Bind(replacement) {
		t.Fatal("replacement binding failed")
	}
	if controller.soundIndex.find(controller.library.Sounds, initial.Sounds[255].ID) != nil {
		t.Fatal("successful rebind retained an old ID")
	}
	controller.running = true
	if controller.Bind(initial) || len(controller.library.Sounds) != 1 {
		t.Fatal("running controller changed its binding")
	}
}

func TestNativeIndexedValidationAndBindingDoNotAllocate(t *testing.T) {
	library := indexedSoundLibrary(256)
	var controller Controller
	if allocations := testing.AllocsPerRun(100, func() {
		if !ValidateLibrary(library) || !controller.Bind(library) {
			panic("valid library rejected")
		}
	}); allocations != 0 {
		t.Fatalf("indexed validation/binding allocated%g objects", allocations)
	}
}

var nativeSoundIndexBenchmarkSink *Sound

func BenchmarkNativeSoundLookup(b *testing.B) {
	for _, count := range []int{4, 256} {
		library := indexedSoundLibrary(count)
		var controller Controller
		controller.Bind(library)
		id := library.Sounds[count-1].ID
		name := "four"
		if count == 256 {
			name = "full"
		}
		b.Run(name+"/linear", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nativeSoundIndexBenchmarkSink = findSoundIn(library.Sounds, id)
			}
		})
		b.Run(name+"/indexed", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nativeSoundIndexBenchmarkSink = controller.soundIndex.find(library.Sounds, id)
			}
		})
	}
}
