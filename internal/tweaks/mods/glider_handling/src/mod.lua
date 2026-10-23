-- @tweak name="Glider handling" group="World"
-- @desc Scales how every glider flies. More lift gives a flatter glide that goes
-- @desc further; less drag keeps more speed. Each value multiplies the glider's
-- @desc own, so better gliders stay better.
-- @param LIFT label="Lift (higher is flatter)" min=0.5 max=5 step=0.1 default=2.5 kind=float
-- @param DRAG label="Forward drag (lower is faster)" min=0.1 max=3 step=0.05 default=1 kind=float
-- @param SIDE_DRAG label="Sideways drag (higher drifts less)" min=0.1 max=3 step=0.05 default=1 kind=float
-- @param THRUST label="Forward acceleration" min=0.5 max=3 step=0.1 default=1 kind=float
-- @param YAW label="Turn speed" min=0.5 max=3 step=0.1 default=1 kind=float
-- @param PITCH label="Pitch speed" min=0.5 max=3 step=0.1 default=1 kind=float
-- @param ROLL label="Roll speed" min=0.5 max=3 step=0.1 default=1 kind=float
-- @param UPDRAFT_START label="Updraft charge at start" min=1 max=5 step=0.5 default=1 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local LIFT = 2.5
local DRAG = 1
local SIDE_DRAG = 1
local THRUST = 1
local YAW = 1
local PITCH = 1
local ROLL = 1
local UPDRAFT_START = 1

local n = 0
for _, item in ipairs(game.assets.get_resources_by_type("keen::ItemInfo")) do
  local e = item.data.equipment
  if e.slot == "Glider" then
    local g = e.gliderConfig
    g.airResistanceVertical = g.airResistanceVertical * LIFT
    g.airResistanceLongitudinal = g.airResistanceLongitudinal * DRAG
    g.airResistanceLateral = g.airResistanceLateral * SIDE_DRAG
    g.accelerationForward = g.accelerationForward * THRUST
    g.yawAngleSpeed = g.yawAngleSpeed * YAW
    g.pitchAngleSpeed = g.pitchAngleSpeed * PITCH
    g.rollAngleSpeed = g.rollAngleSpeed * ROLL
    -- a charge fraction: full is 1
    g.updraftStrengthChargeStartValue = math.min(1, g.updraftStrengthChargeStartValue * UPDRAFT_START)
    n = n + 1
  end
end
print("glider_handling: lift x" .. LIFT .. ", drag x" .. DRAG .. " on " .. n .. " gliders")
