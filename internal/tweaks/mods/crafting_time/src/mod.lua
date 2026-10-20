-- @tweak name="Faster crafting" group="Crafting"
-- @desc Scales how long timed recipes take: smelters, kilns, looms and the rest.
-- @param CRAFT_TIME_FACTOR label="Crafting time" min=0.01 max=1 step=0.01 default=0.1 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local CRAFT_TIME_FACTOR = 0.1

local reg = game.assets.get_resources_by_type("keen::RecipeRegistryResource")[1].data
local n = 0
for _, recipe in ipairs(reg.recipes) do
  local d = recipe.craftingDuration.value   -- nanoseconds
  if d > 0 then
    recipe.craftingDuration.value = math.max(1, math.floor(d * CRAFT_TIME_FACTOR))
    n = n + 1
  end
end
print("crafting_time: scaled " .. n)
