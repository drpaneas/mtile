tick=0
function _update() tick=tick+1 end
function _draw()
 cls(0)
 local fx=tick%2==1
 local fy=flr(tick/2)%2==1
 pspr(0,38,38,fx,fy)
 anim("wide",110,38,fx,fy,flr(tick/4)/8)
 anim("tall",180,60,fx,fy,flr(tick/4)/8)
end
