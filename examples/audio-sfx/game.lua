-- PICO-8 instrument/effect/filter and SFX runtime example.

show_audio_stats = false

function _init()
  sfx(0)
end

function _update()
  if btnp(5) then sfx(1) end
  if btnp(4) then sfx(2) end
  if btnp(2) then sfx(3, 0) end
  if btnp(3) then sfx(-1, 0) end
  if btnp(0) then sfx(4) end
  if btnp(1) then sfx(5) end
  if btnp(6) then
    if btn(7) then sfx(-1) else sfx(6) end
  end
  if btnp(7) and not btn(6) then show_audio_stats = not show_audio_stats end
end

function _draw()
  cls(1)
  if show_audio_stats then
    print("LIVE AUDIO STATS", 8, 8, 15)

    print("underrun", 8, 32, 18)
    print(stat(80), 80, 32, 15)
    print("queued", 8, 48, 18)
    print(stat(81), 80, 48, 15)
    print("voices", 8, 64, 18)
    print(stat(82), 80, 64, 15)
    print("peak", 8, 80, 18)
    print(stat(83), 80, 80, 15)
    print("steals", 8, 96, 18)
    print(stat(84), 80, 96, 15)

    print("failed", 128, 32, 18)
    print(stat(85), 200, 32, 15)
    print("rewinds", 128, 48, 18)
    print(stat(86), 200, 48, 15)
    print("fallback", 128, 64, 18)
    print(stat(87), 200, 64, 15)
    print("dropped", 128, 80, 18)
    print(stat(88), 200, 80, 15)
    print("max late", 128, 96, 18)
    print(stat(89), 200, 96, 15)

    print("SELECT: CONTROLS", 8, 144, 6)
  else
    print("MTILE SFX", 8, 8, 15)
    print("A: DETUNE COIN", 8, 32, 18)
    print("B: NOIZ/DAMPEN HIT", 8, 48, 18)
    print("LEFT: TILTED ARP", 8, 64, 18)
    print("RIGHT: ORGAN ARP", 8, 80, 18)
    print("UP: BUZZ SAW LOOP", 8, 96, 18)
    print("DOWN: STOP CHANNEL 0", 8, 112, 18)
    print("START: PHASER/+SEL STOP", 8, 128, 18)
    print("SELECT: AUDIO STATS", 8, 144, 6)
  end
end
