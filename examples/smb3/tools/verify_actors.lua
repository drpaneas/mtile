-- From the example directory, run `lua tools/verify_actors.lua`.
flr, min, max, abs = math.floor, math.min, math.max, math.abs
function sgn(x) if x < 0 then return -1 elseif x > 0 then return 1 end return 0 end
function sfx(id) end
function game_add_score(amount) score = score + amount end
function game_add_life() end
function game_clear() end
function player_grow() grew = grew + 1 end
function player_damage() damage = damage + 1 end
function player_stomp() player_vy = -64 end
function pspr(...) draws[#draws+1] = {...} end
function anim(name, x, y, flip_x, flip_y, seconds)
  assert(name == 'goomba_cycle' and seconds >= 0, 'Invalid Goomba animation call')
  local pose = art_goomba_1
  if flr(seconds * 15) % 4 >= 2 then pose = art_goomba_2 end
  pspr(pose, x, y, flip_x, flip_y)
end
function level_floor(x, y, previous, snap)
  if y >= 416 or (x >= 48 and x < 80 and y >= 384 and previous <= 384) then
    if y < 416 then return 384 end
    return 416
  end
end
function level_solid(x, y)
  return x < 0 or (wall ~= nil and x >= wall and y >= 400)
end
function level_shell_hit(x, y)
  hits[#hits + 1] = {x, y}
  if wall ~= nil and x >= wall then wall = nil return true end
  return false
end

dofile(arg[1] or "actors.lua")
dofile(arg[2] or "course_data.lua")
local function captured(fn, key)
  for i=1,30 do
    local name, value = debug.getupvalue(fn,i)
    if name == key then return value end
  end
  error('missing upvalue '..key)
end
local function reset(objects, px, py)
  course_objects = objects
  course_height = 27
  player_x, player_y, player_top, player_height, player_vx, player_vy = px,py,17,32,0,0
  player_power, player_tail_ticks, player_left = 0,0,false
  score, grew, damage = 0,0,0
  draws, hits, wall = {},{},nil
  actors_reset()
end
local slots = captured(actors_update,'slots')
local items = captured(actors_update,'items')
local fireballs = captured(actors_update,'fireballs')

local foot_rows = {}
local rows = assert(io.popen("python3 tools/koopa_foot_rows.py"))
for line in rows:lines() do
  local pose, row = line:match("^(%d+) (%d+)$")
  foot_rows[assert(tonumber(pose))] = assert(tonumber(row))
end
assert(rows:close(), 'Could not read the imported Koopa poses')

local variants = {
  {108, art_koopa_1, art_koopa_2},
  {109, art_red_koopa_1, art_red_koopa_2},
  {110, art_para_koopa_1, art_para_koopa_2},
}
for _, variant in ipairs(variants) do
  reset({{variant[1],160,400}}, 10, 200)
  local seen_poses = {}
  for _ = 1,32 do
    actors_update(0)
    assert(slots[1].y == 400, 'Grounded Koopa y must be stable')
    draws = {}
    actors_draw()
    local pose, y = draws[1][1], draws[1][3]
    assert(y + assert(foot_rows[pose]) == 415, 'Opaque Koopa feet must meet the terrain')
    seen_poses[pose] = true
  end
  assert(seen_poses[variant[2]] and seen_poses[variant[3]], 'Both walk poses must be verified')
end

reset({{108,48,400}, {114,98,400}}, 0, 220)
actors_update(0)
local shell = slots[1]
shell.timer, shell.vx = 2,3
for _=1,24 do actors_update(0) end
assert(score == 100 and slots[2].crushed > 0, 'A moving shell must defeat the Goomba')
wall = 160
for _=1,25 do actors_update(0) end
assert(#hits > 0 and wall == nil and shell.vx < 0, 'Shell must hit the block before reversing')
actors_update(650)
for _=1,80 do actors_update(0) end
for i=1,5 do assert(slots[i].home ~= 2 or slots[i].kind == 0, 'Defeated Goomba respawned') end

reset({{108,100,400}}, 90, 384)
actor_activate_objects(0)
slots[1].timer, slots[1].vx, player_vx = 1, 0, -8
actors_update(0)
assert(slots[1].timer == 2 and slots[1].vx == 3, 'Kick boost must follow Mario’s direction of motion')

reset({{114,100,400}}, 0, 220)
actors_update(0)
actors_draw()
assert(draws[1][1] == art_goomba_1, 'Goomba must display its first authored pose')
for _=1,8 do actors_update(0) end
draws = {}
actors_draw()
assert(draws[1][1] == art_goomba_2, 'Goomba must advance its authored pose')
slots[1].x = -100
actors_update(0)
for _=1,6 do actors_update(0) end
assert(slots[1].kind == 0, 'Live object must not immediately respawn in the same home window')
actors_update(600)
actors_update(0)
assert(slots[1].kind == 1, 'Live object should spawn after leaving and returning to its home window')

reset({}, 100, 180)
assert(actor_spawn_powerup(64,384,false))
assert(actor_spawn_powerup(128,384,false))
for _=1,16 do actors_update(0) end
assert(items[1].y == 384 and items[2].y == 384)
for _=17,61 do actors_update(0) end
assert(items[1].y == 368 and items[2].y == 368)
for _=1,30 do actors_update(0) end
assert(items[1].x < 64 and items[2].x > 128 and items[2].y > 368, 'Mushrooms must leave the block and fall')
player_x, player_y = items[1].x - 5, items[1].y - 20
actors_update(0)
assert(items[1].kind == 0 and grew == 1, 'Mario must collect the moving mushroom')
actors_update(700)
assert(items[2].kind == 0, 'Culled mushroom must release its slot')

reset({{166,160,368}}, 60, 245)
for i = 1,90 do
  actors_update(0)
  local plant = slots[1]
  draws = {}
  actors_draw()
  if plant.plant_height >= 4 then
    local id,y = draws[1][1],draws[1][3]
    local frame_id = math.min(8, math.floor(plant.plant_height / 4))
    assert(id == art_venus_heights[frame_id] or id == art_venus_open_heights[frame_id])
    assert(y + frame_id * 4 <= plant.base_y, 'Plant draw over the pipe lip')
  end
end
assert(slots[1].x == 168 and slots[1].plant_height == 33)
local shot = fireballs[1]
assert(shot.active and shot.vx == -14/16 and shot.vy == -5/16, 'World 1 tall fire plant aimed shot')
local old_x,old_y=shot.x,shot.y
actors_update(0)
assert(shot.x-old_x == -14/16 and shot.y-old_y == -5/16, 'Shot must not acquire gravity')
for _=1,24 do actors_update(0) end

player_x,player_y = shot.x-5,shot.y-24
actors_update(0)
assert(damage == 1, 'Projectile must damage Mario on contact')
-- Check the actual World 1-1 ledges using its imported terrain and spawn.
ord = string.byte
function band(a, b) return a & b end
dofile("terrain.lua")
local plains = courses[1]
course_rows, course_columns, course_tile_flags = plains.rows, plains.columns, plains.flags
course_slopes = plains.slopes
local koopa_spawn
for _, object in ipairs(plains.objects) do
  if object[1] == 109 and object[2] == 576 then koopa_spawn = object end
end
assert(koopa_spawn, 'Missing World 1-1 red Koopa')
reset({koopa_spawn}, 450, 200)
course_height = plains.height
local koopa = slots[1]
local grounded, turns = false, 0
for _ = 1,450 do
  local previous_vx = koopa.vx
  actors_update(400)
  local bottom = flr(koopa.y) + 16
  if bottom == 384 and koopa.vy == 0 then grounded = true end
  if grounded then
    if koopa.vx ~= previous_vx then turns = turns + 1 end
    local floor = level_floor(flr(koopa.x) + 7, bottom, bottom, false)
    assert(floor == 384 and koopa.vy == 0, 'Red Koopa walked off its World 1-1 platform')
    draws = {}
    actors_draw()
    local pose, y = draws[1][1], draws[1][3]
    assert(y + foot_rows[pose] == floor - 1, 'Red Koopa feet floated above the World 1-1 platform')
  end
end
assert(turns >= 2, 'Red Koopa must turn at both World 1-1 platform edges')

print('Actors verified: support and pose anchors, shell hits, spawn lifecycle, moving mushrooms, clipped fire plant and aimed fireball')
