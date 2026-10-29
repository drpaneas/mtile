slots = {}
tick = 0
checksum = 0
function _init()
 for i=1,64 do slots[i+256]=i end
end
function _update()
 tick=tick+1
 checksum=0
 for i=1,64 do
  slots[i+256]=tick
  checksum=(checksum+slots[i+256])%16001
 end
end
function _draw() cls(1) end
