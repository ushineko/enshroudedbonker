-- @tweak name="Craft from station storage" group="Crafting"
-- @desc Lets crafting draw ingredients from the storage of production stations
-- @desc (smelter, kiln, loom...), the way it draws from magic chests.
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local n = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  local comps = r.data.components
  local factory, stock = false, false
  for _, comp in ipairs(comps) do
    if comp.type == "keen::ecs::FactoryStation" then factory = true end
    if comp.type == "keen::ecs::InventoryCraftingStock" then stock = true end
  end
  if factory and not stock then
    table.insert(comps, { type = "keen::ecs::InventoryCraftingStock", value = { canReceiveAreaDeposit = false } })
    n = n + 1
  end
end
print("factory_storage_access: stations " .. n)
