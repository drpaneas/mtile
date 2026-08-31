function _init()
end

function _update()
end

function _draw()
  cls(225)
  -- One through four arguments: every omitted value keeps the established
  -- zero/default behavior after the explicit logical BG number.
  map(0)
  map(1,0)
  map(2,0,0)
  map(3,0,0,0)
end
