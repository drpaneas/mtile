# Celeste: απλή φάση parallax, 8 Σεπτεμβρίου 2026

Το νέο [`research/celeste-parallax`](../research/celeste-parallax/README.md)
λειτουργεί με το experimental renderer, αλλά **δεν βελτίωσε την επίδοση**
στη συγκεκριμένη διαδρομή. Ο διάμεσος χρόνος αυξήθηκε κατά 2,38%, ενώ ο
παρατηρούμενος ρυθμός άλλαξε από 26,88 σε 26,73 game updates/s.

## Τι συγκρίθηκε

Το σημερινό `examples/celeste` έχει ήδη τα ίδια τέσσερα tiled backgrounds
με το `research/celeste-mode0`: τρία cloud planes με ορθογώνια και ένα
terrain plane. Έλεγχος 4.800 map cells επιβεβαίωσε ίδια pixels και flags,
μετά την αντιστοίχιση των διαφορετικών sprite IDs. Οι παλέτες είναι ίδιες.
Συνεπώς δεν υπήρχε πλέον μετατροπή cloud `rectfill` σε hardware maps να γίνει.

Η ουσιαστική διαφορά είναι η παλιά προσομοίωση 17 σύννεφων. Το `celeste`
κρατά τις αρχικές κατανομές, μετακινήσεις και κλήσεις `rnd()` για να
διατηρεί την παλιά ακολουθία τυχαίων αριθμών, παρότι δεν ζωγραφίζει αυτά τα
σύννεφα. Το νέο αντίγραφο αφαιρεί αυτόν τον πίνακα και τους δύο σχετικούς
βρόχους. Το `draw_background()` ενημερώνει μόνο μία φάση, όπως η απλή
προσέγγιση του `celeste-mode0`.

Όλα τα assets, το `game.lua`, ο ήχος και ο υπόλοιπος κώδικας αντιγράφονται
αυτούσια. Δεν μεταφέρονται οι διαφορετικές υλοποιήσεις player/effects/UI του
Mode 0 παραδείγματος. Η φάση παραμένει στο αρχικό draw timing: μεταφορά της
στο update του Mode 0 παραδείγματος θα άλλαζε επιπλέον το freeze και τη
συμπεριφορά της οθόνης ολοκλήρωσης.

## Φρέσκια native μέτρηση

Χρησιμοποιήθηκε κοινό snapshot του compiler και runtime από το τρέχον
working tree, TinyGo 0.41.1 / LLVM 20.1.1, Go 1.26.4 και ο ίδιος headless
mGBA player. Δεν συγκρίθηκαν τα παλιά reports της 6ης Σεπτεμβρίου με νέο
κώδικα. Οι τρεις ROMs έχουν την ίδια instrumentation και διαδρομή εισόδων.
Οι χρόνοι είναι emulated GBA CPU cycles. Το όριο για ένα update ανά δύο LCD
frames είναι 561.792 cycles, περίπου 29,86 updates/s.

Για τα 162 updates της διαδρομής:

| Project / renderer | Median cycles | p95 cycles | Μέγιστο | Updates πάνω από το όριο | Παρατηρούμενα updates/s |
| --- | ---: | ---: | ---: | ---: | ---: |
| Αρχικό `celeste`, experimental | 477.534 | 600.448 | 1.002.364 | 22/162 | 26,88 |
| Νέο `celeste-parallax`, experimental | 488.908 | 607.805 | 1.021.458 | 28/162 | 26,73 |
| Νέο `celeste-parallax`, Mode 4 | 560.517 | 651.945 | 664.197 | 79/162 | 23,83 |

Η νέα έκδοση χρειάστηκε 362 LCD frames για τη διαδρομή, έναντι 360 της
αρχικής. Η μικρότερη ROM και η αφαίρεση κώδικα δεν μεταφράστηκαν σε καλύτερο
συνολικό frame time. Τα αποτελέσματα δεν αποδεικνύουν σταθερά 30 Hz.

Η οθόνη ολοκλήρωσης παραμένει δυσκολότερη για τον sparse foreground canvas:

| Project / renderer | Median cycles, 7 updates | Παρατηρούμενα updates/s |
| --- | ---: | ---: |
| Αρχικό, experimental | 914.399 | 15,48 |
| Νέο, experimental | 928.177 | 15,48 |
| Νέο, Mode 4 | 591.339 | 19,91 |

Τα παραπάνω είναι διαφορές ανάμεσα σε δύο πραγματικές εκδοχές του
παιχνιδιού, όχι απομονωμένη μέτρηση του κόστους του αφαιρεμένου βρόχου.
Χωρίς τις παλιές κλήσεις `rnd()`, αλλάζουν η θέση των particles, ο καπνός
και το camera shake. Ο experimental renderer έφτασε 61 occupied foreground
blocks αντί για 58. Αυτό δείχνει ότι άλλαξε και το φορτίο του foreground·
δεν τεκμηριώνει από μόνο του ποιο μέρος προκάλεσε όλη τη διαφορά χρόνου.

Instrumented builds:

| Project / renderer | ROM bytes | IWRAM bytes | Ελεύθερα EWRAM bytes |
| --- | ---: | ---: | ---: |
| Αρχικό, experimental | 382.212 | 29.164 | 6.480 |
| Νέο, experimental | 379.792 | 29.164 | 6.504 |
| Νέο, Mode 4 | 376.428 | 30.572 | 26.024 |

Η στατική διαφορά EWRAM δεν μετρά τη ζωντανή χρήση του Lua heap. Τα μεγέθη
αυτά αφορούν τις instrumented ROMs, όχι το ξεχωριστό playable build.

Το κανονικό playable ROM έχει **362.096 bytes**, με 6.624 bytes ελεύθερα
EWRAM. Χτίστηκε μέσω του ίδιου project-scoped opt-in που χρησιμοποιεί ο
editor, χωρίς αντικατάσταση εισόδων ή προσθήκη probe κώδικα στο Lua.

## Έλεγχοι ορθότητας

- **280/280 updates** με ίδια βασικά gameplay watches μεταξύ αρχικού και
  νέου: θέση/ταχύτητα, freeze, completion, spawn/death και lifecycle counts.
  Εξαιρούνται τα screen-shake offsets, που διαφέρουν σε 55 updates.
- **Μηδέν διαφορετικά RGB pixels** μεταξύ του νέου project σε Mode 4 και
  experimental, και στα 280 updates. Με το αρχικό project διαφέρουν και
  τα 280 frames λόγω της αλλαγμένης ακολουθίας τυχαίων εφέ.
- Κάλυψη startup, διαδρομής ολοκλήρωσης, dash freeze, camera shake,
  completion UI, Start restart, εισαγόμενου κεντραρισμένου death burst
  μέσω του πραγματικού `kill_player()`, και κανονικού respawn.
- Ενεργός ήχος, χωρίς audio overflow, heap errors ή VBlank commit
  παραβιάσεις στις δύο experimental ROMs. Οι καταγραφές Mode 4 με copy
  εκτός VBlank είναι η κανονική αντιγραφή στο κρυφό framebuffer και δεν
  αποτελούν αντίστοιχη παραβίαση.
- Ίδια bytes σε 14 κοινά source/asset/config/route αρχεία, με εξαίρεση το
  σκόπιμα αλλαγμένο `presentation.lua`. Η αναγέννηση των cloud assets σε
  προσωρινό αντίγραφο είναι idempotent.
- Πέρασαν `go test ./internal/transpiler ./internal/build ./cmd/mtile`,
  το πλήρες `runtime` suite και τα `TestObjectCanvas`/`TestTileCanvas`
  με το `mtile_tilecanvas` build tag.
- Το πραγματικό playable ROM πέρασε ξεχωριστό smoke test 400 native LCD
  frames με ενεργό ήχο και χωρίς audio overflow. Δύο sessions συμφώνησαν
  στα 240 startup frames· μετά την εισαγωγή φυσικών πλήκτρων η εικόνα
  διέφερε σε 156/160 frames από το session χωρίς πλήκτρα. Το release ROM
  δεν παρέχει debug heap telemetry, οπότε ο έλεγχος heap αφορά τις τρεις
  instrumented ROMs παραπάνω.

Η ισότητα gameplay αφορά το υπάρχον δωμάτιο, που δεν έχει ενεργά entity
markers. Άλλα δωμάτια με τυχαία κινούμενα balloons/chests χρειάζονται νέο
έλεγχο: η αλλαγή στο κοινό RNG μπορεί να επηρεάσει και gameplay εκεί.

## Αναπαραγωγή και evidence

```sh
MTILE_CELESTE_PARALLAX_COMPARE=1 \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestCelesteParallaxCompareDiagnostic$' -count=1 -timeout=15m -v
```

Το `MTILE_CELESTE_PARALLAX_OUT` επιλέγει νέο φάκελο evidence. Το
`MTILE_CELESTE_PARALLAX_ROOT` επιλέγει το root από όπου αντιγράφονται τα
projects και το runtime. Για πλήρως παγωμένο compiler, η εντολή πρέπει να
εκτελεστεί από το ίδιο frozen checkout, όπως σε αυτή τη μέτρηση.

Το [machine-readable summary](benchmarks/celeste-parallax-2026-09-08/summary.json)
περιέχει distributions ανά φάση, hashes πηγών/runtime/player/ROMs και τις
συγκρίσεις ορθότητας. Στον ίδιο φάκελο βρίσκονται τα traces, τα logs και τα
asset checks. Το πλήρες τοπικό snapshot και οι ROMs διατηρούνται στο
`build/experiments/celeste-parallax-2026-09-08`.

Δεν αλλάζει δημόσιο Lua API, renderer classification ή AUTO eligibility.
