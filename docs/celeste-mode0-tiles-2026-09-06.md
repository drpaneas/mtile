# Celeste: έκδοση αποκλειστικά με tiles και hardware sprites

> Ιστορικό αποτέλεσμα της αρχικής μετατροπής. Η νεότερη έκδοση περιλαμβάνει
> γενικές βελτιώσεις του compiler/runtime και καλύτερη χρήση των tilemaps·
> οι τρέχουσες μετρήσεις είναι στις
> [σημειώσεις βελτιστοποίησης](mode0-general-optimizations-2026-09-06.md).

Η μέτρηση έγινε με τοπική έκδοση στο `research/celeste-mode0`. Για την αναπαραγωγή
χρειάζονται χωριστά ο κώδικας Lua και τα assets. Η έκδοση χρησιμοποιεί τον κανονικό
δημόσιο renderer με `project.json: {"renderer":"mode0"}`. Δεν απαιτεί
`-experimental-tilecanvas`, ιδιωτικά build tags ή εξαιρέσεις στον transpiler.
Το `examples/celeste` παραμένει χωριστό.

```sh
go run ./cmd/mtile edit research/celeste-mode0
```

## Τι άλλαξε και πώς

**Assets του παιχνιδιού:** προστέθηκαν 180 editable κελιά 8×8, IDs 240–419.
Τα αρχικά 128 authored sprites διατηρήθηκαν. Ο generator αποθηκεύει τα
κανονικά pixels/flags του Sprite Lab, χωρίς εξάρτηση κατά την εκτέλεση.
Οι κύκλοι των μαλλιών έγιναν οκτώ έτοιμα masks: δύο μεγέθη επί τέσσερα χρώματα.
Οι πέντε κόμβοι κρατούν τις προηγούμενες τροχιές τους. Οι χρωματικές εκδοχές
των επτά player poses αποφεύγουν την αλλαγή draw palette ανά sprite.
Τα shrinking death particles έχουν όλα τα πλάτη/ύψη 1–5 pixels. Τα ambient
particles έχουν επιπλέον τα κομμένα 1×2/2×1 σχήματα για τις άκρες της οθόνης.

**Σύνθεση του GBA:** τα BG0–2 συνεχίζουν να κινούν τα τρία cloud planes.
Το BG3 κρατά το terrain. Τα effects, ο παίκτης και τα entities είναι κανονικά
OBJs, χωρίς software foreground buffer. Το UI γράφει προσωρινά 204 κελιά
στο BG3· ο backup πίνακας προδεσμεύεται και τα κελιά επαναφέρονται στο restart.
Το panel έχει glyph spacing 8px και κρύβει ολόκληρα τα sprites που το
ακουμπούν, έως 7 pixels έξω από το περίγραμμά του. Το terrain shake παγώνει
όσο φαίνεται το panel. Αυτές είναι οι σκόπιμες οπτικές προσαρμογές του UI.

**Lua και transpiler:** τα helpers επιλέγουν έτοιμα frames και γεμίζουν 91
σταθερά sprite slots. Τα `spr`/`map` είναι άμεσα και ανεπιφύλακτα στην `_draw`,
όπως απαιτεί το υπάρχον public Mode 0 proof. Τα pose records χωρίζονται σε
ομάδες έως 32 αριθμητικών πεδίων, ώστε ο υπάρχων transpiler να τα μετατρέπει
σε 364 native numeric fields. Αυτό αποφεύγει εκατοντάδες indexed Lua table
reads/writes ανά frame. Δεν άλλαξε ο compiler ούτε χαλάρωσε κάποια απόδειξη.
Οι generators εκτελούνται μόνο κατά τη συγγραφή, στον υπολογιστή.

**Γενικές διορθώσεις στο mtile:** ο Mode 0 presenter περιμένει την αρχή ενός
πλήρους VBlank αν το frame pacer επιστρέψει αργά. Το `LastBlitWindow` μετρά
τις πραγματικές μεταφορές και εξαιρεί την αναμονή. Οι παλέτες BG/OBJ
ενημερώνονται μόνο όταν αλλάξουν, αντί να ξαναγράφονται 256 OBJ χρώματα για
κάθε αλλαγή θέσης. Τα palette remaps της `_init` μεταφέρονται κατά το
`FinalizeInitHardware`, όσο η οθόνη παραμένει force-blanked. Αποφεύγεται έτσι
η συσσώρευση δύο παλετών και τεσσάρων maps στο πρώτο ορατό commit.
Καμία από αυτές τις αλλαγές δεν γνωρίζει Celeste, sprite IDs ή συγκεκριμένο
level. Δεν προστέθηκε Lua API ούτε άλλαξε classification/AUTO eligibility.

## Native αποτέλεσμα

Native mGBA, ίδια καθορισμένη διαδρομή 162 game updates, στόχος δύο LCD scans
ανά game update: 561.792 cycles. Τα metrics περιλαμβάνουν diagnostic probes.
Η σύγκριση με το προηγούμενο hybrid αφορά ολοκληρωμένες εκδόσεις, όχι
απομονωμένο A/B ενός μόνο compiler optimization: αλλάζει το UI και ο τρόπος
προετοιμασίας της εικόνας.

| Μέτρηση | Προηγούμενο hybrid | Νέο κανονικό Mode 0 |
| --- | ---: | ---: |
| Median cycles/update | 542.957 | 507.083 |
| 95ο εκατοστημόριο | 697.083 | 778.763 |
| Μέγιστο route update | 1.069.611 | 1.735.538 |
| Updates πάνω από 561.792 cycles | 62/162 | 54/162 |
| Μετρημένα updates/s | 23,54 | 22,93 |
| Ελεύθερη EWRAM στο diagnostic build | 7.344 bytes | 61.456 bytes |

Η μνήμη βελτιώθηκε κατά **54.112 bytes**. Ο median CPU χρόνος μειώθηκε, αλλά
οι αιχμές και η παρουσίαση εξακολουθούν να εμποδίζουν σταθερό ρυθμό περίπου
30 Hz. Η μετρημένη cadence δεν βελτιώθηκε έναντι του hybrid. Το Mode 0
αφαίρεσε το bitmap κόστος, αλλά η Lua, η προετοιμασία των slots, η ανανέωση
των παραθύρων των maps και το completion/restart συνεχίζουν να κοστίζουν.
Το πρώτο completion UI update βρίσκεται μέσα στο route maximum. Το Start
restart έφτασε 3.247.345 cycles σε ξεχωριστή φάση· δεν κρύβεται ως route sample.

Η ίδια νέα tile έκδοση χτίστηκε επίσης σε Mode 4 ως pixel oracle. Αυτή η
σύγκριση αποδεικνύει την απόδοση/εικόνα του renderer στην ίδια Lua, αλλά δεν
είναι σωστό baseline για το παλιό παιχνίδι, που σχεδίαζε απευθείας primitives.

Το unmodified playable ROM έχει **476.556 bytes**, **61.584 bytes ελεύθερη
EWRAM** και **23.600/32.672 bytes IWRAM**. Ο compiler αποδεικνύει 103 OAM
entries μαζί με το offscreen diagnostic, και 824/1.226 worst-case OBJ scanline
cycles. Στο trace εμφανίστηκαν έως 43 πραγματικά ενεργά OBJs. Δεν υπάρχει
απώλεια από τα όρια των ομάδων effects στη δοκιμή.

## Verification

- 280 native εικόνες: η νέα Lua/τα ίδια assets δίνουν μηδέν διαφορετικά pixels
  μεταξύ forced Mode 0 και forced Mode 4.
- 272 εικόνες χωρίς completion panel: ίδιο RGB hash με το προηγούμενο hybrid.
  Οι μόνες οκτώ διαφορετικές εικόνες είναι το σκόπιμα νέο completion panel.
- 280 snapshots gameplay/effects: ακριβής συμφωνία με το προηγούμενο port.
- 280 πρώτες σαρώσεις: ίδιο hash με την αντίστοιχη σταθεροποιημένη εικόνα.
  Όλα τα commits έγιναν εντός VBlank: πρώτη μεταφορά scanlines 160–196,
  μέγιστη τελική scanline 214. Δεν παρατηρήθηκε tearing ή queue overflow.
- 180 asset συγκρίσεις απέναντι στον πραγματικό Mode 4 rasterizer/font και
  180 atlas placement συγκρίσεις, μαζί με flips και edge clipping.
- 2.141 helper snapshots: ίδιες logical draw commands, τροχιές, entity state
  και RNG. Επτά UI save/restore κύκλοι ελέγχουν όλα τα 600 room cells και τα
  τέσσερα map planes, μαζί με counters, freeze και επανάληψη.
- PICO-8 oracle: 43 physics scenarios / 1.166 states και έξι lifecycle
  scenarios / 600 states πέρασαν με τα **προϋπάρχοντα, αμετάβλητα** numeric
  tolerances. Δεν πρόκειται για νέο ισχυρισμό απόλυτης αριθμητικής ισότητας
  με το PICO-8 μετά την προηγούμενη στρογγυλοποίηση των physics constants.
- Το κανονικό playable ROM έκανε boot και 160 LCD frames με πραγματικά inputs,
  ενεργό ήχο, χωρίς audio overflow ή heap error.
- Πέρασαν οι απαιτούμενες suites `internal/transpiler`, `internal/build`,
  `cmd/mtile` και το default runtime. Πέρασαν επίσης τα πλήρη relevant Mode 0
  configurations και οι focused ObjectCanvas/TileCanvas regressions.
  Η ιδιωτική tilecanvas full suite έχει γνωστή ασυμβατότητα με ένα Mode 4
  `Pget` test, το οποίο ο ιδιωτικός renderer απορρίπτει ρητά.

## Αρχεία και αναπαραγωγή

- [Οδηγίες Sprite Lab και οργάνωση αρχείων](../research/celeste-mode0/README.md)
- [Συγκεκριμένα budgets και περιορισμοί](../research/celeste-mode0/MODE0.md)
- Native ROMs, PNGs, JSON traces και logs:
  `build/experiments/celeste-mode0/final/`.
- Προηγούμενη έκδοση πριν από palette caching / UI preallocation:
  `build/experiments/celeste-mode0/before-palette-cache/`.
- Τα native results είναι από τον emulator, όχι από φυσική κονσόλα GBA.

```sh
python3 research/celeste-mode0/tests/verify_all.py --gba
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

Ο γενικός editor μπορεί να επεξεργαστεί όλα τα νέα frames. Η αναγέννηση των
assets από το script αντικαθιστά τα συγκεκριμένα authored frames, επομένως
χρειάζεται να διατηρούνται τυχόν χειροκίνητες αλλαγές πριν εκτελεστεί ξανά.
