tick=0
checksum=0
function _init()

end
function _update()
tick=tick+1
local frame=tick%120
checksum=0
end
function _draw()
local frame=tick%120
cls(1)
camera(frame*1*9,frame*3)
map(0,flr((frame*1*9)/256)*32,flr((frame*3)/64)*8,0,0,64,32)
camera(0,0)
end
