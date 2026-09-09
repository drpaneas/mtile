-- Control for nonnumeric keys: warmed string hints retain their existing path.
keys = {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p"}
slots = {}
checksum = 0

function _init()
  for i = 1, 16 do
    slots[keys[i]] = i
  end
end

function _update()
  checksum = 0
  for i = 1, 16 do
    local key = keys[i]
    local value = slots[key] + 1
    slots[key] = value
    checksum = checksum + value
  end
end

function _draw()
  cls(0)
  spr(1, checksum % 232, 60)
end
