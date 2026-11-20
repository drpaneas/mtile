# Celeste με κανονικό Mode 0

Οι σημειώσεις αφορούν μια τοπική έκδοση του `examples/celeste`, με τέσσερα
tile backgrounds και προκατασκευασμένα sprite frames. Οι εντολές προϋποθέτουν
ότι διαθέτεις τον κώδικα Lua και τα assets της έκδοσης στον ίδιο φάκελο.

```sh
go run ./cmd/mtile edit research/celeste-mode0
```

Το `project.json` επιλέγει `"renderer": "mode0"`. Δεν χρειάζεται experimental
flag. Με Cmd/Ctrl+R χτίζεις και δοκιμάζεις το παιχνίδι. Για σκέτο ROM:

```sh
go run ./cmd/mtile build research/celeste-mode0
```

## Πού σχεδιάζεται τι

| Περιεχόμενο | Επιφάνεια του GBA |
| --- | --- |
| Μακρινά / μεσαία / κοντινά σύννεφα | BG0 / BG1 / BG2 |
| Level | BG3 |
| Παίκτης, πέντε κόμβοι μαλλιών, καπνός, particles, score text | Hardware sprites (OBJs) |

Τα μαλλιά και τα particles δεν καταλαμβάνουν background. Στο τέλος του
level η οθόνη είναι `cls(0)` και `print` για θανάτους/χρόνο. Start ξαναρχίζει.
Δεν υπάρχει panel στο BG3 ούτε software framebuffer για το foreground.

## Πώς έγινε η μετατροπή

1. **Σχεδιάζουμε πρώτα τα pixels ως assets.** Στο Sprite Lab μένουν μόνο τα
   κελιά που χρησιμοποιεί το δωμάτιο, ο παίκτης και τα effects. Τα σχέδια
   αποθηκεύονται στο `assets/spritesheet.json`, οπότε μπορείς να τα
   επεξεργαστείς κανονικά με τον editor.
2. **Το animation διαλέγει εικόνα και θέση.** Τα μαλλιά κρατούν την ίδια κίνηση
   των πέντε κόμβων, αλλά κάθε κόμβος εμφανίζει ένα έτοιμο tile ακτίνας 1 ή 2.
   Η αλλαγή χρώματος του dash διαλέγει την αντίστοιχη χρωματική έκδοση.
   Ο καπνός χρησιμοποιεί τα υπάρχοντα frames. Τα death particles διαλέγουν
   έτοιμα μικρά ορθογώνια καθώς συρρικνώνονται.
3. **Η `_draw()` καλεί `cls`/`map`/`spr`.** Τα baked IDs μπαίνουν δίπλα στο
   γεγονός. Ο transpiler αποδεικνύει ότι τα IDs και το OAM χωράνε στο hardware.
4. **Το score στο τέλος είναι `print`.** Μαύρη οθόνη, θάνατοι και χρόνος ως
   OBJ glyphs. Start ξαναρχίζει το δωμάτιο.

| IDs στο Sprite Lab | Χρήση |
| --- | --- |
| 1–7 | Κανονικές πόζες παίκτη |
| 8–14 | Πόζες μετά το dash |
| 15–16, 17–18 | Μαλλιά, δύο μεγέθη, φορτισμένο και μετά το dash |
| 19–21 | Καπνός |
| 22–25 | Spikes |
| 26–44 | Terrain |
| 45 | Σύννεφα στα BG0–BG2 |
| 46–49, 50–53 | Ambient particles, μαζί με κομμένα σχήματα στις άκρες |
| 54–103 | Death particles, πλάτος/ύψος 1–5 pixels |

Η στοίχιση του παλιού panel δεν ισχύει πλέον. Το τέλος είναι `print`.

## Αλλαγές και επαλήθευση

Το `generate_sprite_effects.py` είναι εργαλείο συγγραφής στον υπολογιστή.
Δεν εκτελείται στον GBA. Αν αλλάξεις σχέδια στον editor, μην ξανατρέξεις τον
generator χωρίς να κρατήσεις τις αλλαγές σου: θα αναδημιουργήσει τα συγκεκριμένα
frames από τους κανόνες του script.

Τα τρέχοντα αποτελέσματα και οι περιορισμοί βρίσκονται στο [MODE0.md](MODE0.md).
Με τις γενικές βελτιώσεις του engine, η ίδια native διαδρομή ανέβηκε από
22,93 σε 28,88 updates/s και τα updates πάνω από το CPU budget έπεσαν από
54/162 σε 1/162. Οι αναλυτικές σημειώσεις για το πώς έγινε κάθε αλλαγή είναι
στο [optimization report](../../docs/mode0-general-optimizations-2026-09-06.md).
Τα υπόλοιπα παλιότερα σημειώματα που αντιγράφηκαν από το port είναι ιστορικά.

Για το πλήρες candidate-specific verification (χρειάζεται το τοπικό PICO-8
για την αναφορά φυσικής και Lua 5.3+ για τα helpers):

```sh
python3 research/celeste-mode0/tests/verify_all.py --gba
```

Η σύγκριση με το προηγούμενο hybrid διαβάζει τα αποθηκευμένα native traces στο
`build/experiments/celeste-parallax/after`. Μπορείς να ορίσεις άλλο αντίστοιχο
αρχείο με `MTILE_CELESTE_MODE0_BASELINE_TRACES`.
