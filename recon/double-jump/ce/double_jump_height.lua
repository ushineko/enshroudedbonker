-- Raises Enshrouded's double jump height in the running game's memory.
-- Build 1283531 only (enshrouded.exe sha256 0403981fd2e8ad18...). It checks the code
-- first and changes nothing if the code differs. See ../RESEARCH.md.
--
-- Use: Cheat Engine > Lua Engine (Ctrl+Alt+L) > open this file > Execute, with the
-- game running. Or copy it into CE's autorun/ folder and it applies when CE starts.
-- Lasts until the game exits. Set MODE to "restore" to undo it sooner.
local MODE = DJH_MODE or "apply"   -- "apply" or "restore"
local HEIGHT = DJH_HEIGHT or 6.5   -- metres; vanilla is 2
local SPRINT_TOO = true            -- also raise the double jump taken while sprinting

local IMAGE = 0x140000000
local FUNC = 0x140424840           -- the height switch; takes a JumpType
local HEAD = { 0x80, 0xF9, 0x13, 0x77, 0x4C, 0x48, 0x8D, 0x15 }
local VANILLA_CASE = 0x424888      -- the case returning 2
local DOUBLE_DEFAULT = 0x1404248A0 -- jump-table entries
local DOUBLE_SPRINT = 0x1404248A4

local function say(s)
  if DJH_LOG then
    local f = io.open(DJH_LOG, "a"); f:write(s .. "\n"); f:close()
  else
    print(s)
  end
end

local function right_build()
  local b = readBytes(FUNC, #HEAD, true)
  if not b then return false end
  for i, v in ipairs(HEAD) do
    if b[i] ~= v then return false end
  end
  return true
end

-- A jump-table entry is 32 bits from the image base, and the new case addresses its
-- constant relatively, so the block has to sit just above the image.
local function allocate_case()
  for at = 0x142A40000, 0x1F0000000, 0x100000 do
    local got = allocateMemory(64, at)
    if got and got ~= 0 then
      if got > IMAGE and got - IMAGE < 0x7FFFFFFF then return got end
      deAlloc(got)
    end
  end
end

local function apply()
  if readInteger(DOUBLE_DEFAULT) ~= VANILLA_CASE or readInteger(DOUBLE_SPRINT) ~= VANILLA_CASE then
    say("double_jump_height: table is not vanilla (already applied?), nothing changed")
    return
  end
  local case = allocate_case()
  if not case then say("double_jump_height: could not allocate near the image"); return end
  -- movss xmm0,[rip+1] ; ret ; <float>
  writeBytes(case, 0xF3, 0x0F, 0x10, 0x05, 0x01, 0x00, 0x00, 0x00, 0xC3)
  writeFloat(case + 9, HEIGHT)
  writeInteger(DOUBLE_DEFAULT, case - IMAGE)
  if SPRINT_TOO then writeInteger(DOUBLE_SPRINT, case - IMAGE) end
  say(string.format("double_jump_height: applied, height %s, case at %X", tostring(HEIGHT), case))
end

local function restore()
  writeInteger(DOUBLE_DEFAULT, VANILLA_CASE)
  writeInteger(DOUBLE_SPRINT, VANILLA_CASE)
  say("double_jump_height: restored")
end

local function run()
  if not right_build() then
    say("double_jump_height: this is not build 1283531's code, nothing changed")
  elseif MODE == "restore" then
    restore()
  else
    apply()
  end
end

-- CE's autorun runs before a process is open, so wait for the game.
local timer = createTimer(nil, false)
timer.Interval = 500
timer.OnTimer = function()
  if getOpenedProcessID() == 0 then
    openProcess("enshrouded.exe")
    if getOpenedProcessID() == 0 then return end
  end
  timer.Enabled = false
  run()
end
timer.Enabled = true
