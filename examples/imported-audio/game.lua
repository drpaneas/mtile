-- Imported MOD music and WAV effects can coexist with hardware PSG notes.
function _init()
  pcm_music(0)
end

function _update()
  if btnp(5) then sample(0) end          -- A: imported WAV bell
  if btnp(4) then gba_note(1, 69, 0, 24) end -- B: PSG pulse ping
  if btnp(8) then pcm_music(-1) end     -- L: stop imported music
  if btnp(9) then pcm_music(0) end      -- R: restart it
end

function _draw()
  cls(1)
end
