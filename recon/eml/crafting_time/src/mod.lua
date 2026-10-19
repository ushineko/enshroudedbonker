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
