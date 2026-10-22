-- @tweak name="Big stacks" group="Inventory"
-- @desc Multiplies how many of each stackable item fit in one slot: materials,
-- @desc ammunition, food, runes. Items that do not stack stay at one.
-- @param STACK_MULTIPLIER label="Stack multiplier" min=1 max=100 step=1 default=10
-- @param STACK_CAP label="Largest stack" min=250 max=65535 step=250 default=65535
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local STACK_MULTIPLIER = 10
local STACK_CAP = 65535

local n = 0
for _, item in ipairs(game.assets.get_resources_by_type("keen::ItemInfo")) do
  local d = item.data
  if d.maxStackSize > 1 then
    d.maxStackSize = math.min(d.maxStackSize * STACK_MULTIPLIER, STACK_CAP)   -- u16
    n = n + 1
  end
end
print("big_stacks: x" .. STACK_MULTIPLIER .. " on " .. n .. " items")
