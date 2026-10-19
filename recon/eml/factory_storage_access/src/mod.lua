-- Production stations (smelter, kiln, loom, ...) get the component magic
-- chests have, so crafting can draw on what is stored in them. Area deposit
-- stays off for them.
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
