# Κανονικό Mode 0: τι αποδεικνύει αυτή η έκδοση

Αυτές οι σημειώσεις αφορούν το `examples/celeste-mode0`, με `renderer: mode0`.
Αντικαθιστούν τις παλιές σημειώσεις του μικρού prototype που είχαν αντιγραφεί
αρχικά εδώ. Άνοιγμα και επεξεργασία: [README.md](README.md).

## Rendering

- Τέσσερα backgrounds: τρία parallax planes και BG3 terrain.
- Μαλλιά, player, particles, καπνός και το score text χρησιμοποιούν hardware OBJs.
- Όλα τα sprite pixels προϋπάρχουν στο ROM. Το score στο τέλος είναι `print`
  με στατικά glyphs. Δεν γίνεται runtime σχεδίαση κύκλων/ορθογωνίων.
- Οι `spr`/`map`/`print` είναι δημόσιο Mode 0: η `_draw` έχει `cls` και `map`,
  και τα helpers καλούν `spr`/`print` με αποδεδειγμένα πεπερασμένα IDs/strings.
- Κατά το freeze επαναχρησιμοποιούνται τα ίδια poses. Η φυσική και η φάση του
  draw παραμένουν όπως στο προηγούμενο port.
- Στο τέλος του level η display pal κρύβει το map (φαίνεται `cls(0)`) και το
  `print` γράφει θανάτους και χρόνο. Start καλεί `pal()` και ξαναρχίζει.

## Όρια του συγκεκριμένου παιχνιδιού

| Ομάδα | Θέσεις 8×8 |
| --- | ---: |
| Μαλλιά | 5 |
| Παίκτης | 1 |
| Smoke | 32 |
| Ambient particles | 25 |
| Death particles | 8 |
| Σύνολο | 71 |

Ο transpiler αποδεικνύει **106/128 OAM entries** και **848/1226 worst-case
scanline cycles** από τις κλήσεις `spr`/`print` κάτω από `_draw`. Το όριο δεν
σημαίνει ότι υπάρχουν 106 ενεργά sprites κάθε frame. Στο gameplay μετράει
player/effects. Στο τέλος μετράει τα glyphs του score.

Δεν υπάρχουν entity sprites. Για ένα πυκνότερο level με αντικείμενα πρέπει να
χωράει στο όριο 128 OAM του Mode 0, με νέα απόδειξη Mode 0/native δοκιμή.

Τα numeric pose records έχουν το πολύ 32 πεδία. Έτσι ο υπάρχων transpiler τα
μετατρέπει σε native αριθμητικά πεδία, χωρίς indexed Lua tables ανά sprite.

## Ακρίβεια και μετρήσεις

Τα tiles των effects έχουν ελεγχθεί απέναντι στα παλιά raster primitives,
μαζί με χρώματα, flips και clipping. Το τέλος του level είναι `cls(0)` και
`print` για το score, όχι το παλιό tile panel.

Οι τρέχουσες μετρήσεις CPU/EWRAM και το verification βρίσκονται στις
[γενικές βελτιώσεις του mtile](../../docs/mode0-general-optimizations-2026-09-06.md).
Η αρχική μετατροπή από hybrid σε tiles διατηρείται ως
[ιστορικό αποτέλεσμα](../../docs/celeste-mode0-tiles-2026-09-06.md).

Η απουσία Mode 4 δεν εγγυάται σταθερά 30 updates/s. Το game logic, η προετοιμασία
των sprite slots και η αρχικοποίηση ενός νέου παιχνιδιού εξακολουθούν να
κοστίζουν CPU. Τα σταθερά cloud maps πλέον μένουν στη VRAM και μετακινούνται
με hardware scrolling· δεν ανακατασκευάζονται σε κάθε πέρασμα tile.
Αυτή είναι δοκιμαστική έκδοση με σαφή όρια, όχι γενική υπόσχεση
ότι οποιοδήποτε παιχνίδι ή επίπεδο χωράει στο ίδιο frame budget.
