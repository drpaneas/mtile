function _init()
  counter=0
end

function _update()
  counter += 1
  if counter == 5*4+1 then
    counter=1
  end
end

function _draw()
  cls(0)
  local frame=0
  if counter <=5 then frame=0
  elseif counter <=10 then frame=1
  elseif counter <=15 then frame=2
  else frame=3 end
  pspr(frame, 10, 10)
end
