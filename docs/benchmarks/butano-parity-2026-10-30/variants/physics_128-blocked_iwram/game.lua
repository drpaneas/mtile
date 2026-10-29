local state_a={0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
local state_b={0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
tick=0
checksum=0
function _init()
for k=1,253,4 do
local i=(k-1)/4+1
local j=i-1
state_a[k]=8+(j*13)%220
state_a[k+1]=8+(j*17)%136
state_a[k+2]=((j%7)-3)*0.125
state_a[k+3]=((j%5)-2)*0.125
end
for k=1,253,4 do
local i=(k-1)/4+65
local j=i-1
state_b[k]=8+(j*13)%220
state_b[k+1]=8+(j*17)%136
state_b[k+2]=((j%7)-3)*0.125
state_b[k+3]=((j%5)-2)*0.125
end
end
function simulate()
tick=tick+1
local frame=tick%120
checksum=0
for k=1,253,4 do
local x=state_a[k]
local y=state_a[k+1]
local dx=state_a[k+2]
local dy=state_a[k+3]+0.0625
x=x+dx
y=y+dy
if x<4 then x=4 dx=abs(dx) elseif x>235 then x=235 dx=-abs(dx) end
if y<4 then y=4 dy=abs(dy) elseif y>155 then y=155 dy=-abs(dy) end
state_a[k]=x state_a[k+1]=y state_a[k+2]=dx state_a[k+3]=dy
checksum=(checksum+flr(x)+flr(y))%16001
end
for k=1,253,4 do
local x=state_b[k]
local y=state_b[k+1]
local dx=state_b[k+2]
local dy=state_b[k+3]+0.0625
x=x+dx
y=y+dy
if x<4 then x=4 dx=abs(dx) elseif x>235 then x=235 dx=-abs(dx) end
if y<4 then y=4 dy=abs(dy) elseif y>155 then y=155 dy=-abs(dy) end
state_b[k]=x state_b[k+1]=y state_b[k+2]=dx state_b[k+3]=dy
checksum=(checksum+flr(x)+flr(y))%16001
end
end
function _draw()
local frame=tick%120
cls(1)
camera(0,0)
end

function _update() simulate() end
