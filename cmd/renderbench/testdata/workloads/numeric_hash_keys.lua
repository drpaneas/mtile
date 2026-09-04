-- Negative integer keys always use the hash part, even when the table grows.
-- Each tick reads and updates all 64 keys through dynamic bracket indexing.
slots = {}
checksum = 0

function _init()
  for i = 1, 64 do
    slots[-i] = i
  end
end

function _update()
  checksum = 0
  for i = 1, 64 do
    local key = -i
    local value = slots[key] + 1
    slots[key] = value
    checksum = checksum + value
  end
end

function _draw()
  cls(0)
  spr(1, checksum % 232, 60)
end
