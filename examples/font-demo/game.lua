-- Select one project font in the editor's FONT workspace, then rebuild.
-- All fonts keep the same 4x5 artwork / 6x8 advance. Unicode uses Mode 4.
function _draw()
  cls(1)
  print("MTILE FONT DEMO", 12, 8, 7)
  print("Hello GBA! 0123456789", 12, 24, 10)
  print("Ελλάδα: Καλημέρα κόσμε!", 12, 40, 11)
  print("ΆΈΉΊΌΎΏ άέήίόύώ ϊϋΐΰ", 12, 56, 11)
  print("Grüße aus Deutschland!", 12, 72, 12)
  print("ÄÖÜ äöü ß ẞ", 12, 88, 12)
  print("Same grid: 40 columns x 20 rows", 12, 112, 7)
  print("ONE FONT IN ROM", 12, 136, 10)
end
