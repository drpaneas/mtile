# Celeste με απλή φάση parallax

Αντίγραφο του `examples/celeste` για το ιδιωτικό experimental renderer:

```sh
go run ./cmd/mtile edit -experimental-tilecanvas research/celeste-parallax
```

Πάτησε **Cmd/Ctrl+R** για build και preview. Το `project.json` κρατά `auto`:
το experimental ενεργοποιείται μόνο με το flag. Χωρίς αυτό, το ίδιο παιχνίδι
χρησιμοποιεί το κανονικό renderer policy, που επιλέγει Mode 4.

## Τι αλλάζει

Το σημερινό `examples/celeste` έχει ήδη τα ίδια tiled backgrounds με το
`research/celeste-mode0`: τρία επαναλαμβανόμενα cloud planes στο BG0–2, με
ταχύτητες 1, 2 και 4 pixels ανά ενεργό draw, και το terrain στο BG3. Τα tile
IDs διαφέρουν μεταξύ των δύο projects, αλλά τα cloud pixels είναι ίδια.

Το αρχικό project εξακολουθεί να δημιουργεί και να ενημερώνει 17 αόρατα
σύννεφα, μόνο για να καταναλώνει τους παλιούς τυχαίους αριθμούς. Εδώ αυτή η
προσομοίωση αφαιρείται. Το `draw_background()` ενημερώνει μόνο το
`cloud_phase`, όπως η απλή φάση του Mode 0 παραδείγματος.

Το `game.lua`, οι υπόλοιπες συναρτήσεις παρουσίασης, η φυσική, τα entities,
ο ήχος και όλα τα assets αντιγράφονται από το `examples/celeste`. Δεν
μεταφέρονται τα έτοιμα effect sprites ή η απλοποιημένη οθόνη ολοκλήρωσης του
`celeste-mode0`. Η φάση παραμένει στο αρχικό draw timing, ώστε dash freeze και
ολοκλήρωση να διατηρούν την ίδια χρονική συμπεριφορά των backgrounds.

Η αφαίρεση των παλιών `rnd()` αλλάζει τη σειρά των τυχαίων αριθμών για
particles, smoke και camera shake. Επομένως δεν αναμένεται κάθε pixel να
συμφωνεί με το αρχικό παιχνίδι. Η ακριβής σύγκριση rendering γίνεται ανάμεσα
σε Mode 4 και experimental **του ίδιου νέου project**. Η σύγκριση gameplay
με το αρχικό αφορά το υπάρχον δωμάτιο· νέα δωμάτια με τυχαία κινούμενα
entities χρειάζονται δικό τους έλεγχο.

## Μέτρηση

Η επαναλήψιμη native σύγκριση εκτελεί την ίδια διαδρομή και στα δύο projects,
με κοινό snapshot του runtime:

```sh
MTILE_CELESTE_PARALLAX_COMPARE=1 \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestCelesteParallaxCompareDiagnostic$' -count=1 -timeout=15m -v
```

Χρειάζονται TinyGo και ο headless player στο `build/player/mtile-player`.
Η μεταβλητή `MTILE_CELESTE_PARALLAX_OUT` επιλέγει τον φάκελο evidence.
Τα αποτελέσματα και οι περιορισμοί καταγράφονται στο
[report του πειράματος](../../docs/celeste-parallax-comparison-2026-09-08.md).

Η αλλαγή μειώνει δουλειά του παιχνιδιού. Δεν αλλάζει τη μηχανή του renderer
ούτε διευρύνει το δημόσιο Mode 0/AUTO API.
