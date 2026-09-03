items={1,2,3,4,5,6,7,8}
total=0

function take_last()
  return deli(items)
end

function measure()
  return count(items)
end

function _update()
  for i=1,128 do
    local value=take_last()
    total=total+measure()
    add(items,value)
  end
end

function _draw()
  cls(0)
end
