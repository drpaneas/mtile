-- Selected-font Mode 0/4 parity and cost fixture: static HUD, numeric score,
-- and a complete 40-column row. The project selects "international".
score=0
function _update() score=(score+1)%1000 end
function _draw()
  cls(1)
  print("SCORE",0,0,7)
  print(score,36,0,7)
  print("ABCDABCDABCDABCDABCDABCDABCDABCDABCDABCD",0,16,7)
end
