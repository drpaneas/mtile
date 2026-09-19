-- All 128 map occurrences share tile 1 and therefore the same animation phase.
-- Only its first two rows change: one 64-byte tile upload per game tick.
local tick=0
function _update()
 tick=tick+1
 local color=2
 if tick%2==1 then color=3 end
 sset(8,0,color)
 sset(9,0,color)
 sset(10,0,color)
 sset(11,0,color)
 sset(12,0,color)
 sset(13,0,color)
 sset(14,0,color)
 sset(15,0,color)
 sset(8,1,color)
 sset(9,1,color)
 sset(10,1,color)
 sset(11,1,color)
 sset(12,1,color)
 sset(13,1,color)
 sset(14,1,color)
 sset(15,1,color)
end
function _draw()
 cls(21)
 map(0,0,0,0,0,30,20)
end
