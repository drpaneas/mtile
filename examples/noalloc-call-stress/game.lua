-- Dedicated GC-root-wrapper A/B workload.
-- A live table parameter remains in scope across 384 pure numeric calls/tick.

local state = {value = 0.25, ticks = 0}

function blend(value, input)
  return value + input * 0.0007
end

function decay(value, input)
  return value * 0.9993 - input * 0.0002
end

function run_kernel(holder, seed)
  local value = seed
  for i = 1, 192 do
    value = blend(value, i)
    value = decay(value, i)
  end
  holder.value = value
  holder.ticks = holder.ticks + 1
end

function _update()
  run_kernel(state, state.value)
end

function _draw()
  cls(1)
  local marker = flr(abs(state.value)) % 220
  rectfill(8, 70, 231, 89, 5)
  rectfill(8, 70, 8 + marker, 89, 11)
  print("pure calls:384 tick:" .. tostr(state.ticks) .. " sum:" .. tostr(flr(state.value)), 4, 4, 7)
end
