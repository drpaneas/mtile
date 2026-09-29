tick=0
function _update() tick=tick+1 end
function _draw()
 cls(0)
 local fx=tick%2==1 and btn(5)
 local fy=flr(tick/2)%2==1 and btn(5)
 pspr(0,100,60,fx,fy)
end
