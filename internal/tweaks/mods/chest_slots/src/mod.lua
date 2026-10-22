-- @tweak name="More chest slots" group="Inventory"
-- @desc Multiplies the slots of every chest you build, plain and magic.
-- @desc Chests already placed keep their size until you rebuild them, and bigger
-- @desc chests stay bigger in the save after the mod is removed.
-- @param SLOT_MULTIPLIER label="Slot multiplier" min=1 max=4 step=1 default=2
-- @param SLOT_CAP label="Most slots per chest" min=24 max=255 step=8 default=96
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

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
print("chest_slots: x" .. SLOT_MULTIPLIER .. " on " .. n .. " chest types")
