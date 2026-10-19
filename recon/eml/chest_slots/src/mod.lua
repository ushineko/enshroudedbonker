local SLOT_MULTIPLIER = 2
local SLOT_CAP = 96

-- Player-built storage (Prop_Decoration_T0..T6_Storage_*, plain and magic).
-- Chests already placed keep their old size until rebuilt.
local n = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  local t = r.data
  if string.match(t.name, "^Prop_Decoration_T%d_Storage_") then
    for _, comp in ipairs(t.components) do
      if comp.type == "keen::ecs::InventorySetup" then
        local slots = math.min(comp.value.genericSlotCount * SLOT_MULTIPLIER, SLOT_CAP)
        comp.value.genericSlotCount = slots
        comp.value.availableSlotCount = slots
        n = n + 1
      elseif comp.type == "keen::ecs::InventorySetupOverwrite" then
        comp.value.genericSlotCount = math.min(comp.value.genericSlotCount * SLOT_MULTIPLIER, SLOT_CAP)
      end
    end
  end
end
print("chest_slots: resized " .. n)
