-- Pong for Mtile/GBA.
-- Ported from the gosprite64 Pong example.
--
-- The original runs at 60 Hz on a wider canvas. Each Mtile update performs
-- two simulation ticks, and the court is fitted to the GBA's 240x160 display.

local court_left = 12
local court_right = 227
local court_top = 16
local court_bottom = 151
local center_x = flr((court_right + court_left) / 2)
local center_y = flr((court_bottom + court_top) / 2)
local line_len = 4

local white = 7
local red = 8
local yellow = 10
local blue = 12

local sfx_paddle_computer = 0
local sfx_paddle_player = 1
local sfx_score_computer = 2
local sfx_score_player = 3
local sfx_start = 4
local sfx_wall = 5

local player = {}
local computer = {}
local ball = {}
local player_score = 0
local computer_score = 0
local scored = ""

function reset_round()
  local difficulty = 1.5
  local paddle_height = 20
  local paddle_width = 4

  player = {
    x = court_left + paddle_width * 2,
    y = center_y + paddle_height / 2,
    width = paddle_width,
    height = paddle_height,
    speed = difficulty,
    color = blue
  }
  computer = {
    x = court_right - paddle_width * 3,
    y = center_y + paddle_height / 2,
    width = paddle_width,
    height = paddle_height,
    speed = 0.75 * difficulty,
    color = red
  }
  ball = {
    x = center_x,
    y = center_y,
    size = 2,
    dx = difficulty,
    dy = flr(rnd(2)) - 0.5,
    speed = difficulty,
    boost = 0.05 * difficulty,
    color = white
  }

  if scored == "player" then
    sfx(sfx_score_player)
  elseif scored == "computer" then
    sfx(sfx_score_computer)
  else
    sfx(sfx_start)
  end
end

function collide(b, p)
  return b.x + b.size >= p.x and b.x <= p.x + p.width and
    b.y + b.size >= p.y and b.y <= p.y + p.height
end

function sign(v)
  if v < 0 then
    return -1
  end
  return 1
end

function _init()
  reset_round()
  music(0)
end

function update_tick()
  -- Player input.
  if btn(2) and player.y > court_top + 1 then
    player.y = player.y - player.speed
  end
  if btn(3) and player.y + player.height < court_bottom - 1 then
    player.y = player.y + player.speed
  end

  -- Track the ball while it approaches; otherwise drift back to center.
  local mid = computer.y + computer.height / 2
  if ball.dx > 0 then
    if mid > ball.y and computer.y > court_top + 1 then
      computer.y = computer.y - computer.speed
    end
    if mid < ball.y and computer.y + computer.height < court_bottom - 1 then
      computer.y = computer.y + computer.speed
    end
  else
    if mid > (center_y + player.height / 2) + player.height then
      computer.y = computer.y - computer.speed
    end
    if mid < (center_y + player.height / 2) - player.height then
      computer.y = computer.y + computer.speed
    end
  end

  -- Ball against paddles.
  if collide(ball, computer) then
    ball.dx = -(ball.dx + ball.boost)
    sfx(sfx_paddle_computer)
  end
  if collide(ball, player) then
    if btn(2) or btn(3) then
      ball.dy = ball.dy + sign(ball.dy) * ball.boost * 2
    end
    ball.dx = -(ball.dx - ball.boost)
    sfx(sfx_paddle_player)
  end

  -- Ball against the top and bottom court edges.
  if ball.y <= court_top + 1 or ball.y + ball.size >= court_bottom - 1 then
    ball.dy = -ball.dy
    sfx(sfx_wall)
  end

  -- Crossing a side edge scores and immediately serves a new ball.
  if ball.x > court_right then
    player_score = player_score + 1
    scored = "player"
    reset_round()
  end
  if ball.x < court_left then
    computer_score = computer_score + 1
    scored = "computer"
    reset_round()
  end

  ball.x = ball.x + ball.dx
  ball.y = ball.y + ball.dy
end

function _update()
  update_tick()
  update_tick()
end

function _draw()
  cls(0)

  rect(court_left, court_top, court_right, court_bottom, white)
  for y = court_top, court_bottom - 1, line_len * 2 do
    line(center_x, y, center_x, y + line_len, white)
  end

  rectfill(flr(ball.x), flr(ball.y), flr(ball.x + ball.size), flr(ball.y + ball.size), ball.color)
  rectfill(flr(player.x), flr(player.y), flr(player.x + player.width), flr(player.y + player.height), player.color)
  rectfill(flr(computer.x), flr(computer.y), flr(computer.x + computer.width), flr(computer.y + computer.height), computer.color)

  print(tostr(player_score), center_x - 24, 4, yellow)
  print(tostr(computer_score), center_x + 16, 4, yellow)
end
